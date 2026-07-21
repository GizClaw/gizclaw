package peergenx

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type SayRequest struct {
	Text       string
	VoiceAlias string
}

type SayResponse struct {
	Accepted bool
}

func (s *Service) Say(ctx context.Context, request SayRequest) (SayResponse, error) {
	if s == nil {
		return SayResponse{}, ErrNotConfigured
	}
	if s.AudioOutput == nil {
		return SayResponse{}, fmt.Errorf("%w: audio output is required", ErrNotConfigured)
	}
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return SayResponse{}, fmt.Errorf("%w: text is required", ErrInvalid)
	}
	pattern, err := request.transformerPattern()
	if err != nil {
		return SayResponse{}, err
	}
	output, err := s.Transformer().Transform(ctx, pattern, newTextStream(text))
	if err != nil {
		return SayResponse{}, err
	}
	if output != nil {
		defer output.Close()
	}
	if err := s.AudioOutput.ConsumeAgentOutput(ctx, output); err != nil {
		return SayResponse{}, err
	}
	return SayResponse{Accepted: true}, nil
}

func (r SayRequest) transformerPattern() (string, error) {
	if voiceAlias := strings.TrimSpace(r.VoiceAlias); voiceAlias != "" {
		return "voice/" + voiceAlias, nil
	}
	return "", fmt.Errorf("%w: voice_alias is required", ErrInvalid)
}

func newTextStream(text string) genx.Stream {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	_ = builder.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text)}, genx.NewTextEndOfStream())
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}
