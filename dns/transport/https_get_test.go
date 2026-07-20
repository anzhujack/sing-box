package transport

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestDoHGetDestinationPreservesExistingQuery(t *testing.T) {
	destination := url.URL{
		Scheme:   "https",
		Host:     "dns.example",
		Path:     "/dns-query",
		RawQuery: "token=abc&dns=stale",
	}
	message := []byte{0x01, 0x02, 0xff}
	actual := dohGetDestination(destination, message)
	query := actual.Query()
	if query.Get("token") != "abc" {
		t.Fatalf("existing query parameter lost: %q", actual.RawQuery)
	}
	expectedDNS := base64.RawURLEncoding.EncodeToString(message)
	if query.Get("dns") != expectedDNS {
		t.Fatalf("dns query = %q, want %q", query.Get("dns"), expectedDNS)
	}
	if destination.RawQuery != "token=abc&dns=stale" {
		t.Fatalf("input URL mutated: %q", destination.RawQuery)
	}
}
