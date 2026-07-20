package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProviderUpdateDelays(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	updateInterval := time.Hour

	require.Equal(t, providerInitialRetryInterval, initialProviderUpdateDelay(time.Time{}, updateInterval, now))
	require.Equal(t, 30*time.Minute, initialProviderUpdateDelay(now.Add(-30*time.Minute), updateInterval, now))
	require.Zero(t, initialProviderUpdateDelay(now.Add(-2*time.Hour), updateInterval, now))
	require.Equal(t, providerInitialRetryInterval, providerRetryDelay(time.Time{}, updateInterval))
	require.Equal(t, updateInterval, providerRetryDelay(now, updateInterval))
}
