package peergenx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type modelAliasResolver interface {
	ResolveModelAlias(string) (string, bool)
}

type voiceAliasResolver interface {
	ResolveVoiceAlias(string) (string, bool)
}

func (s *Service) Transcribe(ctx context.Context, modelAlias, language string, input genx.Stream) (string, error) {
	if s == nil || s.Models == nil {
		return "", ErrNotConfigured
	}
	if _, ok := s.Models.(modelAliasResolver); !ok {
		return "", fmt.Errorf("%w: model alias resolver", ErrNotConfigured)
	}
	pattern := "model/" + strings.TrimSpace(modelAlias)
	if language = strings.TrimSpace(language); language != "" {
		pattern += "?language=" + url.QueryEscape(language)
	}
	cfg, err := s.ResolveTransformer(ctx, pattern)
	if err != nil {
		return "", err
	}
	if cfg.Model == nil || cfg.Model.Kind != apitypes.ModelKindAsr {
		return "", fmt.Errorf("%w: model alias %q is not an ASR model", ErrInvalid, modelAlias)
	}
	transformer, err := s.builder().BuildTransformer(ctx, cfg)
	if err != nil {
		return "", err
	}
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		return "", err
	}
	if output == nil {
		return "", fmt.Errorf("%w: transcription output", ErrInvalid)
	}
	defer output.Close()
	var transcript strings.Builder
	for {
		chunk, err := output.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			transcript.WriteString(string(text))
		}
	}
	return transcript.String(), nil
}

type SpeechSynthesis struct {
	Stream       genx.Stream
	ContentType  string
	SampleRateHz *int32
	Channels     *int32
}

func (s *Service) Synthesize(ctx context.Context, voiceAlias, text string, acceptedContentTypes []string) (SpeechSynthesis, error) {
	if s == nil || s.Voices == nil {
		return SpeechSynthesis{}, ErrNotConfigured
	}
	if _, ok := s.Voices.(voiceAliasResolver); !ok {
		return SpeechSynthesis{}, fmt.Errorf("%w: voice alias resolver", ErrNotConfigured)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return SpeechSynthesis{}, fmt.Errorf("%w: text is required", ErrInvalid)
	}
	cfg, err := s.ResolveTransformer(ctx, "voice/"+strings.TrimSpace(voiceAlias))
	if err != nil {
		return SpeechSynthesis{}, err
	}
	format, contentType, raw, err := selectSpeechSynthesisFormat(cfg.Tenant.Kind, acceptedContentTypes)
	if err != nil {
		return SpeechSynthesis{}, err
	}
	sampleRate := int32(defaultTTSAudioSampleRate)
	if raw {
		sampleRate, err = speechSynthesisSampleRate(cfg)
		if err != nil {
			return SpeechSynthesis{}, err
		}
	}
	if cfg.Params == nil {
		cfg.Params = make(map[string]any)
	}
	cfg.Params["format"] = format
	transformer, err := s.builder().BuildTransformer(ctx, cfg)
	if err != nil {
		return SpeechSynthesis{}, err
	}
	stream, err := transformer.Transform(ctx, newTextStream(text))
	if err != nil {
		return SpeechSynthesis{}, err
	}
	result := SpeechSynthesis{Stream: stream, ContentType: contentType}
	if raw {
		channels := int32(1)
		result.SampleRateHz = &sampleRate
		result.Channels = &channels
	}
	return result, nil
}

func speechSynthesisSampleRate(cfg TransformerConfig) (int32, error) {
	if cfg.Voice == nil || cfg.Tenant.Kind != string(apitypes.VoiceProviderKindMinimaxTenant) || cfg.Voice.ProviderData == nil {
		return int32(defaultTTSAudioSampleRate), nil
	}
	providerData, err := cfg.Voice.ProviderData.AsMiniMaxTenantVoiceProviderData()
	if err != nil {
		return 0, fmt.Errorf("%w: decode minimax voice provider_data: %w", ErrInvalid, err)
	}
	if providerData.SampleRate == nil {
		return int32(defaultTTSAudioSampleRate), nil
	}
	sampleRate := int64(*providerData.SampleRate)
	if sampleRate <= 0 || sampleRate > int64(1<<31-1) {
		return 0, fmt.Errorf("%w: voice %q has invalid sample_rate %d", ErrInvalid, cfg.Voice.Id, *providerData.SampleRate)
	}
	return int32(sampleRate), nil
}

func selectSpeechSynthesisFormat(provider string, accepted []string) (format, contentType string, raw bool, err error) {
	supported := map[string]string{}
	switch provider {
	case string(apitypes.VoiceProviderKindVolcTenant):
		supported = map[string]string{"audio/ogg": "ogg_opus", "audio/mpeg": "mp3", "audio/pcm": "pcm"}
	case string(apitypes.VoiceProviderKindMinimaxTenant):
		supported = map[string]string{"audio/mpeg": "mp3", "audio/pcm": "pcm", "audio/flac": "flac", "audio/wav": "wav"}
	default:
		return "", "", false, fmt.Errorf("%w: speech synthesis provider %q", ErrUnsupported, provider)
	}
	for _, value := range accepted {
		mediaType, _, parseErr := mime.ParseMediaType(value)
		if parseErr != nil {
			continue
		}
		mediaType = strings.ToLower(mediaType)
		if selected, ok := supported[mediaType]; ok {
			return selected, mediaType, mediaType == "audio/pcm", nil
		}
	}
	return "", "", false, fmt.Errorf("%w: no accepted speech synthesis format", ErrUnsupported)
}
