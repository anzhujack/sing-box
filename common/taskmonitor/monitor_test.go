package taskmonitor

import (
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/log"
)

func TestFinishWithoutStartIsSafe(t *testing.T) {
	monitor := New(log.NewNOPFactory().NewLogger("task"), time.Hour)
	monitor.Finish()
	monitor.Finish()
}

func TestConcurrentStartAndFinishIsSafe(t *testing.T) {
	monitor := New(log.NewNOPFactory().NewLogger("task"), time.Hour)
	const workers = 64
	const iterations = 100
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for range workers {
		go func() {
			defer waitGroup.Done()
			for range iterations {
				monitor.Start("test")
				monitor.Finish()
			}
		}()
	}
	waitGroup.Wait()
	monitor.Finish()
}
