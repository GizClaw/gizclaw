package gizwebrtc

import (
	"strings"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// Receiver metadata is configured during SDP negotiation, before the packet
// DataChannel opens. Inspect it once, outside the metrics lock, so a silent but
// negotiated uplink counts without requiring its first RTP packet.
func (c *Conn) hasNegotiatedOpusReceiver() bool {
	if c.pc == nil {
		return false
	}
	for _, receiver := range c.pc.GetReceivers() {
		track := receiver.Track()
		if track == nil || track.Kind() != webrtc.RTPCodecTypeAudio {
			continue
		}
		for _, codec := range receiver.GetParameters().Codecs {
			if strings.EqualFold(codec.MimeType, MediaStreamOpus) {
				return true
			}
		}
	}
	return false
}

func (c *Conn) markRTPReceiveEstablished() {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()
	if !c.closed.Load() {
		c.markRTPReceiveEstablishedLocked()
	}
}

func (c *Conn) markRTPReceiveEstablishedLocked() {
	if c.rtpReceiveMetrics == nil {
		c.rtpReceiveMetrics = &c.trafficMetrics().rtp[0]
		c.rtpReceiveMetrics.active.Add(1)
	}
}

// Track binding measures negotiated send tracks, rather than counting the
// unbound audio track allocated for every connection. The writer measures actual
// RTP packets, including samples split into multiple packets.
// Each connection owns one sender, whose Pion lock serializes Bind and Unbind.
type meteredAudioTrack struct {
	webrtc.TrackLocal
	conn     *Conn
	mu       sync.Mutex
	bindings map[string]*rtpMetrics
}

func (t *meteredAudioTrack) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	counts := &t.conn.trafficMetrics().rtp[1]
	wrapped := &meteredTrackContext{TrackLocalContext: ctx, writer: &meteredRTPWriter{TrackLocalWriter: ctx.WriteStream(), counts: counts}}
	codec, err := t.TrackLocal.Bind(wrapped)
	if err != nil {
		return codec, err
	}
	id := ctx.ID()
	t.mu.Lock()
	if t.bindings == nil {
		t.bindings = make(map[string]*rtpMetrics)
	}
	if _, exists := t.bindings[id]; !exists {
		t.bindings[id] = counts
		counts.active.Add(1)
	}
	t.mu.Unlock()
	return codec, nil
}

func (t *meteredAudioTrack) Unbind(ctx webrtc.TrackLocalContext) error {
	if err := t.TrackLocal.Unbind(ctx); err != nil {
		return err
	}
	id := ctx.ID()
	t.mu.Lock()
	if counts := t.bindings[id]; counts != nil {
		delete(t.bindings, id)
		counts.active.Add(-1)
	}
	t.mu.Unlock()
	return nil
}

type meteredTrackContext struct {
	webrtc.TrackLocalContext
	writer webrtc.TrackLocalWriter
}

func (c *meteredTrackContext) WriteStream() webrtc.TrackLocalWriter { return c.writer }

type meteredRTPWriter struct {
	webrtc.TrackLocalWriter
	counts *rtpMetrics
}

func (w *meteredRTPWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	n, err := w.TrackLocalWriter.WriteRTP(header, payload)
	if err == nil {
		w.counts.packets.Add(1)
	}
	return n, err
}

func (w *meteredRTPWriter) Write(packet []byte) (int, error) {
	n, err := w.TrackLocalWriter.Write(packet)
	if err == nil && n == len(packet) {
		w.counts.packets.Add(1)
	}
	return n, err
}
