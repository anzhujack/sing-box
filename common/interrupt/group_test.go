package interrupt

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

type blockingCloseConn struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
	closeCalls  atomic.Int32
}

func newBlockingCloseConn() *blockingCloseConn {
	return &blockingCloseConn{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (c *blockingCloseConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *blockingCloseConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *blockingCloseConn) LocalAddr() net.Addr              { return testAddr("local") }
func (c *blockingCloseConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (c *blockingCloseConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingCloseConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingCloseConn) SetWriteDeadline(time.Time) error { return nil }
func (c *blockingCloseConn) Close() error {
	c.closeCalls.Add(1)
	c.startedOnce.Do(func() { close(c.started) })
	<-c.release
	return nil
}

func (c *blockingCloseConn) unblock() {
	c.releaseOnce.Do(func() { close(c.release) })
}

type noOpConn struct{}

func (*noOpConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (*noOpConn) Write(p []byte) (int, error)      { return len(p), nil }
func (*noOpConn) Close() error                     { return nil }
func (*noOpConn) LocalAddr() net.Addr              { return testAddr("local") }
func (*noOpConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (*noOpConn) SetDeadline(time.Time) error      { return nil }
func (*noOpConn) SetReadDeadline(time.Time) error  { return nil }
func (*noOpConn) SetWriteDeadline(time.Time) error { return nil }

type blockingClosePacketConn struct {
	*blockingCloseConn
}

func (c *blockingClosePacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, c.RemoteAddr(), io.EOF
}

func (*blockingClosePacketConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	return len(p), nil
}

type noOpPacketConn struct {
	*noOpConn
}

func (c *noOpPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, c.RemoteAddr(), io.EOF
}

func (*noOpPacketConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	return len(p), nil
}

type trackingCloseConn struct {
	*noOpConn
	closeCalls atomic.Int32
	closeErr   error
}

func newTrackingCloseConn(closeErr error) *trackingCloseConn {
	return &trackingCloseConn{noOpConn: &noOpConn{}, closeErr: closeErr}
}

func (c *trackingCloseConn) Close() error {
	c.closeCalls.Add(1)
	return c.closeErr
}

type trackingClosePacketConn struct {
	*trackingCloseConn
}

func (c *trackingClosePacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, c.RemoteAddr(), io.EOF
}

func (*trackingClosePacketConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	return len(p), nil
}

func waitForSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func assertClosePendingAndUnderlyingCalledOnce(t *testing.T, result <-chan error, calls *atomic.Int32) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("concurrent Close returned before the first Close was released: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	if count := calls.Load(); count != 1 {
		t.Fatalf("underlying Close called %d times while closes overlap; want 1", count)
	}
}

func TestGroupInterruptDoesNotBlockNewConnDuringUnderlyingClose(t *testing.T) {
	group := NewGroup()
	blocker := newBlockingCloseConn()
	group.NewConn(blocker, false)
	t.Cleanup(blocker.unblock)

	interruptDone := make(chan struct{})
	go func() {
		group.Interrupt(false)
		close(interruptDone)
	}()
	waitForSignal(t, blocker.started, "Interrupt did not reach the underlying Close")

	registered := make(chan net.Conn, 1)
	go func() {
		registered <- group.NewConn(&noOpConn{}, false)
	}()

	select {
	case conn := <-registered:
		blocker.unblock()
		waitForSignal(t, interruptDone, "Interrupt did not finish after Close was released")
		if err := conn.Close(); err != nil {
			t.Fatalf("close newly registered connection: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		blocker.unblock()
		waitForSignal(t, interruptDone, "Interrupt did not finish after Close was released")
		t.Fatal("NewConn blocked behind an unrelated underlying Close")
	}
}

func TestConnCloseDoesNotBlockNewConnDuringUnderlyingClose(t *testing.T) {
	group := NewGroup()
	blocker := newBlockingCloseConn()
	wrapped := group.NewConn(blocker, false)
	t.Cleanup(blocker.unblock)

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- wrapped.Close()
	}()
	waitForSignal(t, blocker.started, "wrapped connection did not reach the underlying Close")

	registered := make(chan net.Conn, 1)
	go func() {
		registered <- group.NewConn(&noOpConn{}, false)
	}()

	select {
	case conn := <-registered:
		blocker.unblock()
		if err := <-closeResult; err != nil {
			t.Fatalf("close wrapped connection: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close newly registered connection: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		blocker.unblock()
		if err := <-closeResult; err != nil {
			t.Fatalf("close wrapped connection after release: %v", err)
		}
		t.Fatal("NewConn blocked behind an unrelated wrapped connection Close")
	}
}

func TestInterruptAndConnCloseCloseUnderlyingOnce(t *testing.T) {
	group := NewGroup()
	blocker := newBlockingCloseConn()
	wrapped := group.NewConn(blocker, false)
	t.Cleanup(blocker.unblock)

	interruptDone := make(chan struct{})
	go func() {
		group.Interrupt(false)
		close(interruptDone)
	}()
	waitForSignal(t, blocker.started, "Interrupt did not reach the underlying Close")

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- wrapped.Close()
	}()
	assertClosePendingAndUnderlyingCalledOnce(t, closeResult, &blocker.closeCalls)
	blocker.unblock()
	waitForSignal(t, interruptDone, "Interrupt did not finish after Close was released")
	if err := <-closeResult; err != nil {
		t.Fatalf("close wrapped connection: %v", err)
	}
	if calls := blocker.closeCalls.Load(); calls != 1 {
		t.Fatalf("underlying Close called %d times during Interrupt/Close race; want 1", calls)
	}
}

func TestPacketConnCloseDoesNotBlockNewPacketConnDuringUnderlyingClose(t *testing.T) {
	group := NewGroup()
	blocker := newBlockingCloseConn()
	wrapped := group.NewPacketConn(&blockingClosePacketConn{blockingCloseConn: blocker}, false)
	t.Cleanup(blocker.unblock)

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- wrapped.Close()
	}()
	waitForSignal(t, blocker.started, "wrapped packet connection did not reach the underlying Close")

	registered := make(chan net.PacketConn, 1)
	go func() {
		registered <- group.NewPacketConn(&noOpPacketConn{noOpConn: &noOpConn{}}, false)
	}()

	select {
	case conn := <-registered:
		blocker.unblock()
		if err := <-closeResult; err != nil {
			t.Fatalf("close wrapped packet connection: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close newly registered packet connection: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		blocker.unblock()
		if err := <-closeResult; err != nil {
			t.Fatalf("close wrapped packet connection after release: %v", err)
		}
		t.Fatal("NewPacketConn blocked behind an unrelated wrapped packet connection Close")
	}
}

func TestInterruptAndPacketConnCloseCloseUnderlyingOnce(t *testing.T) {
	group := NewGroup()
	blocker := newBlockingCloseConn()
	wrapped := group.NewPacketConn(&blockingClosePacketConn{blockingCloseConn: blocker}, false)
	t.Cleanup(blocker.unblock)

	interruptDone := make(chan struct{})
	go func() {
		group.Interrupt(false)
		close(interruptDone)
	}()
	waitForSignal(t, blocker.started, "Interrupt did not reach the underlying packet Close")

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- wrapped.Close()
	}()
	assertClosePendingAndUnderlyingCalledOnce(t, closeResult, &blocker.closeCalls)
	blocker.unblock()
	waitForSignal(t, interruptDone, "Interrupt did not finish after packet Close was released")
	if err := <-closeResult; err != nil {
		t.Fatalf("close wrapped packet connection: %v", err)
	}
	if calls := blocker.closeCalls.Load(); calls != 1 {
		t.Fatalf("underlying packet Close called %d times during Interrupt/Close race; want 1", calls)
	}
}

func TestConnCloseIsIdempotentAndReturnsFirstError(t *testing.T) {
	group := NewGroup()
	wantErr := errors.New("close failed")
	underlying := newTrackingCloseConn(wantErr)
	wrapped := group.NewConn(underlying, false)

	for attempt := 1; attempt <= 2; attempt++ {
		if err := wrapped.Close(); !errors.Is(err, wantErr) {
			t.Fatalf("Close attempt %d returned %v; want %v", attempt, err, wantErr)
		}
	}
	if calls := underlying.closeCalls.Load(); calls != 1 {
		t.Fatalf("underlying Close called %d times; want 1", calls)
	}
	if length := group.connections.Len(); length != 0 {
		t.Fatalf("group contains %d connections after Close; want 0", length)
	}
}

func TestPacketConnCloseIsIdempotentAndReturnsFirstError(t *testing.T) {
	group := NewGroup()
	wantErr := errors.New("packet close failed")
	underlying := newTrackingCloseConn(wantErr)
	wrapped := group.NewPacketConn(&trackingClosePacketConn{trackingCloseConn: underlying}, false)

	for attempt := 1; attempt <= 2; attempt++ {
		if err := wrapped.Close(); !errors.Is(err, wantErr) {
			t.Fatalf("Close attempt %d returned %v; want %v", attempt, err, wantErr)
		}
	}
	if calls := underlying.closeCalls.Load(); calls != 1 {
		t.Fatalf("underlying packet Close called %d times; want 1", calls)
	}
	if length := group.connections.Len(); length != 0 {
		t.Fatalf("group contains %d packet connections after Close; want 0", length)
	}
}

func TestWrapperCloseReturnsErrorCachedByInterrupt(t *testing.T) {
	testCases := []struct {
		name string
		wrap func(*Group, *trackingCloseConn) io.Closer
	}{
		{
			name: "Conn",
			wrap: func(group *Group, underlying *trackingCloseConn) io.Closer {
				return group.NewConn(underlying, false)
			},
		},
		{
			name: "PacketConn",
			wrap: func(group *Group, underlying *trackingCloseConn) io.Closer {
				return group.NewPacketConn(&trackingClosePacketConn{trackingCloseConn: underlying}, false)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			group := NewGroup()
			wantErr := errors.New("interrupt close failed")
			underlying := newTrackingCloseConn(wantErr)
			wrapped := testCase.wrap(group, underlying)

			group.Interrupt(false)
			if err := wrapped.Close(); !errors.Is(err, wantErr) {
				t.Fatalf("wrapper Close returned %v after Interrupt; want %v", err, wantErr)
			}
			if calls := underlying.closeCalls.Load(); calls != 1 {
				t.Fatalf("underlying Close called %d times; want 1", calls)
			}
			if length := group.connections.Len(); length != 0 {
				t.Fatalf("group contains %d connections after Interrupt; want 0", length)
			}
		})
	}
}

func TestGroupInterruptPreservesConnectionFiltering(t *testing.T) {
	group := NewGroup()
	internal := newTrackingCloseConn(nil)
	external := newTrackingCloseConn(nil)
	provider := newTrackingCloseConn(nil)
	externalProvider := newTrackingCloseConn(nil)

	wrappedInternal := group.NewConn(internal, false)
	wrappedExternal := group.NewConn(external, true)
	wrappedProvider := group.NewConn(provider, false, true)
	wrappedExternalProvider := group.NewConn(externalProvider, true, true)

	group.Interrupt(false)
	if calls := internal.closeCalls.Load(); calls != 1 {
		t.Fatalf("internal connection Close called %d times after Interrupt(false); want 1", calls)
	}
	for name, conn := range map[string]*trackingCloseConn{
		"external":          external,
		"provider":          provider,
		"external provider": externalProvider,
	} {
		if calls := conn.closeCalls.Load(); calls != 0 {
			t.Fatalf("%s connection Close called %d times after Interrupt(false); want 0", name, calls)
		}
	}
	if length := group.connections.Len(); length != 3 {
		t.Fatalf("group contains %d connections after Interrupt(false); want 3", length)
	}

	group.Interrupt(true)
	if calls := external.closeCalls.Load(); calls != 1 {
		t.Fatalf("external connection Close called %d times after Interrupt(true); want 1", calls)
	}
	for name, conn := range map[string]*trackingCloseConn{
		"provider":          provider,
		"external provider": externalProvider,
	} {
		if calls := conn.closeCalls.Load(); calls != 0 {
			t.Fatalf("%s connection Close called %d times after Interrupt(true); want 0", name, calls)
		}
	}
	if length := group.connections.Len(); length != 2 {
		t.Fatalf("group contains %d connections after Interrupt(true); want 2", length)
	}

	for name, conn := range map[string]net.Conn{
		"internal":          wrappedInternal,
		"external":          wrappedExternal,
		"provider":          wrappedProvider,
		"external provider": wrappedExternalProvider,
	} {
		if err := conn.Close(); err != nil {
			t.Fatalf("close %s connection: %v", name, err)
		}
	}
	for name, conn := range map[string]*trackingCloseConn{
		"internal":          internal,
		"external":          external,
		"provider":          provider,
		"external provider": externalProvider,
	} {
		if calls := conn.closeCalls.Load(); calls != 1 {
			t.Fatalf("%s connection Close called %d times after cleanup; want 1", name, calls)
		}
	}
	if length := group.connections.Len(); length != 0 {
		t.Fatalf("group contains %d connections after cleanup; want 0", length)
	}
}

func TestGroupConcurrentInterruptCloseAndRegistration(t *testing.T) {
	const iterations = 1000
	for iteration := 0; iteration < iterations; iteration++ {
		group := NewGroup()
		underlying := newTrackingCloseConn(nil)
		var wrapped io.Closer
		if iteration%2 == 0 {
			wrapped = group.NewConn(underlying, false)
		} else {
			wrapped = group.NewPacketConn(&trackingClosePacketConn{trackingCloseConn: underlying}, false)
		}
		start := make(chan struct{})
		var waitGroup sync.WaitGroup
		waitGroup.Add(4)

		for _, interruptExternal := range []bool{false, true} {
			go func(interruptExternal bool) {
				defer waitGroup.Done()
				<-start
				group.Interrupt(interruptExternal)
			}(interruptExternal)
		}
		go func() {
			defer waitGroup.Done()
			<-start
			_ = wrapped.Close()
		}()
		go func() {
			defer waitGroup.Done()
			<-start
			if iteration%2 == 0 {
				registered := group.NewConn(&noOpConn{}, false)
				_ = registered.Close()
			} else {
				registered := group.NewPacketConn(&noOpPacketConn{noOpConn: &noOpConn{}}, false)
				_ = registered.Close()
			}
		}()

		close(start)
		waitGroup.Wait()
		if calls := underlying.closeCalls.Load(); calls != 1 {
			t.Fatalf("iteration %d: underlying Close called %d times; want 1", iteration, calls)
		}
		if length := group.connections.Len(); length != 0 {
			t.Fatalf("iteration %d: group contains %d connections; want 0", iteration, length)
		}
	}
}
