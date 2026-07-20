package route

import (
	"errors"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sagernet/sing-tun"
)

func isTransientUnreachable(err error) bool {
	return errors.Is(err, tun.ErrNoRoute) || errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH)
}

type logThrottle struct {
	lastNano atomic.Int64
}

func (t *logThrottle) allow() bool {
	now := time.Now().UnixNano()
	previous := t.lastNano.Load()
	if now-previous < int64(time.Second) {
		return false
	}
	return t.lastNano.CompareAndSwap(previous, now)
}

var transientDNSLogThrottle logThrottle
