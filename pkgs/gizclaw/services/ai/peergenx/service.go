package peergenx

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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

type VoiceGetter interface {
	GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error)
}

type CredentialGetter interface {
	GetCredential(context.Context, adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error)
}

type ProviderTenantGetter interface {
	GetDeepSeekTenant(context.Context, adminhttp.GetDeepSeekTenantRequestObject) (adminhttp.GetDeepSeekTenantResponseObject, error)
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
	mctx, err = modelContextForGenerator(cfg, mctx)
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
	mctx, err = modelContextForGenerator(cfg, mctx)
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

type generatorModelContext struct {
	genx.ModelContext
	params *genx.ModelParams
}

func (m generatorModelContext) Params() *genx.ModelParams { return m.params }

func modelContextForGenerator(cfg GeneratorConfig, mctx genx.ModelContext) (genx.ModelContext, error) {
	if mctx == nil || mctx.Params() == nil || mctx.Params().Thinking == nil {
		return mctx, nil
	}
	thinkingConfig, err := modelThinkingConfigFor(cfg.Model)
	if err != nil {
		return nil, err
	}
	params := *mctx.Params()
	params.ExtraFields = maps.Clone(params.ExtraFields)
	thinkingFields := modelThinkingExtraFields(thinkingConfig, params.Thinking)
	if len(thinkingFields) > 0 {
		if params.ExtraFields == nil {
			params.ExtraFields = map[string]any{}
		}
		maps.Copy(params.ExtraFields, thinkingFields)
	}
	params.Thinking = nil
	return generatorModelContext{ModelContext: mctx, params: &params}, nil
}

type modelThinkingConfig struct {
	supported    bool
	param        *string
	levelParam   *string
	defaultLevel *string
}

func modelThinkingConfigFor(model apitypes.Model) (modelThinkingConfig, error) {
	switch model.Provider.Kind {
	case apitypes.ModelProviderKindOpenaiTenant:
		value, err := model.ProviderData.AsOpenAITenantModelProviderData()
		if err != nil {
			return modelThinkingConfig{}, fmt.Errorf("%w: decode openai model provider_data: %w", ErrInvalid, err)
		}
		return modelThinkingConfig{boolValue(value.SupportThinking), value.ThinkingParam, value.ThinkingLevelParam, value.DefaultThinkingLevel}, nil
	case apitypes.ModelProviderKindGeminiTenant:
		value, err := model.ProviderData.AsGeminiTenantModelProviderData()
		if err != nil {
			return modelThinkingConfig{}, fmt.Errorf("%w: decode gemini model provider_data: %w", ErrInvalid, err)
		}
		return modelThinkingConfig{boolValue(value.SupportThinking), value.ThinkingParam, value.ThinkingLevelParam, value.DefaultThinkingLevel}, nil
	case apitypes.ModelProviderKindDashscopeTenant:
		value, err := model.ProviderData.AsDashScopeTenantModelProviderData()
		if err != nil {
			return modelThinkingConfig{}, fmt.Errorf("%w: decode dashscope model provider_data: %w", ErrInvalid, err)
		}
		return modelThinkingConfig{boolValue(value.SupportThinking), value.ThinkingParam, value.ThinkingLevelParam, value.DefaultThinkingLevel}, nil
	case apitypes.ModelProviderKindVolcTenant:
		value, err := model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return modelThinkingConfig{}, fmt.Errorf("%w: decode volc model provider_data: %w", ErrInvalid, err)
		}
		return modelThinkingConfig{boolValue(value.SupportThinking), value.ThinkingParam, value.ThinkingLevelParam, value.DefaultThinkingLevel}, nil
	case apitypes.ModelProviderKindDeepseekTenant:
		value, err := model.ProviderData.AsDeepSeekTenantModelProviderData()
		if err != nil {
			return modelThinkingConfig{}, fmt.Errorf("%w: decode deepseek model provider_data: %w", ErrInvalid, err)
		}
		return modelThinkingConfig{boolValue(value.SupportThinking), value.ThinkingParam, value.ThinkingLevelParam, value.DefaultThinkingLevel}, nil
	case apitypes.ModelProviderKindMinimaxTenant:
		value, err := model.ProviderData.AsMiniMaxTenantModelProviderData()
		if err != nil {
			return modelThinkingConfig{}, fmt.Errorf("%w: decode minimax model provider_data: %w", ErrInvalid, err)
		}
		return modelThinkingConfig{boolValue(value.SupportThinking), value.ThinkingParam, value.ThinkingLevelParam, value.DefaultThinkingLevel}, nil
	default:
		return modelThinkingConfig{}, fmt.Errorf("%w: unsupported model provider kind %q", ErrInvalid, model.Provider.Kind)
	}
}

func modelThinkingExtraFields(config modelThinkingConfig, request *genx.ThinkingParams) map[string]any {
	if !config.supported || request == nil {
		return nil
	}
	level := strings.TrimSpace(request.Level)
	out := map[string]any{}
	if level != "" {
		param := firstString(config.levelParam, config.param)
		if !strings.EqualFold(param, "reasoning_effort") || !isDisabledThinkingLevel(level) {
			setNestedExtraField(out, param, openAIThinkingValue(param, level))
		}
	}
	if request.Enabled == nil || (level != "" && config.levelParam == nil) {
		return out
	}
	param := firstString(config.param)
	switch {
	case strings.EqualFold(param, "reasoning_effort"):
		if *request.Enabled {
			defaultLevel := firstString(config.defaultLevel)
			if defaultLevel != "" && !isDisabledThinkingLevel(defaultLevel) {
				setNestedExtraField(out, param, defaultLevel)
			}
		}
	case strings.EqualFold(param, "enable_thinking"):
		setNestedExtraField(out, param, *request.Enabled)
	case param != "":
		value := "disabled"
		if *request.Enabled {
			value = "enabled"
		}
		setNestedExtraField(out, param, value)
	}
	return out
}
