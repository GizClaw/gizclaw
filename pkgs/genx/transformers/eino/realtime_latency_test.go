package eino

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// This is the provider-independent portion of the peer realtime path. Push is
// the server's ingress boundary; WebRTC transport and provider VAD are not
// simulated as measured GizClaw overhead. The audio route stays open, as it does
// in realtime mode; ASR closes a definite text segment independently.
func TestRealtimeFirstResponseLatency(t *testing.T) {
	for _, test := range []struct {
		name                   string
		vad, final, token, tts time.Duration
		history, recall        time.Duration
	}{
		{name: "immediate"},
		{name: "controlled", vad: 40 * time.Millisecond, final: 60 * time.Millisecond, token: 90 * time.Millisecond, tts: 120 * time.Millisecond},
		{name: "history_and_memory", history: 40 * time.Millisecond, recall: 60 * time.Millisecond},
		{name: "slow_tts", token: 20 * time.Millisecond, tts: 400 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var ingress, endpoint, final, agentEOS, modelStart, token, coreText, ttsStart, ttsInput, audio atomic.Int64
			var historyStart, historyEnd, recallStart, recallEnd atomic.Int64
			mark := func(stamp *atomic.Int64) { stamp.CompareAndSwap(0, time.Now().UnixNano()) }
			asr := latencyTransformer(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
				out := streamkit.NewOutput(streamkit.OutputConfig{})
				go func() {
					defer out.Close()
					for {
						chunk, err := input.Next()
						if err != nil {
							return
						}
						if blob, ok := chunk.Part.(*genx.Blob); !ok || len(blob.Data) == 0 {
							continue
						}
						mark(&ingress)
						// An interim transcript exists before endpointing, but is not a turn.
						_ = out.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "transcript", BeginOfStream: true}})
						if !latencyWait(ctx, test.vad) {
							return
						}
						mark(&endpoint)
						if !latencyWait(ctx, test.final) {
							return
						}
						mark(&final)
						_ = out.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "transcript", EndOfStream: true}})
					}
				}()
				return out, nil
			})
			release := make(chan struct{})
			defer close(release)
			chat := &latencyChat{release: release, started: &modelStart, token: &token, delay: test.token}
			config := chatConfig(&componentMapResolver{chat: chat})
			if test.history > 0 {
				config.History = &HistoryConfig{Scope: "latency", Limit: 10, Store: &latencyHistory{recordingHistoryStore: recordingHistoryStore{events: &eventRecorder{}}, start: &historyStart, end: &historyEnd, delay: test.history}}
			}
			if test.recall > 0 {
				config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "recalled", Type: StateString, Merge: MergeReplace})
				config.Graph.Nodes[0].Inputs["memory"] = Binding{From: "recalled"}
				config.Graph.Nodes[0].Prompt.Messages[0].Template = "{memory}"
				config.Memory = &MemoryConfig{Scope: memory.Scope{AppID: "latency", UserID: "user", AgentID: "assistant"}, Store: &latencyMemory{recordingMemoryStore: recordingMemoryStore{events: &eventRecorder{}}, start: &recallStart, end: &recallEnd, delay: test.recall}, Recall: []RecallDefinition{{QueryFrom: "input.text", Output: "recalled", TopK: 1}}}
			}
			core, err := New(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			agent := latencyTransformer(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
				out, err := core.Transform(ctx, &latencyTap{Stream: input, observe: func(c *genx.MessageChunk) {
					if c.IsEndOfStream() {
						mark(&agentEOS)
					}
				}})
				if err != nil {
					return nil, err
				}
				return &latencyTap{Stream: out, observe: func(c *genx.MessageChunk) {
					if text, ok := c.Part.(genx.Text); ok && len(text) > 0 {
						mark(&coreText)
					}
				}}, nil
			})
			tts := latencyMux(func(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
				mark(&ttsStart)
				out := streamkit.NewOutput(streamkit.OutputConfig{})
				go func() {
					defer out.Close()
					for {
						chunk, err := input.Next()
						if err != nil {
							return
						}
						if text, ok := chunk.Part.(genx.Text); ok && len(text) > 0 {
							mark(&ttsInput)
							if !latencyWait(ctx, test.tts) {
								return
							}
							mark(&audio)
							_ = out.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "voice", BeginOfStream: true}})
						}
						if chunk.IsEndOfStream() {
							_ = out.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "voice", EndOfStream: true}})
							return
						}
					}
				}()
				return out, nil
			})
			dock, err := audiodock.New(audiodock.Config{Agent: agent, ASR: asr, TTS: tts, ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) { return "voice", nil }})
			if err != nil {
				t.Fatal(err)
			}
			input := genx.NewRealtimeStream()
			defer input.Close()
			output, err := dock.Transform(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			// Ensure a stalled pipeline fails instead of blocking the test forever.
			stop := context.AfterFunc(ctx, func() { _ = input.Close(); _ = output.Close() })
			defer stop()
			speechEnded := time.Now()
			err = input.Push(ctx, &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "microphone", BeginOfStream: true, Timestamp: speechEnded.UnixMilli()}})
			if err != nil {
				t.Fatal(err)
			}
			var firstText, firstAudio time.Time
			for firstText.IsZero() || firstAudio.IsZero() {
				chunk, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					t.Fatal(chunk.Ctrl.Error)
				}
				if chunk.Role != genx.RoleModel {
					continue
				}
				if text, ok := chunk.Part.(genx.Text); ok && len(text) > 0 && firstText.IsZero() {
					firstText = time.Now()
				}
				if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 && firstAudio.IsZero() {
					firstAudio = time.Now()
				}
			}
			if firstText.Sub(speechEnded) >= 2*time.Second || firstAudio.Sub(speechEnded) >= 3*time.Second {
				t.Fatalf("gate: text=%s audio=%s", firstText.Sub(speechEnded), firstAudio.Sub(speechEnded))
			}
			if modelStart.Load() < final.Load() {
				t.Fatal("model started before definite transcript")
			}
			if test.tts > 0 && firstText.UnixNano() >= audio.Load() {
				t.Fatal("text waited for TTS audio")
			}
			delta := func(a, b int64) time.Duration { return time.Duration(b - a) }
			t.Logf("reorder=%s provider_vad=%s provider_final=%s dock_to_eino_EOS=%s eino_start_history_prompt=%s provider_token=%s token_to_core_text=%s core_to_dock_output=%s text_to_tts_start=%s tts_input_handoff=%s provider_tts=%s audio_handoff=%s total_text=%s total_audio=%s history_store=%s memory_recall=%s",
				delta(speechEnded.UnixNano(), ingress.Load()), delta(ingress.Load(), endpoint.Load()), delta(endpoint.Load(), final.Load()), delta(final.Load(), agentEOS.Load()), delta(agentEOS.Load(), modelStart.Load()), delta(modelStart.Load(), token.Load()), delta(token.Load(), coreText.Load()), delta(coreText.Load(), firstText.UnixNano()), delta(coreText.Load(), ttsStart.Load()), delta(ttsStart.Load(), ttsInput.Load()), delta(ttsInput.Load(), audio.Load()), delta(audio.Load(), firstAudio.UnixNano()), firstText.Sub(speechEnded), firstAudio.Sub(speechEnded), delta(historyStart.Load(), historyEnd.Load()), delta(recallStart.Load(), recallEnd.Load()))
		})
	}
}

type latencyTransformer func(context.Context, genx.Stream) (genx.Stream, error)

func (f latencyTransformer) Transform(ctx context.Context, s genx.Stream) (genx.Stream, error) {
	return f(ctx, s)
}

type latencyMux func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f latencyMux) Transform(ctx context.Context, p string, s genx.Stream) (genx.Stream, error) {
	return f(ctx, p, s)
}

type latencyTap struct {
	genx.Stream
	observe func(*genx.MessageChunk)
}

func (s *latencyTap) Next() (*genx.MessageChunk, error) {
	c, e := s.Stream.Next()
	if e == nil && c != nil {
		s.observe(c)
	}
	return c, e
}

type latencyChat struct {
	fakeChatModel
	started, token *atomic.Int64
	delay          time.Duration
	release        <-chan struct{}
}

func (m *latencyChat) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.started.Store(time.Now().UnixNano())
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		if !latencyWait(ctx, m.delay) {
			writer.Send(nil, ctx.Err())
			return
		}
		m.token.Store(time.Now().UnixNano())
		writer.Send(&schema.Message{Role: schema.Assistant, Content: "hello"}, nil)
		select {
		case <-ctx.Done():
		case <-m.release:
		}
	}()
	return reader, nil
}
func latencyWait(ctx context.Context, d time.Duration) bool {
	if d == 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// These substitutes measure the serial Store boundaries without external I/O.
type latencyHistory struct {
	recordingHistoryStore
	start, end *atomic.Int64
	delay      time.Duration
}

func (s *latencyHistory) Query(ctx context.Context, q logstore.Query) (logstore.Page, error) {
	s.start.Store(time.Now().UnixNano())
	defer func() { s.end.Store(time.Now().UnixNano()) }()
	if !latencyWait(ctx, s.delay) {
		return logstore.Page{}, ctx.Err()
	}
	return s.recordingHistoryStore.Query(ctx, q)
}

type latencyMemory struct {
	recordingMemoryStore
	start, end *atomic.Int64
	delay      time.Duration
}

func (s *latencyMemory) Recall(ctx context.Context, q memory.Query) (memory.RecallResult, error) {
	s.start.Store(time.Now().UnixNano())
	defer func() { s.end.Store(time.Now().UnixNano()) }()
	if !latencyWait(ctx, s.delay) {
		return memory.RecallResult{}, ctx.Err()
	}
	return s.recordingMemoryStore.Recall(ctx, q)
}
