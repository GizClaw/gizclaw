package gizwebrtc

import "github.com/pion/webrtc/v4"

// Diagnostics need only the selected ICE pair. Pion can close the PeerConnection
// internally without passing through Conn.close, so its unsynchronized RTP stats
// getter must not be read through PeerConnection.GetStats here.
func (c *Conn) collectStats() webrtc.StatsReport {
	if c == nil || c.pc == nil || !c.beginStats() {
		return nil
	}
	defer c.endStats()
	sctp := c.pc.SCTP()
	if sctp == nil || sctp.Transport() == nil || sctp.Transport().ICETransport() == nil {
		return nil
	}
	ice := sctp.Transport().ICETransport()
	before, ok := ice.GetSelectedCandidatePairStats()
	if !ok {
		return nil
	}
	pair, err := ice.GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Local == nil || pair.Remote == nil {
		return nil
	}
	stats, ok := ice.GetSelectedCandidatePairStats()
	if !ok || before.ID != stats.ID || before.LocalCandidateID != stats.LocalCandidateID ||
		before.RemoteCandidateID != stats.RemoteCandidateID {
		return nil
	}
	// Project only the native candidate fields consumed by our ICE diagnostics.
	// No RTP, media, or synthetic counters are added to this private report.
	return webrtc.StatsReport{
		stats.ID: stats,
		stats.LocalCandidateID: webrtc.ICECandidateStats{
			ID: stats.LocalCandidateID, IP: pair.Local.Address, Port: int32(pair.Local.Port),
			Protocol: pair.Local.Protocol.String(), CandidateType: pair.Local.Typ,
		},
		stats.RemoteCandidateID: webrtc.ICECandidateStats{
			ID: stats.RemoteCandidateID, IP: pair.Remote.Address, Port: int32(pair.Remote.Port),
			Protocol: pair.Remote.Protocol.String(), CandidateType: pair.Remote.Typ,
		},
	}
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
