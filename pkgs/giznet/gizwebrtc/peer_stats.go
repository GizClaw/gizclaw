package gizwebrtc

import "github.com/pion/webrtc/v4"

// Pion's GetStats and Close do not synchronize ownership of its RTP stats
// collector. Register snapshots before reading that native state;
// Conn.close stops admission and waits for readers before destroying the Peer.
func (c *Conn) collectStats() webrtc.StatsReport {
	if c == nil || c.pc == nil || !c.beginStats() {
		return nil
	}
	defer c.endStats()
	return c.pc.GetStats()
}

func (c *Conn) beginStats() bool {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	if c.statsClosed {
		return false
	}
	if c.statsReaders == 0 {
		c.statsIdle = make(chan struct{})
	}
	c.statsReaders++
	return true
}

func (c *Conn) endStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.statsReaders--
	if c.statsReaders == 0 {
		close(c.statsIdle)
	}
}

// stopStats returns the completion signal for every admitted snapshot. The
// caller waits outside statsMu, so readers can finish without blocking shutdown.
func (c *Conn) stopStats() <-chan struct{} {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.statsClosed = true
	return c.statsIdle
}
