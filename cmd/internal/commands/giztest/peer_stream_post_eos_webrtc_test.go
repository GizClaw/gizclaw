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
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, remote, events := postEOSWebRTCClient(t, ctx)
	first := voiceTonePacketsCount(t, 300, 1)[0]
	second := voiceTonePacketsCount(t, 600, 1)[0]
	late := voiceTonePacketsCount(t, 900, 2)
	t.Setenv("GIZCLAW_TEST_POST_EOS_FIRST_DIGEST", voicePacketDigest([][]byte{first}))
	t.Setenv("GIZCLAW_TEST_POST_EOS_SECOND_DIGEST", voicePacketDigest([][]byte{second}))
	observed := make(chan *genx.MessageChunk, 32)
	played := make(chan []byte, 8)
	d := &postEOSWebRTCDriver{driver: newDriver(false, func(_, _ string, packet []byte, _ bool) error {
		if len(packet) > 0 {
			played <- bytes.Clone(packet)
		}
		return nil
	}), client: client, observed: observed}
	fixtureDone := make(chan error, 1)
	go func() {
		defer close(fixtureDone)
		err := servePostEOSWebRTC(ctx, remote, events, observed, [][]byte{first, second}, late)
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
	t.Log("real WebRTC: EOS observed before 2 late RTP packets; both ignored; next BOS delivered its own audio; 2 Giztest steps passed")
}

type postEOSWebRTCDriver struct {
	*driver
	client   *gizcli.Client
	observed chan *genx.MessageChunk
}

func (d *postEOSWebRTCDriver) Open(ctx context.Context, _ *giztest.Document, _ *giztest.Variables) (giztest.Session, error) {
	stream, err := d.client.OpenPeerStream(32)
	if err != nil {
		return nil, err
	}
	// Retain the reader across the two text steps, as in the voice fixture.
	session := newPeerStreamSession("peer", &postEOSObservedStream{PeerStream: stream, ctx: ctx, observed: d.observed})
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

func postEOSWebRTCClient(t *testing.T, ctx context.Context) (*gizcli.Client, giznet.Conn, net.Conn) {
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
		return gizwebrtc.Dial(ctx, key, serverKey.Public, gizwebrtc.DialConfig{SignalingURL: signaling.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
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

func servePostEOSWebRTC(ctx context.Context, remote giznet.Conn, events net.Conn, observed <-chan *genx.MessageChunk, audio, late [][]byte) error {
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
		if i == 0 {
			// This receipt, rather than a sleep, establishes EOS-before-RTP.
			for _, tail := range late {
				if _, err := remote.Write(giznet.ProtocolOpusPacket, tail); err != nil {
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
