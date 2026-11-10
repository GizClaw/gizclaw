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

type routeTerminalTranscription struct {
	chunks []*genx.MessageChunk
	reads  int
	closed bool
}

func (s *routeTerminalTranscription) Transform(context.Context, genx.Stream) (genx.Stream, error) {
	return s, nil
}
func (s *routeTerminalTranscription) Next() (*genx.MessageChunk, error) {
	s.reads++
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}
func (s *routeTerminalTranscription) Close() error { s.closed = true; return nil }
func (s *routeTerminalTranscription) CloseWithError(error) error {
	return s.Close()
}

func TestTranscribePropagatesRouteTerminalBeforeOuterCompletion(t *testing.T) {
	for _, partial := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("partial=%t/failed=%t", partial, failed), func(t *testing.T) {
				prefix := ""
				if partial {
					prefix = "partial transcript"
				}
				cause := &transcriptionFailure{cause: fmt.Errorf("provider route: %w", io.EOF)}
				terminal := &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "asr", EndOfStream: true}}
				if failed {
					genx.SetStreamError(terminal.Ctrl, cause)
				}
				stream := &routeTerminalTranscription{chunks: []*genx.MessageChunk{
					nil,
					{Part: genx.Text(prefix), Ctrl: &genx.StreamCtrl{StreamID: "asr"}},
					terminal,
					{Part: genx.Text("tail")},
				}}
				events := []string{}
				service := New(Service{
					Models:      aliasModels{fakeModels: fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: string(apitypes.ModelProviderKindVolcTenant)}, aliases: map[string]string{"asr": "canonical-asr"}},
					Credentials: fakeCredentials{events: &events}, ProviderTenants: fakeTenants{events: &events},
					Builder: asrInputBuilder{build: func(TransformerConfig) (genx.Transformer, error) { return stream, nil }},
				})
				text, err := service.Transcribe(t.Context(), "asr", "", newTextStream("audio"))
				if !stream.closed {
					t.Fatal("transcription output was not closed")
				}
				if failed {
					code, _, _, public := genx.PublicErrorDetails(err)
					if text != "" || !errors.Is(err, cause) || !public || code != "ASR_STREAM_ERROR" || stream.reads != 3 {
						t.Fatalf("failed route returned text=%q error=%v reads=%d", text, err, stream.reads)
					}
				} else if err != nil || text != prefix+"tail" || stream.reads != 5 {
					t.Fatalf("successful route returned text=%q error=%v reads=%d", text, err, stream.reads)
				}
			})
		}
	}
}

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
