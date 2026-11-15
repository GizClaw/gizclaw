package gizwebrtc

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type rtpWriter interface {
	WriteRTP(*rtp.Packet) error
}

// opusSampleWriter supplies a media timeline for payload-only callers while
// the same RTP track can forward explicitly timestamped packets unchanged.
type opusSampleWriter struct {
	track   rtpWriter
	mu      sync.Mutex
	seq     uint16
	ts      uint32
	started bool
	clock   opusmedia.Clock
	local   bool
}

func newOpusSampleWriter(track *webrtc.TrackLocalStaticRTP) *opusSampleWriter {
	return &opusSampleWriter{track: track, seq: uint16(rand.Uint32()), ts: rand.Uint32()}
}

func (w *opusSampleWriter) WriteSample(sample media.Sample) error {
	ticks := uint32(sample.Duration * 48000 / time.Second)
	w.mu.Lock()
	w.seq += sample.PrevDroppedPackets
	w.ts += uint32(sample.PrevDroppedPackets) * ticks
	// Payload-only samples advance one continuous media clock. Mark only its
	// start; a marker on every sample would disguise ordinary loss as DTX.
	header := rtp.Header{Version: 2, SequenceNumber: w.seq, Timestamp: w.ts, Marker: !w.started || w.local}
	w.local = false
	w.started = true
	w.seq++
	w.ts += ticks
	w.mu.Unlock()
	// Assignment of the sequence is local state; transport I/O holds no state
	// mutex. Receivers retain sequence numbers even if writers complete out of order.
	return w.track.WriteRTP(&rtp.Packet{Header: header, Payload: sample.Data})
}

func (w *opusSampleWriter) WriteLocalAudio(ctx context.Context, payload []byte, timing *packet.OpusTiming, segment string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	w.mu.Lock()
	if !w.local {
		w.clock = opusmedia.Clock{Sequence: w.seq, Timestamp: w.ts}
	}
	info, err := w.clock.Next(payload, timing, segment)
	if err == nil {
		w.seq, w.ts = w.clock.Sequence, w.clock.Timestamp
		w.started, w.local = true, true
	}
	w.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := w.track.WriteRTP(&rtp.Packet{Header: info.Header, Payload: payload}); err != nil {
		return 0, err
	}
	return len(payload), nil
}
