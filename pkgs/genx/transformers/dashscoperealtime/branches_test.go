package dashscoperealtime

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
)

func TestDashScopeStreamIDsFallbackRoutes(t *testing.T) {
	var ids dashScopeStreamIDs
	ids.pushInput("")

	inputID, responseID := ids.bindTranscription()
	if inputID == "" || responseID == "" || inputID == responseID {
		t.Fatalf("generated transcription routes = (%q, %q)", inputID, responseID)
	}
	boundInputID, boundResponseID := ids.bindResponse("provider-1")
	if boundInputID != inputID || boundResponseID != responseID {
		t.Fatalf("response routes = (%q, %q), want (%q, %q)", boundInputID, boundResponseID, inputID, responseID)
	}
	if repeatedInputID, repeatedResponseID := ids.bindResponse("provider-1"); repeatedInputID != inputID || repeatedResponseID != responseID {
		t.Fatalf("repeated response routes = (%q, %q)", repeatedInputID, repeatedResponseID)
	}
	if got := ids.response(""); got != responseID {
		t.Fatalf("current response route = %q, want %q", got, responseID)
	}

	transcriptID := ids.responseTranscript("")
	if transcriptID == "" || transcriptID == responseID {
		t.Fatalf("transcript route = %q, response = %q", transcriptID, responseID)
	}
	if got := ids.responseTranscript("provider-1"); got != transcriptID {
		t.Fatalf("provider transcript route = %q, want %q", got, transcriptID)
	}

	var unbound dashScopeStreamIDs
	first := unbound.response("provider-2")
	if first == "" || unbound.response("provider-2") != first {
		t.Fatalf("unbound provider response route was not stable: %q", first)
	}
	_, second := unbound.bindResponse("")
	if second == "" || second == first {
		t.Fatalf("fallback response route = %q, first = %q", second, first)
	}
}

func TestDashScopeResponseIDSources(t *testing.T) {
	tests := []struct {
		name  string
		event *dashscope.RealtimeEvent
		want  string
	}{
		{name: "direct", event: &dashscope.RealtimeEvent{ResponseID: "direct", Response: &dashscope.ResponseInfo{ID: "nested"}}, want: "direct"},
		{name: "nested", event: &dashscope.RealtimeEvent{Response: &dashscope.ResponseInfo{ID: "nested"}}, want: "nested"},
		{name: "empty", event: &dashscope.RealtimeEvent{}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dashScopeResponseID(test.event); got != test.want {
				t.Fatalf("dashScopeResponseID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDashScopeOutputAudioMIMETypes(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{format: dashscope.AudioFormatMP3, want: "audio/mpeg"},
		{format: dashscope.AudioFormatWAV, want: "audio/wav"},
		{format: dashscope.AudioFormatPCM16, want: pcm.L16Mono24K.String()},
		{want: pcm.L16Mono24K.String()},
	}
	for _, test := range tests {
		transformer := newTransformer(nil, withOutputAudioFormat(test.format))
		if got := transformer.getOutputAudioMIMEType(); got != test.want {
			t.Errorf("format %q MIME = %q, want %q", test.format, got, test.want)
		}
	}
}

func TestDashScopeStreamControls(t *testing.T) {
	session := &dashScopeBranchSession{}
	stream := &Stream{session: session}
	voice := "Cherry"
	instructions := "help"
	inputFormat := dashscope.AudioFormatWAV
	outputFormat := dashscope.AudioFormatMP3
	turnDetection := &dashscope.TurnDetection{Type: "server_vad", Threshold: 0.6}
	if err := stream.Update(&UpdateRequest{
		Voice:             &voice,
		Instructions:      &instructions,
		Modalities:        []string{dashscope.ModalityText},
		InputAudioFormat:  &inputFormat,
		OutputAudioFormat: &outputFormat,
		TurnDetection:     turnDetection,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	config := session.lastUpdate()
	if config == nil || config.Voice != voice || config.Instructions != instructions ||
		len(config.Modalities) != 1 || config.Modalities[0] != dashscope.ModalityText ||
		config.InputAudioFormat != inputFormat || config.OutputAudioFormat != outputFormat ||
		config.TurnDetection != turnDetection {
		t.Fatalf("Update() config = %#v", config)
	}
	if err := stream.Update(&UpdateRequest{}); err != nil {
		t.Fatalf("empty Update() error = %v", err)
	}
	if config := session.lastUpdate(); config == nil || config.Voice != "" || config.TurnDetection != nil {
		t.Fatalf("empty Update() config = %#v", config)
	}

	if err := stream.CancelResponse(); err != nil {
		t.Fatalf("CancelResponse() error = %v", err)
	}
	if err := stream.ClearAudioBuffer(); err != nil {
		t.Fatalf("ClearAudioBuffer() error = %v", err)
	}
	if err := stream.TriggerResponse(); err != nil {
		t.Fatalf("TriggerResponse() error = %v", err)
	}
	if session.cancelCalls != 1 || session.clearCalls != 1 || session.commitCalls != 1 || session.createCalls != 1 {
		t.Fatalf("control calls = cancel:%d clear:%d commit:%d create:%d", session.cancelCalls, session.clearCalls, session.commitCalls, session.createCalls)
	}
}

func TestDashScopeStreamControlErrors(t *testing.T) {
	wantErr := errors.New("provider control failed")
	tests := []struct {
		name    string
		session *dashScopeBranchSession
		call    func(*Stream) error
	}{
		{name: "update", session: &dashScopeBranchSession{updateErr: wantErr}, call: func(stream *Stream) error { return stream.Update(&UpdateRequest{}) }},
		{name: "cancel", session: &dashScopeBranchSession{cancelErr: wantErr}, call: (*Stream).CancelResponse},
		{name: "clear", session: &dashScopeBranchSession{clearErr: wantErr}, call: (*Stream).ClearAudioBuffer},
		{name: "commit", session: &dashScopeBranchSession{commitErr: wantErr}, call: (*Stream).TriggerResponse},
		{name: "create", session: &dashScopeBranchSession{createErr: wantErr}, call: (*Stream).TriggerResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(&Stream{session: test.session}); !errors.Is(err, wantErr) {
				t.Fatalf("control error = %v, want %v", err, wantErr)
			}
			if test.name == "commit" && test.session.createCalls != 0 {
				t.Fatalf("CreateResponse() calls = %d after commit failure", test.session.createCalls)
			}
		})
	}
}

func TestDashScopeTransformSetupErrors(t *testing.T) {
	wantErr := errors.New("setup failed")
	tests := []struct {
		name        string
		transformer *Transformer
		input       genx.Stream
		want        string
	}{
		{name: "nil transformer", transformer: nil, input: emptyDashScopeStream{}, want: "not initialized"},
		{name: "missing realtime", transformer: &Transformer{}, input: emptyDashScopeStream{}, want: "not initialized"},
		{name: "nil input", transformer: newTransformer(nil), want: "input stream is required"},
		{name: "connect", transformer: dashScopeBranchTransformer(nil, wantErr), input: emptyDashScopeStream{}, want: "dashscope connect"},
		{name: "session event", transformer: dashScopeBranchTransformer(&dashScopeBranchSession{eventRuns: [][]dashScopeBranchEvent{{{err: wantErr}}}}, nil), input: emptyDashScopeStream{}, want: "wait session"},
		{name: "missing session created", transformer: dashScopeBranchTransformer(&dashScopeBranchSession{eventRuns: [][]dashScopeBranchEvent{{}}}, nil), input: emptyDashScopeStream{}, want: "session.created not received"},
		{name: "update", transformer: dashScopeBranchTransformer(&dashScopeBranchSession{updateErr: wantErr, eventRuns: [][]dashScopeBranchEvent{{{event: &dashscope.RealtimeEvent{Type: dashscope.EventTypeSessionCreated}}}}}, nil), input: emptyDashScopeStream{}, want: "update session"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream, err := test.transformer.Transform(t.Context(), test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Transform() = (%#v, %v), want error containing %q", stream, err, test.want)
			}
		})
	}
}

func TestDashScopeTransformBuildsVADConfiguration(t *testing.T) {
	session := &dashScopeBranchSession{eventRuns: [][]dashScopeBranchEvent{
		{{event: &dashscope.RealtimeEvent{Type: dashscope.EventTypeSessionCreated}}},
		{},
	}}
	transformer := newTransformer(nil, withVAD("server_vad"))
	transformer.realtime = &dashScopeBranchOpener{session: session}
	stream, err := transformer.Transform(t.Context(), emptyDashScopeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer stream.Close()
	config := session.lastUpdate()
	if config == nil || config.TurnDetection == nil || config.TurnDetection.Type != "server_vad" {
		t.Fatalf("session config = %#v", config)
	}
}

func TestPrepareInputAudioBranches(t *testing.T) {
	transformer := newTransformer(nil)
	inputs := doubaorealtime.NewSharedAudioInputs("pcm", 16000, 1, true)
	defer inputs.Close()

	raw := &genx.Blob{MIMEType: "audio/L16", Data: []byte{1, 2}}
	data, err := transformer.prepareInputAudio(inputs, &genx.MessageChunk{Part: raw}, raw)
	if err != nil || string(data) != string(raw.Data) {
		t.Fatalf("raw input = (%v, %v)", data, err)
	}

	transformer.inputAudioFormat = dashscope.AudioFormatMP3
	opusBlob := &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}}
	if _, err := transformer.prepareInputAudio(inputs, &genx.MessageChunk{Part: opusBlob}, opusBlob); err == nil || !strings.Contains(err.Error(), "cannot accept peer Opus") {
		t.Fatalf("incompatible Opus error = %v", err)
	}

	transformer.inputAudioFormat = dashscope.AudioFormatPCM16
	if _, err := transformer.prepareInputAudio(inputs, &genx.MessageChunk{Part: opusBlob}, opusBlob); err == nil || !strings.Contains(err.Error(), "decode peer Opus") {
		t.Fatalf("invalid Opus error = %v", err)
	}
}

type dashScopeBranchEvent struct {
	event *dashscope.RealtimeEvent
	err   error
}

type dashScopeBranchSession struct {
	mu sync.Mutex

	eventRuns [][]dashScopeBranchEvent
	eventCall int
	update    *dashscope.SessionConfig

	updateErr error
	commitErr error
	clearErr  error
	createErr error
	cancelErr error

	commitCalls int
	clearCalls  int
	createCalls int
	cancelCalls int
	closeCalls  int
}

func (s *dashScopeBranchSession) UpdateSession(config *dashscope.SessionConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *config
	clone.Modalities = append([]string(nil), config.Modalities...)
	s.update = &clone
	return s.updateErr
}

func (s *dashScopeBranchSession) AppendAudio([]byte) error { return nil }

func (s *dashScopeBranchSession) CommitInput() error {
	s.commitCalls++
	return s.commitErr
}

func (s *dashScopeBranchSession) ClearInput() error {
	s.clearCalls++
	return s.clearErr
}

func (s *dashScopeBranchSession) CreateResponse(*dashscope.ResponseCreateOptions) error {
	s.createCalls++
	return s.createErr
}

func (s *dashScopeBranchSession) SubmitFunctionCallOutput(string, string) error { return nil }

func (s *dashScopeBranchSession) CancelResponse() error {
	s.cancelCalls++
	return s.cancelErr
}

func (s *dashScopeBranchSession) Events() iter.Seq2[*dashscope.RealtimeEvent, error] {
	s.mu.Lock()
	index := s.eventCall
	s.eventCall++
	var events []dashScopeBranchEvent
	if index < len(s.eventRuns) {
		events = append(events, s.eventRuns[index]...)
	}
	s.mu.Unlock()
	return func(yield func(*dashscope.RealtimeEvent, error) bool) {
		for _, result := range events {
			if !yield(result.event, result.err) {
				return
			}
		}
	}
}

func (s *dashScopeBranchSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalls++
	return nil
}

func (s *dashScopeBranchSession) lastUpdate() *dashscope.SessionConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.update
}

type dashScopeBranchOpener struct {
	session dashScopeRealtimeSession
	err     error
}

func (o *dashScopeBranchOpener) Connect(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
	return o.session, o.err
}

func dashScopeBranchTransformer(session dashScopeRealtimeSession, err error) *Transformer {
	transformer := newTransformer(nil)
	transformer.realtime = &dashScopeBranchOpener{session: session, err: err}
	return transformer
}
