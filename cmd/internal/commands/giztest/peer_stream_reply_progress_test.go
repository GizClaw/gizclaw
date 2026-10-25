package giztestcmd

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

type repliesWhileInputEndsStream struct {
	*fakeRelayStream
	packet        []byte
	readNew       chan struct{}
	lateFirstText bool
}

func (s *repliesWhileInputEndsStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if err := s.fakeRelayStream.Push(ctx, chunk); err != nil {
		return err
	}
	if chunk.Part != nil || !chunk.IsEndOfStream() {
		return nil
	}
	for _, id := range []string{"first", "second"} {
		text := assistantText(id, id, false)
		text.Ctrl.BeginOfStream = true
		audio := assistantBlob(id, s.packet, false)
		audio.Ctrl.BeginOfStream = true
		s.in <- text
		s.in <- audio
		if id == "first" {
			if !s.lateFirstText {
				s.in <- assistantText(id, "", true)
			}
			s.in <- assistantBlob(id, nil, true)
		}
	}
	if s.lateFirstText {
		s.in <- assistantText("first", "", true)
	}
	return nil
}

func (s *repliesWhileInputEndsStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.fakeRelayStream.Next()
	if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == "second" {
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			close(s.readNew)
		}
	}
	return chunk, err
}

func TestPeerStreamFullReplyWaitsForResponseStartedBeforeInputEnds(t *testing.T) {
	testPeerStreamFullReplyWaits(t, false)
}

func TestPeerStreamLatePreviousEOSDoesNotCompleteNewResponse(t *testing.T) {
	testPeerStreamFullReplyWaits(t, true)
}

func TestPeerStreamInterruptedPrefixDoesNotCountAsKeptReply(t *testing.T) {
	stream := newFakeRelayStream()
	_, packets := testOggOpus(t)
	for _, id := range []string{"interrupted", "reply"} {
		text := assistantText(id, id, false)
		text.Ctrl.BeginOfStream = true
		audio := assistantBlob(id, packets[0], false)
		audio.Ctrl.BeginOfStream = true
		textEOS, audioEOS := assistantText(id, "", true), assistantBlob(id, nil, true)
		if id == "interrupted" {
			textEOS.Ctrl.Error, audioEOS.Ctrl.Error = "interrupted", "interrupted"
		}
		for _, chunk := range []*genx.MessageChunk{text, audio, textEOS, audioEOS} {
			stream.in <- chunk
		}
	}
	close(stream.in)
	result, err := invokePeerStream(t.Context(), nil, func() (peerStream, error) { return stream, nil },
		giztest.Step{PeerStream: &giztest.PeerStreamOperation{Mode: "text"}}, "question", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	if object["reply"] != "reply" || object["audio_bytes"] != len(packets[0]) || object["response_count"] != 1 {
		t.Fatalf("interrupted prefix leaked into the kept reply: %#v", result.evidence)
	}
	if integrity := object["audio_integrity"].(map[string]any); integrity["streams"] != 2 || integrity["open"] != 0 || integrity["violations"] != 0 {
		t.Fatalf("raw lifecycle evidence was lost: %#v", integrity)
	}
}

func testPeerStreamFullReplyWaits(t *testing.T, lateFirstText bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	audio, packets := testOggOpus(t)
	readNew := make(chan struct{})
	stream := &repliesWhileInputEndsStream{fakeRelayStream: newFakeRelayStream(), packet: packets[0], readNew: readNew, lateFirstText: lateFirstText}
	defer stream.Close()
	type outcome struct {
		result operationResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := invokePeerStreamOnStream(ctx, nil, nil, stream, nil, "input", giztest.Step{
			ID: "reply", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
				Mode: "push-to-talk", HoldBeforeAudio: "0ms", Pacing: "0ms", ReplyObservation: "5ms",
			},
		}, audio, 0, nil)
		done <- outcome{result, err}
	}()
	select {
	case <-readNew:
	case got := <-done:
		t.Fatalf("stopped before observing the second response: %v %#v", got.err, got.result.evidence)
	case <-ctx.Done():
		t.Fatal("the second response did not start during input push")
	}
	select {
	case got := <-done:
		t.Fatalf("returned before the second response EOS: %v %#v", got.err, got.result.evidence)
	case <-time.After(25 * time.Millisecond):
	}
	stream.in <- assistantText("second", "", true)
	stream.in <- assistantBlob("second", nil, true)
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		integrity := got.result.assertion.(map[string]any)["audio_integrity"].(map[string]any)
		if integrity["open"] != 0 || integrity["violations"] != 0 {
			t.Fatalf("audio lifecycle = %#v", integrity)
		}
	case <-ctx.Done():
		t.Fatal("the second response did not finish")
	}
}
