package transport

import (
	"context"
	"net"
	"time"
)

func setConnDeadline(ctx context.Context, conn net.Conn, needClose bool) func() {
	if needClose {
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		return func() { stop() }
	}
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
		return func() { _ = conn.SetDeadline(time.Time{}) }
	}
	return func() {}
}
