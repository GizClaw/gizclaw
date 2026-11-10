package peergenx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type transcriptionFailure struct{ cause error }

func (e *transcriptionFailure) Error() string { return "ASR startup failed: " + e.cause.Error() }
func (e *transcriptionFailure) Unwrap() error { return e.cause }
func (*transcriptionFailure) PublicError() (string, string, bool) {
	return "ASR_STREAM_ERROR", "ASR startup failed", true
}

type terminalTranscription struct{ err error }

func (s terminalTranscription) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	return s, nil
}
func (s terminalTranscription) Next() (*genx.MessageChunk, error) { return nil, s.err }
func (terminalTranscription) Close() error                        { return nil }
func (terminalTranscription) CloseWithError(error) error          { return nil }

func TestTranscribePreservesProviderFailureWrappingCompletion(t *testing.T) {
	for _, cause := range []error{io.EOF, genx.ErrDone} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/failure=%t", cause, failed), func(t *testing.T) {
				terminal := fmt.Errorf("stream: %w", cause)
				var failure *transcriptionFailure
				if failed {
					failure = &transcriptionFailure{terminal}
					terminal = fmt.Errorf("adapter: %w", failure)
				}
				events := []string{}
				service := New(Service{
					Models:      aliasModels{fakeModels: fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: string(apitypes.ModelProviderKindVolcTenant)}, aliases: map[string]string{"asr": "canonical-asr"}},
					Credentials: fakeCredentials{events: &events}, ProviderTenants: fakeTenants{events: &events},
					Builder: asrInputBuilder{build: func(TransformerConfig) (genx.Transformer, error) { return terminalTranscription{terminal}, nil }},
				})
				text, err := service.Transcribe(t.Context(), "asr", "", newTextStream("audio"))
				if failed {
					if !errors.Is(err, failure) || !errors.Is(err, cause) {
						t.Fatalf("Transcribe returned text=%q err=%v, lost provider failure", text, err)
					}
				} else if err != nil || text != "" {
					t.Fatalf("completion returned %q %v", text, err)
				}
			})
		}
	}
}
