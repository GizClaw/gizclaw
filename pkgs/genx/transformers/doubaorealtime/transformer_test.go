package doubaorealtime

import (
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

func TestNew(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New(Config{}) succeeded without a client")
	}
	if _, err := New(Config{Client: doubaospeech.NewClient("")}); err == nil {
		t.Fatal("New() succeeded without a model")
	}
	transformer, err := New(Config{Client: doubaospeech.NewClient(""), Model: "O"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer == nil {
		t.Fatal("New() returned nil")
	}
	if cfg := transformer.realtimeConfig().TTS.AudioConfig; cfg.Format != doubaospeech.FormatPCMS16LE || cfg.SampleRate != doubaospeech.SampleRate16000 || cfg.Channel != 1 {
		t.Fatalf("default TTS audio config = %#v, want 16 kHz mono PCM16", cfg)
	}
	if got := transformer.outputMIMEType(); got != "audio/L16; rate=16000; channels=1" {
		t.Fatalf("default output MIME = %q", got)
	}
}

func TestNewCopiesConfigAndBuildsConfiguredDelegate(t *testing.T) {
	speechRate := 1
	loudness := 2
	transcode := false
	asr := &doubaospeech.RealtimeASRExtra{}
	tts := &doubaospeech.RealtimeTTSExtra{}
	dialog := &doubaospeech.RealtimeDialogExtra{}
	transformer, err := New(Config{
		Client:            doubaospeech.NewClient(""),
		Speaker:           "speaker",
		Format:            "ogg_opus",
		SampleRate:        24000,
		Channels:          1,
		SpeechRate:        &speechRate,
		LoudnessRate:      &loudness,
		InputFormat:       "speech_opus",
		InputSampleRate:   16000,
		InputChannels:     1,
		InputTranscode:    &transcode,
		ASRExtra:          asr,
		TTSExtra:          tts,
		BotName:           "bot",
		Instructions:      "instruction",
		SystemRole:        "role",
		VADWindow:         200,
		SpeakingStyle:     "style",
		CharacterManifest: "character",
		DialogID:          "dialog",
		DialogExtra:       dialog,
		SearchAPIKey:      "search-key",
		Model:             "O",
		Mode:              ModeRealtime,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	speechRate = 9
	transcode = true
	if transformer.speechRate == nil || *transformer.speechRate != 1 {
		t.Fatal("New() retained caller-owned SpeechRate pointer")
	}
	if transformer.inputTranscode {
		t.Fatal("New() retained caller-owned InputTranscode pointer")
	}
	if transformer.asrExtra == asr || transformer.ttsExtra == tts || transformer.dialogExtra == dialog {
		t.Fatal("New() retained caller-owned provider config pointers")
	}
	if transformer.speaker != "speaker" || transformer.format != "ogg_opus" ||
		transformer.sampleRate != 24000 || transformer.channels != 1 ||
		transformer.inputFormat != "speech_opus" || transformer.inputSampleRate != 16000 ||
		transformer.inputChannels != 1 || transformer.botName != "bot" ||
		transformer.instructions != "instruction" || transformer.systemRole != "role" || transformer.vadWindowMs != 200 ||
		transformer.speakingStyle != "style" || transformer.characterManifest != "character" ||
		transformer.dialogID != "dialog" || transformer.model != "O" || transformer.mode != ModeRealtime {
		t.Fatalf("configured transformer = %#v", transformer)
	}
	if cfg := transformer.realtimeConfig().TTS.AudioConfig; cfg.Format != doubaospeech.FormatOGG || cfg.SampleRate != doubaospeech.SampleRate24000 {
		t.Fatalf("explicit TTS audio config = %#v, want Ogg/Opus at 24 kHz", cfg)
	}
}

func TestNewDoesNotInjectModelSpecificBotName(t *testing.T) {
	transformer, err := New(Config{Client: doubaospeech.NewClient(""), Model: "SC"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer.botName != "" || transformer.realtimeConfig().Dialog.BotName != "" {
		t.Fatalf("SC transformer bot name = %q, want empty", transformer.botName)
	}
}

func TestCloneJSONRejectsUnsupportedValue(t *testing.T) {
	value := make(chan int)
	if _, err := cloneJSON(&value); err == nil {
		t.Fatal("cloneJSON() succeeded for a channel")
	}
}
