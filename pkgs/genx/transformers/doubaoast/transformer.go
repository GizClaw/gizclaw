package doubaoast

import (
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

const defaultLanguage = "zhen"

// Config contains immutable Doubao AST dependencies and session options.
type Config struct {
	Client               *doubaospeech.Client
	ResourceID           string
	Mode                 doubaospeech.ASTTranslateMode
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

// New constructs a Doubao AST transformer without opening a provider session.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubao ast: client is required")
	}
	if config.ResourceID == "" {
		config.ResourceID = doubaospeech.ResourceASTTranslate
	}
	if config.Mode == "" {
		config.Mode = doubaospeech.ASTTranslateModeS2T
	}
	if config.SourceLanguage == "" {
		config.SourceLanguage = defaultLanguage
	}
	if config.TargetLanguage == "" {
		config.TargetLanguage = defaultLanguage
	}
	config.Denoise = cloneBool(config.Denoise)
	config.RealtimePacing = cloneBool(config.RealtimePacing)
	opts := []option{
		withResourceID(config.ResourceID),
		withMode(config.Mode),
		withSourceLanguage(config.SourceLanguage),
		withTargetLanguage(config.TargetLanguage),
		withSpeakerID(config.SpeakerID),
		withCustomSpeaker(config.CustomSpeaker),
		withTTSResourceID(config.TTSResourceID),
		withSpeechRate(config.SpeechRate),
		withSourceLanguageDetect(config.SourceLanguageDetect),
	}
	if config.Denoise != nil {
		opts = append(opts, withDenoise(*config.Denoise))
	}
	if config.RealtimePacing != nil {
		opts = append(opts, withRealtimePacing(*config.RealtimePacing))
	}
	return newTransformer(config.Client, opts...), nil
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
