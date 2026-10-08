package giztestcmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// One clock and registry belong to one task, including its parallel children.
// Time.Sub keeps Go's monotonic clock; no wall-clock serialization is used.
type peerPacketClock struct {
	origin time.Time
	sends  sync.Map // SHA-256 of a uniquely padded packet -> *peerPacketSend
}

func newPeerPacketClock() *peerPacketClock { return &peerPacketClock{origin: time.Now()} }

type peerPacketSend struct {
	id, client, step, identity string
	started, sent              time.Time
	completed                  atomic.Pointer[time.Time]
}

type peerPacketReceipt struct {
	send     *peerPacketSend
	received time.Time
}

const maxFirstPacketReceipts = 128

// packetTimedPeerStream measures successful nonempty audio Push calls and
// receipts before audibility decoding. The packet marker is opt-in and never
// applied to normal conversation inputs or empty control/BOS chunks.
type packetTimedPeerStream struct {
	peerStream
	clock                  *peerPacketClock
	step                   giztest.Step
	identity               string
	started                time.Time
	sent                   *peerPacketSend // written only by the input_sent sender goroutine
	listenStart, listenEnd time.Time       // set before the reader starts

	mu       sync.Mutex
	closed   bool
	receipts []peerPacketReceipt
	dropped  int
}

func (s *packetTimedPeerStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if chunk == nil {
		return s.peerStream.Push(ctx, chunk)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !s.step.PeerStream.MeasureFirstPacket || s.sent != nil || !ok || len(blob.Data) == 0 || !relayOpusMIME(blob.MIMEType) {
		return s.peerStream.Push(ctx, chunk)
	}
	id := rand.Text()
	packet, marked, err := markFirstOpusPacket(blob.Data, id)
	if err != nil {
		return fmt.Errorf("mark first audio packet: %w", err)
	}
	if marked {
		copyBlob := *blob
		copyBlob.Data = packet
		copyChunk := *chunk
		copyChunk.Part = &copyBlob
		chunk = &copyChunk
	} else {
		id = "" // Preserve SFU's canonical silence/DTX classification.
	}
	s.sent = &peerPacketSend{id: id, client: s.step.Client, step: s.step.ID, identity: s.identity, started: s.started, sent: time.Now()}
	if marked {
		// Publish before Push: a full-duplex receipt may precede its return.
		// completed remains nil if the actual nonempty Push fails.
		s.clock.sends.Store(sha256.Sum256(packet), s.sent)
	}
	if err := s.peerStream.Push(ctx, chunk); err != nil {
		return err
	}
	completed := time.Now()
	s.sent.completed.Store(&completed)
	return nil
}

func (s *packetTimedPeerStream) observePacketArrival(chunk *genx.MessageChunk, received time.Time) {
	if !s.listenStart.IsZero() && (received.Before(s.listenStart) || !received.Before(s.listenEnd)) {
		return
	}
	if chunk == nil || chunk.Ctrl == nil || !strings.HasPrefix(chunk.Ctrl.StreamID, "sfu/") {
		return
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || len(blob.Data) == 0 || !relayOpusMIME(blob.MIMEType) {
		return
	}
	value, ok := s.clock.sends.Load(sha256.Sum256(blob.Data))
	if !ok {
		return
	}
	send := value.(*peerPacketSend)
	if send.client == s.step.Client || (send.identity != "" && chunk.Ctrl.Label != send.identity) || received.Before(send.sent) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for _, receipt := range s.receipts {
		if receipt.send == send {
			return // Retransmissions do not create extra first-packet samples.
		}
	}
	if len(s.receipts) == maxFirstPacketReceipts {
		s.dropped++
		return
	}
	s.receipts = append(s.receipts, peerPacketReceipt{send: send, received: received})
}

func (s *packetTimedPeerStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.peerStream.Close()
}

func packetMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func (s *packetTimedPeerStream) sendEvidence(send *peerPacketSend) map[string]any {
	completed := send.completed.Load()
	if completed == nil {
		return nil
	}
	result := map[string]any{
		"clock": "task_monotonic", "sender_client": send.client, "sender_step": send.step,
		"send_start_ms":     packetMilliseconds(send.sent.Sub(s.clock.origin)),
		"send_completed_ms": packetMilliseconds(completed.Sub(s.clock.origin)),
		"startup_ms":        packetMilliseconds(send.sent.Sub(send.started)),
	}
	if send.id != "" {
		result["packet_id"] = send.id
	} else {
		result["uncorrelated_reason"] = "silence_or_dtx"
	}
	return result
}

func (s *packetTimedPeerStream) addEvidence(result *operationResult) {
	fields := map[string]any{}
	if s.sent != nil {
		if send := s.sendEvidence(s.sent); send != nil {
			fields["audio_first_packet"] = send
		}
	}
	if s.step.PeerStream.Mode == "listen" {
		s.mu.Lock()
		receipts, dropped := slices.Clone(s.receipts), s.dropped
		s.mu.Unlock()
		samples := make([]any, 0, len(receipts))
		for _, receipt := range receipts {
			sample := s.sendEvidence(receipt.send)
			if sample == nil {
				continue // No successful send proof; never invent zero latency.
			}
			sample["receive_ms"] = packetMilliseconds(receipt.received.Sub(s.clock.origin))
			sample["latency_ms"] = packetMilliseconds(receipt.received.Sub(receipt.send.sent))
			samples = append(samples, sample)
		}
		fields["audio_first_packets"] = samples
		if dropped > 0 {
			fields["audio_first_packets_dropped"] = dropped
		}
	}
	result.evidence = mapsWith(result.evidence, fields)
	if object, ok := result.assertion.(map[string]any); ok {
		mapsWith(object, fields)
	}
	if object, ok := result.saved.(map[string]any); ok {
		mapsWith(object, fields)
	}
}

// markFirstOpusPacket preserves frame bytes and adds a unique test marker in
// decoder-ignored Opus padding (RFC 6716 section 3.2.5). This is a test probe,
// not an Opus encoder. Original padding is retained. Canonical SFU silence
// packets are left unchanged so instrumentation cannot acquire the floor.
func markFirstOpusPacket(packet []byte, id string) ([]byte, bool, error) {
	if len(packet) <= 1 || (bytes.HasPrefix(packet, []byte{0xf8, 0xff, 0xfe}) && bytes.Count(packet[3:], []byte{0}) == len(packet)-3) {
		return packet, false, nil
	}
	frames := packet[1:]
	var count byte
	var padding []byte
	switch packet[0] & 3 {
	case 0:
		count = 1
	case 1:
		count = 2
	case 2:
		count = 0x82 // VBR: frames already starts with the first frame length.
	case 3:
		count = packet[1]
		frames = packet[2:]
		if count&0x3f == 0 {
			return nil, false, fmt.Errorf("invalid Opus frame count")
		}
		if count&0x40 != 0 {
			length := 0
			for {
				if len(frames) == 0 {
					return nil, false, fmt.Errorf("truncated Opus padding length")
				}
				n := int(frames[0])
				frames = frames[1:]
				length += min(n, 254)
				if length > len(frames) {
					return nil, false, fmt.Errorf("Opus padding exceeds packet")
				}
				if n != 255 {
					break
				}
			}
			padding = frames[len(frames)-length:]
			frames = frames[:len(frames)-length]
		}
	}
	marker := []byte("\x00giztest/" + id)
	length := len(padding) + len(marker)
	out := []byte{packet[0]&0xfc | 3, count | 0x40}
	for length >= 254 {
		out = append(out, 255)
		length -= 254
	}
	out = append(out, byte(length))
	out = append(out, frames...)
	out = append(out, padding...)
	out = append(out, marker...)
	return out, true, nil
}
