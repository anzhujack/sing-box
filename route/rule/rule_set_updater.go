package rule

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

type RuleSetUpdater struct {
	ctx         context.Context
	cancel      context.CancelFunc
	ruleSets    []*RemoteRuleSet
	lifecycleMu sync.Mutex
	started     bool
	closed      bool
	done        chan struct{}
}

func NewRuleSetUpdater(ctx context.Context, ruleSets []adapter.RuleSet) *RuleSetUpdater {
	var remoteRuleSets []*RemoteRuleSet
	for _, ruleSet := range ruleSets {
		remoteRuleSet, isRemote := ruleSet.(*RemoteRuleSet)
		if isRemote {
			remoteRuleSets = append(remoteRuleSets, remoteRuleSet)
		}
	}
	if len(remoteRuleSets) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	return &RuleSetUpdater{
		ctx:      ctx,
		cancel:   cancel,
		ruleSets: remoteRuleSets,
		done:     make(chan struct{}),
	}
}

func (u *RuleSetUpdater) Start() {
	u.lifecycleMu.Lock()
	defer u.lifecycleMu.Unlock()
	if u.started || u.closed {
		return
	}
	u.started = true
	go func() {
		defer close(u.done)
		u.loopUpdate()
	}()
}

func (u *RuleSetUpdater) Close() error {
	u.lifecycleMu.Lock()
	if !u.closed {
		u.closed = true
		u.cancel()
	}
	started := u.started
	done := u.done
	u.lifecycleMu.Unlock()
	if started {
		<-done
	}
	return nil
}

func (u *RuleSetUpdater) loopUpdate() {
	now := time.Now()
	nextUpdates := make([]time.Time, len(u.ruleSets))
	for i, ruleSet := range u.ruleSets {
		nextUpdates[i] = now.Add(ruleSet.initialUpdateDelay(now))
	}
	timer := time.NewTimer(waitUntilNext(nextUpdates))
	defer timer.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-timer.C:
			if u.ctx.Err() != nil {
				return
			}
		}
		now = time.Now()
		var updated bool
		for i, ruleSet := range u.ruleSets {
			if u.ctx.Err() != nil {
				return
			}
			if now.Before(nextUpdates[i]) {
				continue
			}
			succeeded := ruleSet.update(u.ctx)
			completedAt := time.Now()
			nextDelay := ruleSet.updateInterval
			if !succeeded && ruleSet.UpdatedTime().IsZero() {
				nextDelay = ruleSetInitialRetryInterval
			}
			nextUpdates[i] = completedAt.Add(nextDelay)
			updated = true
		}
		if u.ctx.Err() != nil {
			return
		}
		if updated {
			runtime.GC()
		}
		u.lifecycleMu.Lock()
		if u.closed || u.ctx.Err() != nil {
			u.lifecycleMu.Unlock()
			return
		}
		timer.Reset(waitUntilNext(nextUpdates))
		u.lifecycleMu.Unlock()
	}
}

func initialRuleSetUpdateDelay(lastUpdated time.Time, updateInterval time.Duration, now time.Time) time.Duration {
	if lastUpdated.IsZero() {
		return ruleSetInitialRetryInterval
	}
	wait := lastUpdated.Add(updateInterval).Sub(now)
	if wait < 0 {
		return 0
	}
	return wait
}

func waitUntilNext(nextUpdates []time.Time) time.Duration {
	next := nextUpdates[0]
	for _, nextUpdate := range nextUpdates[1:] {
		if nextUpdate.Before(next) {
			next = nextUpdate
		}
	}
	wait := time.Until(next)
	if wait < 0 {
		return 0
	}
	return wait
}
