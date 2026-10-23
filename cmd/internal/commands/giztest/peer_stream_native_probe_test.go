package giztestcmd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// The peer may write a response while processing the final input EOS. Waiting
// to read until Push returns deadlocks a duplex transport in that situation.
type nativeProbeDuplexStream struct {
	*fakeRelayStream
	read chan struct{}
	once sync.Once
}

func (s *nativeProbeDuplexStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if err := s.fakeRelayStream.Push(ctx, chunk); err != nil {
		return err
	}
	if chunk.IsEndOfStream() && chunk.Part == nil {
		s.in <- assistantText("reply", "answer", false)
		select {
		case <-s.read:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	return nil
}

func (s *nativeProbeDuplexStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.fakeRelayStream.Next()
	if chunk != nil && chunk.Part == genx.Text("answer") {
		s.once.Do(func() { close(s.read) })
	}
	return chunk, err
}

func TestNativeFirstResponseReadsWhileSendingEOS(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	stream := &nativeProbeDuplexStream{fakeRelayStream: newFakeRelayStream(), read: make(chan struct{})}
	defer stream.Close()
	audio, _ := testOggOpus(t)
	noAudio := false
	step := giztest.Step{ID: "probe", PeerStream: &giztest.PeerStreamOperation{
		Mode: "push-to-talk", HoldBeforeAudio: "0ms", Pacing: "0ms",
		Completion: "first_response", FirstTextTimeout: "100ms", RequireAudio: &noAudio,
	}}
	result, err := invokePeerStreamOnStream(ctx, nil, nil, stream, nil, "input", step, audio, 0, nil)
	if err != nil {
		t.Fatalf("native probe failed to drain the duplex response: %v", err)
	}
	if got := result.assertion.(map[string]any)["reply"]; got != "answer" {
		t.Fatalf("reply = %v", got)
	}
}
