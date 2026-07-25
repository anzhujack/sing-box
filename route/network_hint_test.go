package route

import (
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing-tun"
	"github.com/stretchr/testify/require"
)

type forceUpdateMonitor struct {
	tun.DefaultInterfaceMonitor
	calls atomic.Int32
}

func (m *forceUpdateMonitor) ForceUpdate() {
	m.calls.Add(1)
}

func TestHintUnreachableForcesInterfaceRefresh(t *testing.T) {
	manager := &NetworkManager{}
	manager.HintUnreachable()

	monitor := &forceUpdateMonitor{}
	manager.interfaceMonitor = monitor
	manager.HintUnreachable()
	require.Equal(t, int32(1), monitor.calls.Load())
}

func TestHintUnreachableCoalescesBurst(t *testing.T) {
	monitor := &forceUpdateMonitor{}
	manager := &NetworkManager{interfaceMonitor: monitor}
	for range 16 {
		manager.HintUnreachable()
	}
	require.Equal(t, int32(1), monitor.calls.Load())
}
