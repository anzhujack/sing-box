package clashapi

import (
	"encoding/json"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/trafficcontrol"
	M "github.com/sagernet/sing/common/metadata"
)

func decodeConnectionMetadata(t *testing.T, c connectionObject) map[string]any {
	t.Helper()
	data, err := c.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	metadata, ok := decoded["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata has type %T, want object", decoded["metadata"])
	}
	return metadata
}

func TestConnectionObjectHostPrefersDomainThenFqdnThenSniffHost(t *testing.T) {
	base := connectionObject(trafficcontrol.TrackerMetadata{
		Metadata: adapter.InboundContext{
			Network:   "tcp",
			Domain:    "domain.example",
			SniffHost: "sniff.example",
			Destination: M.Socksaddr{
				Fqdn: "fqdn.example",
				Port: 443,
			},
		},
		Upload:   new(atomic.Int64),
		Download: new(atomic.Int64),
	})

	metadata := decodeConnectionMetadata(t, base)
	if got := metadata["host"]; got != "domain.example" {
		t.Fatalf("host = %v, want metadata.Domain", got)
	}

	base.Metadata.Domain = ""
	metadata = decodeConnectionMetadata(t, base)
	if got := metadata["host"]; got != "fqdn.example" {
		t.Fatalf("host = %v, want Destination.Fqdn", got)
	}

	base.Metadata.Destination.Fqdn = ""
	metadata = decodeConnectionMetadata(t, base)
	if got := metadata["host"]; got != "sniff.example" {
		t.Fatalf("host = %v, want SniffHost fallback", got)
	}
	if got := metadata["sniffHost"]; got != "sniff.example" {
		t.Fatalf("sniffHost = %v, want sniff.example", got)
	}
}

func TestConnectionObjectDestinationIPUsesResolvedAddressFallback(t *testing.T) {
	c := connectionObject(trafficcontrol.TrackerMetadata{
		Metadata: adapter.InboundContext{
			Network: "tcp",
			Domain:  "example.com",
			Destination: M.Socksaddr{
				Addr: netip.MustParseAddr("192.0.2.1"),
				Port: 443,
			},
			DestinationAddresses: []netip.Addr{netip.MustParseAddr("203.0.113.9")},
		},
		Upload:   new(atomic.Int64),
		Download: new(atomic.Int64),
	})

	metadata := decodeConnectionMetadata(t, c)
	if got := metadata["destinationIP"]; got != "203.0.113.9" {
		t.Fatalf("destinationIP = %v, want first resolved destination address", got)
	}
	if got := metadata["host"]; got != "example.com@203.0.113.9" {
		t.Fatalf("host = %v, want domain decorated with resolved destination address", got)
	}
	if got := metadata["remoteDestination"]; got != "203.0.113.9:443" {
		t.Fatalf("remoteDestination = %v, want resolved address with port", got)
	}
}
