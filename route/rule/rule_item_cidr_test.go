package rule

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIPCIDRItemReportsOriginalValue(t *testing.T) {
	_, err := NewIPCIDRItem(false, []string{"192.0.2.0/24", "not-an-ip"})
	require.ErrorContains(t, err, `ipcidr[1]="not-an-ip"`)
	require.ErrorContains(t, err, "expected forms")
}
