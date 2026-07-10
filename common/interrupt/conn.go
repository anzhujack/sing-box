package interrupt

import (
	"net"

	"github.com/sagernet/sing/common/bufio"
)

type Conn struct {
	net.Conn
	group *Group
	item  *groupConnItem
}

func (c *Conn) Close() error {
	return c.group.close(c.item)
}

func (c *Conn) ReaderReplaceable() bool {
	return true
}

func (c *Conn) WriterReplaceable() bool {
	return true
}

func (c *Conn) Upstream() any {
	return c.Conn
}

type PacketConn struct {
	net.PacketConn
	group *Group
	item  *groupConnItem
}

func (c *PacketConn) Close() error {
	return c.group.close(c.item)
}

func (c *PacketConn) ReaderReplaceable() bool {
	return true
}

func (c *PacketConn) WriterReplaceable() bool {
	return true
}

func (c *PacketConn) Upstream() any {
	return bufio.NewPacketConn(c.PacketConn)
}
