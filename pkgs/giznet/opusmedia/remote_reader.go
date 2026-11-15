package opusmedia

import (
	"context"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

// EncodedPacket is an owned real payload with opaque media timing. Business
// consumers may select or forward it without interpreting RTP.
type EncodedPacket struct {
	Data   []byte
	Timing *packet.OpusTiming
}

// EncodedReader reads real media. Cancellation ends the read, not its parent
// connection. One owner serializes reads.
type EncodedReader interface {
	ReadAudio(context.Context) (*EncodedPacket, error)
}

// RemoteRTPReader is the media surface exposed by a subscribed WebRTC track.
type RemoteRTPReader interface {
	ReadRTP() (*rtp.Packet, interceptor.Attributes, error)
	SetReadDeadline(time.Time) error
}

// RemoteReader adapts a subscribed RTP track into owned encoded media. RTP
// parsing, source identity and cancellation mechanics stay at this boundary.
type RemoteReader struct {
	reader RemoteRTPReader
	source string
}

// NewRemoteReader creates an idle adapter; it starts no goroutines or I/O.
func NewRemoteReader(reader RemoteRTPReader, source string) *RemoteReader {
	return &RemoteReader{reader: reader, source: source}
}

// ReadAudio copies a real packet before handing it to the consumer.
func (r *RemoteReader) ReadAudio(ctx context.Context) (*EncodedPacket, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline, _ := ctx.Deadline()
	if err := r.reader.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	var finished sync.WaitGroup
	finished.Add(1)
	stop := context.AfterFunc(ctx, func() { defer finished.Done(); _ = r.reader.SetReadDeadline(time.Now()) })
	defer func() {
		if !stop() {
			finished.Wait()
		}
	}()
	p, _, err := r.reader.ReadRTP()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return EncodedRTP(p, r.source), nil
}

// EncodedRTP snapshots a received RTP packet at a media adapter boundary.
func EncodedRTP(p *rtp.Packet, source string) *EncodedPacket {
	if p == nil {
		return nil
	}
	info := &packet.OpusInfo{SourceID: source, Header: p.Header, Duration48: packet.OpusDuration48(p.Payload)}
	return &EncodedPacket{Data: append([]byte(nil), p.Payload...), Timing: packet.NewOpusTiming(info)}
}

// Reorder orders real originals without decoding or synthesizing gap payloads.
// Its owner serializes calls and drives Flush on idle or route boundaries.
type Reorder struct {
	buffer *reorderBuffer
	source string
}

// NewReorder creates a bounded encoded reorder buffer. Zero bounds select the
// defaults: five packets and 100 ms of timestamp span.
func NewReorder(maxDepth int, maxSpan uint32) *Reorder {
	return &Reorder{buffer: newReorderBuffer(maxDepth, maxSpan)}
}

// Push releases deliverable real packets with their timing intact.
func (r *Reorder) Push(p *EncodedPacket) []*EncodedPacket {
	if p == nil || len(p.Data) > 64*1024 {
		return nil
	}
	info := p.Timing.MediaInfo()
	if info == nil {
		return []*EncodedPacket{p}
	}
	if r.source != info.SourceID {
		r.buffer = newReorderBuffer(r.buffer.maxDepth, r.buffer.maxSpan)
		r.source = info.SourceID
	}
	return encodedPackets(r.buffer.Push(&rtp.Packet{Header: info.Header, Payload: p.Data}), info.SourceID)
}

// Flush releases only already-held originals, retaining gaps in their clocks.
func (r *Reorder) Flush() []*EncodedPacket { return encodedPackets(r.buffer.Flush(), r.source) }

// Len reports retained originals.
func (r *Reorder) Len() int { return r.buffer.Len() }

func encodedPackets(packets []*rtp.Packet, source string) []*EncodedPacket {
	out := make([]*EncodedPacket, 0, len(packets))
	for _, p := range packets {
		out = append(out, EncodedRTP(p, source))
	}
	return out
}
