package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// Cross-channel ordering uses actual RTP receipt and delivered SDK boundaries as
// barriers. The negative controls mutate only fixture media, not the protocol:
// successful EOS must not make lost/reordered/starved audio pass Giztest.
func TestPeerStreamChannelOrderWebRTCGiztest(t *testing.T) {
	for _, tc := range []struct{ name, fault, rejected string }{
		{name: "boundaries"},
		{name: "missing-frame", fault: "drop", rejected: "/audio_pacing/packets"},
		{name: "reordered-frames", fault: "reorder", rejected: "/audio_integrity/sha256"},
		{name: "cross-turn-tail", fault: "old-tail", rejected: "/audio_integrity/sha256"},
		{name: "playback-starvation", fault: "stall", rejected: "/audio_pacing/underruns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runChannelOrderWebRTCGiztest(t, tc.fault, tc.rejected)
		})
	}
}

func runChannelOrderWebRTCGiztest(t *testing.T, fault, rejected string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	audio := [][][]byte{voiceTonePacketsCount(t, 300, 32), voiceTonePacketsCount(t, 600, 32), voiceTonePacketsCount(t, 900, 32)}
	late := voiceTonePacketsCount(t, 1200, 2)
	raw := make(chan []byte, 256)
	client, remote, events := postEOSWebRTCClientWithWrapper(t, ctx, nil, 0, func(conn giznet.Conn) giznet.Conn {
		return &channelOrderReceiptConn{Conn: conn, ctx: ctx, raw: raw}
	})
	stream, err := client.OpenPeerStream(64)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	observed := make(chan *genx.MessageChunk, 64)
	played := make(chan []byte, 256)
	base := &postEOSWebRTCDriver{driver: newDriver(false, func(_, _ string, packet []byte, _ bool) error {
		if len(packet) > 0 {
			played <- bytes.Clone(packet)
		}
		return nil
	}), stream: &postEOSObservedStream{PeerStream: stream, ctx: ctx, observed: observed}}
	d := &channelOrderDriver{postEOSWebRTCDriver: base}
	fixtureDone := make(chan error, 1)
	go func() {
		defer close(fixtureDone)
		err := serveChannelOrderWebRTC(ctx, remote, events, observed, raw, audio, late, stream, fault)
		fixtureDone <- err
		if err != nil {
			cancel()
		}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-fixtureDone:
		case <-time.After(3 * time.Second):
			t.Error("channel-order fixture did not stop")
		}
	})
	for i, name := range []string{"FIRST", "SECOND", "THIRD"} {
		t.Setenv("GIZCLAW_TEST_BOUNDARY_"+name+"_DIGEST", voicePacketDigest(audio[i]))
	}
	doc, err := giztest.LoadDocument("../../../../tests/gizclaw-e2e/testdata/peer-audio/channel-order.giztest.yaml", d)
	if err != nil {
		t.Fatal(err)
	}
	if fault == "drop" {
		// Isolate frame count: a checksum failure must not mask this assertion.
		delete(doc.Steps[2].Expect, "/audio_integrity/sha256")
	}
	if fault == "old-tail" {
		delete(doc.Steps[2].Expect, "/audio_pacing/packets")
	}
	report := giztest.Run(ctx, []*giztest.Document{doc}, giztest.Options{Driver: d, Parallel: 1, Out: io.Discard})
	if len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 3 || len(d.results) != 3 {
		cancel()
		t.Fatalf("incomplete Giztest: %+v", report.Tasks)
	}
	select {
	case err := <-fixtureDone:
		if err != nil {
			t.Fatalf("fixture: %v; Giztest=%+v", err, report.Tasks)
		}
	case <-ctx.Done():
		t.Fatalf("fixture did not finish: %v", ctx.Err())
	}
	if len(report.Tasks) != 1 || len(report.Tasks[0].Steps) != 3 || len(d.results) != 3 {
		t.Fatalf("incomplete Giztest: %+v", report.Tasks)
	}
	task := report.Tasks[0]
	for i, result := range d.results {
		value := result.Value.(map[string]any)
		if value["audio_eos"] != true || value["text_eos"] != true || value["audio_bytes"].(int) == 0 {
			t.Fatalf("turn %d did not complete real nonempty audio: %+v", i+1, value)
		}
		if i < 2 && task.Steps[i].Status != "passed" {
			t.Fatalf("boundary recovery failed: %+v", task.Steps[i])
		}
	}
	if rejected == "" {
		if report.Status != "passed" {
			t.Fatalf("boundary Giztest: %+v", task)
		}
	} else if report.Status != "failed" || !strings.Contains(task.Error, rejected) {
		t.Fatalf("%s escaped its audio assertion %s: %+v", fault, rejected, task)
	}
	// Compare actual playback with the deliberate fixture mutation. Discarded
	// post-EOS tails must stay absent; the injected wrong-turn negative remains
	// visible so the digest assertion cannot silently hide contamination.
	want := append(append([][]byte{}, audio[0]...), audio[1]...)
	last := append([][]byte{}, audio[2]...)
	if fault == "drop" {
		last = append(last[:1], last[2:]...)
	}
	if fault == "reorder" {
		last[1], last[2] = last[2], last[1]
	}
	if fault == "old-tail" {
		last = append([][]byte{audio[1][len(audio[1])-1]}, last...)
	}
	want = append(want, last...)
	for i, packet := range want {
		select {
		case got := <-played:
			if !bytes.Equal(got, packet) {
				t.Fatalf("playback packet %d has wrong-turn or unexpected audio", i)
			}
		default:
			t.Fatalf("playback missing packet %d", i)
		}
	}
	if len(played) != 0 {
		t.Fatal("late audio escaped into playback")
	}
	if out := os.Getenv("GIZCLAW_PEER_AUDIO_BOUNDARY_EVIDENCE"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		name := strings.ReplaceAll(t.Name(), "/", "-")
		if err := giztest.WriteReport(filepath.Join(out, name+".json"), report); err != nil {
			t.Fatal(err)
		}
		value, err := json.MarshalIndent(map[string]any{"fault": fault, "expected_rejection": rejected, "turns": len(d.results), "one_peer_stream": true, "initial_rtp_before_bos": true, "eos_then_late_packets": 2, "old_tail_before_next_bos": 1, "assertions": d.results}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name+"-observations.json"), append(value, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("same WebRTC PeerStream: initial RTP before BOS; EOS before two tail packets; old tail before next BOS; fault=%q rejected=%q", fault, rejected)
}

type channelOrderReceiptConn struct {
	giznet.Conn
	ctx context.Context
	raw chan<- []byte
}

func (c *channelOrderReceiptConn) Read(buf []byte) (byte, int, error) {
	protocol, n, err := c.Conn.Read(buf)
	if err == nil && protocol == giznet.ProtocolOpusPacket {
		select {
		case c.raw <- bytes.Clone(buf[:n]):
		case <-c.ctx.Done():
			return protocol, 0, c.ctx.Err()
		}
	}
	return protocol, n, err
}

type channelOrderDriver struct {
	*postEOSWebRTCDriver
	results []giztest.StepResult
}

func (d *channelOrderDriver) Open(ctx context.Context, doc *giztest.Document, vars *giztest.Variables) (giztest.Session, error) {
	session, err := d.postEOSWebRTCDriver.Open(ctx, doc, vars)
	if err != nil {
		return nil, err
	}
	return &channelOrderSession{Session: session, results: &d.results}, nil
}

type channelOrderSession struct {
	giztest.Session
	results *[]giztest.StepResult
}

func (s *channelOrderSession) Execute(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	result, err := s.Session.Execute(ctx, req)
	*s.results = append(*s.results, result)
	return result, err
}

func serveChannelOrderWebRTC(ctx context.Context, remote giznet.Conn, events net.Conn, observed <-chan *genx.MessageChunk, raw <-chan []byte, audio [][][]byte, late [][]byte, diagnostics peerStreamAudioDiagnostics, fault string) error {
	stop := context.AfterFunc(ctx, func() { _ = events.Close() })
	defer stop()
	if err := events.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
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
	awaitRaw := func(packet []byte) error {
		for {
			select {
			case got := <-raw:
				if bytes.Equal(got, packet) {
					return nil
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	writePacket := func(packet []byte) error { _, err := remote.Write(giznet.ProtocolOpusPacket, packet); return err }
	awaitPacket := func(id string, packet []byte) error {
		return await(id, func(chunk *genx.MessageChunk) bool {
			blob, ok := chunk.Part.(*genx.Blob)
			return ok && bytes.Equal(blob.Data, packet)
		})
	}
	for turn, packets := range audio {
		for {
			event, err := gizcli.ReadPeerStreamEvent(events)
			if err != nil {
				return err
			}
			if event.GetTextDone() != nil {
				break
			}
		}
		id := fmt.Sprintf("reply-%d", turn+1)
		if turn == 1 {
			if err := writePacket(late[0]); err != nil {
				return err
			}
			if err := awaitPostEOSIgnored(ctx, diagnostics, 3); err != nil {
				return err
			}
		}
		text := &eventpb.PeerEvent{Version: eventpb.Version, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA, Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{StreamId: id, Label: "assistant", Text: id}}}
		if err := gizcli.WritePeerStreamEvent(events, text); err != nil {
			return err
		}
		if turn == 0 {
			if err := writePacket(packets[0]); err != nil {
				return err
			}
			if err := awaitRaw(packets[0]); err != nil {
				return err
			}
		}
		if err := gizcli.WritePeerStreamEvent(events, postEOSWebRTCControl(id, true, true)); err != nil {
			return err
		}
		if err := await(id, (*genx.MessageChunk).IsBeginOfStream); err != nil {
			return err
		}
		order := make([]int, len(packets))
		for i := range order {
			order[i] = i
		}
		if turn == 0 {
			order = order[1:]
			if err := awaitPacket(id, packets[0]); err != nil {
				return err
			}
		}
		if turn == 2 && fault == "old-tail" {
			packet := audio[1][len(audio[1])-1]
			if err := writePacket(packet); err != nil {
				return err
			}
			if err := awaitPacket(id, packet); err != nil {
				return err
			}
		}
		if turn == 2 && fault == "drop" {
			order = append(order[:1], order[2:]...)
		}
		if turn == 2 && fault == "reorder" {
			order[1], order[2] = order[2], order[1]
		}
		for i, index := range order {
			if turn == 2 && fault == "stall" && i == 27 {
				// Deliberate media starvation, not a sleep establishing ordering.
				timer := time.NewTimer(750 * time.Millisecond)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				}
			}
			if err := writePacket(packets[index]); err != nil {
				return err
			}
			if err := awaitPacket(id, packets[index]); err != nil {
				return err
			}
		}
		if err := gizcli.WritePeerStreamEvent(events, postEOSWebRTCControl(id, false, true)); err != nil {
			return err
		}
		if err := await(id, (*genx.MessageChunk).IsEndOfStream); err != nil {
			return err
		}
		if turn == 0 {
			for _, packet := range late {
				if err := writePacket(packet); err != nil {
					return err
				}
			}
			if err := awaitPostEOSIgnored(ctx, diagnostics, 2); err != nil {
				return err
			}
		}
		if err := gizcli.WritePeerStreamEvent(events, postEOSWebRTCControl(id, false, false)); err != nil {
			return err
		}
	}
	return nil
}
