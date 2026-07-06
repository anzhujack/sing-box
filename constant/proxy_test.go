package constant

import "testing"

func TestProxyDisplayNameIncludesSmart(t *testing.T) {
	if got := ProxyDisplayName(TypeSmart); got != "Smart" {
		t.Fatalf("ProxyDisplayName(TypeSmart) = %q, want %q", got, "Smart")
	}
}
