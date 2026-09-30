package einoconfig

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const audioInputGraph = `{
	"graph": {
		"name": "audio-input",
		"compile": {"node_trigger_mode": "any_predecessor"},
		"state": {"fields": [
			{"name": "answer", "type": "string", "merge": "replace"},
			{"name": "summary", "type": "string", "merge": "replace"}
		]},
		"nodes": [
			{"id": "summarize", "type": "chat_model", "inputs": {"messages": {"from": "history.messages"}}, "outputs": {"text": "summary"}, "model": "llm"},
			{"id": "answer", "type": "chat_model", "inputs": {"messages": {"from": "input.messages"}}, "outputs": {"text": "answer"}, "model": "audio-llm", "audio_transcript": true}
		],
		"edges": [{"from": "start", "to": "summarize"}, {"from": "summarize", "to": "answer"}, {"from": "answer", "to": "end"}],
		"branches": [],
		"outputs": [{"node": "answer", "field": "answer", "name": "assistant", "mime_type": "text/plain", "primary": true}]
	}
}`

func TestValidateAcceptsBothAudioInputDeclarations(t *testing.T) {
	t.Parallel()
	asr, voice := "asr", "speech.voice"
	spec := decodeSpec(t, audioInputGraph)
	for _, adapter := range []*apitypes.VoiceAdapter{
		nil,
		{DefaultVoice: &voice},
		{AsrModel: &asr},
		{AsrModel: &asr, DefaultVoice: &voice},
	} {
		candidate := cloneSpec(t, spec)
		candidate.VoiceAdapter = adapter
		if err := Validate(candidate); err != nil {
			t.Fatalf("Validate(voice adapter %#v) error = %v", adapter, err)
		}
	}

	duplicate := decodeSpec(t, strings.Replace(audioInputGraph, `"model": "llm"`, `"model": "llm", "audio_transcript": true`, 1))
	if err := Validate(duplicate); err == nil || !strings.Contains(err.Error(), "already set") {
		t.Fatalf("Validate(two audio_transcript nodes) error = %v", err)
	}
}

func TestWorkflowAudioInput(t *testing.T) {
	t.Parallel()
	asr, blank := " asr ", "  "
	spec := decodeSpec(t, audioInputGraph)
	spec.VoiceAdapter = &apitypes.VoiceAdapter{AsrModel: &asr}
	support, err := WorkflowAudioInput(spec)
	if err != nil {
		t.Fatalf("WorkflowAudioInput() error = %v", err)
	}
	if want := (AudioInputSupport{ASRModel: "asr", TranscriptNode: "answer", TranscriptModel: "audio-llm"}); support != want {
		t.Fatalf("WorkflowAudioInput() = %#v, want %#v", support, want)
	}

	textOnly := decodeSpec(t, strings.Replace(audioInputGraph, `, "audio_transcript": true`, "", 1))
	textOnly.VoiceAdapter = &apitypes.VoiceAdapter{AsrModel: &blank}
	support, err = WorkflowAudioInput(textOnly)
	if err != nil || support != (AudioInputSupport{}) {
		t.Fatalf("WorkflowAudioInput(text only) = %#v, %v", support, err)
	}
}

func TestResolveAudioInput(t *testing.T) {
	t.Parallel()
	asr, model := apitypes.AudioInputPathAsr, apitypes.AudioInputPathModel
	both := AudioInputSupport{ASRModel: "asr", TranscriptNode: "answer", TranscriptModel: "audio-llm"}
	asrOnly := AudioInputSupport{ASRModel: "asr"}
	nodeOnly := AudioInputSupport{TranscriptNode: "answer", TranscriptModel: "audio-llm"}
	for _, test := range []struct {
		name     string
		support  AudioInputSupport
		selected *apitypes.AudioInputPath
		realtime bool
		capable  bool
		want     apitypes.AudioInputPath
		wantErr  string
	}{
		// Workflow defaults, no selection.
		{name: "default both declared uses asr", support: both, capable: true, want: asr},
		{name: "default asr only", support: asrOnly, want: asr},
		{name: "default node only capable", support: nodeOnly, capable: true, want: model},
		{name: "default node only text model", support: nodeOnly, wantErr: `node "answer" model "audio-llm" does not accept audio input`},
		{name: "default text only", support: AudioInputSupport{}, want: ""},
		{name: "default text only realtime", support: AudioInputSupport{}, realtime: true, want: ""},

		// model preferred.
		{name: "model capable", support: both, selected: &model, capable: true, want: model},
		{name: "model text model falls back to asr", support: both, selected: &model, want: asr},
		{name: "model without node falls back to asr", support: asrOnly, selected: &model, capable: true, want: asr},
		{name: "model node only capable", support: nodeOnly, selected: &model, capable: true, want: model},
		{name: "model node only text model", support: nodeOnly, selected: &model, wantErr: "has no voice_adapter.asr_model"},
		{name: "model on text-only workflow is ignored", support: AudioInputSupport{}, selected: &model, capable: true, want: ""},

		// asr preferred.
		{name: "asr both declared", support: both, selected: &asr, capable: true, want: asr},
		{name: "asr only", support: asrOnly, selected: &asr, want: asr},
		{name: "asr without asr_model falls back to model", support: nodeOnly, selected: &asr, capable: true, want: model},
		{name: "asr node only text model", support: nodeOnly, selected: &asr, wantErr: "does not accept audio input"},
		{name: "asr on text-only workflow is ignored", support: AudioInputSupport{}, selected: &asr, want: ""},

		// Realtime input keeps the ASR stage whatever is selected.
		{name: "realtime default", support: both, realtime: true, capable: true, want: asr},
		{name: "realtime model preferred", support: both, selected: &model, realtime: true, capable: true, want: asr},
		{name: "realtime asr preferred", support: both, selected: &asr, realtime: true, want: asr},
		{name: "realtime asr only model preferred", support: asrOnly, selected: &model, realtime: true, want: asr},
		{name: "realtime node only", support: nodeOnly, realtime: true, capable: true, wantErr: "realtime input requires voice_adapter.asr_model"},
		{name: "realtime node only model preferred", support: nodeOnly, selected: &model, realtime: true, capable: true, wantErr: "realtime input requires voice_adapter.asr_model"},
		{name: "realtime text-only workflow is ignored", support: AudioInputSupport{}, selected: &model, realtime: true, want: ""},

		{name: "unknown selection", support: both, selected: new(apitypes.AudioInputPath("direct")), wantErr: `unsupported audio_input "direct"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveAudioInput(AudioInputRequest{
				Support: test.support, Selected: test.selected,
				Realtime: test.realtime, ModelAcceptsAudio: test.capable,
			})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ResolveAudioInput() = %q, %v; want error containing %q", got, err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("ResolveAudioInput() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestAudioInputSupportValidateSelection(t *testing.T) {
	t.Parallel()
	both := AudioInputSupport{ASRModel: "asr", TranscriptNode: "answer", TranscriptModel: "audio-llm"}
	for _, path := range []apitypes.AudioInputPath{apitypes.AudioInputPathAsr, apitypes.AudioInputPathModel} {
		if err := both.ValidateSelection(path); err != nil {
			t.Fatalf("ValidateSelection(%s) error = %v", path, err)
		}
		if err := (AudioInputSupport{}).ValidateSelection(path); err == nil {
			t.Fatalf("ValidateSelection(%s) accepted a text-only Workflow", path)
		}
	}
	if err := both.ValidateSelection("direct"); err == nil {
		t.Fatal("ValidateSelection accepted an unknown path")
	}
}
