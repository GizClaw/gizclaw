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

func TestTransformerOwnsEveryGeneratedRouteLifecycle(t *testing.T) {
	session := newDashScopeToolSession([]*dashscope.RealtimeEvent{
		{Type: dashscope.EventTypeResponseCreated, ResponseID: "response-1"},
		{Type: dashscope.EventTypeInputAudioTranscriptionCompleted, Transcript: "heard"},
		{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "response-1", Delta: "answer"},
		{Type: dashscope.EventTypeResponseTextDone, ResponseID: "response-1"},
		{Type: dashscope.EventTypeResponseTranscriptDelta, ResponseID: "response-1", Delta: "spoken"},
		{Type: dashscope.EventTypeResponseTranscriptDone, ResponseID: "response-1"},
		{Type: dashscope.EventTypeResponseAudioDelta, ResponseID: "response-1", Audio: []byte{1, 2}},
		{Type: dashscope.EventTypeResponseAudioDone, ResponseID: "response-1"},
	})
	transformer := newTransformer(nil)
	transformer.realtime = &dashScopeFixedOpener{session: session}
	output, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks, err := collectDashScopeToolOutput(output)
	if err != nil {
		t.Fatalf("collect output: %v", err)
	}

	routes := make(map[string][]*genx.MessageChunk)
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		key := chunk.Ctrl.StreamID + "\x00" + mimeType
		routes[key] = append(routes[key], chunk)
	}
	if len(routes) != 4 {
		t.Fatalf("generated MIME routes = %d, want input text, response text, response transcript, and response audio: %#v", len(routes), chunks)
	}
	for key, route := range routes {
		if route[0].Ctrl.StreamID == "" {
			t.Fatalf("route %q has empty StreamID: %#v", key, route)
		}
		if len(route) != 3 || !route[0].IsBeginOfStream() || route[1].IsBeginOfStream() || route[1].IsEndOfStream() || !route[2].IsEndOfStream() {
			t.Fatalf("route %q lifecycle = %#v, want BOS/data/EOS", key, route)
		}
	}
}

func TestTransformerCreatesCompleteEmptyTranscriptLifecycle(t *testing.T) {
	session := newDashScopeToolSession([]*dashscope.RealtimeEvent{
		{Type: dashscope.EventTypeInputAudioTranscriptionCompleted},
	})
	transformer := newTransformer(nil)
	transformer.realtime = &dashScopeFixedOpener{session: session}
	output, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks, err := collectDashScopeToolOutput(output)
	if err != nil {
		t.Fatalf("collect output: %v", err)
	}
	if len(chunks) != 2 || chunks[0].Ctrl == nil || chunks[0].Ctrl.StreamID == "" ||
		chunks[1].Ctrl == nil || chunks[1].Ctrl.StreamID != chunks[0].Ctrl.StreamID ||
		!chunks[0].IsBeginOfStream() || chunks[0].IsEndOfStream() ||
		chunks[1].IsBeginOfStream() || !chunks[1].IsEndOfStream() {
		t.Fatalf("empty transcript lifecycle = %#v, want non-empty StreamID BOS/EOS", chunks)
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
	mu                   sync.Mutex
	eventCalls           int
	events               []*dashscope.RealtimeEvent
	eventsDrained        chan struct{}
	eventsDrainOnce      sync.Once
	updateConfig         *dashscope.SessionConfig
	submitted            []dashScopeSubmittedToolResult
	submitErr            error
	createErr            error
	responseCreates      int
	pendingToolResponses int
	toolResponseCreates  int
}

func (s *fakeDashScopeSession) UpdateSession(config *dashscope.SessionConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *config
	if config.Tools != nil {
		clone.Tools = append(make([]dashscope.FunctionTool, 0, len(config.Tools)), config.Tools...)
	}
	s.updateConfig = &clone
	return nil
}
func (s *fakeDashScopeSession) AppendAudio([]byte) error { return nil }
func (s *fakeDashScopeSession) CommitInput() error       { return nil }
func (s *fakeDashScopeSession) ClearInput() error        { return nil }
func (s *fakeDashScopeSession) CreateResponse(*dashscope.ResponseCreateOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responseCreates++
	if s.pendingToolResponses > 0 {
		s.pendingToolResponses--
		s.toolResponseCreates++
	}
	return s.createErr
}
func (s *fakeDashScopeSession) SubmitFunctionCallOutput(callID, output string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.submitErr != nil {
		return s.submitErr
	}
	s.submitted = append(s.submitted, dashScopeSubmittedToolResult{callID: callID, output: output})
	s.pendingToolResponses++
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
			return
		}
		defer func() {
			if s.eventsDrained != nil {
				s.eventsDrainOnce.Do(func() {
					close(s.eventsDrained)
				})
			}
		}()
		for _, event := range s.events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

type dashScopeSubmittedToolResult struct {
	callID string
	output string
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
	if transformer.model != dashscope.ModelQwenOmniTurboRealtimeLatest {
		t.Fatalf("model without tools = %q, want legacy default", transformer.model)
	}
	if transformer.voice != dashscope.VoiceChelsie {
		t.Fatalf("voice without tools = %q, want legacy default", transformer.voice)
	}
	if _, err := New(Config{
		Client:       dashscope.NewClient(""),
		MaxToolCalls: -1,
	}); err == nil {
		t.Fatal("New() succeeded with negative MaxToolCalls")
	}
	invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
	withTools, err := New(Config{
		Client:      dashscope.NewClient(""),
		ToolInvoker: invoker,
	})
	if err != nil {
		t.Fatalf("New() with default tool model error = %v", err)
	}
	if withTools.model != dashscope.ModelQwen35OmniFlashRealtime {
		t.Fatalf("default tool model = %q", withTools.model)
	}
	if withTools.voice != dashScopeQwen35DefaultVoice {
		t.Fatalf("default tool voice = %q, want Tina", withTools.voice)
	}
	for _, model := range []string{
		dashscope.ModelQwen35OmniPlusRealtime,
		dashscope.ModelQwen35OmniPlusRealtime20260315,
		dashscope.ModelQwen35OmniFlashRealtime,
		dashscope.ModelQwen35OmniFlashRealtime20260315,
	} {
		transformer, err := New(Config{
			Client: dashscope.NewClient(""),
			Model:  model,
		})
		if err != nil {
			t.Fatalf("New() Qwen 3.5 model %q error = %v", model, err)
		}
		if transformer.voice != dashScopeQwen35DefaultVoice {
			t.Fatalf("Qwen 3.5 model %q default voice = %q, want Tina", model, transformer.voice)
		}
	}
	for _, model := range []string{
		dashscope.ModelQwenOmniTurboRealtimeLatest,
		dashscope.ModelQwen3OmniFlashRealtimeLatest,
	} {
		if _, err := New(Config{
			Client:      dashscope.NewClient(""),
			Model:       model,
			ToolInvoker: invoker,
		}); err == nil || !strings.Contains(err.Error(), "does not support function calling") {
			t.Fatalf("New() unsupported tool model %q error = %v", model, err)
		}
	}
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	temperature := 0.5
	maxTokens := 10
	enableASR := false
	modalities := []string{"text", "audio"}
	turnDetection := &dashscope.TurnDetection{Type: "server_vad"}
	invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
	transformer, err := New(Config{
		Client:            dashscope.NewClient(""),
		Model:             dashscope.ModelQwen35OmniPlusRealtime,
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
		ToolInvoker:       invoker,
		MaxToolCalls:      7,
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
	if transformer.model != dashscope.ModelQwen35OmniPlusRealtime || transformer.voice != "voice" ||
		transformer.instructions != "instructions" || transformer.vadType != "server_vad" ||
		transformer.maxOutputTokens == nil || *transformer.maxOutputTokens != 10 ||
		transformer.enableInputAudioTranscription || transformer.inputAudioTranscriptionModel != "asr-model" ||
		transformer.inputAudioFormat != "pcm16" || transformer.outputAudioFormat != "pcm16" ||
		transformer.toolInvoker != invoker || transformer.maxToolCalls != 7 {
		t.Fatalf("configured transformer = %#v", transformer)
	}
}
