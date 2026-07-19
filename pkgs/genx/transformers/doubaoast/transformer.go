package doubaoast

import (
	"context"
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	legacy "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/agentkit"
)

// InputMode controls how input boundaries are sent to Doubao AST.
type InputMode string

const (
	// InputModeRealtime continuously streams input audio.
	InputModeRealtime InputMode = "realtime"
	// InputModePushToTalk treats each input stream boundary as one utterance.
	InputModePushToTalk InputMode = "push-to-talk"
)

// Config contains immutable Doubao AST dependencies and session options.
type Config struct {
	Client               *doubaospeech.Client
	ResourceID           string
	Mode                 doubaospeech.ASTTranslateMode
	InputMode            InputMode
	SourceLanguage       string
	TargetLanguage       string
	SpeakerID            string
	CustomSpeaker        bool
	TTSResourceID        string
	SpeechRate           int
	SourceLanguageDetect bool
	Denoise              *bool
	RealtimePacing       *bool
}

// Transformer adapts Doubao AST to the GenX stream-to-stream contract.
type Transformer struct {
	config      Config
	newDelegate func() genx.Transformer
}

var _ genx.Transformer = (*Transformer)(nil)

// New constructs a Doubao AST transformer without opening a provider session.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubao ast: client is required")
	}
	config.Denoise = cloneBool(config.Denoise)
	config.RealtimePacing = cloneBool(config.RealtimePacing)
	return &Transformer{config: config}, nil
}

func (t *Transformer) delegate() genx.Transformer {
	if t.newDelegate != nil {
		return t.newDelegate()
	}
	config := t.config
	opts := []legacy.DoubaoASTTranslateOption{
		legacy.WithDoubaoASTTranslateResourceID(config.ResourceID),
		legacy.WithDoubaoASTTranslateMode(config.Mode),
		legacy.WithDoubaoASTTranslateSourceLanguage(config.SourceLanguage),
		legacy.WithDoubaoASTTranslateTargetLanguage(config.TargetLanguage),
		legacy.WithDoubaoASTTranslateSpeakerID(config.SpeakerID),
		legacy.WithDoubaoASTTranslateCustomSpeaker(config.CustomSpeaker),
		legacy.WithDoubaoASTTranslateTTSResourceID(config.TTSResourceID),
		legacy.WithDoubaoASTTranslateSpeechRate(config.SpeechRate),
		legacy.WithDoubaoASTTranslateSourceLanguageDetect(config.SourceLanguageDetect),
	}
	if config.InputMode != "" {
		opts = append(opts, legacy.WithDoubaoASTTranslateInputMode(legacy.DoubaoASTTranslateInputMode(config.InputMode)))
	}
	if config.Denoise != nil {
		opts = append(opts, legacy.WithDoubaoASTTranslateDenoise(*config.Denoise))
	}
	if config.RealtimePacing != nil {
		opts = append(opts, legacy.WithDoubaoASTTranslateRealtimePacing(*config.RealtimePacing))
	}
	return legacy.NewDoubaoASTTranslate(config.Client, opts...)
}

// Transform starts one independent Doubao AST invocation.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.config.Client == nil && t.newDelegate == nil {
		return nil, fmt.Errorf("doubao ast: transformer is not initialized")
	}
	output, err := t.delegate().Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return agentkit.NewResponseStream(output)
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
