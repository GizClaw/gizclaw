package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/api"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// This reproduces the listen-start metric's inclusion of the device's PTT
// startup hold, even when the actual packet delivery takes only 40 ms.
func TestListenFirstAudioIncludesPTTStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	sender, receiver := newFakeRelayStream(), newFakeRelayStream()
	defer sender.Close()
	defer receiver.Close()
	received := make(chan operationResult, 1)
	go func() {
		result, err := listenPeerStream(ctx, receiver, listenStep("800ms"), 0)
		if err != nil {
			t.Error(err)
		}
		received <- result
	}()
	delivered := make(chan time.Duration, 1)
	go func() {
		for {
			select {
			case chunk := <-sender.pushes:
				blob, ok := chunk.Part.(*genx.Blob)
				if !ok || len(blob.Data) == 0 {
					continue
				}
				started := time.Now()
				if waitPeerInput(ctx, 40*time.Millisecond) != nil {
					return
				}
				receiver.in <- assistantBlob("remote", blob.Data, false)
				delivered <- time.Since(started)
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	step := giztest.Step{ID: "send", Client: "alice", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Completion: "input_sent"}}
	if _, err := invokePeerStream(ctx, nil, func() (peerStream, error) { return sender, nil }, step, testAudibleOpus(t), 0); err != nil {
		t.Fatal(err)
	}
	result := <-received
	first := result.evidence["first_audio_ms"].(int64)
	delivery := <-delivered
	if first < 500 || delivery >= 200*time.Millisecond {
		t.Fatalf("listen first_audio_ms=%d, packet delivery=%s", first, delivery)
	}
	t.Logf("listen first_audio_ms=%d includes default hold=500ms; packet delivery=%s", first, delivery)
}

func TestFirstPacketMarkerPreservesDecodedAudio(t *testing.T) {
	packet := testAudibleOpus(t)
	if packet[0]&3 != 0 || len(packet)-1 > 251 {
		t.Fatal("test encoder must supply one small Opus frame")
	}
	frame := packet[1:]
	code1 := append([]byte{packet[0]&0xfc | 1}, bytes.Repeat(frame, 2)...)
	code2 := append([]byte{packet[0]&0xfc | 2, byte(len(frame))}, bytes.Repeat(frame, 2)...)
	code3 := append([]byte{packet[0]&0xfc | 3, 2}, bytes.Repeat(frame, 2)...)
	padded := append([]byte{packet[0]&0xfc | 3, 0x41, 255, 46}, frame...)
	padded = append(padded, bytes.Repeat([]byte{0}, 300)...)
	for name, input := range map[string][]byte{"code0": packet, "code1": code1, "code2": code2, "code3": code3, "existing_padding": padded} {
		t.Run(name, func(t *testing.T) {
			marked, ok, err := markFirstOpusPacket(input, "unique")
			if err != nil || !ok {
				t.Fatalf("mark = %v/%v", ok, err)
			}
			decode := func(data []byte) []int16 {
				decoder, err := opus.NewDecoder(16000, 1)
				if err != nil {
					t.Fatal(err)
				}
				defer decoder.Close()
				pcm, err := decoder.Decode(data, 1920, false)
				if err != nil {
					t.Fatal(err)
				}
				return pcm
			}
			if !slices.Equal(decode(input), decode(marked)) || codecconv.OpusPacketRTPTicks(input) != codecconv.OpusPacketRTPTicks(marked) {
				t.Fatal("marker changed PCM or audio duration")
			}
			if !bytes.HasSuffix(marked, []byte("\x00giztest/unique")) {
				t.Fatal("marker missing")
			}
		})
	}
	for _, input := range [][]byte{{}, {0xf8}, {0xf8, 0xff, 0xfe}, {0xf8, 0xff, 0xfe, 0, 0}} {
		marked, ok, err := markFirstOpusPacket(input, "unique")
		if err != nil || ok || !bytes.Equal(input, marked) {
			t.Fatalf("silence changed: %x -> %x", input, marked)
		}
	}
	for _, input := range [][]byte{{0xff, 0}, {0xff, 0x41}, {0xff, 0x41, 255}, {0xff, 0x41, 5, 1}} {
		if _, _, err := markFirstOpusPacket(input, "unique"); err == nil {
			t.Fatalf("accepted malformed padding: %x", input)
		}
	}
}

type packetProbeStream struct {
	*fakeRelayStream
	ready   chan struct{}
	once    sync.Once
	forward func(*genx.MessageChunk) error
}

func (s *packetProbeStream) Next() (*genx.MessageChunk, error) {
	if s.ready != nil {
		s.once.Do(func() { close(s.ready) })
	}
	return s.fakeRelayStream.Next()
}

func (s *packetProbeStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if s.forward != nil {
		if err := s.forward(chunk); err != nil {
			return err
		}
	}
	return s.fakeRelayStream.Push(ctx, chunk)
}

func TestFirstPacketTimingExcludesHoldAndCorrelatesGroup(t *testing.T) {
	clock := newPeerPacketClock()
	input := testAudibleOpus(t)
	previousID := ""
	for _, hold := range []string{"500ms", "1s"} {
		t.Run(hold, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			names := []string{"bob", "carol", "alice"}
			streams := make([]*packetProbeStream, len(names))
			results := make([]chan operationResult, len(names))
			for i, name := range names {
				stream := &packetProbeStream{fakeRelayStream: newFakeRelayStream(), ready: make(chan struct{})}
				streams[i] = stream
				results[i] = make(chan operationResult, 1)
				go func() {
					probe := peerStreamInvocation{open: func() (peerStream, error) { return stream, nil }, step: listenChild("listen", name, "1500ms"), packetClock: clock}
					result, err := probe.run(ctx, nil)
					if err != nil {
						t.Error(err)
					}
					results[i] <- result
				}()
				<-stream.ready
			}
			transmit := &packetProbeStream{fakeRelayStream: newFakeRelayStream()}
			transmit.forward = func(chunk *genx.MessageChunk) error {
				blob, ok := chunk.Part.(*genx.Blob)
				if !ok || len(blob.Data) == 0 {
					return nil
				}
				if err := waitPeerInput(ctx, 40*time.Millisecond); err != nil {
					return err
				}
				for _, stream := range streams {
					stream.in <- &genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "sfu/alice/1", Label: "alice"}}
				}
				// Receipt precedes Push completion, exercising duplex publication.
				return waitPeerInput(ctx, 100*time.Millisecond)
			}
			step := giztest.Step{ID: "send-" + hold, Client: "alice", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Completion: "input_sent", HoldBeforeAudio: hold, MeasureFirstPacket: true}}
			probe := peerStreamInvocation{open: func() (peerStream, error) { return transmit, nil }, step: step, input: input, packetClock: clock}
			sent, err := probe.run(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			send := sent.evidence["audio_first_packet"].(map[string]any)
			validatePacketTimingEvidence(t, sent.evidence)
			id := send["packet_id"].(string)
			if id == previousID {
				t.Fatal("reused packet id for identical audio in another round")
			}
			previousID = id
			minimumHold, _ := time.ParseDuration(hold)
			if send["startup_ms"].(float64) < packetMilliseconds(minimumHold) {
				t.Fatalf("startup=%v", send)
			}
			for i, name := range names {
				result := <-results[i]
				validatePacketTimingEvidence(t, result.evidence)
				packets := result.evidence["audio_first_packets"].([]any)
				if name == "alice" {
					if len(packets) != 0 {
						t.Fatal("self-listen created a latency sample")
					}
					continue
				}
				if len(packets) != 1 {
					t.Fatalf("%s samples=%v", name, packets)
				}
				sample := packets[0].(map[string]any)
				if sample["packet_id"] != id || sample["sender_step"] != step.ID {
					t.Fatalf("%s samples=%v", name, packets)
				}
				latency := sample["latency_ms"].(float64)
				if latency < 35 || latency > 200 {
					t.Fatalf("%s latency=%v includes hold or lost delay", name, latency)
				}
				if result.evidence["first_audio_ms"].(int64) < minimumHold.Milliseconds() {
					t.Fatal("legacy listen clock changed")
				}
				if _, ok := result.assertion.(map[string]any)["audio_first_packets"]; !ok {
					t.Fatal("timing absent from expectation/capture value")
				}
				if value, ok := giztest.JSONPointer(result.assertion, "/audio_first_packets/0/packet_id"); !ok || value != id {
					t.Fatal("timing array cannot be traversed by Giztest expectations/capture")
				}
				t.Logf("%s hold=%s startup_ms=%.3f first_audio_ms=%v latency_ms=%.3f", name, hold, send["startup_ms"], result.evidence["first_audio_ms"], latency)
			}
		})
	}
}

func validatePacketTimingEvidence(t *testing.T, evidence map[string]any) {
	t.Helper()
	data, err := api.Files.ReadFile("giztest/audio-packet-timing.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("timing.json", resource); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("timing.json")
	if err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(serialized))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
}

func TestFirstPacketCorrelationRejectsUnrelatedAndLostPackets(t *testing.T) {
	clock := newPeerPacketClock()
	step := giztest.Step{ID: "send", Client: "alice", PeerStream: &giztest.PeerStreamOperation{MeasureFirstPacket: true}}
	base := newFakeRelayStream()
	sender := &packetTimedPeerStream{peerStream: base, clock: clock, step: step, identity: "alice-key", started: time.Now()}
	input := testAudibleOpus(t)
	if err := sender.Push(t.Context(), &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: input}}); err != nil {
		t.Fatal(err)
	}
	marked := nextPush(t, base).Part.(*genx.Blob).Data
	receiver := &packetTimedPeerStream{clock: clock, step: listenChild("listen", "bob", "1s")}
	chunk := func(data []byte, identity string) *genx.MessageChunk {
		return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: "sfu/" + identity + "/1", Label: identity}}
	}
	receiver.observePacketArrival(chunk(input, "alice-key"), time.Now()) // Later unchanged packet is not the marked first one.
	receiver.observePacketArrival(chunk(marked, "unrelated-key"), time.Now())
	receiver.observePacketArrival(chunk(marked, "alice-key"), sender.sent.sent.Add(-time.Nanosecond))
	otherTask := &packetTimedPeerStream{clock: newPeerPacketClock(), step: receiver.step}
	otherTask.observePacketArrival(chunk(marked, "alice-key"), time.Now())
	if len(receiver.receipts) != 0 || len(otherTask.receipts) != 0 {
		t.Fatal("associated lost, unrelated, earlier or cross-task receipt")
	}
	receiver.observePacketArrival(chunk(marked, "alice-key"), time.Now())
	receiver.observePacketArrival(chunk(marked, "alice-key"), time.Now())
	if len(receiver.receipts) != 1 {
		t.Fatal("duplicate packet counted twice")
	}
	window := &packetTimedPeerStream{clock: clock, step: receiver.step, listenStart: time.Now(), listenEnd: time.Now().Add(time.Second)}
	window.observePacketArrival(chunk(marked, "alice-key"), window.listenStart.Add(-time.Nanosecond))
	window.observePacketArrival(chunk(marked, "alice-key"), window.listenEnd)
	if len(window.receipts) != 0 {
		t.Fatal("receipt outside the listen window counted")
	}
	base.Close()
}

func TestFirstPacketTimingDoesNotInventMissingOrFailedDelivery(t *testing.T) {
	clock := newPeerPacketClock()
	step := giztest.Step{ID: "send", Client: "alice", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Completion: "input_sent", MeasureFirstPacket: true}}
	base := &packetProbeStream{fakeRelayStream: newFakeRelayStream(), forward: func(*genx.MessageChunk) error { return errors.New("write rejected") }}
	sender := &packetTimedPeerStream{peerStream: base, clock: clock, step: step, started: time.Now()}
	if err := sender.Push(t.Context(), &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: testAudibleOpus(t)}}); err == nil {
		t.Fatal("expected failed send")
	}
	var sendResult operationResult
	sender.addEvidence(&sendResult)
	if sendResult.evidence["audio_first_packet"] != nil {
		t.Fatal("failed send reported a successful packet")
	}
	receiver := &packetTimedPeerStream{clock: clock, step: listenChild("listen", "bob", "1s")}
	var received operationResult
	receiver.addEvidence(&received)
	if len(received.evidence["audio_first_packets"].([]any)) != 0 {
		t.Fatal("no-audio produced a latency sample")
	}
}
