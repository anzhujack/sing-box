package route

import (
	"testing"

	"github.com/sagernet/sing-tun"
	"github.com/stretchr/testify/require"
)

type forceUpdateMonitor struct {
	tun.DefaultInterfaceMonitor
	calls int
}

func (m *forceUpdateMonitor) ForceUpdate() {
	m.calls++
}

func TestHintUnreachableForcesInterfaceRefresh(t *testing.T) {
	manager := &NetworkManager{}
	manager.HintUnreachable()

	monitor := &forceUpdateMonitor{}
	manager.interfaceMonitor = monitor
	manager.HintUnreachable()
	require.Equal(t, 1, monitor.calls)
}
