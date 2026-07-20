package interrupt

import (
	"net"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
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

func (c *PacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	if packetReader, ok := c.PacketConn.(N.PacketReader); ok {
		return packetReader.ReadPacket(buffer)
	}
	_, addr, err := buffer.ReadPacketFrom(c.PacketConn)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return M.SocksaddrFromNet(addr).Unwrap(), err
}

func (c *PacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	if packetWriter, ok := c.PacketConn.(N.PacketWriter); ok {
		return packetWriter.WritePacket(buffer, destination)
	}
	defer buffer.Release()
	_, err := c.PacketConn.WriteTo(buffer.Bytes(), destination.UDPAddr())
	return err
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
