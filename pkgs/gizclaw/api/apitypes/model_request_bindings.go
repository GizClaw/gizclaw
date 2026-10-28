package apitypes

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

const (
	// MaxModelRequestHeaders bounds the effective extension headers per request.
	MaxModelRequestHeaders    = 16
	maxModelRequestKeyBytes   = 64
	maxModelRequestPointer    = 256
	maxModelRequestDepth      = 16
	maxModelRequestHeaderName = 128

	// MaxModelRequestHeaderBytes bounds one resolved header value.
	MaxModelRequestHeaderBytes = 1024
	// MaxModelRequestHeadersBytes bounds the complete injected header lines.
	MaxModelRequestHeadersBytes = 8192
)

// ValidateModelRequestBindings checks declarations without reading Runtime.
// Empty bindings disable Runtime resolution.
func ValidateModelRequestBindings(kind ModelKind, provider ModelProviderKind, bindings *ModelRequestBindings) error {
	if bindings == nil || len(*bindings) == 0 {
		return nil
	}
	if kind != ModelKindLlm || provider != ModelProviderKindOpenaiTenant {
		return errors.New("request_bindings require an openai-tenant llm model")
	}
	if len(*bindings) > MaxModelRequestHeaders {
		return errors.New("too many request_bindings")
	}
	names := make(map[string]bool, len(*bindings))
	constantBytes := 0
	for i, binding := range *bindings {
		if binding.Target.Kind != ModelRequestTargetKindHttpHeader || !modelRequestHeaderNameAllowed(binding.Target.Name) {
			return fmt.Errorf("request_bindings[%d]: invalid or reserved target", i)
		}
		name := strings.ToLower(binding.Target.Name)
		if names[name] {
			return fmt.Errorf("request_bindings[%d]: duplicate header target", i)
		}
		names[name] = true
		if err := validateModelRequestSource(binding.Source); err != nil {
			return fmt.Errorf("request_bindings[%d]: invalid source", i)
		}
		value, err := modelRequestConstant(binding.Source)
		if err != nil {
			return fmt.Errorf("request_bindings[%d]: invalid constant", i)
		}
		if value != nil {
			if err := ValidateModelRequestHeader(binding.Target.Name, *value); err != nil {
				return fmt.Errorf("request_bindings[%d]: invalid constant", i)
			}
			constantBytes += len(binding.Target.Name) + len(*value) + 4
			if constantBytes > MaxModelRequestHeadersBytes {
				return errors.New("constant request headers exceed byte budget")
			}
		}
	}
	return nil
}

func validateModelRequestSource(source ModelRequestSource) error {
	raw, err := source.MarshalJSON()
	if err != nil {
		return err
	}
	var branches map[string]json.RawMessage
	if err := json.Unmarshal(raw, &branches); err != nil {
		return err
	}
	if len(branches) != 1 {
		return errors.New("request binding source requires exactly one branch")
	}
	if _, ok := branches["constant"]; ok {
		var value struct {
			Constant string `json:"constant"`
		}
		if err := decodeStrictJSON(raw, &value); err != nil || value.Constant == "" {
			return errors.New("invalid constant source")
		}
		return nil
	}
	if _, ok := branches["peer_runtime_kv"]; !ok {
		return errors.New("unsupported request binding source")
	}
	var value ModelRequestPeerRuntimeKVSource
	if err := decodeStrictJSON(raw, &value); err != nil {
		return err
	}
	selector := value.PeerRuntimeKv
	if selector.Key == "" || len(selector.Key) > maxModelRequestKeyBytes || !utf8.ValidString(selector.Key) {
		return errors.New("invalid Runtime KV key")
	}
	_, err = ModelRequestPointerSegments(selector.Pointer)
	return err
}

func modelRequestConstant(source ModelRequestSource) (*string, error) {
	raw, err := source.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var value struct {
		Constant *string `json:"constant"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value.Constant, nil
}

// ValidateOpenAICredentialHeaders checks protected fixed extension headers.
// Errors never contain header names or values.
func ValidateOpenAICredentialHeaders(headers *map[string]string) error {
	if headers == nil {
		return nil
	}
	if len(*headers) > MaxModelRequestHeaders {
		return errors.New("too many credential headers")
	}
	names := make(map[string]bool, len(*headers))
	total := 0
	for name, value := range *headers {
		canonical := strings.ToLower(name)
		if names[canonical] || ValidateModelRequestHeader(name, value) != nil {
			return errors.New("invalid credential header")
		}
		names[canonical] = true
		total += len(name) + len(value) + 4
		if total > MaxModelRequestHeadersBytes {
			return errors.New("credential headers exceed byte budget")
		}
	}
	return nil
}

func modelRequestHeaderNameAllowed(name string) bool {
	if len(name) < 3 || len(name) > maxModelRequestHeaderName || !httpguts.ValidHeaderFieldName(name) {
		return false
	}
	name = strings.ToLower(name)
	if !strings.HasPrefix(name, "x-") {
		return false
	}
	for _, prefix := range []string{"x-stainless-", "x-forwarded-", "x-openai-", "x-goog-", "x-amz-", "x-auth-"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	switch name {
	case "x-api-key", "x-auth-token", "x-access-token", "x-authorization", "x-real-ip", "x-http-method-override":
		return false
	}
	return true
}

// ValidateModelRequestHeader validates extension header names and values.
// Errors deliberately exclude both the header name and its value.
func ValidateModelRequestHeader(name, value string) error {
	if !modelRequestHeaderNameAllowed(name) || len(value) == 0 || len(value) > MaxModelRequestHeaderBytes || strings.TrimSpace(value) != value {
		return errors.New("invalid request header")
	}
	for i := range len(value) {
		if value[i] < 0x20 || value[i] > 0x7e {
			return errors.New("invalid request header")
		}
	}
	return nil
}

// ModelRequestPointerSegments decodes a bounded JSON Pointer. An empty pointer
// selects the key's whole JSON value; URI fragments and expressions are invalid.
func ModelRequestPointerSegments(pointer *string) ([]string, error) {
	if pointer == nil || *pointer == "" {
		return nil, nil
	}
	value := *pointer
	if len(value) > maxModelRequestPointer || !utf8.ValidString(value) || value[0] != '/' {
		return nil, errors.New("invalid JSON pointer")
	}
	parts := strings.Split(value[1:], "/")
	if len(parts) > maxModelRequestDepth {
		return nil, errors.New("invalid JSON pointer")
	}
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' {
				j++
				if j == len(part) || part[j] != '0' && part[j] != '1' {
					return nil, errors.New("invalid JSON pointer")
				}
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}
