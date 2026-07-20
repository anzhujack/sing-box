package interrupt

import (
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing/common/x/list"
)

type Group struct {
	access      sync.Mutex
	connections list.List[*groupConnItem]
}

type groupConnItem struct {
	conn       io.Closer
	isExternal bool
	isProvider bool
	// element is protected by Group.access and is nil after the item is detached.
	element *list.Element[*groupConnItem]
	// closeOnce keeps Interrupt and wrapper Close from closing the same connection twice.
	closeOnce sync.Once
	closeErr  error
}

func (i *groupConnItem) close() error {
	i.closeOnce.Do(func() {
		i.closeErr = i.conn.Close()
	})
	return i.closeErr
}

func NewGroup() *Group {
	return &Group{}
}

func (g *Group) NewConn(conn net.Conn, isExternal, isProvider bool) net.Conn {
	g.access.Lock()
	defer g.access.Unlock()
	item := &groupConnItem{conn: conn, isExternal: isExternal, isProvider: isProvider}
	item.element = g.connections.PushBack(item)
	return &Conn{Conn: conn, group: g, item: item}
}

func (g *Group) NewPacketConn(conn net.PacketConn, isExternal, isProvider bool) net.PacketConn {
	g.access.Lock()
	defer g.access.Unlock()
	item := &groupConnItem{conn: conn, isExternal: isExternal, isProvider: isProvider}
	item.element = g.connections.PushBack(item)
	return &PacketConn{PacketConn: conn, group: g, item: item}
}

func (g *Group) close(item *groupConnItem) error {
	g.access.Lock()
	if item.element != nil {
		g.connections.Remove(item.element)
		item.element = nil
	}
	g.access.Unlock()
	return item.close()
}

// Interrupt detaches matching connections before closing them. Close operations
// are serialized per connection, but concurrent Interrupt calls do not wait for
// connections already detached by another call.
func (g *Group) Interrupt(interruptExternalConnections bool) {
	g.access.Lock()
	var toClose []*groupConnItem
	for element := g.connections.Front(); element != nil; {
		nextElement := element.Next()
		if !element.Value.isProvider && (!element.Value.isExternal || interruptExternalConnections) {
			g.connections.Remove(element)
			element.Value.element = nil
			toClose = append(toClose, element.Value)
		}
		element = nextElement
	}
	g.access.Unlock()

	// A Close implementation may block indefinitely. Never call it while holding
	// Group.access, otherwise unrelated NewConn and Close calls stall behind it.
	for _, item := range toClose {
		item.close()
	}
}
