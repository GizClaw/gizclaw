package peergenx

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type Tenant struct {
	Kind      string
	OpenAI    *apitypes.OpenAITenant
	Gemini    *apitypes.GeminiTenant
	DashScope *apitypes.DashScopeTenant
	MiniMax   *apitypes.MiniMaxTenant
	Volc      *apitypes.VolcTenant
}

type GeneratorConfig struct {
	Pattern    string
	Model      apitypes.Model
	Tenant     Tenant
	Credential apitypes.Credential
}

type TransformerConfig struct {
	Pattern    string
	Model      *apitypes.Model
	Voice      *apitypes.Voice
	Tenant     Tenant
	Credential apitypes.Credential
	Params     map[string]any
	Agent      bool
}

func (s *Service) ListAccessibleGeneratorConfigs(ctx context.Context) ([]GeneratorConfig, error) {
	if s == nil || s.Models == nil {
		return nil, fmt.Errorf("%w: model getter is required", ErrNotConfigured)
	}
	lister, ok := s.Models.(ModelLister)
	if !ok {
		return nil, fmt.Errorf("%w: model lister is required", ErrNotConfigured)
	}
	limit := int32(200)
	var cursor *string
	var out []GeneratorConfig
	for {
		resp, err := lister.ListModels(ctx, adminhttp.ListModelsRequestObject{
			Params: adminhttp.ListModelsParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return nil, err
		}
		list, ok := resp.(adminhttp.ListModels200JSONResponse)
		if !ok {
			return nil, fmt.Errorf("%w: list models returned %T", ErrInvalid, resp)
		}
		for _, model := range list.Items {
			cfg, include, err := s.generatorConfigFromListedModel(ctx, model)
			if err != nil {
				return nil, err
			}
			if include {
				out = append(out, cfg)
			}
		}
		if !list.HasNext || list.NextCursor == nil || strings.TrimSpace(*list.NextCursor) == "" {
			break
		}
		cursor = list.NextCursor
	}
	return out, nil
}

func (s *Service) ResolveGenerator(ctx context.Context, pattern string) (GeneratorConfig, error) {
	modelID, ok := parsePattern(pattern, "model", "models")
	if !ok {
		return GeneratorConfig{}, fmt.Errorf("%w: generator pattern %q must target model/<id>", ErrInvalid, pattern)
	}
	model, tenant, credential, err := s.resolveModel(ctx, modelID)
	if err != nil {
		return GeneratorConfig{}, err
	}
	if model.Kind != apitypes.ModelKindLlm {
		return GeneratorConfig{}, fmt.Errorf("%w: model %q kind %q is not a generator", ErrInvalid, model.Id, model.Kind)
	}
	return GeneratorConfig{
		Pattern:    pattern,
		Model:      model,
		Tenant:     tenant,
		Credential: credential,
	}, nil
}

func (s *Service) generatorConfigFromListedModel(ctx context.Context, model apitypes.Model) (GeneratorConfig, bool, error) {
	if model.Kind != apitypes.ModelKindLlm {
		return GeneratorConfig{}, false, nil
	}
	tenant, credentialName, err := s.resolveModelTenant(ctx, model)
	if err != nil {
		if errors.Is(err, ErrDenied) {
			return GeneratorConfig{}, false, nil
		}
		return GeneratorConfig{}, false, err
	}
	credential, err := s.resolveCredential(ctx, credentialName)
	if err != nil {
		if errors.Is(err, ErrDenied) {
			return GeneratorConfig{}, false, nil
		}
		return GeneratorConfig{}, false, err
	}
	if err := s.authorizeModelCredential(model, credential); err != nil {
		if errors.Is(err, ErrDenied) {
			return GeneratorConfig{}, false, nil
		}
		return GeneratorConfig{}, false, err
	}
	return GeneratorConfig{
		Pattern:    "model/" + string(model.Id),
		Model:      model,
		Tenant:     tenant,
		Credential: credential,
	}, true, nil
}

func (s *Service) ResolveTransformer(ctx context.Context, pattern string) (TransformerConfig, error) {
	basePattern, params, err := splitPatternParams(pattern)
	if err != nil {
		return TransformerConfig{}, err
	}
	if voiceID, ok := parsePattern(basePattern, "voice", "voices"); ok {
		voice, tenant, credential, err := s.resolveVoice(ctx, voiceID)
		if err != nil {
			return TransformerConfig{}, err
		}
		return TransformerConfig{
			Pattern:    pattern,
			Voice:      &voice,
			Tenant:     tenant,
			Credential: credential,
			Params:     params,
		}, nil
	}

	modelID, ok := parsePattern(basePattern, "model", "models")
	if !ok {
		return TransformerConfig{}, fmt.Errorf("%w: transformer pattern %q must target model/<id> or voice/<id>", ErrInvalid, pattern)
	}
	model, tenant, credential, err := s.resolveModel(ctx, modelID)
	if err != nil {
		return TransformerConfig{}, err
	}
	switch model.Kind {
	case apitypes.ModelKindAsr, apitypes.ModelKindTts, apitypes.ModelKindRealtime, apitypes.ModelKindTranslation:
	default:
		return TransformerConfig{}, fmt.Errorf("%w: model %q kind %q is not a transformer", ErrInvalid, model.Id, model.Kind)
	}
	return TransformerConfig{
		Pattern:    pattern,
		Model:      &model,
		Tenant:     tenant,
		Credential: credential,
		Params:     params,
	}, nil
}

func (s *Service) resolveModel(ctx context.Context, id string) (apitypes.Model, Tenant, apitypes.Credential, error) {
	if s == nil || s.Models == nil {
		return apitypes.Model{}, Tenant{}, apitypes.Credential{}, fmt.Errorf("%w: model getter is required", ErrNotConfigured)
	}
	model, err := s.getModel(ctx, id)
	if err != nil {
		return apitypes.Model{}, Tenant{}, apitypes.Credential{}, err
	}
	tenant, credentialName, err := s.resolveModelTenant(ctx, model)
	if err != nil {
		return apitypes.Model{}, Tenant{}, apitypes.Credential{}, err
	}
	credential, err := s.resolveCredential(ctx, credentialName)
	if err != nil {
		return apitypes.Model{}, Tenant{}, apitypes.Credential{}, err
	}
	if err := s.authorizeModelCredential(model, credential); err != nil {
		return apitypes.Model{}, Tenant{}, apitypes.Credential{}, err
	}
	return model, tenant, credential, nil
}

func (s *Service) authorizeModelCredential(model apitypes.Model, credential apitypes.Credential) error {
	if model.OwnerPublicKey == nil {
		return nil
	}
	if s == nil {
		return fmt.Errorf("%w: owned model %q has no service", ErrDenied, model.Id)
	}
	if checker, ok := s.Models.(ProfileModelChecker); ok && checker.ProfileAllowsModel(string(model.Id)) {
		return nil
	}
	if s.Peer == nil {
		return fmt.Errorf("%w: owned model %q has no authenticated peer", ErrDenied, model.Id)
	}
	caller := s.Peer.PublicKey().String()
	if strings.TrimSpace(*model.OwnerPublicKey) != caller {
		return fmt.Errorf("%w: model %q belongs to another peer", ErrDenied, model.Id)
	}
	if credential.OwnerPublicKey != nil && strings.TrimSpace(*credential.OwnerPublicKey) == caller {
		return nil
	}
	return fmt.Errorf("%w: owned model %q cannot use credential %q", ErrDenied, model.Id, credential.Name)
}

func (s *Service) resolveVoice(ctx context.Context, id string) (apitypes.Voice, Tenant, apitypes.Credential, error) {
	if s == nil || s.Voices == nil {
		return apitypes.Voice{}, Tenant{}, apitypes.Credential{}, fmt.Errorf("%w: voice getter is required", ErrNotConfigured)
	}
	voice, err := s.getVoice(ctx, id)
	if err != nil {
		return apitypes.Voice{}, Tenant{}, apitypes.Credential{}, err
	}
	tenant, credentialName, err := s.resolveVoiceTenant(ctx, voice)
	if err != nil {
		return apitypes.Voice{}, Tenant{}, apitypes.Credential{}, err
	}
	credential, err := s.resolveCredential(ctx, credentialName)
	if err != nil {
		return apitypes.Voice{}, Tenant{}, apitypes.Credential{}, err
	}
	return voice, tenant, credential, nil
}

func (s *Service) resolveCredential(ctx context.Context, name string) (apitypes.Credential, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return apitypes.Credential{}, fmt.Errorf("%w: credential name is required", ErrInvalid)
	}
	if s == nil || s.Credentials == nil {
		return apitypes.Credential{}, fmt.Errorf("%w: credential getter is required", ErrNotConfigured)
	}
	return s.getCredential(ctx, name)
}

func (s *Service) resolveModelTenant(ctx context.Context, model apitypes.Model) (Tenant, string, error) {
	if s == nil || s.ProviderTenants == nil {
		return Tenant{}, "", fmt.Errorf("%w: provider tenant getter is required", ErrNotConfigured)
	}
	kind := string(model.Provider.Kind)
	name := strings.TrimSpace(model.Provider.Name)
	switch kind {
	case string(apitypes.ModelProviderKindOpenaiTenant):
		tenant, err := s.getOpenAITenant(ctx, name)
		if err != nil {
			return Tenant{}, "", err
		}
		return Tenant{Kind: kind, OpenAI: &tenant}, tenant.CredentialName, nil
	case string(apitypes.ModelProviderKindGeminiTenant):
		tenant, err := s.getGeminiTenant(ctx, name)
		if err != nil {
			return Tenant{}, "", err
		}
		return Tenant{Kind: kind, Gemini: &tenant}, tenant.CredentialName, nil
	case string(apitypes.ModelProviderKindDashscopeTenant):
		tenant, err := s.getDashScopeTenant(ctx, name)
		if err != nil {
			return Tenant{}, "", err
		}
		return Tenant{Kind: kind, DashScope: &tenant}, tenant.CredentialName, nil
	case string(apitypes.VoiceProviderKindVolcTenant):
		tenant, err := s.getVolcTenant(ctx, name)
		if err != nil {
			return Tenant{}, "", err
		}
		return Tenant{Kind: kind, Volc: &tenant}, tenant.CredentialName, nil
	default:
		return Tenant{}, "", fmt.Errorf("%w: model provider %q", ErrUnsupported, kind)
	}
}

func (s *Service) resolveVoiceTenant(ctx context.Context, voice apitypes.Voice) (Tenant, string, error) {
	if s == nil || s.ProviderTenants == nil {
		return Tenant{}, "", fmt.Errorf("%w: provider tenant getter is required", ErrNotConfigured)
	}
	kind := string(voice.Provider.Kind)
	name := strings.TrimSpace(voice.Provider.Name)
	switch kind {
	case string(apitypes.VoiceProviderKindMinimaxTenant):
		tenant, err := s.getMiniMaxTenant(ctx, name)
		if err != nil {
			return Tenant{}, "", err
		}
		return Tenant{Kind: kind, MiniMax: &tenant}, tenant.CredentialName, nil
	case string(apitypes.VoiceProviderKindVolcTenant):
		tenant, err := s.getVolcTenant(ctx, name)
		if err != nil {
			return Tenant{}, "", err
		}
		return Tenant{Kind: kind, Volc: &tenant}, tenant.CredentialName, nil
	default:
		return Tenant{}, "", fmt.Errorf("%w: voice provider %q", ErrUnsupported, kind)
	}
}

func (s *Service) getModel(ctx context.Context, id string) (apitypes.Model, error) {
	response, err := s.Models.GetModel(ctx, adminhttp.GetModelRequestObject{Id: id})
	if err != nil {
		return apitypes.Model{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetModel200JSONResponse:
		return apitypes.Model(typed), nil
	case adminhttp.GetModel404JSONResponse:
		return apitypes.Model{}, fmt.Errorf("%w: model %q", ErrNotFound, id)
	default:
		return apitypes.Model{}, fmt.Errorf("%w: get model %q returned %T", ErrInvalid, id, response)
	}
}

func (s *Service) getVoice(ctx context.Context, id string) (apitypes.Voice, error) {
	response, err := s.Voices.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: id})
	if err != nil {
		return apitypes.Voice{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetVoice200JSONResponse:
		return apitypes.Voice(typed), nil
	case adminhttp.GetVoice404JSONResponse:
		return apitypes.Voice{}, fmt.Errorf("%w: voice %q", ErrNotFound, id)
	default:
		return apitypes.Voice{}, fmt.Errorf("%w: get voice %q returned %T", ErrInvalid, id, response)
	}
}

func (s *Service) getCredential(ctx context.Context, name string) (apitypes.Credential, error) {
	response, err := s.Credentials.GetCredential(ctx, adminhttp.GetCredentialRequestObject{Name: name})
	if err != nil {
		return apitypes.Credential{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetCredential200JSONResponse:
		return apitypes.Credential(typed), nil
	case adminhttp.GetCredential404JSONResponse:
		return apitypes.Credential{}, fmt.Errorf("%w: credential %q", ErrNotFound, name)
	default:
		return apitypes.Credential{}, fmt.Errorf("%w: get credential %q returned %T", ErrInvalid, name, response)
	}
}

func (s *Service) getOpenAITenant(ctx context.Context, name string) (apitypes.OpenAITenant, error) {
	response, err := s.ProviderTenants.GetOpenAITenant(ctx, adminhttp.GetOpenAITenantRequestObject{Name: name})
	if err != nil {
		return apitypes.OpenAITenant{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetOpenAITenant200JSONResponse:
		return apitypes.OpenAITenant(typed), nil
	case adminhttp.GetOpenAITenant404JSONResponse:
		return apitypes.OpenAITenant{}, fmt.Errorf("%w: openai tenant %q", ErrNotFound, name)
	default:
		return apitypes.OpenAITenant{}, fmt.Errorf("%w: get openai tenant %q returned %T", ErrInvalid, name, response)
	}
}

func (s *Service) getGeminiTenant(ctx context.Context, name string) (apitypes.GeminiTenant, error) {
	response, err := s.ProviderTenants.GetGeminiTenant(ctx, adminhttp.GetGeminiTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.GeminiTenant{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetGeminiTenant200JSONResponse:
		return apitypes.GeminiTenant(typed), nil
	case adminhttp.GetGeminiTenant404JSONResponse:
		return apitypes.GeminiTenant{}, fmt.Errorf("%w: gemini tenant %q", ErrNotFound, name)
	default:
		return apitypes.GeminiTenant{}, fmt.Errorf("%w: get gemini tenant %q returned %T", ErrInvalid, name, response)
	}
}

func (s *Service) getDashScopeTenant(ctx context.Context, name string) (apitypes.DashScopeTenant, error) {
	response, err := s.ProviderTenants.GetDashScopeTenant(ctx, adminhttp.GetDashScopeTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.DashScopeTenant{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetDashScopeTenant200JSONResponse:
		return apitypes.DashScopeTenant(typed), nil
	case adminhttp.GetDashScopeTenant404JSONResponse:
		return apitypes.DashScopeTenant{}, fmt.Errorf("%w: dashscope tenant %q", ErrNotFound, name)
	default:
		return apitypes.DashScopeTenant{}, fmt.Errorf("%w: get dashscope tenant %q returned %T", ErrInvalid, name, response)
	}
}

func (s *Service) getMiniMaxTenant(ctx context.Context, name string) (apitypes.MiniMaxTenant, error) {
	response, err := s.ProviderTenants.GetMiniMaxTenant(ctx, adminhttp.GetMiniMaxTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.MiniMaxTenant{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetMiniMaxTenant200JSONResponse:
		return apitypes.MiniMaxTenant(typed), nil
	case adminhttp.GetMiniMaxTenant404JSONResponse:
		return apitypes.MiniMaxTenant{}, fmt.Errorf("%w: minimax tenant %q", ErrNotFound, name)
	default:
		return apitypes.MiniMaxTenant{}, fmt.Errorf("%w: get minimax tenant %q returned %T", ErrInvalid, name, response)
	}
}

func (s *Service) getVolcTenant(ctx context.Context, name string) (apitypes.VolcTenant, error) {
	response, err := s.ProviderTenants.GetVolcTenant(ctx, adminhttp.GetVolcTenantRequestObject{Name: name})
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	switch typed := response.(type) {
	case adminhttp.GetVolcTenant200JSONResponse:
		return apitypes.VolcTenant(typed), nil
	case adminhttp.GetVolcTenant404JSONResponse:
		return apitypes.VolcTenant{}, fmt.Errorf("%w: volc tenant %q", ErrNotFound, name)
	default:
		return apitypes.VolcTenant{}, fmt.Errorf("%w: get volc tenant %q returned %T", ErrInvalid, name, response)
	}
}

func parsePattern(pattern string, prefixes ...string) (string, bool) {
	pattern = strings.TrimSpace(pattern)
	if base, _, ok := strings.Cut(pattern, "?"); ok {
		pattern = base
	}
	for _, prefix := range prefixes {
		head := prefix + "/"
		if id, ok := strings.CutPrefix(pattern, head); ok {
			id = strings.TrimSpace(id)
			return id, id != ""
		}
	}
	return "", false
}

func splitPatternParams(pattern string) (string, map[string]any, error) {
	pattern = strings.TrimSpace(pattern)
	base, rawQuery, ok := strings.Cut(pattern, "?")
	if !ok {
		return pattern, nil, nil
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", nil, fmt.Errorf("%w: invalid transformer pattern query: %w", ErrInvalid, err)
	}
	params := make(map[string]any, len(values))
	for key, value := range values {
		if len(value) == 0 {
			continue
		}
		params[key] = parsePatternParamValue(value[len(value)-1])
	}
	return strings.TrimSpace(base), params, nil
}

func parsePatternParamValue(value string) any {
	text := strings.TrimSpace(value)
	if boolValue, err := strconv.ParseBool(text); err == nil {
		return boolValue
	}
	if intValue, err := strconv.Atoi(text); err == nil {
		return intValue
	}
	return text
}

func isDenied(err error) bool {
	return errors.Is(err, ErrDenied)
}
