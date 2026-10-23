package peergenx

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type transcriptionBuilder struct {
	Builder
	call func(context.Context) (string, genx.Usage, error)
}

type asrInputBuilder struct {
	Builder
	build func(TransformerConfig) (genx.Transformer, error)
}

func (b asrInputBuilder) BuildTransformer(_ context.Context, config TransformerConfig) (genx.Transformer, error) {
	return b.build(config)
}

func TestBuildASRSelectsExplicitModelWithoutNativeFallback(t *testing.T) {
	if got, err := (*Service)(nil).BuildASR(t.Context(), "", apitypes.WorkspaceInputModeRealtime); err != nil || got != nil {
		t.Fatalf("native path constructed a provider: %v %v", got, err)
	}
	for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
		events := []string{}
		builds, grants, releases := 0, 0, 0
		service := New(Service{
			Models:      fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: "volc-tenant"},
			Credentials: fakeCredentials{events: &events}, ProviderTenants: fakeTenants{events: &events},
			Authorize: func(ctx context.Context) (context.Context, func(), error) {
				grants++
				return ctx, func() { releases++ }, nil
			},
			Builder: asrInputBuilder{build: func(config TransformerConfig) (genx.Transformer, error) {
				builds++
				parsed, err := url.Parse(config.Pattern)
				if err != nil {
					t.Fatal(err)
				}
				query := parsed.Query()
				if parsed.Path != "model/speech.input" || config.Model.Id != "speech.input" || query.Get("realtime_pacing") != "false" {
					t.Fatalf("unexpected ASR resolution: %s model=%s", config.Pattern, config.Model.Id)
				}
				if mode == apitypes.WorkspaceInputModeRealtime {
					if query.Get("emit_interim") != "true" || query.Get("end_window_size") != "200" || query.Get("force_to_speech_time") != "1000" {
						t.Fatalf("missing realtime segmentation: %s", query)
					}
				} else if query.Has("emit_interim") || query.Has("end_window_size") || query.Has("force_to_speech_time") {
					t.Fatalf("PTT inherited realtime segmentation: %s", query)
				}
				return fakeTransformer{events: &events}, nil
			}},
		})
		asr, err := service.BuildASR(t.Context(), "speech.input", mode)
		if err != nil || asr == nil || builds != 1 || grants != 0 {
			t.Fatalf("ASR=%v error=%v builds=%d grants=%d", asr, err, builds, grants)
		}
		output, err := asr.Transform(t.Context(), fakeStream{})
		if err != nil {
			t.Fatal(err)
		}
		_ = output.Close()
		if grants != 1 || releases != 1 {
			t.Fatalf("quota boundary: grants=%d releases=%d", grants, releases)
		}
		service.Models = fakeModels{events: &events, modelKind: apitypes.ModelKindRealtime, providerKind: "volc-tenant"}
		if asr, err := service.BuildASR(t.Context(), "speech.input", mode); !errors.Is(err, ErrInvalid) || asr != nil || builds != 1 {
			t.Fatalf("wrong Model kind fell back: ASR=%v error=%v builds=%d", asr, err, builds)
		}
		if _, err := service.BuildASR(t.Context(), "speech.input?unsafe=true", mode); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe alias error=%v", err)
		}
		service.Models = fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: "volc-tenant"}
		service.Authorize = nil
		service.Builder = asrInputBuilder{build: func(TransformerConfig) (genx.Transformer, error) { return nil, nil }}
		if asr, err := service.BuildASR(t.Context(), "speech.input", mode); !errors.Is(err, ErrInvalid) || asr != nil {
			t.Fatalf("nil builder fell back: ASR=%v error=%v", asr, err)
		}
	}
}
