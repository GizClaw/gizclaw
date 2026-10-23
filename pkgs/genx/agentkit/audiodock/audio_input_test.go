package audiodock

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestAudioAgentRepliesBeforeIndependentASR(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	finish := make(chan []byte, 1)
	packet := []byte{1, 2, 3}
	asr := transformerFunc(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer output.Close()
			defer input.Close()
			for {
				_, err := input.Next()
				if err != nil {
					break
				}
			}
			select {
			case <-release:
			case <-ctx.Done():
				return
			}
			_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("heard"), Ctrl: &genx.StreamCtrl{StreamID: "speech", Label: "transcript", BeginOfStream: true, EndOfStream: true}})
		}()
		return output, nil
	})
	agent := transformerFunc(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer output.Close()
			var audio []byte
			for {
				chunk, err := input.Next()
				if err != nil {
					return
				}
				if text, ok := chunk.Part.(genx.Text); ok && text != "" {
					t.Errorf("ASR text leaked into the audio Agent: %q", text)
				}
				if blob, ok := chunk.Part.(*genx.Blob); ok {
					audio = append(audio, blob.Data...)
				}
				if chunk.IsEndOfStream() {
					finish <- audio
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("reply"), Ctrl: &genx.StreamCtrl{StreamID: "answer", Label: "assistant", BeginOfStream: true, EndOfStream: true}})
					return
				}
			}
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent, ASR: asr, AgentAudioInput: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(ctx, &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "speech", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "speech", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var transcript strings.Builder
	begin, end := 0, 0
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("reply waited for ASR: %v", err)
		}
		if chunk.Role == genx.RoleUser && chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" {
			if chunk.IsBeginOfStream() {
				begin++
			}
		}
		if chunk.Role == genx.RoleModel && chunk.Part == genx.Text("reply") {
			if begin != 1 {
				t.Fatal("user route was not declared before the reply")
			}
			break
		}
	}
	if audio := <-finish; !slices.Equal(audio, packet) {
		t.Fatalf("Agent audio=%v, want %v", audio, packet)
	}
	close(release)
	for {
		chunk, err := output.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			break
		}
		if chunk.Role == genx.RoleUser && chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" {
			if chunk.IsBeginOfStream() {
				begin++
			}
			if chunk.IsEndOfStream() {
				end++
			}
			if text, ok := chunk.Part.(genx.Text); ok {
				transcript.WriteString(string(text))
			}
		}
	}
	if transcript.String() != "heard" || begin != 1 || end != 1 {
		t.Fatalf("transcript=%q BOS=%d EOS=%d", transcript.String(), begin, end)
	}
}

func TestAudioSidebandCancellationStopsBothComponents(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started := make(chan struct{}, 2)
	exited := make(chan struct{}, 2)
	component := transformerFunc(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer func() { exited <- struct{}{} }()
			defer output.Close()
			defer input.Close()
			started <- struct{}{}
			<-ctx.Done()
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: component, ASR: component, AgentAudioInput: true})
	if err != nil {
		t.Fatal(err)
	}
	input := streamkit.NewOutput(streamkit.OutputConfig{})
	output, err := dock.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("components did not start")
		}
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-exited:
		case <-ctx.Done():
			t.Fatal("component survived output close")
		}
	}
}

func TestAudioAgentSynchronousStartupWithUnnamedTurns(t *testing.T) {
	for _, control := range []bool{false, true} {
		t.Run(map[bool]string{false: "audio boundaries", true: "control and audio boundaries"}[control], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			var input []*genx.MessageChunk
			for range 2 {
				if control {
					input = append(input, &genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{BeginOfStream: true}})
				}
				input = append(input,
					&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}},
					&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}},
				)
				if control {
					input = append(input, &genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{EndOfStream: true}})
				}
			}
			asr := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
				out := streamkit.NewOutput(streamkit.OutputConfig{})
				go func() {
					defer out.Close()
					for {
						chunk, err := source.Next()
						if err != nil {
							return
						}
						if chunk.IsEndOfStream() {
							id := dockStreamID(chunk)
							if id == "" {
								id = "audio"
							}
							_ = out.Push(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("heard"), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "transcript", BeginOfStream: true, EndOfStream: true}})
						}
					}
				}()
				return out, nil
			})
			agentIDs := make(chan string, 8)
			agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
				// A Transformer may consume input before returning its output.
				first, err := source.Next()
				if err != nil {
					return nil, err
				}
				agentIDs <- dockStreamID(first)
				out := streamkit.NewOutput(streamkit.OutputConfig{})
				go func() {
					defer out.Close()
					defer close(agentIDs)
					for {
						chunk, err := source.Next()
						if err != nil {
							return
						}
						agentIDs <- dockStreamID(chunk)
					}
				}()
				return out, nil
			})
			dock, err := New(Config{Agent: agent, ASR: asr, AgentAudioInput: true})
			if err != nil {
				t.Fatal(err)
			}
			output, err := dock.Transform(ctx, &sliceStream{chunks: input})
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			type route struct {
				begins, ends int
				text         string
			}
			routes := make(map[string]*route)
			for _, chunk := range readAll(t, output) {
				id := dockStreamID(chunk)
				if id == "" {
					t.Fatal("transcript has no stream ID")
				}
				if routes[id] == nil {
					routes[id] = &route{}
				}
				r := routes[id]
				if chunk.IsBeginOfStream() {
					r.begins++
				}
				if chunk.IsEndOfStream() {
					r.ends++
				}
				if text, ok := chunk.Part.(genx.Text); ok {
					r.text += string(text)
				}
			}
			if len(routes) != 2 {
				t.Fatalf("transcript routes = %+v, want two distinct turns", routes)
			}
			for id, r := range routes {
				if r.begins != 1 || r.ends != 1 || r.text != "heard" {
					t.Fatalf("route %q = %+v", id, r)
				}
			}
			for id := range agentIDs {
				if routes[id] == nil {
					t.Fatalf("Agent ID %q differs from ASR transcript IDs", id)
				}
			}
			for _, chunk := range input {
				if dockStreamID(chunk) != "" {
					t.Fatal("router mutated its caller's input")
				}
			}
		})
	}
}
