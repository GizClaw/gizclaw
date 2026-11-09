package audiodock

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

type transcriptPublicationTransformer func(context.Context, genx.Stream) (genx.Stream, error)

func (f transcriptPublicationTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return f(ctx, input)
}

// Gate one visible transcript pull while the real router and Agent continue.
// Holding text or EOS also catches a barrier that acknowledges only bare BOS.
type transcriptPublicationGate struct {
	genx.Stream
	ctx    context.Context
	gate   <-chan struct{}
	before int
	reads  int
}

func (s *transcriptPublicationGate) Next() (*genx.MessageChunk, error) {
	if s.reads == s.before {
		select {
		case <-s.gate:
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		}
	}
	s.reads++
	return s.Stream.Next()
}

func (s *transcriptPublicationGate) ObserveOutput(chunk *genx.MessageChunk) {
	if observer, ok := s.Stream.(interface{ ObserveOutput(*genx.MessageChunk) }); ok {
		observer.ObserveOutput(chunk)
	}
}

type transcriptPublicationRead struct {
	chunk *genx.MessageChunk
	err   error
}

func startTranscriptPublication(t *testing.T, gateBefore int) (context.CancelFunc, chan struct{}, *dockStream, <-chan transcriptPublicationRead) {
	t.Helper()
	parent, cancel := context.WithCancel(context.Background())
	invocation := streamkit.NewInvocation(parent, streamkit.OutputConfig{})
	ctx := invocation.Context()
	input := streamkit.NewOutput(streamkit.OutputConfig{})
	asr := transcriptPublicationTransformer(func(_ context.Context, audio genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer output.Close()
			for {
				chunk, err := audio.Next()
				if err != nil {
					return
				}
				if chunk == nil || !chunk.IsBeginOfStream() {
					continue
				}
				for _, next := range []*genx.MessageChunk{
					{Role: genx.RoleUser, Name: "transcript", Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", BeginOfStream: true}},
					{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript"}},
					{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", EndOfStream: true}},
				} {
					if err := output.Push(next); err != nil {
						return
					}
				}
			}
		}()
		return output, nil
	})
	agent := transcriptPublicationTransformer(func(_ context.Context, text genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer output.Close()
			var question string
			for {
				chunk, err := text.Next()
				if err != nil {
					return
				}
				if chunk == nil {
					continue
				}
				if chunk.IsBeginOfStream() {
					question = ""
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					question += string(part)
				}
				if !chunk.IsEndOfStream() || question == "" {
					continue
				}
				for _, next := range []*genx.MessageChunk{
					{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", BeginOfStream: true}},
					{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("reply"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant"}},
					{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", EndOfStream: true}},
				} {
					if err := output.Push(next); err != nil {
						return
					}
				}
				question = ""
			}
		}()
		return output, nil
	})
	router, err := newInputRouter(ctx, input, asr, false)
	if err != nil {
		t.Fatal(err)
	}
	run := &dockRun{
		dock:             &Dock{config: Config{Agent: agent, ASR: asr}},
		invocation:       invocation,
		router:           router,
		routes:           make(map[string]*dockRoute),
		discardSourceIDs: make(map[string]bool),
	}
	gate := make(chan struct{})
	transcriptDone := make(chan error, 1)
	go func() {
		transcriptDone <- run.forwardTranscripts(&transcriptPublicationGate{Stream: router.TranscriptOutput(), ctx: ctx, gate: gate, before: gateBefore})
	}()
	agentOutput, err := agent.Transform(ctx, router.AgentInput())
	if err != nil {
		t.Fatal(err)
	}
	run.source, err = streamkit.NewResponseStream(agentOutput)
	if err != nil {
		t.Fatal(err)
	}
	run.source.DeferOutputObservation()
	if pending := router.ActivateEvents(); len(pending) != 0 {
		t.Fatalf("unexpected startup events: %v", pending)
	}
	go run.execute(transcriptDone)
	output := &dockStream{Output: invocation.Output(), invocation: invocation}
	reads := make(chan transcriptPublicationRead, 16)
	go func() {
		for {
			chunk, err := output.Next()
			reads <- transcriptPublicationRead{chunk: chunk, err: err}
			if err != nil {
				return
			}
		}
	}()
	if err := input.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1"},
		Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "user", BeginOfStream: true},
	}); err != nil {
		t.Fatal(err)
	}
	return cancel, gate, output, reads
}

func drainTranscriptPublication(reads <-chan transcriptPublicationRead) []transcriptPublicationRead {
	var result []transcriptPublicationRead
	for {
		select {
		case read := <-reads:
			result = append(result, read)
		default:
			return result
		}
	}
}

func TestDockPublishesASRTranscriptBeforeAgentResponse(t *testing.T) {
	for index, name := range []string{"BOS", "text", "EOS"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cancel, gate, output, reads := startTranscriptPublication(t, index)
				defer cancel()
				synctest.Wait()
				beforeRelease := drainTranscriptPublication(reads)
				close(gate)
				synctest.Wait()
				all := append(beforeRelease, drainTranscriptPublication(reads)...)
				_ = output.Close()
				cancel()
				synctest.Wait()
				for _, read := range beforeRelease {
					if read.chunk != nil && read.chunk.Role == genx.RoleModel {
						t.Fatalf("Agent published before visible transcript %s was released", name)
					}
				}
				text, terminal, response := false, false, false
				for _, read := range all {
					if read.err != nil {
						t.Fatalf("output before completion: %v", read.err)
					}
					chunk := read.chunk
					if chunk == nil {
						continue
					}
					if chunk.Role == genx.RoleUser {
						text = text || chunk.Part == genx.Text("question")
						terminal = terminal || chunk.IsEndOfStream()
					}
					if chunk.Role == genx.RoleModel {
						response = true
						if !text || !terminal {
							t.Fatal("Agent response preceded actual transcript text/EOS publication")
						}
					}
				}
				if !text || !terminal || !response {
					t.Fatalf("incomplete output: text=%t EOS=%t response=%t", text, terminal, response)
				}
			})
		})
	}
}

func TestDockTranscriptPublicationCancellation(t *testing.T) {
	for _, closeOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("consumer_close_%t", closeOutput), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cancel, _, output, reads := startTranscriptPublication(t, 2)
				defer cancel()
				synctest.Wait()
				if closeOutput {
					if err := output.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
				synctest.Wait()
				results := drainTranscriptPublication(reads)
				if len(results) == 0 || results[len(results)-1].err == nil {
					t.Fatal("output reader did not terminate while transcript publication was blocked")
				}
			})
		})
	}
}
