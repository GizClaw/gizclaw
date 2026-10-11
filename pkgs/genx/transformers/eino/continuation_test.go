package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func continuationConfig(generate func(context.Context, map[string]any) (map[string]any, error)) Config {
	config := textConfig()
	config.ContinueFrom = "continue"
	config.Graph.Compile = GraphCompileConfig{NodeTriggerMode: NodeTriggerAnyPredecessor, MaxRunSteps: 4}
	config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "continue", Type: StateBoolean, Merge: MergeReplace})
	config.Graph.Nodes = []NodeDefinition{{
		ID: "answer", Lambda: &LambdaRefNode{Lambda: "narrate"},
		Inputs:  map[string]Binding{"text": {From: "input.text"}, "messages": {From: "input.messages"}},
		Outputs: map[string]string{"text": "answer", "continue": "continue"},
	}}
	config.Lambdas = staticLambdaResolver{resolved: ResolvedLambda{
		Lambda:  compose.InvokableLambda(generate),
		Inputs:  map[string]StateType{"text": StateString, "messages": StateMessages},
		Outputs: map[string]StateType{"text": StateString, "continue": StateBoolean},
	}}
	return config
}

type continuationTTSMux struct {
	synthesize streamkit.TTSSynthesizer
}

func (m continuationTTSMux) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus", m.synthesize), nil
}

// Run the actual Eino -> Audio Dock -> shared TTS chain with final delivery
// deferred, as in MixerOutput. Provider completion cannot release the next
// generation while downstream audio is stalled. Input interruption must still
// progress while that same delivery is outstanding.
func TestContinuationAudioBackpressure(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(fmt.Sprintf("interrupt=%v", interrupt), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const iterations = 12
				var calls, syntheses atomic.Int32
				var want strings.Builder
				config := continuationConfig(func(_ context.Context, input map[string]any) (map[string]any, error) {
					if input["text"] == "stop" {
						return map[string]any{"text": "Stopped.", "continue": false}, nil
					}
					n := calls.Add(1)
					text := strings.Repeat(fmt.Sprintf("Topic %d has a sentence long enough to need its own synthesis request. ", n), 6)
					want.WriteString(text)
					return map[string]any{"text": text, "continue": n < iterations}, nil
				})
				store := &recordingHistoryStore{events: &eventRecorder{}}
				config.History = &HistoryConfig{Store: store, Scope: "podcast", Limit: 50}
				transformer, err := New(t.Context(), config)
				if err != nil {
					t.Fatal(err)
				}
				defer transformer.Close()
				dock, err := audiodock.New(audiodock.Config{
					Agent: transformer, Backpressure: true,
					ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) { return "voice/host", nil },
					TTS: continuationTTSMux{synthesize: func(_ context.Context, _ string, meta streamkit.TTSMeta, _ string, emit func([]byte) error) error {
						syntheses.Add(1)
						for i := range 2 {
							if err := emit([]byte{byte(meta.SegmentIndex), byte(i)}); err != nil {
								return err
							}
						}
						return nil
					}},
				})
				if err != nil {
					t.Fatal(err)
				}
				input := newInputBuilder()
				output, err := dock.Transform(t.Context(), input.Stream())
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				observer := output.(interface {
					DeferOutputObservation()
					ObserveOutput(*genx.MessageChunk)
				})
				observer.DeferOutputObservation()
				addTextTurn(t, input, "podcast")
				if !interrupt {
					if err := input.Done(genx.Usage{}); err != nil {
						t.Fatal(err)
					}
				}
				var chunks []*genx.MessageChunk
				var held *genx.MessageChunk
				for held == nil {
					chunk, err := output.Next()
					if err != nil {
						t.Fatal(err)
					}
					chunks = append(chunks, chunk)
					if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
						held = chunk
					} else {
						observer.ObserveOutput(chunk)
					}
				}
				synctest.Wait()
				before, providers := calls.Load(), syntheses.Load()
				if before != 1 || providers > 2 {
					t.Fatalf("stalled audio ran ahead: generations=%d syntheses=%d", before, providers)
				}
				time.Sleep(5 * time.Minute)
				synctest.Wait()
				if calls.Load() != before || syntheses.Load() != providers {
					t.Fatal("stalled audio kept generating or synthesizing")
				}
				if interrupt {
					addTextTurn(t, input, "stop")
					if err := input.Done(genx.Usage{}); err != nil {
						t.Fatal(err)
					}
					// Cancellation releases the producer even if the final consumer
					// never acknowledges its last claimed audio chunk.
				} else {
					observer.ObserveOutput(held)
				}
				for {
					chunk, err := output.Next()
					if err != nil && isStreamEnd(err) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					chunks = append(chunks, chunk)
					observer.ObserveOutput(chunk)
				}
				synctest.Wait()
				messages := continuationRecordedMessages(t, store)
				if interrupt {
					firstID := held.Ctrl.StreamID
					var prefix strings.Builder
					interrupted := 0
					for _, chunk := range chunks {
						if chunk.Ctrl.StreamID == firstID {
							if text, ok := chunk.Part.(genx.Text); ok {
								prefix.WriteString(string(text))
							}
							if chunk.IsEndOfStream() && chunk.Ctrl.ResponseEpochEnd && chunk.Ctrl.Error == "interrupted" {
								interrupted++
							}
						}
					}
					if interrupted != 1 || calls.Load() != before || len(messages) != 4 || messages[1].Content != prefix.String() || messages[3].Content != "Stopped." {
						t.Fatalf("interrupted=%d generations=%d History=%+v", interrupted, calls.Load(), messages)
					}
				} else {
					if calls.Load() != iterations || joinedText(chunks) != want.String() || len(messages) != iterations+1 || continuationAssistantText(messages) != want.String() {
						t.Fatalf("generations=%d History=%+v", calls.Load(), messages)
					}
					textBOS, textEOS, audioBOS, audioEOS := 0, 0, 0, 0
					for _, chunk := range chunks {
						if chunk.Ctrl.StreamID != held.Ctrl.StreamID || chunk.Ctrl.Error != "" {
							t.Fatalf("split or failed continuous route: %+v", chunk.Ctrl)
						}
						if _, ok := chunk.Part.(genx.Text); ok {
							if chunk.IsBeginOfStream() {
								textBOS++
							}
							if chunk.IsEndOfStream() {
								textEOS++
							}
						} else {
							if chunk.IsBeginOfStream() {
								audioBOS++
							}
							if chunk.IsEndOfStream() {
								audioEOS++
							}
						}
					}
					if textBOS != 1 || textEOS != 1 || audioBOS != 1 || audioEOS != 1 {
						t.Fatalf("text BOS/EOS=%d/%d audio BOS/EOS=%d/%d", textBOS, textEOS, audioBOS, audioEOS)
					}
				}
			})
		})
	}
}

func TestContinuationSharesRouteAndRecordsEachGeneration(t *testing.T) {
	t.Parallel()
	const iterations = 40
	var calls int
	var want strings.Builder
	config := continuationConfig(func(_ context.Context, input map[string]any) (map[string]any, error) {
		calls++
		messages := input["messages"].([]*schema.Message)
		if len(messages) != 1+min(calls-1, maxContinuationMessages) {
			t.Errorf("iteration %d context has %d messages", calls, len(messages))
		}
		if calls > 1 && messages[len(messages)-1].Content != fmt.Sprintf("topic %d. ", calls-1) {
			t.Errorf("iteration %d lost previous narration: %#v", calls, messages)
		}
		text := fmt.Sprintf("topic %d. ", calls)
		want.WriteString(text)
		return map[string]any{"text": text, "continue": calls < iterations}, nil
	})
	events := &eventRecorder{}
	store := &recordingHistoryStore{events: events}
	config.History = &HistoryConfig{Store: store, Scope: "podcast", Limit: 50}
	memories := &recordingMemoryStore{events: events}
	config.Memory = &MemoryConfig{Store: memories, Scope: memory.Scope{AppID: "podcast"}, Observe: ObservePolicy{Enabled: true}}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	output, err := transformer.Transform(t.Context(), textInput("podcast"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	chunks := drain(t, output)
	if calls != iterations || joinedText(chunks) != want.String() {
		t.Fatalf("generations=%d output=%q, want %d complete topics", calls, joinedText(chunks), iterations)
	}
	id, begins, ends := "", 0, 0
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			t.Fatal("missing route control")
		}
		if id == "" {
			id = chunk.Ctrl.StreamID
		}
		if chunk.Ctrl.StreamID != id || chunk.Ctrl.Error != "" {
			t.Fatalf("split or failed narration: %+v", chunk.Ctrl)
		}
		if chunk.IsBeginOfStream() {
			begins++
		}
		if chunk.IsEndOfStream() {
			ends++
		}
	}
	if begins != 1 || ends != 1 {
		t.Fatalf("BOS=%d EOS=%d, want one uninterrupted route", begins, ends)
	}
	messages := continuationRecordedMessages(t, store)
	if len(messages) != iterations+1 || continuationAssistantText(messages) != want.String() {
		t.Fatalf("History=%+v, want one user and one assistant message per generation", messages)
	}
	for i, message := range messages[1:] {
		if message.Role != schema.Assistant || message.Content != fmt.Sprintf("topic %d. ", i+1) {
			t.Fatalf("generation %d History=%+v", i+1, message)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != iterations+1 {
		t.Fatalf("persisted records=%d, want one user and %d narration segments", len(store.records), iterations)
	}
	memories.mu.Lock()
	defer memories.mu.Unlock()
	ids := make(map[string]bool)
	for _, observation := range memories.observations {
		if observation.ID == "" || ids[observation.ID] {
			t.Fatalf("generations reused Memory observation identity: %+v", observation)
		}
		ids[observation.ID] = true
	}
	if len(ids) != iterations {
		t.Fatalf("Memory observed %d of %d generations", len(ids), iterations)
	}
}

func TestContinuationInterruptCancelsGenerationAndRecordsOnePrefix(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	blocked, cancelled := make(chan struct{}), make(chan struct{})
	config := continuationConfig(func(ctx context.Context, input map[string]any) (map[string]any, error) {
		if input["text"] == "stop" {
			return map[string]any{"text": "stopped", "continue": false}, nil
		}
		n := calls.Add(1)
		if n == 3 {
			close(blocked)
			<-ctx.Done()
			close(cancelled)
			return nil, context.Cause(ctx)
		}
		return map[string]any{"text": fmt.Sprintf("topic %d. ", n), "continue": true}, nil
	})
	store := &recordingHistoryStore{events: &eventRecorder{}}
	config.History = &HistoryConfig{Store: store, Scope: "podcast", Limit: 50}
	transformer, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer transformer.Close()
	input := newInputBuilder()
	output, err := transformer.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	addTextTurn(t, input, "podcast")
	var delivered strings.Builder
	firstID := ""
	completed := 0
	for completed != 2 {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if chunk.IsEndOfStream() {
			t.Fatal("narration ended between topics")
		}
		firstID = chunk.Ctrl.StreamID
		if text, ok := chunk.Part.(genx.Text); ok {
			delivered.WriteString(string(text))
		}
		if chunk.Ctrl.MessageEnd {
			completed++
		}
	}
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal("next generation did not start")
	}
	store.mu.Lock()
	recordsBeforeInterruption := len(store.records)
	store.mu.Unlock()
	if recordsBeforeInterruption != 3 {
		t.Fatalf("History before interruption=%d, want one user and two completed generations", recordsBeforeInterruption)
	}
	addTextTurn(t, input, "stop")
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	chunks := drain(t, output)
	select {
	case <-cancelled:
	default:
		t.Fatal("interruption did not cancel the in-flight generation")
	}
	interrupted := 0
	for _, chunk := range chunks {
		if chunk.Ctrl.StreamID == firstID && chunk.IsEndOfStream() && chunk.Ctrl.Error == "interrupted" {
			interrupted++
		}
	}
	if interrupted != 1 || joinedText(chunks) != "stopped" || calls.Load() != 3 {
		t.Fatalf("interrupted=%d suffix=%q calls=%d", interrupted, joinedText(chunks), calls.Load())
	}
	messages := continuationRecordedMessages(t, store)
	if len(messages) != 5 || continuationAssistantText(messages[:3]) != delivered.String() || messages[4].Content != "stopped" {
		t.Fatalf("History=%+v, want completed generations followed by the replacement reply", messages)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 5 || store.records[1].Attributes["interrupted"] != "" || store.records[2].Attributes["interrupted"] != "" {
		t.Fatalf("History records=%+v", store.records)
	}
}

func continuationAssistantText(messages []*schema.Message) string {
	var text strings.Builder
	for _, message := range messages {
		if message.Role == schema.Assistant {
			text.WriteString(message.Content)
		}
	}
	return text.String()
}

func TestContinuationAudioFailureAndConsumerCloseReleaseGeneration(t *testing.T) {
	for _, providerFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider-failure=%v", providerFailure), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var calls atomic.Int32
				config := continuationConfig(func(context.Context, map[string]any) (map[string]any, error) {
					calls.Add(1)
					return map[string]any{"text": "A complete spoken sentence.", "continue": true}, nil
				})
				core, err := New(t.Context(), config)
				if err != nil {
					t.Fatal(err)
				}
				defer core.Close()
				dock, err := audiodock.New(audiodock.Config{
					Agent: core, Backpressure: true,
					ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) { return "voice/host", nil },
					TTS: continuationTTSMux{synthesize: func(ctx context.Context, _ string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
						if err := emit([]byte{1, 2}); err != nil {
							return err
						}
						if providerFailure {
							return errors.New("synthesis failed")
						}
						<-ctx.Done()
						return context.Cause(ctx)
					}},
				})
				if err != nil {
					t.Fatal(err)
				}
				output, err := dock.Transform(t.Context(), textInput("podcast"))
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				failed := false
				for {
					chunk, err := output.Next()
					if err != nil {
						if !providerFailure || !strings.Contains(err.Error(), "synthesis failed") {
							t.Fatalf("terminal=%v", err)
						}
						break
					}
					if !providerFailure {
						if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) != 0 {
							if err := output.Close(); err != nil {
								t.Fatal(err)
							}
							break
						}
					} else if chunk.IsEndOfStream() && strings.Contains(chunk.Ctrl.Error, "synthesis failed") {
						failed = true
					}
				}
				synctest.Wait()
				if calls.Load() != 1 || (providerFailure && !failed) {
					t.Fatalf("generation survived terminal failure: calls=%d error EOS=%v", calls.Load(), failed)
				}
			})
		})
	}
}

func TestContinuationStopResumeAndRedirect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := continuationConfig(func(_ context.Context, input map[string]any) (map[string]any, error) {
			request := input["text"].(string)
			messages := input["messages"].([]*schema.Message)
			if request == "resume" && !strings.Contains(continuationAssistantText(messages), "science") {
				t.Error("resume lost the preceding narration")
			}
			return map[string]any{"text": request + ". ", "continue": request != "stop"}, nil
		})
		config.History = &HistoryConfig{Limit: 50}
		transformer, err := New(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		defer transformer.Close()
		input := newInputBuilder()
		output, err := transformer.Transform(t.Context(), input.Stream())
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		ids := make(map[string]bool)
		requests := []string{"science", "stop", "resume", "space", "stop"}
		for _, request := range requests {
			addTextTurn(t, input, request)
			synctest.Wait()
			var text strings.Builder
			for {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					text.WriteString(string(part))
				}
				if chunk.Ctrl.MessageEnd {
					if ids[chunk.Ctrl.StreamID] || text.String() != request+". " {
						t.Fatalf("request=%q response=%q ctrl=%+v", request, text.String(), chunk.Ctrl)
					}
					ids[chunk.Ctrl.StreamID] = true
					synctest.Wait()
					break
				}
			}
			if request == "stop" {
				chunk, err := output.Next()
				if err != nil || !chunk.IsEndOfStream() || chunk.Ctrl.Error != "" {
					t.Fatalf("stop did not finish cleanly: %+v, %v", chunk, err)
				}
			}
		}
		if err := input.Done(genx.Usage{}); err != nil {
			t.Fatal(err)
		}
		drain(t, output)
		messages, err := transformer.history.load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 2*len(requests) {
			t.Fatalf("History contains extra user turns or duplicate narration: %+v", messages)
		}
		for i, request := range requests {
			if messages[2*i].Role != schema.User || messages[2*i].Content != request || messages[2*i+1].Content != request+". " {
				t.Fatalf("request %d History=%+v", i, messages[2*i:2*i+2])
			}
		}
	})
}

func continuationRecordedMessages(t *testing.T, store *recordingHistoryStore) []*schema.Message {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	messages := make([]*schema.Message, 0, len(store.records))
	for _, record := range store.records {
		var payload struct {
			Message *schema.Message `json:"message"`
		}
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, payload.Message)
	}
	return messages
}

func TestContinuationValidation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing", "answer"} {
		config := textConfig()
		config.ContinueFrom = name
		if err := ValidateConfig(config); err == nil || !strings.Contains(err.Error(), "boolean root State field") {
			t.Fatalf("ContinueFrom=%q validation=%v", name, err)
		}
	}
	config := textConfig()
	config.ContinueFrom = "continue"
	config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "continue", Type: StateBoolean, Merge: MergeReplace})
	config.Graph.State.Fields[0].Merge = MergeAppend
	if err := ValidateConfig(config); err == nil || !strings.Contains(err.Error(), "one text/plain string output") {
		t.Fatalf("append output validation=%v", err)
	}
}

func TestContinuationRejectsEmptyNarration(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"", " \n\t"} {
		t.Run(fmt.Sprintf("text=%q", text), func(t *testing.T) {
			config := continuationConfig(func(context.Context, map[string]any) (map[string]any, error) {
				return map[string]any{"text": text, "continue": true}, nil
			})
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer transformer.Close()
			output, err := transformer.Transform(t.Context(), textInput("podcast"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			chunks := drain(t, output)
			if last := chunks[len(chunks)-1]; !last.IsEndOfStream() || !strings.Contains(last.Ctrl.Error, "narration") {
				t.Fatalf("empty continuation did not fail: %#v", last)
			}
		})
	}
}
