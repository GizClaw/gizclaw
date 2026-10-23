package apitypes

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

const (
	maxModelRequestBindings   = 16
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
	if len(*bindings) > maxModelRequestBindings {
		return errors.New("too many request_bindings")
	}
	names := make(map[string]bool, len(*bindings))
	for i, binding := range *bindings {
		if binding.Target.Kind != ModelRequestTargetKindHttpHeader || !modelRequestHeaderNameAllowed(binding.Target.Name) {
			return fmt.Errorf("request_bindings[%d]: invalid or reserved target", i)
		}
		name := strings.ToLower(binding.Target.Name)
		if names[name] {
			return fmt.Errorf("request_bindings[%d]: duplicate header target", i)
		}
		names[name] = true
		if binding.Source.Kind != ModelRequestSourceKindPeerRuntimeKv || binding.Source.Key == "" ||
			len(binding.Source.Key) > maxModelRequestKeyBytes || !utf8.ValidString(binding.Source.Key) {
			return fmt.Errorf("request_bindings[%d]: invalid source", i)
		}
		if _, err := ModelRequestPointerSegments(binding.Source.Pointer); err != nil {
			return fmt.Errorf("request_bindings[%d]: invalid JSON pointer", i)
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

// ValidateModelRequestHeader validates resolved non-secret business metadata.
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
