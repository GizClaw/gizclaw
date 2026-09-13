package toolkit

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// FromSpec validates an HTTP Tool declaration.
func FromSpec(id string, spec apitypes.ToolSpec) (Tool, error) {
	tool, err := commonToolFromAPI(id, spec.InvokeName, ToolType(spec.Type), spec.Description, spec.Enabled, spec.Version, spec.InputSchema, spec.Triggers, spec.Metadata)
	if err != nil {
		return Tool{}, err
	}
	tool.HTTP, err = httpRequestFromAPI(spec.Http)
	if err != nil {
		return Tool{}, err
	}
	return normalizeToolDeclaration(tool)
}

// ToSpec returns the complete HTTP Tool declaration.
func ToSpec(tool Tool) (apitypes.ToolSpec, error) { return toSpec(tool, true) }

// ToRedactedSpec returns the Admin read representation without direct secrets.
func ToRedactedSpec(tool Tool) (apitypes.ToolSpec, error) { return toSpec(tool, false) }

func toSpec(tool Tool, secrets bool) (apitypes.ToolSpec, error) {
	tool, err := NormalizeTool(tool)
	if err != nil {
		return apitypes.ToolSpec{}, err
	}
	metadata, err := rawToMap(tool.Metadata)
	if err != nil {
		return apitypes.ToolSpec{}, err
	}
	triggers, err := triggersToAPI(tool.Triggers)
	if err != nil {
		return apitypes.ToolSpec{}, err
	}
	httpConfig, err := httpRequestToAPI(tool.HTTP, secrets)
	if err != nil {
		return apitypes.ToolSpec{}, err
	}
	return apitypes.ToolSpec{Type: apitypes.ToolSpecTypeHttpRequest, InvokeName: tool.InvokeName, Description: tool.Description, Enabled: &tool.Enabled, Http: httpConfig, InputSchema: tool.InputSchema, Metadata: metadata, Triggers: triggers, Version: tool.Version}, nil
}

func commonToolFromAPI(
	id string,
	invokeName string,
	toolType ToolType,
	description *string,
	enabled *bool,
	version *string,
	inputSchema apitypes.ToolJSONSchema,
	triggers *[]apitypes.ToolTrigger,
	metadata *map[string]any,
) (Tool, error) {
	value := true
	if enabled != nil {
		value = *enabled
	}
	tool := Tool{
		ID:          id,
		InvokeName:  invokeName,
		Type:        toolType,
		Description: description,
		Enabled:     value,
		Version:     version,
		InputSchema: inputSchema,
	}
	var err error
	if tool.Metadata, err = mapToRaw(metadata); err != nil {
		return Tool{}, err
	}
	if triggers != nil {
		if tool.Triggers, err = triggersFromAPI(*triggers); err != nil {
			return Tool{}, err
		}
	}
	return tool, nil
}

func httpRequestFromAPI(in apitypes.ToolHTTPRequest) (HTTPRequest, error) {
	timeout, err := time.ParseDuration(in.Timeout)
	if err != nil {
		return HTTPRequest{}, fmt.Errorf("%w: http.timeout: %v", ErrInvalidTool, err)
	}
	auth, err := httpAuthFromAPI(in.Auth)
	if err != nil {
		return HTTPRequest{}, err
	}
	out := HTTPRequest{
		URL:                in.Url,
		Method:             string(in.Method),
		Auth:               auth,
		Headers:            cloneMap(in.Headers),
		Query:              bindingsFromAPI(in.Query),
		Body:               bindingsFromAPI(in.Body),
		ResponsePointer:    in.ResponsePointer,
		SuccessStatusCodes: cloneIntSlice(in.SuccessStatusCodes),
		Timeout:            timeout,
		MaxResponseBytes:   int64(in.MaxResponseBytes),
	}
	return out, nil
}

func httpRequestToAPI(in HTTPRequest, includeSecrets bool) (apitypes.ToolHTTPRequest, error) {
	auth, err := httpAuthToAPI(in.Auth, includeSecrets)
	if err != nil {
		return apitypes.ToolHTTPRequest{}, err
	}
	return apitypes.ToolHTTPRequest{
		Auth:               auth,
		Body:               bindingsToAPI(in.Body),
		Headers:            mapPtr(in.Headers),
		MaxResponseBytes:   int(in.MaxResponseBytes),
		Method:             apitypes.ToolHTTPMethod(in.Method),
		Query:              bindingsToAPI(in.Query),
		ResponsePointer:    in.ResponsePointer,
		SuccessStatusCodes: intSlicePtr(in.SuccessStatusCodes),
		Timeout:            in.Timeout.String(),
		Url:                in.URL,
	}, nil
}

func httpAuthFromAPI(in apitypes.ToolHTTPAuth) (HTTPAuth, error) {
	method, err := in.Discriminator()
	if err != nil {
		return HTTPAuth{}, fmt.Errorf("%w: decode http.auth.method: %v", ErrInvalidTool, err)
	}
	out := HTTPAuth{Method: method}
	switch method {
	case "none":
		_, err = in.AsToolHTTPAuthNone()
	case "bearer":
		var value apitypes.ToolHTTPAuthBearer
		value, err = in.AsToolHTTPAuthBearer()
		out.BearerToken = value.BearerToken
	case "header_api_key":
		var value apitypes.ToolHTTPAuthHeaderAPIKey
		value, err = in.AsToolHTTPAuthHeaderAPIKey()
		out.Header, out.APIKey = &value.Header, value.ApiKey
	case "volc_ark":
		var value apitypes.ToolHTTPAuthVolcArk
		value, err = in.AsToolHTTPAuthVolcArk()
		out.Credential = &value.Credential
	case "volc_search":
		var value apitypes.ToolHTTPAuthVolcSearch
		value, err = in.AsToolHTTPAuthVolcSearch()
		out.Credential = &value.Credential
	case "volc_openapi":
		var value apitypes.ToolHTTPAuthVolcOpenAPI
		value, err = in.AsToolHTTPAuthVolcOpenAPI()
		out.Credential, out.Region, out.Service = &value.Credential, &value.Region, &value.Service
	case "aliyun_app_code":
		var value apitypes.ToolHTTPAuthAliyunAppCode
		value, err = in.AsToolHTTPAuthAliyunAppCode()
		out.Credential = &value.Credential
	case "aliyun_openapi_v3":
		var value apitypes.ToolHTTPAuthAliyunOpenAPIV3
		value, err = in.AsToolHTTPAuthAliyunOpenAPIV3()
		out.Credential, out.Action, out.Version = &value.Credential, &value.Action, &value.Version
	default:
		return HTTPAuth{}, fmt.Errorf("%w: unsupported HTTP auth method %q", ErrInvalidTool, method)
	}
	if err != nil {
		return HTTPAuth{}, fmt.Errorf("%w: decode HTTP auth %q: %v", ErrInvalidTool, method, err)
	}
	return out, nil
}

func httpAuthToAPI(in HTTPAuth, includeSecrets bool) (apitypes.ToolHTTPAuth, error) {
	var out apitypes.ToolHTTPAuth
	var err error
	switch in.Method {
	case "none":
		err = out.FromToolHTTPAuthNone(apitypes.ToolHTTPAuthNone{})
	case "bearer":
		var token *string
		if includeSecrets {
			token = in.BearerToken
		}
		err = out.FromToolHTTPAuthBearer(apitypes.ToolHTTPAuthBearer{BearerToken: token})
	case "header_api_key":
		var key *string
		if includeSecrets {
			key = in.APIKey
		}
		err = out.FromToolHTTPAuthHeaderAPIKey(apitypes.ToolHTTPAuthHeaderAPIKey{Header: valueOrEmpty(in.Header), ApiKey: key})
	case "volc_ark":
		err = out.FromToolHTTPAuthVolcArk(apitypes.ToolHTTPAuthVolcArk{Credential: valueOrEmpty(in.Credential)})
	case "volc_search":
		err = out.FromToolHTTPAuthVolcSearch(apitypes.ToolHTTPAuthVolcSearch{Credential: valueOrEmpty(in.Credential)})
	case "volc_openapi":
		err = out.FromToolHTTPAuthVolcOpenAPI(apitypes.ToolHTTPAuthVolcOpenAPI{
			Credential: valueOrEmpty(in.Credential),
			Region:     valueOrEmpty(in.Region),
			Service:    valueOrEmpty(in.Service),
		})
	case "aliyun_app_code":
		err = out.FromToolHTTPAuthAliyunAppCode(apitypes.ToolHTTPAuthAliyunAppCode{Credential: valueOrEmpty(in.Credential)})
	case "aliyun_openapi_v3":
		err = out.FromToolHTTPAuthAliyunOpenAPIV3(apitypes.ToolHTTPAuthAliyunOpenAPIV3{
			Credential: valueOrEmpty(in.Credential),
			Action:     valueOrEmpty(in.Action),
			Version:    valueOrEmpty(in.Version),
		})
	default:
		return apitypes.ToolHTTPAuth{}, fmt.Errorf("%w: unsupported HTTP auth method %q", ErrInvalidTool, in.Method)
	}
	return out, err
}

func bindingsFromAPI(in *[]apitypes.ToolHTTPArgumentBinding) []HTTPArgumentBinding {
	if in == nil {
		return nil
	}
	out := make([]HTTPArgumentBinding, len(*in))
	for i, binding := range *in {
		required := true
		if binding.Required != nil {
			required = *binding.Required
		}
		out[i] = HTTPArgumentBinding{
			ArgumentPointer: binding.ArgumentPointer,
			Target:          binding.Target,
			Required:        required,
		}
	}
	return out
}

func bindingsToAPI(in []HTTPArgumentBinding) *[]apitypes.ToolHTTPArgumentBinding {
	if in == nil {
		return nil
	}
	out := make([]apitypes.ToolHTTPArgumentBinding, len(in))
	for i, binding := range in {
		required := binding.Required
		out[i] = apitypes.ToolHTTPArgumentBinding{
			ArgumentPointer: binding.ArgumentPointer,
			Target:          binding.Target,
			Required:        &required,
		}
	}
	return &out
}

func mapToRaw(in *map[string]any) (json.RawMessage, error) {
	if in == nil {
		return nil, nil
	}
	return json.Marshal(*in)
}

func rawToMap(in json.RawMessage) (*map[string]any, error) {
	if len(in) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func triggersFromAPI(in []apitypes.ToolTrigger) ([]ToolTrigger, error) {
	out := make([]ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = ToolTrigger{Name: trigger.Name, Description: trigger.Description}
		var err error
		if out[i].Metadata, err = mapToRaw(trigger.Metadata); err != nil {
			return nil, err
		}
		if trigger.Patterns != nil {
			out[i].Patterns = append([]string(nil), (*trigger.Patterns)...)
		}
		if trigger.Examples != nil {
			out[i].Examples = make([]ToolTriggerExample, len(*trigger.Examples))
			for j, example := range *trigger.Examples {
				out[i].Examples[j] = ToolTriggerExample{Input: example.Input, Output: example.Output}
				if out[i].Examples[j].Args, err = mapToRaw(example.Args); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

func triggersToAPI(in []ToolTrigger) (*[]apitypes.ToolTrigger, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]apitypes.ToolTrigger, len(in))
	for i, trigger := range in {
		out[i] = apitypes.ToolTrigger{Name: trigger.Name, Description: trigger.Description}
		if trigger.Patterns != nil {
			patterns := append([]string(nil), trigger.Patterns...)
			out[i].Patterns = &patterns
		}
		var err error
		if out[i].Metadata, err = rawToMap(trigger.Metadata); err != nil {
			return nil, err
		}
		if trigger.Examples != nil {
			examples := make([]apitypes.ToolTriggerExample, len(trigger.Examples))
			for j, example := range trigger.Examples {
				examples[j] = apitypes.ToolTriggerExample{Input: example.Input, Output: example.Output}
				if examples[j].Args, err = rawToMap(example.Args); err != nil {
					return nil, err
				}
			}
			out[i].Examples = &examples
		}
	}
	return &out, nil
}

func cloneMap(in *map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(*in))
	maps.Copy(out, *in)
	return out
}

func mapPtr(in map[string]string) *map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return &out
}

func cloneIntSlice(in *[]int) []int {
	if in == nil {
		return nil
	}
	return append([]int(nil), (*in)...)
}

func intSlicePtr(in []int) *[]int {
	if in == nil {
		return nil
	}
	out := append([]int(nil), in...)
	return &out
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
