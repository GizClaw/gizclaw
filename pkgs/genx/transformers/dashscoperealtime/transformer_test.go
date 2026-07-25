package dashscoperealtime

import (
	"context"
	"iter"
	"strings"
	"sync"
	"testing"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
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
		wg.Go(func() {
			stream, err := transformer.Transform(context.Background(), emptyDashScopeStream{})
			if err != nil {
				errs <- err
				return
			}
			streams <- stream.(*Stream)
		})
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
	ids.pushInput("turn-2")

	// response.created may arrive before ASR completion. Binding it must not
	// consume turn-2 when the turn-1 transcription arrives later.
	inputID, firstResponseID := ids.bindResponse("provider-response-1")
	if inputID != "turn-1" {
		t.Fatalf("input StreamID = %q, want turn-1", inputID)
	}
	if firstResponseID == "" || firstResponseID == inputID {
		t.Fatalf("response StreamID = %q, input StreamID = %q", firstResponseID, inputID)
	}
	inputID, sameResponseID := ids.bindTranscription()
	if inputID != "turn-1" {
		t.Fatalf("transcription StreamID = %q, want turn-1", inputID)
	}
	if sameResponseID != firstResponseID {
		t.Fatalf("transcription response StreamID = %q, want %q", sameResponseID, firstResponseID)
	}

	// The provider ID keeps interleaved events on the response that created it.
	if routed := ids.response("provider-response-1"); routed != firstResponseID {
		t.Fatalf("provider response StreamID = %q, want %q", routed, firstResponseID)
	}
	transcriptResponseID := ids.responseTranscript("provider-response-1")
	if transcriptResponseID == "" || transcriptResponseID == firstResponseID {
		t.Fatalf("response transcript StreamID = %q, response = %q", transcriptResponseID, firstResponseID)
	}
	if repeated := ids.responseTranscript("provider-response-1"); repeated != transcriptResponseID {
		t.Fatalf("repeated response transcript StreamID = %q, want %q", repeated, transcriptResponseID)
	}

	// The next ASR completion now consumes turn-2, independent of whether its
	// response.created event arrives before or after it.
	inputID, secondResponseID := ids.bindTranscription()
	if inputID != "turn-2" {
		t.Fatalf("second input StreamID = %q, want turn-2", inputID)
	}
	if secondResponseID == "" || secondResponseID == inputID || secondResponseID == firstResponseID {
		t.Fatalf("second response StreamID = %q, first = %q, input = %q", secondResponseID, firstResponseID, inputID)
	}
	responseInputID, routedSecondResponseID := ids.bindResponse("provider-response-2")
	if responseInputID != "turn-2" || routedSecondResponseID != secondResponseID {
		t.Fatalf("second response binding = (%q, %q), want (turn-2, %q)", responseInputID, routedSecondResponseID, secondResponseID)
	}
}

func TestPrepareInputAudioDecodesPeerOpusToPCM16(t *testing.T) {
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("create Opus encoder: %v", err)
	}
	defer encoder.Close()
	samples := make([]int16, 320)
	for index := range samples {
		samples[index] = int16(index * 50)
	}
	packet, err := encoder.Encode(samples, len(samples))
	if err != nil {
		t.Fatalf("encode Opus packet: %v", err)
	}

	transformer := newTransformer(nil, withInputAudioFormat(dashscope.AudioFormatPCM16))
	inputs := doubaorealtime.NewSharedAudioInputs("pcm", 16000, 1, true)
	defer inputs.Close()
	data, err := transformer.prepareInputAudio(inputs, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus; rate=16000", Data: packet},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}, &genx.Blob{MIMEType: "audio/opus; rate=16000", Data: packet})
	if err != nil {
		t.Fatalf("prepareInputAudio() error = %v", err)
	}
	if len(data) == 0 || len(data)%2 != 0 || string(data) == string(packet) {
		t.Fatalf("prepareInputAudio() returned %d bytes, want decoded PCM16", len(data))
	}
}

func TestPrepareInputAudioRejectsOggOpusContainer(t *testing.T) {
	transformer := newTransformer(nil, withInputAudioFormat(dashscope.AudioFormatPCM16))
	inputs := doubaorealtime.NewSharedAudioInputs("pcm", 16000, 1, true)
	defer inputs.Close()
	_, err := transformer.prepareInputAudio(inputs, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: []byte("OggS")},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}, &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: []byte("OggS")})
	if err == nil || !strings.Contains(err.Error(), "Ogg/Opus input is unsupported") {
		t.Fatalf("prepareInputAudio() error = %v, want explicit Ogg/Opus rejection", err)
	}
}

func TestPCMOutputCarriesProviderSampleRate(t *testing.T) {
	transformer := newTransformer(nil, withOutputAudioFormat(dashscope.AudioFormatPCM16))
	if got := transformer.getOutputAudioMIMEType(); got != pcm.L16Mono24K.String() {
		t.Fatalf("getOutputAudioMIMEType() = %q, want %q", got, pcm.L16Mono24K.String())
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
