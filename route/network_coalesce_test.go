package route

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/x/list"
	"github.com/stretchr/testify/require"
)

func TestNetworkResetCoalescesCallbackBurst(t *testing.T) {
	manager := new(NetworkManager)
	var resetCount atomic.Int32
	for range 5 {
		manager.scheduleResetAfter(func() { resetCount.Add(1) }, 30*time.Millisecond)
		time.Sleep(5 * time.Millisecond)
	}
	require.Eventually(t, func() bool { return resetCount.Load() == 1 }, 250*time.Millisecond, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int32(1), resetCount.Load())
}

func TestNetworkCloseCancelsPendingReset(t *testing.T) {
	manager := &NetworkManager{logger: log.NewNOPFactory().NewLogger("network")}
	var resetCount atomic.Int32
	manager.scheduleResetAfter(func() { resetCount.Add(1) }, 50*time.Millisecond)
	require.NoError(t, manager.Close())
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, resetCount.Load())
}

func TestAutoDetectInterfaceFallsBackToKernelDuringMonitorGap(t *testing.T) {
	manager := &NetworkManager{
		interfaceFinder:  control.NewDefaultInterfaceFinder(),
		interfaceMonitor: new(emptyDefaultInterfaceMonitor),
	}
	controlFunc := manager.AutoDetectInterfaceFunc()
	require.NotNil(t, controlFunc)
	require.NoError(t, controlFunc("tcp", "example.com:443", nil))
}

type emptyDefaultInterfaceMonitor struct{}

func (*emptyDefaultInterfaceMonitor) Start() error { return nil }
func (*emptyDefaultInterfaceMonitor) Close() error { return nil }
func (*emptyDefaultInterfaceMonitor) DefaultInterface() *control.Interface {
	return nil
}
func (*emptyDefaultInterfaceMonitor) OverrideAndroidVPN() bool { return false }
func (*emptyDefaultInterfaceMonitor) AndroidVPNEnabled() bool  { return false }
func (*emptyDefaultInterfaceMonitor) RegisterCallback(callback tun.DefaultInterfaceUpdateCallback) *list.Element[tun.DefaultInterfaceUpdateCallback] {
	return nil
}
func (*emptyDefaultInterfaceMonitor) UnregisterCallback(*list.Element[tun.DefaultInterfaceUpdateCallback]) {
}
func (*emptyDefaultInterfaceMonitor) RegisterMyInterface(string) {}
func (*emptyDefaultInterfaceMonitor) MyInterfaces() []string     { return nil }
