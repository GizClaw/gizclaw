package doubaoast

import (
	"fmt"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

// InputMode controls how input boundaries are sent to Doubao AST.
type InputMode string

const (
	// InputModeRealtime continuously streams input audio.
	InputModeRealtime InputMode = "realtime"
	// InputModePushToTalk treats each input stream boundary as one utterance.
	InputModePushToTalk InputMode = "push-to-talk"

	defaultLanguage = "zhen"
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
	if config.InputMode == "" {
		config.InputMode = InputModeRealtime
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
	if config.InputMode != "" {
		opts = append(opts, withInputMode(config.InputMode))
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
