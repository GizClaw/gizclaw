package peergenx

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

type SayRequest struct {
	Text           string
	VoiceID        string
	ModelID        string
	CredentialName string
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
	if strings.TrimSpace(request.CredentialName) != "" {
		return SayResponse{}, fmt.Errorf("%w: credential override is not supported", ErrUnsupported)
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
	if voiceID := strings.TrimSpace(r.VoiceID); voiceID != "" {
		return "voice/" + voiceID, nil
	}
	if modelID := strings.TrimSpace(r.ModelID); modelID != "" {
		return "", fmt.Errorf("%w: model_id %q is not supported for audio say", ErrUnsupported, modelID)
	}
	return "", fmt.Errorf("%w: voice_id is required", ErrInvalid)
}

func newTextStream(text string) genx.Stream {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	_ = builder.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text)}, genx.NewTextEndOfStream())
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}
