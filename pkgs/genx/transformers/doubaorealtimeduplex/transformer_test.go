package doubaorealtimeduplex

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
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	transcode := false
	speed := 1
	loudness := 2
	extension := &doubaospeech.RealtimeDuplexExtension{}
	transformer, err := New(Config{
		Client:          doubaospeech.NewClient(""),
		Speaker:         "speaker",
		Format:          "ogg_opus",
		SampleRate:      24000,
		InputFormat:     "speech_opus",
		InputSampleRate: 16000,
		InputChannels:   1,
		InputTranscode:  &transcode,
		Model:           "model",
		SessionID:       "session",
		Instructions:    "instructions",
		OutputSpeed:     &speed,
		OutputLoudness:  &loudness,
		Extension:       extension,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	transcode = true
	speed = 9
	if transformer.inputTranscode {
		t.Fatal("New() retained caller-owned InputTranscode pointer")
	}
	if transformer.outputSpeed == nil || *transformer.outputSpeed != 1 {
		t.Fatal("New() retained caller-owned OutputSpeed pointer")
	}
	if transformer.extension == extension {
		t.Fatal("New() retained caller-owned Extension pointer")
	}
	if transformer.outputVoice != "speaker" || transformer.outputFormat != "ogg_opus" ||
		transformer.outputSampleRate != 24000 || transformer.inputFormat != "speech_opus" ||
		transformer.inputSampleRate != 16000 || transformer.inputChannels != 1 ||
		transformer.model != "model" || transformer.sessionID != "session" ||
		transformer.instructions != "instructions" || transformer.outputLoudness == nil ||
		*transformer.outputLoudness != 2 {
		t.Fatalf("configured transformer = %#v", transformer)
	}
}
