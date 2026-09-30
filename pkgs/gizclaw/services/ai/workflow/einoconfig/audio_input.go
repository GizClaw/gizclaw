package einoconfig

import (
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// AudioInputSupport lists the live-audio paths one Eino Workflow declares.
type AudioInputSupport struct {
	// ASRModel is the voice_adapter.asr_model alias. Empty means the Workflow
	// declares no ASR stage.
	ASRModel string
	// TranscriptNode is the root chat_model node that sets audio_transcript
	// and TranscriptModel is its Model alias. Both are empty when the Graph
	// declares no such node.
	TranscriptNode  string
	TranscriptModel string
}

// WorkflowAudioInput returns the audio input paths public declares.
func WorkflowAudioInput(public apitypes.EinoWorkflowSpec) (AudioInputSupport, error) {
	var support AudioInputSupport
	if public.VoiceAdapter != nil {
		support.ASRModel = strings.TrimSpace(stringValue(public.VoiceAdapter.AsrModel))
	}
	for index, raw := range public.Graph.Nodes {
		discriminator, err := raw.Discriminator()
		if err != nil {
			return AudioInputSupport{}, fmt.Errorf("graph.nodes[%d]: %w", index, err)
		}
		if discriminator != "chat_model" {
			continue
		}
		node, err := raw.AsEinoChatModelNode()
		if err != nil {
			return AudioInputSupport{}, fmt.Errorf("graph.nodes[%d]: %w", index, err)
		}
		if boolValue(node.AudioTranscript) {
			support.TranscriptNode = strings.TrimSpace(node.Id)
			support.TranscriptModel = strings.TrimSpace(node.Model)
			break
		}
	}
	return support, nil
}

// ValidateSelection rejects a path the Workflow does not declare. RuntimeProfile
// bindings use it because a binding names one Workflow; a Workspace parameter
// is only a preference and goes through ResolveAudioInput instead.
func (support AudioInputSupport) ValidateSelection(selected apitypes.AudioInputPath) error {
	switch selected {
	case apitypes.AudioInputPathAsr:
		if support.ASRModel == "" {
			return fmt.Errorf("audio_input %q requires voice_adapter.asr_model", selected)
		}
	case apitypes.AudioInputPathModel:
		if support.TranscriptNode == "" {
			return fmt.Errorf("audio_input %q requires a chat_model node that sets audio_transcript", selected)
		}
	default:
		return fmt.Errorf("unsupported audio_input %q", selected)
	}
	return nil
}

// AudioInputRequest describes one Agent generation that needs an audio input
// path.
type AudioInputRequest struct {
	Support AudioInputSupport
	// Selected is the preferred path: the Workspace parameter, or else the
	// RuntimeProfile Workflow binding. Nil applies the Workflow default.
	Selected *apitypes.AudioInputPath
	// Realtime reports Workspace input realtime, whose utterance segmentation
	// needs the streaming ASR stage.
	Realtime bool
	// ModelAcceptsAudio reports whether the Model bound to
	// Support.TranscriptModel accepts user audio and reports its transcript.
	ModelAcceptsAudio bool
}

// PrefersModel reports whether the model path is tried before asr, which is
// when the capability of the audio_transcript node's Model decides the result.
func (request AudioInputRequest) PrefersModel() bool {
	if request.Selected != nil {
		return *request.Selected == apitypes.AudioInputPathModel
	}
	return request.Support.ASRModel == ""
}

// ResolveAudioInput returns the audio input path one Agent generation uses.
// An empty path means the Workflow declares no audio input and the Agent
// accepts text turns only, whatever is selected.
//
// The selection is a preference between the paths the Workflow declares.
// Without one the Workflow default applies: asr when the Workflow declares
// voice_adapter.asr_model, otherwise model. The preferred path is used when it
// can run and the other declared path otherwise. asr needs asr_model; model
// needs an audio_transcript node whose Model accepts audio and push-to-talk
// input, because realtime input is segmented by the ASR stage. It is an error
// when the Workflow declares audio input but no declared path can run.
func ResolveAudioInput(request AudioInputRequest) (apitypes.AudioInputPath, error) {
	support := request.Support
	if err := apitypes.ValidateAudioInputPath(request.Selected); err != nil {
		return "", err
	}
	asrReady := support.ASRModel != ""
	if !asrReady && support.TranscriptNode == "" {
		return "", nil
	}
	if request.Realtime {
		if !asrReady {
			return "", fmt.Errorf("realtime input requires voice_adapter.asr_model")
		}
		return apitypes.AudioInputPathAsr, nil
	}
	modelReady := support.TranscriptNode != "" && request.ModelAcceptsAudio
	switch {
	case modelReady && (request.PrefersModel() || !asrReady):
		return apitypes.AudioInputPathModel, nil
	case asrReady:
		return apitypes.AudioInputPathAsr, nil
	}
	return "", fmt.Errorf(
		"chat_model node %q model %q does not accept audio input and the Workflow has no voice_adapter.asr_model",
		support.TranscriptNode, support.TranscriptModel,
	)
}
