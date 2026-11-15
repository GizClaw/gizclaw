package opusmedia

import (
	"context"
	"errors"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/rtp"
)

// RTPWriter is the destination track surface used by an encoded media writer.
type RTPWriter interface{ WriteRTP(*rtp.Packet) error }

// Writer forwards selected encoded media onto one destination RTP track. Clock
// allocation is serialized; transport I/O runs outside the state mutex. The
// connection owner closes the track/connection to interrupt an in-flight write.
type Writer struct {
	track RTPWriter
	mu    sync.Mutex
	clock Clock
}

// NewWriter creates an idle, codec-free writer without starting I/O.
func NewWriter(track RTPWriter) *Writer { return &Writer{track: track} }

// Write forwards one real payload, preserving its timing within the segment.
func (w *Writer) Write(ctx context.Context, payload []byte, timing *packet.OpusTiming, segment string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	info, err := w.clock.Next(payload, timing, segment)
	w.mu.Unlock()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return w.track.WriteRTP(&rtp.Packet{Header: info.Header, Payload: payload})
}

// Clock allocates a destination RTP timeline for local audio. Timed segments
// preserve source sequence and timestamp differences; new segments rebase onto
// the same destination clock used by ordinary untimed audio. Its owner serializes
// calls, independently of transport I/O. It retains constant-size state.
type Clock struct {
	Sequence   uint16
	Timestamp  uint32
	started    bool
	timed      bool
	segment    string
	source     string
	ssrc       uint32
	inputSeq   uint16
	inputTime  uint32
	outputSeq  uint16
	outputTime uint32
}

// Next allocates one real packet. A different segment, source, or explicit media
// discontinuity starts a marked talkspurt, without synthesizing a time gap.
// Empty segment names select ordinary payload-only audio.
func (c *Clock) Next(payload []byte, timing *packet.OpusTiming, segment string) (*packet.OpusInfo, error) {
	duration := packet.OpusDuration48(payload)
	if len(payload) == 0 || duration == 0 || duration > 5760 {
		return nil, errors.New("opusmedia: invalid local Opus packet")
	}
	info := timing.MediaInfo()
	if info != nil && (info.Missing || info.Header.Version != 2 || info.Duration48 != duration) {
		return nil, errors.New("opusmedia: invalid encoded timing")
	}
	seq, ts := c.Sequence, c.Timestamp
	marker := !c.started
	if info != nil {
		continuous := c.timed && c.segment == segment && c.source == info.SourceID && c.ssrc == info.Header.SSRC && !info.Discontinuity && !info.Header.Marker
		if continuous {
			deltaSeq := uint16(info.Header.SequenceNumber - c.inputSeq)
			deltaTime := int32(info.Header.Timestamp - c.inputTime)
			// The receive media layer already orders originals. Reject stale duplicates;
			// ambiguous jumps start a new segment rather than manufacturing a huge gap.
			if deltaSeq == 0 || (int16(deltaSeq) < 0 && deltaTime <= 0) {
				return nil, errors.New("opusmedia: stale encoded packet")
			}
			if deltaSeq < 3000 && deltaTime > 0 && uint32(deltaTime) <= packet.OpusClockRate/2+5760 {
				seq = c.outputSeq + deltaSeq
				ts = c.outputTime + uint32(deltaTime)
			} else {
				marker = true
			}
		} else {
			marker = true
		}
		c.timed = true
		c.segment, c.source, c.ssrc = segment, info.SourceID, info.Header.SSRC
		c.inputSeq, c.inputTime = info.Header.SequenceNumber, info.Header.Timestamp
		c.outputSeq, c.outputTime = seq, ts
	} else {
		marker = marker || c.timed || c.segment != segment
		c.timed = false
		c.segment = segment
	}
	c.Sequence, c.Timestamp = seq+1, ts+duration
	c.started = true
	return &packet.OpusInfo{Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: ts, Marker: marker, PayloadType: 111, SSRC: 1}, Duration48: duration}, nil
}
