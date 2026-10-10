package peergenx

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
)

// inputTranscriber is an optional Generator capability. A Graph input stage
// needs the current utterance before it can build the actual reply request.
type inputTranscriber interface {
	TranscribeInput(context.Context, string, genx.ModelContext) (string, genx.Usage, error)
}

// BuildASR resolves the RuntimeProfile-selected external ASR stage. Empty alias
// means direct Model audio input and builds no ASR. Realtime requests streaming
// utterance boundaries; device audio already arrives at wall-clock cadence.
func (s *Service) BuildASR(ctx context.Context, alias string, mode apitypes.WorkspaceInputMode) (genx.Transformer, error) {
	if alias == "" {
		return nil, nil
	}
	if s == nil {
		return nil, ErrNotConfigured
	}
	if !mode.Valid() {
		return nil, fmt.Errorf("%w: ASR input mode %q", ErrInvalid, mode)
	}
	if err := runtimealias.Validate("ASR Model alias", alias); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	pattern := "model/" + alias + "?realtime_pacing=false"
	if mode == apitypes.WorkspaceInputModeRealtime {
		pattern += "&emit_interim=true&end_window_size=200&force_to_speech_time=1000"
	}
	config, err := s.ResolveTransformer(ctx, pattern)
	if err != nil {
		return nil, err
	}
	if config.Model == nil || config.Model.Kind != apitypes.ModelKindAsr {
		return nil, fmt.Errorf("%w: Profile ASR Model %q is not an ASR resource", ErrInvalid, alias)
	}
	transformer, err := s.builder().BuildTransformer(ctx, config)
	if err == nil && transformer == nil {
		return nil, fmt.Errorf("%w: Profile ASR Model %q returned no transformer", ErrInvalid, alias)
	}
	return transformer, err
}

// TranscribeInput obtains the current user audio's transcript from the selected
// LLM, applying the same resource resolution, quota and metering as Generator.
// The Model must support input transcription independently of reply generation.
func (s *Service) TranscribeInput(ctx context.Context, pattern string, input genx.ModelContext) (string, genx.Usage, error) {
	if s == nil {
		return "", genx.Usage{}, ErrNotConfigured
	}
	config, err := s.ResolveGenerator(ctx, pattern)
	if err != nil {
		return "", genx.Usage{}, err
	}
	impl, err := s.builder().BuildGenerator(ctx, config)
	if err != nil {
		return "", genx.Usage{}, err
	}
	input, err = modelContextForGenerator(config, input)
	if err != nil {
		return "", genx.Usage{}, err
	}
	return transcribeInput(ctx, impl, pattern, input)
}

func transcribeInput(ctx context.Context, generator genx.Generator, pattern string, input genx.ModelContext) (string, genx.Usage, error) {
	transcriber, ok := generator.(inputTranscriber)
	if !ok {
		return "", genx.Usage{}, fmt.Errorf("%w: Model %q cannot transcribe input", ErrUnsupported, pattern)
	}
	return transcriber.TranscribeInput(ctx, pattern, input)
}
