package doubaoast

import (
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

func TestNew(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New(Config{}) succeeded without a client")
	}
	transformer, err := New(Config{Client: doubaospeech.NewClient("")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer == nil {
		t.Fatal("New() returned nil")
	}
	if transformer.resourceID != doubaospeech.ResourceASTTranslate ||
		transformer.mode != doubaospeech.ASTTranslateModeS2T ||
		transformer.inputMode != InputModeRealtime ||
		transformer.sourceLanguage != defaultLanguage ||
		transformer.targetLanguage != defaultLanguage {
		t.Fatalf("minimal Config defaults = %#v", transformer)
	}
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	denoise := true
	pacing := false
	transformer, err := New(Config{
		Client:               doubaospeech.NewClient(""),
		ResourceID:           "resource",
		Mode:                 doubaospeech.ASTTranslateModeS2S,
		InputMode:            InputModePushToTalk,
		SourceLanguage:       "zh",
		TargetLanguage:       "en",
		SpeakerID:            "speaker",
		CustomSpeaker:        true,
		TTSResourceID:        "tts-resource",
		SpeechRate:           10,
		SourceLanguageDetect: true,
		Denoise:              &denoise,
		RealtimePacing:       &pacing,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	denoise = false
	pacing = true
	if transformer.denoise == nil || !*transformer.denoise {
		t.Fatal("New() retained caller-owned Denoise pointer")
	}
	if transformer.realtimePacing {
		t.Fatal("New() retained caller-owned RealtimePacing pointer")
	}
	if transformer.resourceID != "resource" || transformer.mode != doubaospeech.ASTTranslateModeS2S ||
		transformer.inputMode != InputModePushToTalk || transformer.sourceLanguage != "zh" ||
		transformer.targetLanguage != "en" || transformer.speakerID != "speaker" ||
		!transformer.isCustomSpeaker || transformer.ttsResourceID != "tts-resource" ||
		transformer.speechRate != 10 || !transformer.enableSourceLanguageDetect {
		t.Fatalf("configured transformer = %#v", transformer)
	}
}
