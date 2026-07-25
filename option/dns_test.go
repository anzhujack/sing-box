package option

import (
	"context"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"github.com/stretchr/testify/require"
)

type stubDNSTransportOptionsRegistry struct{}

func (stubDNSTransportOptionsRegistry) CreateOptions(transportType string) (any, bool) {
	switch transportType {
	case C.DNSTypeUDP:
		return new(RemoteDNSServerOptions), true
	case C.DNSTypeFakeIP:
		return new(FakeIPDNSServerOptions), true
	default:
		return nil, false
	}
}

func TestDNSOptionsRejectsLegacyFakeIPOptions(t *testing.T) {
	t.Parallel()

	ctx := service.ContextWith[DNSTransportOptionsRegistry](context.Background(), stubDNSTransportOptionsRegistry{})
	var options DNSOptions
	err := json.UnmarshalContext(ctx, []byte(`{
		"fakeip": {
			"enabled": true,
			"inet4_range": "198.18.0.0/15"
		}
	}`), &options)
	require.EqualError(t, err, legacyDNSFakeIPRemovedMessage)
}

func TestDNSServerOptionsRejectsLegacyFormats(t *testing.T) {
	t.Parallel()

	ctx := service.ContextWith[DNSTransportOptionsRegistry](context.Background(), stubDNSTransportOptionsRegistry{})
	testCases := []string{
		`{"address":"1.1.1.1"}`,
		`{"type":"legacy","address":"1.1.1.1"}`,
	}
	for _, content := range testCases {
		var options DNSServerOptions
		err := json.UnmarshalContext(ctx, []byte(content), &options)
		require.EqualError(t, err, legacyDNSServerRemovedMessage)
	}
}

func TestPrefetchDNSOptionsRepeatedUnmarshalResetsTuning(t *testing.T) {
	var options PrefetchDNSOptions
	require.NoError(t, json.Unmarshal([]byte(`{
		"enabled": true,
		"metadata_size": 128,
		"qps": 16,
		"backoff_multiplier": 2
	}`), &options))
	require.Equal(t, uint32(128), options.MetadataSize)
	require.Equal(t, uint32(16), options.QPS)
	require.Equal(t, 2.0, options.BackoffMultiplier)

	require.NoError(t, json.Unmarshal([]byte(`false`), &options))
	require.False(t, options.Enabled)
	require.Zero(t, options.MetadataSize)
	require.Zero(t, options.QPS)
	require.Zero(t, options.BackoffMultiplier)
}
