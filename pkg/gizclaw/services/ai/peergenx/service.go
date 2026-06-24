package peergenx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
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

type Authorizer interface {
	Authorize(context.Context, acl.AuthorizeRequest) error
}

type ModelGetter interface {
	GetModel(context.Context, adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error)
}

type ModelLister interface {
	ListModels(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error)
}

type VoiceGetter interface {
	GetVoice(context.Context, adminservice.GetVoiceRequestObject) (adminservice.GetVoiceResponseObject, error)
}

type CredentialGetter interface {
	GetCredential(context.Context, adminservice.GetCredentialRequestObject) (adminservice.GetCredentialResponseObject, error)
}

type ProviderTenantGetter interface {
	GetOpenAITenant(context.Context, adminservice.GetOpenAITenantRequestObject) (adminservice.GetOpenAITenantResponseObject, error)
	GetGeminiTenant(context.Context, adminservice.GetGeminiTenantRequestObject) (adminservice.GetGeminiTenantResponseObject, error)
	GetDashScopeTenant(context.Context, adminservice.GetDashScopeTenantRequestObject) (adminservice.GetDashScopeTenantResponseObject, error)
	GetMiniMaxTenant(context.Context, adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error)
	GetVolcTenant(context.Context, adminservice.GetVolcTenantRequestObject) (adminservice.GetVolcTenantResponseObject, error)
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
	Authorizer      Authorizer
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
var _ genx.Transformer = (*Transformer)(nil)

func New(service Service) *Service {
	if service.Builder == nil {
		service.Builder = DefaultBuilder{}
	}
	return &service
}

func (s *Service) Generator() genx.Generator {
	return &Generator{service: s}
}

func (s *Service) Transformer() genx.Transformer {
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
	return impl.Transform(ctx, pattern, input)
}

func (s *Service) builder() Builder {
	if s != nil && s.Builder != nil {
		return s.Builder
	}
	return DefaultBuilder{}
}

func (s *Service) subject() (apitypes.ACLSubject, error) {
	if s == nil || s.Peer == nil {
		return apitypes.ACLSubject{}, fmt.Errorf("%w: peer is required", ErrNotConfigured)
	}
	publicKey := strings.TrimSpace(s.Peer.PublicKey().String())
	if publicKey == "" {
		return apitypes.ACLSubject{}, fmt.Errorf("%w: peer public key is required", ErrInvalid)
	}
	return acl.PublicKeySubject(publicKey), nil
}

func (s *Service) authorize(ctx context.Context, resource apitypes.ACLResource, permission apitypes.ACLPermission) error {
	if s == nil || s.Authorizer == nil {
		return fmt.Errorf("%w: authorizer is required", ErrNotConfigured)
	}
	subject, err := s.subject()
	if err != nil {
		return err
	}
	if err := s.Authorizer.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    subject,
		Resource:   resource,
		Permission: permission,
	}); err != nil {
		if errors.Is(err, acl.ErrDenied) {
			return fmt.Errorf("%w: %s %s %s", ErrDenied, subject.Id, permission, resource.Id)
		}
		return err
	}
	return nil
}

func modelResource(id string) apitypes.ACLResource {
	return apitypes.ACLResource{Kind: apitypes.ACLResourceKindModel, Id: id}
}

func voiceResource(id string) apitypes.ACLResource {
	return apitypes.ACLResource{Kind: apitypes.ACLResourceKindVoice, Id: id}
}

func credentialResource(name string) apitypes.ACLResource {
	return apitypes.ACLResource{Kind: apitypes.ACLResourceKindCredential, Id: name}
}
