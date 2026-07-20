package rule

import (
	"context"
	"runtime"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

type RuleSetUpdater struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ruleSets []*RemoteRuleSet
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
	}
}

func (u *RuleSetUpdater) Start() {
	go u.loopUpdate()
}

func (u *RuleSetUpdater) Close() error {
	u.cancel()
	return nil
}

func (u *RuleSetUpdater) loopUpdate() {
	now := time.Now()
	nextUpdates := make([]time.Time, len(u.ruleSets))
	for i, ruleSet := range u.ruleSets {
		nextUpdates[i] = now.Add(initialRuleSetUpdateDelay(ruleSet.lastUpdated, ruleSet.updateInterval, now))
	}
	timer := time.NewTimer(waitUntilNext(nextUpdates))
	defer timer.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-timer.C:
		}
		now = time.Now()
		var updated bool
		for i, ruleSet := range u.ruleSets {
			if now.Before(nextUpdates[i]) {
				continue
			}
			succeeded := ruleSet.update()
			nextDelay := ruleSet.updateInterval
			if !succeeded && ruleSet.lastUpdated.IsZero() {
				nextDelay = ruleSetInitialRetryInterval
			}
			nextUpdates[i] = now.Add(nextDelay)
			updated = true
		}
		if updated {
			runtime.GC()
		}
		timer.Reset(waitUntilNext(nextUpdates))
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
