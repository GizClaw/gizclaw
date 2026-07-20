package peergenx

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type aliasModels struct {
	fakeModels
	aliases map[string]string
}

func (m aliasModels) ResolveModelAlias(alias string) (string, bool) {
	value, ok := m.aliases[alias]
	return value, ok
}

type aliasVoices struct {
	fakeVoices
	aliases map[string]string
}

func (v aliasVoices) ResolveVoiceAlias(alias string) (string, bool) {
	value, ok := v.aliases[alias]
	return value, ok
}

func TestTranscribeResolvesASRAliasAndRejectsCanonicalID(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Models: aliasModels{
			fakeModels: fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: string(apitypes.ModelProviderKindVolcTenant)},
			aliases:    map[string]string{"asr-model": "canonical-asr"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	transcript, err := svc.Transcribe(context.Background(), "asr-model", "zh-CN", newTextStream("hello"))
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if transcript != "hello" {
		t.Fatalf("Transcribe() = %q", transcript)
	}
	wantPrefix := []string{"get:model:canonical-asr", "get:tenant:volc:main", "get:credential:volc-token"}
	if !reflect.DeepEqual(events[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("events = %#v, want prefix %#v", events, wantPrefix)
	}
	if _, err := svc.Transcribe(context.Background(), "canonical-asr", "", newTextStream("hello")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canonical Transcribe() error = %v, want %v", err, ErrNotFound)
	}
}

func TestTranscribeRejectsWrongModelKind(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Models: aliasModels{
			fakeModels: fakeModels{events: &events, modelKind: apitypes.ModelKindTts, providerKind: string(apitypes.ModelProviderKindVolcTenant)},
			aliases:    map[string]string{"wrong": "canonical-tts"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Transcribe(context.Background(), "wrong", "", newTextStream("audio")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Transcribe() error = %v, want %v", err, ErrInvalid)
	}
}

func TestSynthesizeResolvesVoiceAliasAndRejectsCanonicalID(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Voices: aliasVoices{
			fakeVoices: fakeVoices{events: &events},
			aliases:    map[string]string{"narrator": "canonical-voice"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	result, err := svc.Synthesize(context.Background(), "narrator", "hello", []string{"audio/ogg"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	_ = result.Stream.Close()
	if result.ContentType != "audio/ogg" || result.SampleRateHz != nil || result.Channels != nil {
		t.Fatalf("Synthesize() metadata = %+v", result)
	}
	if len(events) == 0 || events[0] != "get:voice:canonical-voice" {
		t.Fatalf("events = %#v", events)
	}
	if _, err := svc.Synthesize(context.Background(), "canonical-voice", "hello", []string{"audio/ogg"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canonical Synthesize() error = %v, want %v", err, ErrNotFound)
	}
}

func TestSynthesizeReportsConfiguredMiniMaxPCMSampleRate(t *testing.T) {
	events := []string{}
	sampleRate := 24000
	svc := New(Service{
		Voices: aliasVoices{
			fakeVoices: fakeVoices{
				events:       &events,
				providerKind: apitypes.VoiceProviderKindMinimaxTenant,
				sampleRate:   &sampleRate,
			},
			aliases: map[string]string{"narrator": "canonical-voice"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	result, err := svc.Synthesize(context.Background(), "narrator", "hello", []string{"audio/pcm"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	_ = result.Stream.Close()
	if result.SampleRateHz == nil || *result.SampleRateHz != int32(sampleRate) || result.Channels == nil || *result.Channels != 1 {
		t.Fatalf("Synthesize() metadata = %+v, want %d Hz mono", result, sampleRate)
	}
}

func TestSynthesizeRejectsInvalidMiniMaxPCMSampleRate(t *testing.T) {
	events := []string{}
	sampleRate := 0
	svc := New(Service{
		Voices: aliasVoices{
			fakeVoices: fakeVoices{
				events:       &events,
				providerKind: apitypes.VoiceProviderKindMinimaxTenant,
				sampleRate:   &sampleRate,
			},
			aliases: map[string]string{"narrator": "canonical-voice"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Synthesize(context.Background(), "narrator", "hello", []string{"audio/pcm"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Synthesize() error = %v, want %v", err, ErrInvalid)
	}
}

func TestSelectSpeechSynthesisFormatHonorsOrderedPreference(t *testing.T) {
	format, contentType, raw, err := selectSpeechSynthesisFormat(
		string(apitypes.VoiceProviderKindVolcTenant),
		[]string{"audio/mpeg", "audio/ogg"},
	)
	if err != nil || format != "mp3" || contentType != "audio/mpeg" || raw {
		t.Fatalf("selectSpeechSynthesisFormat() = (%q, %q, %t, %v)", format, contentType, raw, err)
	}

	format, contentType, raw, err = selectSpeechSynthesisFormat(
		string(apitypes.VoiceProviderKindMinimaxTenant),
		[]string{"audio/ogg", "audio/pcm"},
	)
	if err != nil || format != "pcm" || contentType != "audio/pcm" || !raw {
		t.Fatalf("selectSpeechSynthesisFormat(raw) = (%q, %q, %t, %v)", format, contentType, raw, err)
	}

	if _, _, _, err := selectSpeechSynthesisFormat(
		string(apitypes.VoiceProviderKindMinimaxTenant),
		[]string{"audio/ogg"},
	); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("selectSpeechSynthesisFormat(unsupported) error = %v, want %v", err, ErrUnsupported)
	}
}
