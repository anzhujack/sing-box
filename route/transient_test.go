package route

import (
	"fmt"
	"syscall"
	"testing"

	"github.com/sagernet/sing-tun"
	"github.com/stretchr/testify/require"
)

func TestIsTransientUnreachable(t *testing.T) {
	require.True(t, isTransientUnreachable(fmt.Errorf("dial: %w", tun.ErrNoRoute)))
	require.True(t, isTransientUnreachable(fmt.Errorf("dial: %w", syscall.ENETUNREACH)))
	require.True(t, isTransientUnreachable(fmt.Errorf("dial: %w", syscall.EHOSTUNREACH)))
	require.False(t, isTransientUnreachable(fmt.Errorf("connection refused")))
}

func TestLogThrottleAllowsOneEventPerWindow(t *testing.T) {
	var throttle logThrottle
	require.True(t, throttle.allow())
	require.False(t, throttle.allow())
}
