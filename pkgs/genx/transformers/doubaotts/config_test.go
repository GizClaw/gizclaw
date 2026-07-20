package doubaotts

import (
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

func TestSeedV2ConfigValidationAndDefaults(t *testing.T) {
	if _, err := NewSeedV2(SeedV2Config{Speaker: "voice"}); err == nil {
		t.Fatal("NewSeedV2() accepted a nil client")
	}
	client := doubaospeech.NewClient("app")
	if _, err := NewSeedV2(SeedV2Config{Client: client}); err == nil {
		t.Fatal("NewSeedV2() accepted an empty speaker")
	}
	transformer, err := NewSeedV2(SeedV2Config{Client: client, Speaker: " voice "})
	if err != nil {
		t.Fatalf("NewSeedV2() error = %v", err)
	}
	if transformer.speaker != "voice" || transformer.resourceID != doubaospeech.ResourceTTSV2 || transformer.format != "ogg_opus" || transformer.sampleRate != 24000 {
		t.Fatalf("Seed V2 defaults = %#v", transformer)
	}
	if transformer.speedRatio != 1 || transformer.volumeRatio != 1 || transformer.pitchRatio != 1 {
		t.Fatalf("Seed V2 ratios = %v/%v/%v", transformer.speedRatio, transformer.volumeRatio, transformer.pitchRatio)
	}
	configured, err := NewSeedV2(SeedV2Config{
		Client:      client,
		Speaker:     "voice",
		ResourceID:  "resource",
		Format:      "pcm",
		SampleRate:  16000,
		BitRate:     64000,
		SpeedRatio:  0.8,
		VolumeRatio: 1.2,
		PitchRatio:  0.9,
		Emotion:     "happy",
		Language:    "zh-CN",
	})
	if err != nil {
		t.Fatalf("NewSeedV2(custom) error = %v", err)
	}
	if configured.resourceID != "resource" || configured.format != "pcm" || configured.sampleRate != 16000 || configured.bitRate != 64000 || configured.speedRatio != 0.8 || configured.volumeRatio != 1.2 || configured.pitchRatio != 0.9 || configured.emotion != "happy" || configured.language != "zh-CN" {
		t.Fatalf("Seed V2 custom config = %#v", configured)
	}
}

func TestICLV2ConfigValidationAndDefaults(t *testing.T) {
	if _, err := NewICLV2(ICLV2Config{Speaker: "voice"}); err == nil {
		t.Fatal("NewICLV2() accepted a nil client")
	}
	client := doubaospeech.NewClient("app")
	if _, err := NewICLV2(ICLV2Config{Client: client}); err == nil {
		t.Fatal("NewICLV2() accepted an empty speaker")
	}
	transformer, err := NewICLV2(ICLV2Config{Client: client, Speaker: " clone "})
	if err != nil {
		t.Fatalf("NewICLV2() error = %v", err)
	}
	if transformer.speaker != "clone" || transformer.format != "ogg_opus" || transformer.sampleRate != 24000 {
		t.Fatalf("ICL V2 defaults = %#v", transformer)
	}
	if transformer.speedRatio != 1 || transformer.volumeRatio != 1 || transformer.pitchRatio != 1 {
		t.Fatalf("ICL V2 ratios = %v/%v/%v", transformer.speedRatio, transformer.volumeRatio, transformer.pitchRatio)
	}
	configured, err := NewICLV2(ICLV2Config{
		Client:      client,
		Speaker:     "clone",
		Format:      "pcm",
		SampleRate:  16000,
		BitRate:     64000,
		SpeedRatio:  0.8,
		VolumeRatio: 1.2,
		PitchRatio:  0.9,
		Emotion:     "happy",
		Language:    "zh-CN",
	})
	if err != nil {
		t.Fatalf("NewICLV2(custom) error = %v", err)
	}
	if configured.format != "pcm" || configured.sampleRate != 16000 || configured.bitRate != 64000 || configured.speedRatio != 0.8 || configured.volumeRatio != 1.2 || configured.pitchRatio != 0.9 || configured.emotion != "happy" || configured.language != "zh-CN" {
		t.Fatalf("ICL V2 custom config = %#v", configured)
	}
}
