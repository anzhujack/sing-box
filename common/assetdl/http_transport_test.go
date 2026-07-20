package assetdl

import (
	"context"
	"net/http"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/service"
	"github.com/stretchr/testify/require"
)

func TestResolveTransportUsesTaggedHTTPClient(t *testing.T) {
	transport := new(testHTTPTransport)
	manager := &testHTTPClientManager{transport: transport}
	ctx := service.ContextWith[adapter.HTTPClientManager](context.Background(), manager)

	resolved, name, err := ResolveTransport(ctx, log.NewNOPFactory().NewLogger("assetdl"), &option.HTTPClientOptions{Tag: "assets"}, "")
	require.NoError(t, err)
	require.Same(t, transport, resolved)
	require.Equal(t, "assets", name)
	require.Equal(t, "assets", manager.options.Tag)
}

func TestResolveTransportConvertsLegacyDetour(t *testing.T) {
	transport := new(testHTTPTransport)
	manager := &testHTTPClientManager{transport: transport}
	ctx := service.ContextWith[adapter.HTTPClientManager](context.Background(), manager)

	resolved, name, err := ResolveTransport(ctx, log.NewNOPFactory().NewLogger("assetdl"), nil, "proxy")
	require.NoError(t, err)
	require.Same(t, transport, resolved)
	require.Equal(t, "proxy", name)
	require.Equal(t, "proxy", manager.options.Detour)
	require.True(t, manager.options.DisableEmptyDirectCheck)
}

func TestResolveTransportRejectsConflictingOptions(t *testing.T) {
	ctx := service.ContextWith[adapter.HTTPClientManager](context.Background(), &testHTTPClientManager{})

	_, _, err := ResolveTransport(ctx, log.NewNOPFactory().NewLogger("assetdl"), &option.HTTPClientOptions{Tag: "assets"}, "proxy")
	require.Error(t, err)
}

type testHTTPClientManager struct {
	transport adapter.HTTPTransport
	options   option.HTTPClientOptions
}

func (m *testHTTPClientManager) ResolveTransport(_ context.Context, _ logger.ContextLogger, options option.HTTPClientOptions) (adapter.HTTPTransport, error) {
	m.options = options
	return m.transport, nil
}

func (m *testHTTPClientManager) DefaultTransport() adapter.HTTPTransport {
	return m.transport
}

func (m *testHTTPClientManager) ResetNetwork() {}

type testHTTPTransport struct{}

func (*testHTTPTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }
func (*testHTTPTransport) CloseIdleConnections()                           {}
func (*testHTTPTransport) Reset()                                          {}
