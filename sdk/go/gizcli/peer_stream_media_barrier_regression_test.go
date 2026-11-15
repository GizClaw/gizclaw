package gizcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
	"github.com/pion/rtp"
)

// Both subscribers use OpenPeerStream and physical Event framing. The real
// Client packet pump owns PacketReader, its jitter deadlines, and its decoder.
func TestPeerEventBoundarySlowSubscriberPreservesActiveMedia(t *testing.T) {
	for _, gap := range []bool{false, true} {
		name := "continuous-decoder"
		if gap {
			name = "known-gap"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				packets := peerBoundaryOpus(t)
				h := newPeerBoundaryHarness(t, &Client{}, true)
				fast, slow := h.open(t, 1), h.open(t, 1)
				h.event(t, bosEvent("a", "assistant", "audio/opus"))
				if !peerBoundaryNext(t, fast).IsBeginOfStream() {
					t.Fatal("missing A BOS")
				}
				// The slow subscriber retains BOS in its one-slot output and
				// blocks on this text, before it can process EOS(A).
				h.event(t, textEvent("a", "assistant", "hold slow Next"))
				peerBoundaryNext(t, fast)
				synctest.Wait()
				h.packet(t, packets[0], 1, 0)
				h.event(t, eosEvent("a", "assistant", "audio/opus", nil))
				first := peerBoundaryNext(t, fast)
				if blob, ok := first.Part.(*genx.Blob); !ok || blob.OriginalAudio == nil || !bytes.Equal(blob.OriginalAudio.Data, packets[0]) {
					t.Fatal("physical EOS overtook the receiver's pending A packet")
				}
				if !peerBoundaryNext(t, fast).IsEndOfStream() {
					t.Fatal("missing A EOS after media dispatch")
				}
				h.event(t, bosEvent("b", "assistant", "audio/opus"))
				if !peerBoundaryNext(t, fast).IsBeginOfStream() {
					t.Fatal("missing B BOS")
				}
				reference := peerBoundaryDecoder(t)
				h.packet(t, packets[1], 2, 960)
				peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "b", packets[1], false)
				// Successful receiver dispatch is independent of slow Next, even
				// when local delivery takes longer than the two-second fence.
				time.Sleep(3 * time.Second)
				peerBoundaryDrainEOS(t, slow, "a")
				if chunk := peerBoundaryNext(t, slow); !chunk.IsBeginOfStream() || chunk.Ctrl.StreamID != "b" {
					t.Fatal("slow subscriber lost the replacement BOS")
				}
				if gap {
					// Sequence 3 is lost within B, with no talkspurt marker. A
					// stale EOS reset would erase the clock needed for this PLC.
					h.packet(t, packets[3], 4, 2880)
					peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "b", nil, true)
					peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "b", packets[3], false)
				} else {
					// Compare samples as well as byte counts: restarting the Opus
					// decoder between consecutive B packets must be observable.
					h.packet(t, packets[2], 3, 1920)
					peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "b", packets[2], false)
				}
			})
		})
	}
}

func TestPeerEventBoundaryReconnectKeepsOldSessionOffNewReceiver(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		name := "closed-old-connection"
		if overlap {
			name = "overlapping-physical-readers"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				packets := peerBoundaryOpus(t)
				client := &Client{}
				old := newPeerBoundaryHarness(t, client, true)
				slow := old.open(t, 1)
				observer, err := client.DialPeerEventStream()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = observer.Close() })
				old.event(t, bosEvent("old", "assistant", "audio/opus"))
				peerBoundaryReadEvent(t, observer)
				old.event(t, textEvent("old", "assistant", "hold old Next"))
				peerBoundaryReadEvent(t, observer)
				synctest.Wait()
				if !overlap {
					old.event(t, eosEvent("old", "assistant", "audio/opus", nil))
					peerBoundaryReadEvent(t, observer)
					synctest.Wait()
					if err := client.Close(); err != nil {
						t.Fatal(err)
					}
					old.stop(t)
				}
				current := newPeerBoundaryHarness(t, client, true)
				fast := current.open(t, 1)
				current.event(t, bosEvent("new", "assistant", "audio/opus"))
				peerBoundaryNext(t, fast)
				reference := peerBoundaryDecoder(t)
				current.packet(t, packets[0], 1, 0)
				peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "new", packets[0], false)
				if overlap {
					// An old physical reader finishing after replacement must fence
					// its original receiver, never look up Client's current one.
					old.event(t, eosEvent("old", "assistant", "audio/opus", nil))
					peerBoundaryReadEvent(t, observer)
					synctest.Wait()
					old.stop(t)
				}
				// Shutdown discards unread physical frames, but this EOS was
				// already queued in a blocked PeerStream before the disconnect.
				peerBoundaryDrainEOS(t, slow, "old")
				_ = slow.Close()
				current.packet(t, packets[2], 3, 1920)
				peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "new", nil, true)
				peerBoundaryPCM(t, peerBoundaryNext(t, fast), reference, "new", packets[2], false)
			})
		})
	}
}

func TestPeerEventBoundaryNativeAndLegacyFormatsAgree(t *testing.T) {
	for _, mode := range []string{"legacy-payload", "rtp-runtime-default", "rtp-encoded", "rtp-untimed-runtime-default", "rtp-untimed-encoded"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				timed := mode != "legacy-payload"
				var outputMode []opusmedia.OutputMode
				if mode == "rtp-encoded" || mode == "rtp-untimed-encoded" {
					outputMode = []opusmedia.OutputMode{opusmedia.OutputEncoded}
				}
				h := newPeerBoundaryHarness(t, &Client{}, timed, outputMode...)
				encoded, unsubscribe := h.client.subscribePeerPackets(giznet.ProtocolOpusPacket, 1)
				defer unsubscribe()
				s := h.open(t, 1)
				h.event(t, bosEvent("format", "assistant", "audio/opus"))
				bos := peerBoundaryNext(t, s)
				payload := []byte{0xf8, 0xff, 0xfe}
				if mode == "rtp-untimed-runtime-default" || mode == "rtp-untimed-encoded" {
					select {
					case h.conn.input <- peerBoundaryPacket{payload: payload}:
					case <-time.After(3 * time.Second):
						t.Fatal("packet pump did not accept untimed media")
					}
				} else {
					h.packet(t, payload, 1, 0)
				}
				var data *genx.MessageChunk
				if !timed {
					// The legacy lanes have no receiver fence: observe delivery
					// before sending EOS rather than assume cross-lane ordering.
					data = peerBoundaryNext(t, s)
				}
				h.event(t, eosEvent("format", "assistant", "audio/opus", nil))
				if timed {
					data = peerBoundaryNext(t, s)
				}
				eos := peerBoundaryNext(t, s)
				wantPCM := timed && h.session.outputMode == opusmedia.OutputPCM
				wantMIME := "audio/opus"
				if wantPCM {
					wantMIME = "audio/pcm;rate=16000;channels=1"
				}
				for _, chunk := range []*genx.MessageChunk{bos, data, eos} {
					blob, ok := chunk.Part.(*genx.Blob)
					if !ok || blob.MIMEType != wantMIME || chunk.Ctrl.StreamID != "format" || chunk.Ctrl.Label != "assistant" {
						t.Fatalf("control/payload format disagreement: %+v", chunk)
					}
				}
				if !bos.IsBeginOfStream() || data.IsBeginOfStream() || data.IsEndOfStream() || !eos.IsEndOfStream() {
					t.Fatal("format conversion changed route boundaries")
				}
				blob := data.Part.(*genx.Blob)
				if wantPCM {
					if len(blob.Data) != 640 || blob.OriginalAudio == nil || !bytes.Equal(blob.OriginalAudio.Data, payload) || blob.ConcealedAudio {
						t.Fatal("native RTP lost PCM output or real encoded provenance")
					}
				} else if !bytes.Equal(blob.Data, payload) || blob.OriginalAudio != nil {
					t.Fatal("encoded output changed its real packet or provenance")
				}
				select {
				case original := <-encoded:
					if !bytes.Equal(original, payload) {
						t.Fatal("encoded subscriber did not receive the real packet")
					}
				default:
					t.Fatal("packet pump lost the encoded subscriber's packet")
				}
				h.event(t, textEvent("after", "assistant", "reliable control remains usable"))
				if chunk := peerBoundaryNext(t, s); chunk.Part != genx.Text("reliable control remains usable") {
					t.Fatal("encoded reception interrupted subsequent reliable control")
				}
			})
		})
	}
}

func TestPeerEventBoundaryIgnoresUnrelatedTerminals(t *testing.T) {
	for _, terminal := range []string{"old-output", "input", "text"} {
		t.Run(terminal, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				packets := peerBoundaryOpus(t)
				h := newPeerBoundaryHarness(t, &Client{}, true)
				view, err := h.client.DialPeerEventStream()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = view.Close() })
				audio, unsubscribe := h.client.subscribePeerAudio(1)
				defer unsubscribe()
				h.event(t, bosEvent("old", "assistant", "audio/opus"))
				peerBoundaryReadEvent(t, view)
				h.event(t, bosEvent("active", "assistant", "audio/opus"))
				peerBoundaryReadEvent(t, view)
				reference := peerBoundaryDecoder(t)
				h.packet(t, packets[0], 1, 0)
				peerBoundarySamples(t, peerBoundaryAudio(t, audio), reference, packets[0], false)
				end := eosEvent("active", "assistant", "audio/opus", nil)
				switch terminal {
				case "old-output":
					end.GetEos().StreamId = "old"
				case "input":
					end.GetEos().Label = "user"
				case "text":
					end.GetEos().Kind = eventpb.StreamKind_STREAM_KIND_TEXT
					end.GetEos().MimeType = "text/plain"
				}
				h.event(t, end)
				peerBoundaryReadEvent(t, view)
				h.packet(t, packets[2], 3, 1920)
				peerBoundarySamples(t, peerBoundaryAudio(t, audio), reference, nil, true)
				peerBoundarySamples(t, peerBoundaryAudio(t, audio), reference, packets[2], false)
			})
		})
	}
}

func TestPeerEventBoundaryTimeoutStopsPublication(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, physical := net.Pipe()
		t.Cleanup(func() { _ = server.Close() })
		failed := make(chan struct{}, 1)
		session := newPeerEventSession(physical, func() { failed <- struct{}{} })
		t.Cleanup(func() { _ = session.close() })
		// A stalled packet pump cannot acknowledge the real PacketReader fence.
		reader, err := opusmedia.NewPacketReader(func(context.Context, []byte) (byte, int, *packet.OpusInfo, error) {
			t.Error("EOS attempted to read future media")
			return 0, 0, nil, io.EOF
		}, opusmedia.Config{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reader.Close() })
		session.mediaReader.Store(reader)
		view, err := session.subscribe()
		if err != nil {
			t.Fatal(err)
		}
		session.start()
		if err := WritePeerStreamEvent(server, bosEvent("stalled", "assistant", "audio/opus")); err != nil {
			t.Fatal(err)
		}
		peerBoundaryReadEvent(t, view)
		started := time.Now()
		if err := WritePeerStreamEvent(server, eosEvent("stalled", "assistant", "audio/opus", nil)); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-failed:
			t.Fatal("receiver fence failed before its deadline")
		default:
		}
		<-failed
		if elapsed := time.Since(started); elapsed != 2*time.Second {
			t.Fatalf("receiver fence deadline = %s, want 2s", elapsed)
		}
		if _, err := ReadPeerStreamEvent(view); !errors.Is(err, io.EOF) {
			t.Fatalf("failed receiver fence published EOS: %v", err)
		}
	})
}

type peerBoundaryPacket struct {
	payload []byte
	info    *packet.OpusInfo
}

type peerBoundaryConn struct {
	recordingGiznetConn
	timed   bool
	input   chan peerBoundaryPacket
	reading chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (c *peerBoundaryConn) RTPAudio() bool { return c.timed }

func (c *peerBoundaryConn) Read(dst []byte) (byte, int, error) {
	protocol, n, _, err := c.ReadPacket(context.Background(), dst)
	return protocol, n, err
}

func (c *peerBoundaryConn) ReadPacket(ctx context.Context, dst []byte) (byte, int, *packet.OpusInfo, error) {
	select {
	case c.reading <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return 0, 0, nil, ctx.Err()
	case <-c.closed:
		return 0, 0, nil, net.ErrClosed
	case p := <-c.input:
		return giznet.ProtocolOpusPacket, copy(dst, p.payload), p.info, nil
	}
}

func (c *peerBoundaryConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

type peerBoundaryHarness struct {
	client  *Client
	conn    *peerBoundaryConn
	session *peerEventSession
	server  net.Conn
	done    chan error
	once    sync.Once
}

func newPeerBoundaryHarness(t *testing.T, client *Client, timed bool, mode ...opusmedia.OutputMode) *peerBoundaryHarness {
	t.Helper()
	server, physical := net.Pipe()
	h := &peerBoundaryHarness{
		client: client,
		conn: &peerBoundaryConn{
			timed: timed, input: make(chan peerBoundaryPacket), reading: make(chan struct{}, 1), closed: make(chan struct{}),
		},
		session: newPeerEventSession(physical, nil), server: server, done: make(chan error, 1),
	}
	if len(mode) > 0 {
		h.session.outputMode = mode[0]
	}
	client.mu.Lock()
	client.conn, client.events = h.conn, h.session
	client.mu.Unlock()
	go func() { h.done <- client.servePackets() }()
	select {
	case <-h.conn.reading:
	case <-time.After(3 * time.Second):
		t.Fatal("packet pump did not start")
	}
	if timed {
		reader := h.session.mediaReader.Load()
		if reader == nil || reader.OutputMode() != h.session.outputMode {
			t.Fatal("packet pump did not bind the physical session's output mode")
		}
	}
	h.session.start()
	t.Cleanup(func() { h.stop(t) })
	return h
}

func (h *peerBoundaryHarness) stop(t *testing.T) {
	t.Helper()
	h.once.Do(func() {
		_ = h.server.Close()
		_ = h.session.close()
		_ = h.conn.Close()
		select {
		case err := <-h.done:
			if err != nil {
				t.Errorf("packet pump shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("packet pump leaked on connection shutdown")
		}
	})
}

func (h *peerBoundaryHarness) open(t *testing.T, size int) *PeerStream {
	t.Helper()
	s, err := h.client.OpenPeerStream(size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func (h *peerBoundaryHarness) event(t *testing.T, event *eventpb.PeerEvent) {
	t.Helper()
	if err := WritePeerStreamEvent(h.server, event); err != nil {
		t.Fatal(err)
	}
}

func (h *peerBoundaryHarness) packet(t *testing.T, payload []byte, sequence uint16, timestamp uint32) {
	t.Helper()
	var info *packet.OpusInfo
	if h.conn.timed {
		info = &packet.OpusInfo{SourceID: "source", Header: rtp.Header{Version: 2, SSRC: 42, SequenceNumber: sequence, Timestamp: timestamp}, Duration48: 960}
	}
	select {
	case h.conn.input <- peerBoundaryPacket{payload: payload, info: info}:
	case <-time.After(3 * time.Second):
		t.Fatal("packet pump did not accept media")
	}
}

func peerBoundaryNext(t *testing.T, s *PeerStream) *genx.MessageChunk {
	t.Helper()
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	ready := make(chan result, 1)
	go func() {
		chunk, err := s.Next()
		ready <- result{chunk: chunk, err: err}
	}()
	select {
	case got := <-ready:
		if got.err != nil {
			t.Fatal(got.err)
		}
		return got.chunk
	case <-time.After(3 * time.Second):
		t.Fatal("PeerStream output stalled")
		return nil
	}
}

func peerBoundaryDrainEOS(t *testing.T, s *PeerStream, id string) {
	t.Helper()
	for range 8 {
		chunk := peerBoundaryNext(t, s)
		if chunk.IsEndOfStream() && chunk.Ctrl.StreamID == id {
			return
		}
	}
	t.Fatalf("slow subscriber did not reach EOS(%s)", id)
}

func peerBoundaryReadEvent(t *testing.T, stream net.Conn) {
	t.Helper()
	if _, err := ReadPeerStreamEvent(stream); err != nil {
		t.Fatal(err)
	}
}

func peerBoundaryOpus(t *testing.T) [][]byte {
	t.Helper()
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native Opus")
	}
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	var packets [][]byte
	for frame := range 4 {
		samples := make([]int16, 320)
		for i := range samples {
			samples[i] = int16((i + frame*320) * 97 % 24000)
		}
		data, err := encoder.Encode(samples, 320)
		if err != nil {
			t.Fatal(err)
		}
		packets = append(packets, bytes.Clone(data))
	}
	return packets
}

func peerBoundaryDecoder(t *testing.T) *opus.Decoder {
	t.Helper()
	decoder, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decoder.Close() })
	return decoder
}

func peerBoundaryPCM(t *testing.T, chunk *genx.MessageChunk, decoder *opus.Decoder, id string, original []byte, concealed bool) {
	t.Helper()
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || chunk.IsBeginOfStream() || chunk.IsEndOfStream() || chunk.Ctrl.StreamID != id || chunk.Ctrl.Label != "assistant" {
		t.Fatalf("unexpected recovered audio route: %+v", chunk)
	}
	peerBoundarySamples(t, blob, decoder, original, concealed)
}

func peerBoundarySamples(t *testing.T, blob *genx.Blob, decoder *opus.Decoder, original []byte, concealed bool) {
	t.Helper()
	samples, err := decoder.Decode(original, 320, false)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(want[i*2:], uint16(sample))
	}
	if blob.MIMEType != "audio/pcm;rate=16000;channels=1" || blob.ConcealedAudio != concealed || !bytes.Equal(blob.Data, want) {
		t.Fatalf("receiver lost continuous decode/PLC state: concealed=%t want=%t, PCM bytes=%d want=%d", blob.ConcealedAudio, concealed, len(blob.Data), len(want))
	}
	if concealed && blob.OriginalAudio != nil || !concealed && (blob.OriginalAudio == nil || !bytes.Equal(blob.OriginalAudio.Data, original)) {
		t.Fatal("receiver changed original-only packet provenance")
	}
}

func peerBoundaryAudio(t *testing.T, audio <-chan *genx.Blob) *genx.Blob {
	t.Helper()
	select {
	case blob := <-audio:
		return blob
	case <-time.After(3 * time.Second):
		t.Fatal("receiver media dispatch stalled")
		return nil
	}
}
