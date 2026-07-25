package rule

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/service"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(request *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRemoteRuleSetType(t *testing.T) {
	t.Parallel()

	ruleSet, err := NewRemoteRuleSet(context.Background(), logger.NOP(), "remote", option.RuleSet{
		Type:   constant.RuleSetTypeRemote,
		Format: constant.RuleSetFormatSource,
		RemoteOptions: option.RemoteRuleSet{
			URL: "https://example.com/rules.json",
		},
	})
	require.NoError(t, err)
	require.Equal(t, constant.RuleSetTypeRemote, ruleSet.Type())
}

func TestRemoteRuleSetConcurrentUpdateSharesAttempt(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var roundTrips atomic.Int32
	ruleSet := &RemoteRuleSet{
		abstractRuleSet: abstractRuleSet{
			ctx:    context.Background(),
			logger: logger.NOP(),
			tag:    "remote",
			sType:  constant.RuleSetTypeRemote,
			format: constant.RuleSetFormatSource,
		},
		url: "https://example.com/rules.json",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			roundTrips.Add(1)
			close(requestStarted)
			<-releaseRequest
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"version": 1,
					"rules": [{"domain": ["example.com"]}]
				}`)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		})},
	}

	firstUpdate := make(chan error, 1)
	secondUpdate := make(chan error, 1)
	go func() { firstUpdate <- ruleSet.Update(context.Background()) }()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("first update did not start")
	}
	go func() { secondUpdate <- ruleSet.Update(context.Background()) }()
	select {
	case err := <-secondUpdate:
		t.Fatalf("concurrent update returned before the active attempt completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseRequest)
	for _, done := range []<-chan error{firstUpdate, secondUpdate} {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("rule-set update did not finish")
		}
	}
	require.Equal(t, int32(1), roundTrips.Load())
}

func TestAbstractRuleSetMetadataConcurrentAccess(t *testing.T) {
	t.Parallel()

	ruleSet := &abstractRuleSet{}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := range 1000 {
			ruleSet.access.Lock()
			ruleSet.ruleCount = uint64(i)
			ruleSet.lastUpdated = ruleSet.lastUpdated.Add(1)
			ruleSet.access.Unlock()
		}
	}()
	for range 1000 {
		ruleSet.RuleCount()
		ruleSet.UpdatedTime()
	}
	<-writerDone
}

type restoreFailureCache struct {
	adapter.CacheFile
	saved     *adapter.SavedBinary
	saveCount int
}

func (c *restoreFailureCache) LoadRuleSet(string) *adapter.SavedBinary { return c.saved }
func (c *restoreFailureCache) SaveRuleSet(_ string, _ *adapter.SavedBinary) error {
	c.saveCount++
	return nil
}

type restoreFailureTransport struct{ roundTripFunc }

func (*restoreFailureTransport) CloseIdleConnections() {}
func (*restoreFailureTransport) Reset()                {}

type restoreFailureHTTPClientManager struct {
	adapter.HTTPClientManager
	transport adapter.HTTPTransport
}

func (m *restoreFailureHTTPClientManager) DefaultTransport() adapter.HTTPTransport {
	return m.transport
}

func TestRemoteRuleSetRestoreFailureDoesNotPersistentlyEvictCache(t *testing.T) {
	cache := &restoreFailureCache{saved: &adapter.SavedBinary{Content: []byte("invalid cached rule-set")}}
	transport := &restoreFailureTransport{roundTripFunc: func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     http.StatusText(http.StatusInternalServerError),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	}}
	ctx := service.ContextWith[adapter.CacheFile](context.Background(), cache)
	ctx = service.ContextWith[adapter.HTTPClientManager](ctx, &restoreFailureHTTPClientManager{transport: transport})
	ruleSet, err := NewRemoteRuleSet(ctx, logger.NOP(), "remote", option.RuleSet{
		Type:   constant.RuleSetTypeRemote,
		Format: constant.RuleSetFormatSource,
		RemoteOptions: option.RemoteRuleSet{
			URL: "https://example.com/rules.json",
		},
	})
	require.NoError(t, err)
	startContext := adapter.NewHTTPStartContext()
	t.Cleanup(startContext.Close)
	fetchStartedAt := time.Now()
	require.NoError(t, ruleSet.StartContext(ctx, startContext))
	require.Zero(t, cache.saveCount, "a failed restore must not destroy persisted metadata before a replacement succeeds")
	retryDeadline := ruleSet.getInitialRetryDeadline()
	require.False(t, retryDeadline.Before(fetchStartedAt.Add(ruleSetInitialRetryInterval)))
	require.False(t, retryDeadline.After(time.Now().Add(ruleSetInitialRetryInterval)))
	require.Positive(t, ruleSet.initialUpdateDelay(time.Now()))
}

func TestCanceledFetchFollowerDoesNotPublishRetryBeforeAttemptCompletion(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseRequest:
		default:
			close(releaseRequest)
		}
	})
	ruleSet := &RemoteRuleSet{
		abstractRuleSet: abstractRuleSet{ctx: context.Background(), logger: logger.NOP(), tag: "remote"},
		url:             "https://example.com/rules.json",
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(requestStarted)
			<-releaseRequest
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     http.StatusText(http.StatusInternalServerError),
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		})},
	}
	ownerDone := make(chan error, 1)
	go func() { ownerDone <- ruleSet.fetch(context.Background(), false) }()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("owner fetch did not start")
	}
	followerCtx, cancelFollower := context.WithCancel(context.Background())
	cancelFollower()
	require.ErrorIs(t, ruleSet.fetch(followerCtx, true), context.Canceled)
	require.True(t, ruleSet.getInitialRetryDeadline().IsZero(), "a canceled follower published retry state before attempt completion")
	close(releaseRequest)
	select {
	case err := <-ownerDone:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("owner fetch did not finish")
	}
	require.False(t, ruleSet.getInitialRetryDeadline().IsZero(), "the completed failed attempt did not publish a retry deadline")
}
