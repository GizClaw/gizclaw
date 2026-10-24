package audiodock

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestTextAgentWaitsForASRTerminalAcrossControlEOSOrder(t *testing.T) {
	for _, controlFirst := range []bool{true, false} {
		name := "audio-before-control"
		if controlFirst {
			name = "control-before-audio"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			partialSeen := make(chan struct{})
			asr := transformerFunc(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
				output := streamkit.NewOutput(streamkit.OutputConfig{})
				go func() {
					defer output.Close()
					for {
						chunk, err := input.Next()
						if err != nil {
							return
						}
						if chunk == nil {
							continue
						}
						if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
							_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: "transcript", BeginOfStream: true}})
						}
						if chunk.IsEndOfStream() {
							_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(" final"), Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: "transcript", EndOfStream: true}})
						}
					}
				}()
				return output, nil
			})
			agent := transformerFunc(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
				output := streamkit.NewOutput(streamkit.OutputConfig{})
				go func() {
					defer output.Close()
					var text strings.Builder
					announced := false
					for {
						chunk, err := input.Next()
						if err != nil {
							return
						}
						if chunk == nil {
							continue
						}
						if part, ok := chunk.Part.(genx.Text); ok {
							text.WriteString(string(part))
							if !announced && string(part) == "partial" {
								close(partialSeen)
								announced = true
							}
						}
						if chunk.IsEndOfStream() && text.Len() > 0 {
							_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text.String()), Ctrl: &genx.StreamCtrl{StreamID: genx.NewStreamID(), BeginOfStream: true, EndOfStream: true}})
						}
					}
				}()
				return output, nil
			})
			dock, err := New(Config{Agent: agent, ASR: asr})
			if err != nil {
				t.Fatal(err)
			}
			input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
			output, err := dock.Transform(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			push := func(part genx.Part, bos, eos bool) {
				t.Helper()
				if err := input.Push(ctx, &genx.MessageChunk{Role: genx.RoleUser, Part: part, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: bos, EndOfStream: eos}}); err != nil {
					t.Fatal(err)
				}
			}
			push(nil, true, false)
			push(&genx.Blob{MIMEType: "audio/pcm"}, true, false)
			push(&genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, false, false)
			select {
			case <-partialSeen:
			case <-ctx.Done():
				t.Fatal("Agent did not receive partial ASR text")
			}
			if controlFirst {
				push(nil, false, true)
			}
			push(&genx.Blob{MIMEType: "audio/pcm"}, false, true)
			if !controlFirst {
				push(nil, false, true)
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			var replies []string
			for {
				chunk, err := output.Next()
				if err != nil {
					if !streamDone(err) {
						t.Fatal(err)
					}
					break
				}
				if chunk != nil && chunk.Role == genx.RoleModel {
					if text, ok := chunk.Part.(genx.Text); ok && text != "" {
						replies = append(replies, string(text))
					}
				}
			}
			if len(replies) != 1 || replies[0] != "partial final" {
				t.Fatalf("replies=%q, want one complete ASR reply", replies)
			}
		})
	}
}
