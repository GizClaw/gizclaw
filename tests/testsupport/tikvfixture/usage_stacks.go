package tikvfixture

import (
	"runtime"
	"time"
)

type usageBlockedFrame struct {
	BlockedAt  time.Time `json:"blocked_at"`
	CapturedAt time.Time `json:"captured_at"`
	ElapsedMS  int64     `json:"elapsed_ms"`
	Truncated  bool      `json:"truncated"`
	Stack      string    `json:"stack"`
}

// captureBlocked preserves observations from the blocked interval across reset.
// Stack collection never holds the control mutex or the native transport locks.
// Unblocking or closing immediately cancels outstanding timers.
func (g *UsageGate) captureBlocked(block <-chan struct{}) {
	g.mu.Lock()
	if g.stopping.Load() || g.center != block {
		g.mu.Unlock()
		return
	}
	g.blockedStacks = nil
	g.stackWorkers.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.stackWorkers.Done()
		started := time.Now()
		for _, offset := range []time.Duration{time.Second, 3 * time.Second} {
			timer := time.NewTimer(time.Until(started.Add(offset)))
			select {
			case <-g.closing:
				timer.Stop()
				return
			case <-block:
				timer.Stop()
				return
			case <-timer.C:
			}
			buffer := make([]byte, 2<<20)
			length := runtime.Stack(buffer, true)
			frame := usageBlockedFrame{BlockedAt: started.UTC(), CapturedAt: time.Now().UTC(), ElapsedMS: time.Since(started).Milliseconds(), Truncated: length == len(buffer), Stack: string(buffer[:length])}
			select {
			case <-g.closing:
				return
			case <-block:
				return
			default:
			}
			g.mu.Lock()
			if g.center == block {
				g.blockedStacks = append(g.blockedStacks, frame)
			}
			g.mu.Unlock()
		}
	}()
}
