package taskmonitor

import (
	"sync"
	"time"

	F "github.com/sagernet/sing/common/format"
	"github.com/sagernet/sing/common/logger"
)

// Monitor may be reached by concurrent lifecycle paths. The original timer
// field was unprotected, so concurrent Start/Finish could race and call Stop
// on an invalid or already-replaced timer.
//
//   - G1.Start() 设 m.timer = T1
//   - G2.Start() 设 m.timer = T2 (丢失 T1)
//   - G1.Finish() 读 m.timer (可能看到 T2 或半写入状态)
//   - 进程被 Go 1.23+ 的 "Stop called on uninitialized Timer"
//     防御性 panic 爆掉
//
// The mutex protects timer ownership and Finish is idempotent. Monitor still
// intentionally represents one active task at a time.
type Monitor struct {
	logger  logger.Logger
	timeout time.Duration
	mu      sync.Mutex
	timer   *time.Timer
}

func New(logger logger.Logger, timeout time.Duration) *Monitor {
	return &Monitor{
		logger:  logger,
		timeout: timeout,
	}
}

// Start 启动一次超时监视。若上一个 Start 没有配对的 Finish (并发调用或
// 调用方漏掉 Finish)，先 Stop 掉旧 timer 再建新的，避免 timer 泄漏。
func (m *Monitor) Start(taskName ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old := m.timer; old != nil {
		old.Stop()
	}
	m.timer = time.AfterFunc(m.timeout, func() {
		m.logger.Warn(F.ToString(taskName...), " take too much time to finish!")
	})
}

// Finish 停止当前 timer。
//   - Start 从未调用过 (m.timer == nil) 时 no-op，不再 panic
//   - 并发多次 Finish 幂等，第二次 no-op (m.timer == nil 之后)
//   - Go 1.23+ 对 zero-value *time.Timer 的 Stop 会 panic
//     "uninitialized Timer"；Finish 用 Swap 后判空确保只对真实
//     AfterFunc 返回的 Timer 做 Stop
func (m *Monitor) Finish() {
	m.mu.Lock()
	t := m.timer
	m.timer = nil
	m.mu.Unlock()
	if t != nil {
		t.Stop()
	}
}
