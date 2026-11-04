package giztestcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// The fixture deliberately sends RTP only after Next has delivered audio EOS.
// Signaling, SCTP events, RTP, SDK merging, Giztest assertions and playback
// observation are real; neither a provider nor a timing race is needed.
func TestPeerStreamPostEOSWebRTCGiztest(t *testing.T) {
	for _, delay := range []time.Duration{0, replyObservationWindow + 100*time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			runPostEOSWebRTCGiztest(t, delay, false)
		})
	}
}

func TestPeerStreamContinuousObservationRetainsFinalTail(t *testing.T) {
	runPostEOSWebRTCGiztest(t, replyObservationWindow+100*time.Millisecond, true)
}

func runPostEOSWebRTCGiztest(t *testing.T, tailDelay time.Duration, finalTail bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	first := voiceTonePacketsCount(t, 300, 1)[0]
	second := voiceTonePacketsCount(t, 600, 1)[0]
	late := voiceTonePacketsCount(t, 900, 2)
	client, remote, events := postEOSWebRTCClient(t, ctx, late[0], tailDelay)
	stream, err := client.OpenPeerStream(32)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	// The pre-fix SDK overlay has no diagnostic getter. Its negative control
	// still runs to completion and fails the same functional YAML assertions.
	diagnostics, _ := any(stream).(peerStreamAudioDiagnostics)
	t.Setenv("GIZCLAW_TEST_POST_EOS_FIRST_DIGEST", voicePacketDigest([][]byte{first}))
	t.Setenv("GIZCLAW_TEST_POST_EOS_SECOND_DIGEST", voicePacketDigest([][]byte{second}))
	observed := make(chan *genx.MessageChunk, 32)
	played := make(chan []byte, 8)
	d := &postEOSWebRTCDriver{driver: newDriver(false, func(_, _ string, packet []byte, _ bool) error {
		if len(packet) > 0 {
			played <- bytes.Clone(packet)
		}
		return nil
	}), stream: &postEOSObservedStream{PeerStream: stream, ctx: ctx, observed: observed}}
	fixtureDone := make(chan error, 1)
	go func() {
		defer close(fixtureDone)
		err := servePostEOSWebRTC(ctx, remote, events, observed, [][]byte{first, second}, late, diagnostics, finalTail)
		if err != nil {
			cancel()
		}
		fixtureDone <- err
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-fixtureDone:
		case <-time.After(5 * time.Second):
			t.Error("WebRTC fixture did not stop")
		}
	})
	doc, err := giztest.LoadDocument("../../../../tests/gizclaw-e2e/testdata/peer-audio/post-eos.giztest.yaml", d)
	if err != nil {
		t.Fatal(err)
	}
	if finalTail {
		continuous, err := giztest.LoadDocument("../../../../tests/gizclaw-e2e/testdata/peer-audio/continuous.giztest.yaml", d)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range continuous.Steps {
			if step.PeerStream != nil {
				for i := range doc.Steps {
					doc.Steps[i].PeerStream.ReplyObservation = step.PeerStream.ReplyObservation
				}
				break
			}
		}
		// The final response also has a delayed tail. Assert its receipt below
		// after the actual document controls when the retained session closes.
		delete(doc.Steps[1].Expect, "/audio_integrity/ignored_post_eos_packets")
	}
	report := giztest.Run(ctx, []*giztest.Document{doc}, giztest.Options{Driver: d, Parallel: 1, Out: io.Discard})
	if report.Status != "passed" || len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 2 {
		t.Fatalf("post-EOS Giztest failed: %+v", report.Tasks)
	}
	select {
	case err := <-fixtureDone:
		if err != nil {
			t.Fatalf("WebRTC fixture: %v; Giztest: %+v", err, report.Tasks)
		}
	case <-ctx.Done():
		t.Fatalf("WebRTC fixture did not finish: %v; Giztest: %+v", ctx.Err(), report.Tasks)
	}
	for _, step := range report.Tasks[0].Steps {
		if step.Status != "passed" {
			t.Fatalf("step %s: %+v", step.ID, step)
		}
	}
	for i, want := range [][]byte{first, second} {
		select {
		case got := <-played:
			if !bytes.Equal(got, want) {
				t.Fatalf("playback packet %d contains late or wrong-turn audio", i)
			}
		default:
			t.Fatalf("missing playback packet %d", i)
		}
	}
	if len(played) != 0 {
		t.Fatalf("playback received %d extra post-EOS packets", len(played))
	}
	if finalTail {
		if diagnostics == nil {
			t.Fatal("final observation requires post-EOS packet diagnostics")
		}
		if ignored := diagnostics.IgnoredPostEOSAudioPackets(); ignored != uint64(2*len(late)) {
			t.Fatalf("final observation closed before both late tails were consumed: ignored=%d, want %d", ignored, 2*len(late))
		}
		integrity := report.Tasks[0].Steps[1].Evidence["audio_integrity"].(map[string]any)
		if ignored, ok := giztest.NumericTarget(integrity["ignored_post_eos_packets"]); !ok || ignored != float64(len(late)) {
			t.Fatalf("final tail missing from completed-step evidence: %v", integrity)
		}
	}
	t.Log("real WebRTC: EOS observed before late RTP; tails ignored without playback; next BOS delivered its own audio")
}

type postEOSWebRTCDriver struct {
	*driver
	stream peerStream
}

func (d *postEOSWebRTCDriver) Open(context.Context, *giztest.Document, *giztest.Variables) (giztest.Session, error) {
	// Retain the reader across the two text steps, as in the voice fixture.
	session := newPeerStreamSession("peer", d.stream)
	session.startReader()
	return &postEOSWebRTCSession{voiceFixtureSession: &voiceFixtureSession{session: session}, observer: d.audioObserver}, nil
}

type postEOSWebRTCSession struct {
	*voiceFixtureSession
	observer audioObserver
}

func (s *postEOSWebRTCSession) Execute(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	input, err := req.Vars.Resolve(req.Step.PeerStream.Input)
	if err != nil {
		return giztest.StepResult{}, err
	}
	result, err := invokePeerStreamOnStream(ctx, nil, nil, s.session.stream, s.session, "", req.Step, input, 0, nil, s.observer)
	return result.stepResult(), err
}

type postEOSObservedStream struct {
	*gizcli.PeerStream
	ctx      context.Context
	observed chan *genx.MessageChunk
}

func (s *postEOSObservedStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.PeerStream.Next()
	if err == nil {
		select {
		case s.observed <- chunk:
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		}
	}
	return chunk, err
}

type postEOSWebRTCPolicy struct{}

func (postEOSWebRTCPolicy) AllowPeer(context.Context, giznet.PeerAdmission) bool { return true }
func (postEOSWebRTCPolicy) AllowService(giznet.PublicKey, uint64) bool           { return true }

func postEOSWebRTCClient(t *testing.T, ctx context.Context, late []byte, delay time.Duration) (*gizcli.Client, giznet.Conn, net.Conn) {
	t.Helper()
	return postEOSWebRTCClientWithWrapper(t, ctx, late, delay, nil)
}

func postEOSWebRTCClientWithWrapper(t *testing.T, ctx context.Context, late []byte, delay time.Duration, wrap func(giznet.Conn) giznet.Conn) (*gizcli.Client, giznet.Conn, net.Conn) {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: postEOSWebRTCPolicy{}}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	signaling := httptest.NewServer(listener.SignalingHandler())
	t.Cleanup(signaling.Close)
	client := &gizcli.Client{KeyPair: clientKey, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		local, conn, err := gizwebrtc.Dial(ctx, key, serverKey.Public, gizwebrtc.DialConfig{SignalingURL: signaling.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
		var transport giznet.Conn = conn
		if err == nil && delay > 0 {
			transport = &postEOSDelayedTailConn{Conn: transport, ctx: ctx, payload: late, delay: delay}
		}
		if err == nil && wrap != nil {
			transport = wrap(transport)
		}
		return local, transport, err
	}}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Dial(serverKey.Public, signaling.URL); err != nil {
		t.Fatal(err)
	}
	remote, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = remote.Close() })
	events, err := remote.ListenService(gizcli.EventStreamAgent).Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Close() })
	served := make(chan error, 1)
	go func() { served <- client.Serve() }()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Error("SDK Serve did not stop")
		}
	})
	return client, remote, events
}

type postEOSDelayedTailConn struct {
	giznet.Conn
	ctx     context.Context
	payload []byte
	delay   time.Duration
}

func (c *postEOSDelayedTailConn) Read(buf []byte) (byte, int, error) {
	protocol, n, err := c.Conn.Read(buf)
	if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(buf[:n], c.payload) {
		return protocol, n, err
	}
	// Hold the first tail after real RTP reception for longer than Giztest's
	// reply-observation window. Correctness must depend on receipt, not timing.
	timer := time.NewTimer(c.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return protocol, n, nil
	case <-c.ctx.Done():
		return protocol, 0, c.ctx.Err()
	}
}

func servePostEOSWebRTC(ctx context.Context, remote giznet.Conn, events net.Conn, observed <-chan *genx.MessageChunk, audio, late [][]byte, diagnostics peerStreamAudioDiagnostics, finalTail bool) error {
	stop := context.AfterFunc(ctx, func() { _ = events.Close() })
	defer stop()
	if err := events.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	await := func(id string, match func(*genx.MessageChunk) bool) error {
		for {
			select {
			case chunk := <-observed:
				if chunk.Ctrl != nil && chunk.Ctrl.StreamID == id && match(chunk) {
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	for i, packet := range audio {
		for {
			event, err := gizcli.ReadPeerStreamEvent(events)
			if err != nil {
				return fmt.Errorf("read input %d: %w", i, err)
			}
			if event.GetTextDone() != nil {
				break
			}
		}
		id := fmt.Sprintf("reply-%d", i+1)
		text := &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA,
			Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{StreamId: id, Label: "assistant", Text: id}},
		}
		if err := gizcli.WritePeerStreamEvent(events, text); err != nil {
			return err
		}
		if err := gizcli.WritePeerStreamEvent(events, postEOSWebRTCControl(id, true, true)); err != nil {
			return err
		}
		if err := await(id, (*genx.MessageChunk).IsBeginOfStream); err != nil {
			return err
		}
		if _, err := remote.Write(giznet.ProtocolOpusPacket, packet); err != nil {
			return err
		}
		if err := await(id, func(chunk *genx.MessageChunk) bool {
			blob, ok := chunk.Part.(*genx.Blob)
			return ok && bytes.Equal(blob.Data, packet)
		}); err != nil {
			return err
		}
		if err := gizcli.WritePeerStreamEvent(events, postEOSWebRTCControl(id, false, true)); err != nil {
			return err
		}
		if err := await(id, (*genx.MessageChunk).IsEndOfStream); err != nil {
			return err
		}
		if i == 0 || finalTail && i == len(audio)-1 {
			// This receipt, rather than a sleep, establishes EOS-before-RTP.
			for _, tail := range late {
				if _, err := remote.Write(giznet.ProtocolOpusPacket, tail); err != nil {
					return err
				}
			}
			// Do not await the final tail before text EOS: the document's
			// observation window must keep the SDK subscription alive itself.
			if i == 0 && diagnostics != nil {
				if err := awaitPostEOSIgnored(ctx, diagnostics, uint64(len(late))); err != nil {
					return err
				}
			}
		}
		if err := gizcli.WritePeerStreamEvent(events, postEOSWebRTCControl(id, false, false)); err != nil {
			return err
		}
	}
	return nil
}

func awaitPostEOSIgnored(ctx context.Context, diagnostics peerStreamAudioDiagnostics, want uint64) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		got := diagnostics.IgnoredPostEOSAudioPackets()
		if got >= want {
			return nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("post-EOS receipt: ignored %d of %d packets: %w", got, want, ctx.Err())
		}
	}
}

func postEOSWebRTCControl(id string, begin, audio bool) *eventpb.PeerEvent {
	kind, mime := eventpb.StreamKind_STREAM_KIND_TEXT, "text/plain"
	if audio {
		kind, mime = eventpb.StreamKind_STREAM_KIND_AUDIO, "audio/opus"
	}
	if begin {
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
			Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{StreamId: id, Kind: kind, Label: "assistant", MimeType: mime}},
		}
	}
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{StreamId: id, Kind: kind, Label: "assistant", MimeType: mime}},
	}
}
