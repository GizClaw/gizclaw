package dashscoperealtime

import (
	"context"
	"iter"
	"sync"
	"testing"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerConcurrentCallsOwnSessions(t *testing.T) {
	opener := &fakeDashScopeOpener{}
	transformer := newTransformer(nil)
	transformer.realtime = opener

	const calls = 8
	streams := make(chan *Stream, calls)
	errs := make(chan error, calls)
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := transformer.Transform(context.Background(), emptyDashScopeStream{})
			if err != nil {
				errs <- err
				return
			}
			streams <- stream.(*Stream)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Transform() error = %v", err)
	}
	close(streams)

	seen := make(map[dashScopeRealtimeSession]struct{}, calls)
	for stream := range streams {
		if _, exists := seen[stream.session]; exists {
			t.Fatal("concurrent Transform calls shared a provider session")
		}
		seen[stream.session] = struct{}{}
	}
	if len(seen) != calls || opener.count() != calls {
		t.Fatalf("sessions = %d, opens = %d, want %d", len(seen), opener.count(), calls)
	}
}

func TestDashScopeStreamIDsSeparateInputAndResponseRoutes(t *testing.T) {
	var ids dashScopeStreamIDs
	ids.pushInput("turn-1")
	ids.pushInput("turn-1")
	ids.bindNextResponse()
	inputID, firstResponseID := ids.current()
	if inputID != "turn-1" {
		t.Fatalf("input StreamID = %q, want turn-1", inputID)
	}
	if firstResponseID == "" || firstResponseID == inputID {
		t.Fatalf("response StreamID = %q, input StreamID = %q", firstResponseID, inputID)
	}

	// A second event that starts the same response must keep its response ID.
	ids.bindNextResponse()
	_, sameResponseID := ids.current()
	if sameResponseID != firstResponseID {
		t.Fatalf("same response StreamID = %q, want %q", sameResponseID, firstResponseID)
	}

	ids.pushInput("turn-2")
	ids.bindNextResponse()
	inputID, secondResponseID := ids.current()
	if inputID != "turn-2" {
		t.Fatalf("second input StreamID = %q, want turn-2", inputID)
	}
	if secondResponseID == "" || secondResponseID == inputID || secondResponseID == firstResponseID {
		t.Fatalf("second response StreamID = %q, first = %q, input = %q", secondResponseID, firstResponseID, inputID)
	}
}

type emptyDashScopeStream struct{}

func (emptyDashScopeStream) Next() (*genx.MessageChunk, error) { return nil, genx.ErrDone }
func (emptyDashScopeStream) Close() error                      { return nil }
func (emptyDashScopeStream) CloseWithError(error) error        { return nil }

type fakeDashScopeOpener struct {
	mu       sync.Mutex
	sessions []*fakeDashScopeSession
}

func (o *fakeDashScopeOpener) Connect(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	session := &fakeDashScopeSession{}
	o.sessions = append(o.sessions, session)
	return session, nil
}

func (o *fakeDashScopeOpener) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.sessions)
}

type fakeDashScopeSession struct {
	mu         sync.Mutex
	eventCalls int
}

func (s *fakeDashScopeSession) UpdateSession(*dashscope.SessionConfig) error { return nil }
func (s *fakeDashScopeSession) AppendAudio([]byte) error                     { return nil }
func (s *fakeDashScopeSession) CommitInput() error                           { return nil }
func (s *fakeDashScopeSession) ClearInput() error                            { return nil }
func (s *fakeDashScopeSession) CreateResponse(*dashscope.ResponseCreateOptions) error {
	return nil
}
func (s *fakeDashScopeSession) CancelResponse() error { return nil }
func (s *fakeDashScopeSession) Close() error          { return nil }
func (s *fakeDashScopeSession) Events() iter.Seq2[*dashscope.RealtimeEvent, error] {
	s.mu.Lock()
	s.eventCalls++
	call := s.eventCalls
	s.mu.Unlock()
	return func(yield func(*dashscope.RealtimeEvent, error) bool) {
		if call == 1 {
			yield(&dashscope.RealtimeEvent{Type: dashscope.EventTypeSessionCreated}, nil)
		}
	}
}

func TestNew(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New(Config{}) succeeded without a client")
	}
	transformer, err := New(Config{Client: dashscope.NewClient("")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	temperature := 0.5
	maxTokens := 10
	enableASR := false
	modalities := []string{"text", "audio"}
	turnDetection := &dashscope.TurnDetection{Type: "server_vad"}
	transformer, err := New(Config{
		Client:            dashscope.NewClient(""),
		Model:             "model",
		Voice:             "voice",
		Instructions:      "instructions",
		Modalities:        modalities,
		VAD:               "server_vad",
		Temperature:       &temperature,
		MaxOutputTokens:   &maxTokens,
		EnableASR:         &enableASR,
		ASRModel:          "asr-model",
		TurnDetection:     turnDetection,
		InputAudioFormat:  "pcm16",
		OutputAudioFormat: "pcm16",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	modalities[0] = "changed"
	temperature = 1
	turnDetection.Type = "changed"
	if transformer.modalities[0] != "text" {
		t.Fatal("New() retained caller-owned Modalities slice")
	}
	if transformer.temperature == nil || *transformer.temperature != 0.5 {
		t.Fatal("New() retained caller-owned Temperature pointer")
	}
	if transformer.turnDetection == nil || transformer.turnDetection.Type != "server_vad" {
		t.Fatal("New() retained caller-owned TurnDetection pointer")
	}
	if transformer.model != "model" || transformer.voice != "voice" ||
		transformer.instructions != "instructions" || transformer.vadType != "server_vad" ||
		transformer.maxOutputTokens == nil || *transformer.maxOutputTokens != 10 ||
		transformer.enableInputAudioTranscription || transformer.inputAudioTranscriptionModel != "asr-model" ||
		transformer.inputAudioFormat != "pcm16" || transformer.outputAudioFormat != "pcm16" {
		t.Fatalf("configured transformer = %#v", transformer)
	}
}
