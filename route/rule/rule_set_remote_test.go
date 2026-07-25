package rule

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

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

func TestRemoteRuleSetRejectsConcurrentUpdate(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
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
			close(requestStarted)
			<-releaseRequest
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"version": 1,
					"rules": [{"domain": ["example.com"]}]
				}`)),
				Header: make(http.Header),
			}, nil
		})},
	}

	firstUpdate := make(chan error, 1)
	go func() {
		firstUpdate <- ruleSet.Update(context.Background())
	}()
	<-requestStarted

	require.ErrorContains(t, ruleSet.Update(context.Background()), "rule-set is updating")
	close(releaseRequest)
	require.NoError(t, <-firstUpdate)
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
	require.NoError(t, ruleSet.StartContext(ctx, startContext))
	require.Zero(t, cache.saveCount, "a failed restore must not destroy persisted metadata before a replacement succeeds")
}
