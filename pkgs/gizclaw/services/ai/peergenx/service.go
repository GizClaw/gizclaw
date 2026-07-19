package peergenx

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var (
	ErrDenied        = errors.New("peergenx: denied")
	ErrNotFound      = errors.New("peergenx: not found")
	ErrInvalid       = errors.New("peergenx: invalid resource")
	ErrUnsupported   = errors.New("peergenx: unsupported resource")
	ErrNotConfigured = errors.New("peergenx: service not configured")
)

type Peer interface {
	PublicKey() giznet.PublicKey
}

type ModelGetter interface {
	GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error)
}

type ModelLister interface {
	ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)
}

type ProfileModelChecker interface {
	ProfileAllowsModel(string) bool
}

type VoiceGetter interface {
	GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error)
}

type CredentialGetter interface {
	GetCredential(context.Context, adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error)
}

type ProviderTenantGetter interface {
	GetOpenAITenant(context.Context, adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error)
	GetGeminiTenant(context.Context, adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error)
	GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error)
	GetMiniMaxTenant(context.Context, adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error)
	GetVolcTenant(context.Context, adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error)
}

type Builder interface {
	BuildGenerator(context.Context, GeneratorConfig) (genx.Generator, error)
	BuildTransformer(context.Context, TransformerConfig) (genx.Transformer, error)
}

type AudioOutput interface {
	ConsumeAgentOutput(context.Context, genx.Stream) error
}

type Service struct {
	Peer            Peer
	Models          ModelGetter
	Voices          VoiceGetter
	Credentials     CredentialGetter
	ProviderTenants ProviderTenantGetter
	Builder         Builder
	AudioOutput     AudioOutput
}

type Generator struct {
	service *Service
}

type Transformer struct {
	service *Service
}

var _ genx.Generator = (*Generator)(nil)
var _ genx.TransformerMux = (*Transformer)(nil)

func New(service Service) *Service {
	if service.Builder == nil {
		service.Builder = DefaultBuilder{}
	}
	return &service
}

func (s *Service) Generator() genx.Generator {
	return &Generator{service: s}
}

func (s *Service) Transformer() genx.TransformerMux {
	return &Transformer{service: s}
}

func (g *Generator) GenerateStream(ctx context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
	if g == nil || g.service == nil {
		return nil, ErrNotConfigured
	}
	cfg, err := g.service.ResolveGenerator(ctx, pattern)
	if err != nil {
		return nil, err
	}
	impl, err := g.service.builder().BuildGenerator(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return impl.GenerateStream(ctx, pattern, mctx)
}

func (g *Generator) Invoke(ctx context.Context, pattern string, mctx genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	if g == nil || g.service == nil {
		return genx.Usage{}, nil, ErrNotConfigured
	}
	cfg, err := g.service.ResolveGenerator(ctx, pattern)
	if err != nil {
		return genx.Usage{}, nil, err
	}
	impl, err := g.service.builder().BuildGenerator(ctx, cfg)
	if err != nil {
		return genx.Usage{}, nil, err
	}
	return impl.Invoke(ctx, pattern, mctx, tool)
}

func (t *Transformer) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.service == nil {
		return nil, ErrNotConfigured
	}
	cfg, err := t.service.ResolveTransformer(ctx, pattern)
	if err != nil {
		return nil, err
	}
	impl, err := t.service.builder().BuildTransformer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return impl.Transform(ctx, input)
}

func (s *Service) builder() Builder {
	if s != nil && s.Builder != nil {
		return s.Builder
	}
	return DefaultBuilder{}
}
