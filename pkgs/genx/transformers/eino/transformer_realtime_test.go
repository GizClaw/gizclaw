package eino

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/cloudwego/eino/schema"
)

func TestAudioDockReplacementTranscriptInterruptionKeepsEinoAlive(t *testing.T) {
	t.Parallel()
	agent, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatal(err)
	}
	dock, err := audiodock.New(audiodock.Config{
		Agent: agent,
		ASR:   scriptedReplacementASR{},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := inputFromChunks(t,
		audioBegin("old-audio"),
		audioBegin("new-audio"),
	)
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var assistantText string
	for _, chunk := range drain(t, output) {
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			assistantText += string(text)
		}
	}
	if assistantText != "fresh" {
		t.Fatalf("assistant text = %q, want fresh", assistantText)
	}
	agent.history.mu.Lock()
	history := append([]*schema.Message(nil), agent.history.live...)
	agent.history.mu.Unlock()
	if len(history) != 2 || history[0].Role != schema.User || history[0].Content != "fresh" ||
		history[1].Role != schema.Assistant || history[1].Content != "fresh" {
		t.Fatalf("History = %#v, want only fresh user and assistant messages", history)
	}
}

type scriptedReplacementASR struct{}

func (scriptedReplacementASR) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	go func() {
		turn := 0
		for {
			chunk, err := input.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					_ = output.Done(genx.Usage{})
				} else {
					_ = output.Abort(err)
				}
				return
			}
			if chunk == nil || !chunk.IsBeginOfStream() {
				continue
			}
			turn++
			switch turn {
			case 1:
				if err := output.Add(textChunk("old-transcript", "stale partial", true, false)); err != nil {
					_ = output.Abort(err)
					return
				}
			case 2:
				if err := output.Add(
					interruptedTextEnd("old-transcript"),
					textChunk("new-transcript", "fresh", true, false),
					textChunk("new-transcript", "", false, true),
				); err != nil {
					_ = output.Abort(err)
					return
				}
			}
		}
	}()
	return output.Stream(), nil
}

func audioBegin(streamID string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
	}
}

var _ genx.Transformer = scriptedReplacementASR{}
