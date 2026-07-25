package rule

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common/logger"
	"github.com/stretchr/testify/require"
)

func TestInitialRuleSetUpdateDelay(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	updateInterval := time.Hour

	require.Equal(t, ruleSetInitialRetryInterval, initialRuleSetUpdateDelay(time.Time{}, updateInterval, now))
	require.Equal(t, 30*time.Minute, initialRuleSetUpdateDelay(now.Add(-30*time.Minute), updateInterval, now))
	require.Zero(t, initialRuleSetUpdateDelay(now.Add(-2*time.Hour), updateInterval, now))
}

func TestReadRuleSetResponseBounds(t *testing.T) {
	content, err := readRuleSetResponse(strings.NewReader("abcd"), 4)
	require.NoError(t, err)
	require.Equal(t, []byte("abcd"), content)

	_, err = readRuleSetResponse(strings.NewReader("abcde"), 4)
	require.ErrorContains(t, err, "exceeds")

	_, err = readRuleSetResponse(strings.NewReader(""), 4)
	require.ErrorContains(t, err, "empty")
}

func updaterTestRuleSet(ctx context.Context, cancel context.CancelFunc, updateInterval time.Duration, transport http.RoundTripper) *RemoteRuleSet {
	ruleSet := &RemoteRuleSet{
		abstractRuleSet: abstractRuleSet{
			ctx:    ctx,
			logger: logger.NOP(),
			tag:    "remote",
			sType:  constant.RuleSetTypeRemote,
			format: constant.RuleSetFormatSource,
		},
		cancel:         cancel,
		url:            "https://example.com/rules.json",
		updateInterval: updateInterval,
		httpClient:     &http.Client{Transport: transport},
	}
	ruleSet.setUpdatedTime(time.Now().Add(-2 * updateInterval))
	return ruleSet
}

func TestRuleSetUpdaterCloseCancelsInFlightFetch(t *testing.T) {
	ruleCtx, ruleCancel := context.WithCancel(context.Background())
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	ruleSet := updaterTestRuleSet(ruleCtx, ruleCancel, time.Hour, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestCanceled)
		return nil, request.Context().Err()
	}))
	updater := NewRuleSetUpdater(context.Background(), []adapter.RuleSet{ruleSet})
	require.NotNil(t, updater)
	t.Cleanup(func() {
		_ = updater.Close()
		ruleCancel()
	})
	updater.Start()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("rule-set fetch did not start")
	}
	require.NoError(t, updater.Close())
	select {
	case <-requestCanceled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RuleSetUpdater.Close did not cancel its in-flight fetch")
	}
}

func TestRuleSetUpdaterSchedulesRetryFromFetchCompletion(t *testing.T) {
	ruleCtx, ruleCancel := context.WithCancel(context.Background())
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	var calls atomic.Int32
	const updateInterval = 200 * time.Millisecond
	ruleSet := updaterTestRuleSet(ruleCtx, ruleCancel, updateInterval, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
			return nil, errors.New("first fetch failed")
		case 2:
			close(secondStarted)
			<-request.Context().Done()
			return nil, request.Context().Err()
		default:
			return nil, errors.New("unexpected extra fetch")
		}
	}))
	updater := NewRuleSetUpdater(context.Background(), []adapter.RuleSet{ruleSet})
	require.NotNil(t, updater)
	t.Cleanup(func() {
		_ = updater.Close()
		ruleCancel()
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
	})
	updater.Start()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first rule-set fetch did not start")
	}
	time.Sleep(updateInterval + 50*time.Millisecond)
	close(releaseFirst)
	select {
	case <-secondStarted:
		t.Fatal("next rule-set fetch started immediately after a slow fetch")
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case <-secondStarted:
	case <-time.After(2 * updateInterval):
		t.Fatal("next rule-set fetch was not scheduled from fetch completion")
	}
}
