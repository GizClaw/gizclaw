package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	MaxHTTPTimeout       = 60 * time.Second
	MaxHTTPResponseBytes = 4 << 20
)

var portableToolName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

var deniedCredentialHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"host":                true,
	"cookie":              true,
	"set-cookie":          true,
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"forwarded":           true,
	"x-forwarded-for":     true,
	"x-forwarded-host":    true,
	"x-forwarded-proto":   true,
}

func NormalizeTool(tool Tool) (Tool, error) {
	return normalizeTool(tool, true)
}

func normalizeToolDeclaration(tool Tool) (Tool, error) {
	return normalizeTool(tool, false)
}

func normalizeTool(tool Tool, requireDirectSecrets bool) (Tool, error) {
	var err error
	tool.InvokeName, err = normalizeToolName(tool.InvokeName)
	if err != nil {
		return Tool{}, err
	}
	tool.Description = normalizedStringPtr(tool.Description)
	tool.Version = normalizedStringPtr(tool.Version)
	switch tool.Type {
	case ToolTypeHTTPRequest:
		if tool.HTTP == nil {
			return Tool{}, fmt.Errorf("%w: http is required for type %q", ErrInvalidTool, tool.Type)
		}
		if err := normalizeHTTPRequest(tool.HTTP, requireDirectSecrets); err != nil {
			return Tool{}, err
		}
	case ToolTypeClientRPC:
		if tool.HTTP != nil {
			return Tool{}, fmt.Errorf("%w: http is forbidden for type %q", ErrInvalidTool, tool.Type)
		}
	default:
		return Tool{}, fmt.Errorf("%w: unsupported type %q", ErrInvalidTool, tool.Type)
	}
	if err := validateInputSchema(tool.InputSchema.Type, tool.InputSchema.Types); err != nil {
		return Tool{}, err
	}
	if _, err := tool.InputSchema.Resolve(nil); err != nil {
		return Tool{}, fmt.Errorf("%w: resolve input_schema: %v", ErrInvalidTool, err)
	}
	if len(tool.Metadata) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal(tool.Metadata, &metadata); err != nil {
			return Tool{}, fmt.Errorf("%w: metadata must be a JSON object", ErrInvalidTool)
		}
	}
	if err := validateTriggers(tool.Triggers); err != nil {
		return Tool{}, err
	}
	return cloneTool(tool), nil
}

func normalizeToolName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: invoke_name is required", ErrInvalidTool)
	}
	if strings.TrimSpace(name) != name {
		return "", fmt.Errorf("%w: invoke_name must not have surrounding whitespace", ErrInvalidTool)
	}
	if !portableToolName.MatchString(name) {
		return "", fmt.Errorf("%w: invoke_name must match %s", ErrInvalidTool, portableToolName)
	}
	return name, nil
}

func validateInputSchema(single string, many []string) error {
	if single == "object" && len(many) == 0 {
		return nil
	}
	if single == "" && len(many) == 1 && many[0] == "object" {
		return nil
	}
	if single == "" && len(many) == 0 {
		return fmt.Errorf("%w: input_schema type is required and must be object", ErrInvalidTool)
	}
	return fmt.Errorf("%w: input_schema type must be object", ErrInvalidTool)
}

func validateToolArgs(tool Tool, args json.RawMessage) error {
	args = normalizeToolArgs(args)
	var value any
	decoder := json.NewDecoder(bytes.NewReader(args))
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: tool arguments must be valid JSON: %v", ErrInvalidTool, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: tool arguments must contain exactly one JSON value", ErrInvalidTool)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("%w: tool arguments must be a JSON object", ErrInvalidTool)
	}
	resolved, err := tool.InputSchema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("%w: resolve input_schema: %v", ErrInvalidTool, err)
	}
	if err := resolved.Validate(value); err != nil {
		return fmt.Errorf("%w: tool arguments do not match input_schema: %v", ErrInvalidTool, err)
	}
	return nil
}

func normalizeToolArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage(`{}`)
	}
	return args
}

func normalizeHTTPRequest(config *HTTPRequest, requireDirectSecrets bool) error {
	parsed, err := url.Parse(config.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: http.url must be an absolute HTTPS URL without userinfo or fragment", ErrInvalidTool)
	}
	if parsed.RawQuery != "" {
		for key := range parsed.Query() {
			if isCredentialFieldName(key) {
				return fmt.Errorf("%w: http.url query must not contain credentials", ErrInvalidTool)
			}
		}
	}
	config.URL = parsed.String()
	config.Method = strings.ToUpper(strings.TrimSpace(config.Method))
	if config.Method != http.MethodGet && config.Method != http.MethodPost {
		return fmt.Errorf("%w: http.method must be GET or POST", ErrInvalidTool)
	}
	if config.Method == http.MethodGet && len(config.Body) > 0 {
		return fmt.Errorf("%w: GET must not declare body bindings", ErrInvalidTool)
	}
	if config.Timeout <= 0 || config.Timeout > MaxHTTPTimeout {
		return fmt.Errorf("%w: http.timeout must be within (0, %s]", ErrInvalidTool, MaxHTTPTimeout)
	}
	if config.MaxResponseBytes <= 0 || config.MaxResponseBytes > MaxHTTPResponseBytes {
		return fmt.Errorf("%w: http.max_response_bytes must be within (0, %d]", ErrInvalidTool, MaxHTTPResponseBytes)
	}
	if err := normalizeHTTPHeaders(config.Headers); err != nil {
		return err
	}
	if err := validateHTTPBindings(config.Query, false); err != nil {
		return fmt.Errorf("%w: http.query: %v", ErrInvalidTool, err)
	}
	if err := validateHTTPBindings(config.Body, true); err != nil {
		return fmt.Errorf("%w: http.body: %v", ErrInvalidTool, err)
	}
	if config.ResponsePointer != nil {
		value := strings.TrimSpace(*config.ResponsePointer)
		if err := validateJSONPointer(value); err != nil {
			return fmt.Errorf("%w: http.response_pointer: %v", ErrInvalidTool, err)
		}
		config.ResponsePointer = &value
	}
	if len(config.SuccessStatusCodes) == 0 {
		config.SuccessStatusCodes = []int{http.StatusOK}
	} else {
		slices.Sort(config.SuccessStatusCodes)
		config.SuccessStatusCodes = slices.Compact(config.SuccessStatusCodes)
		for _, status := range config.SuccessStatusCodes {
			if status < 200 || status > 299 {
				return fmt.Errorf("%w: http.success_status_codes must contain only 2xx codes", ErrInvalidTool)
			}
		}
	}
	return normalizeHTTPAuth(&config.Auth, requireDirectSecrets)
}

func normalizeHTTPHeaders(headers map[string]string) error {
	for name, value := range headers {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if canonical == "" || !validHeaderName(canonical) {
			return fmt.Errorf("%w: invalid fixed HTTP header %q", ErrInvalidTool, name)
		}
		if deniedCredentialHeaders[strings.ToLower(canonical)] || isCredentialFieldName(canonical) {
			return fmt.Errorf("%w: fixed HTTP header %q is denied", ErrInvalidTool, name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%w: fixed HTTP header %q contains a newline", ErrInvalidTool, name)
		}
		if canonical != name {
			delete(headers, name)
			headers[canonical] = value
		}
	}
	return nil
}

func isCredentialFieldName(name string) bool {
	compact := strings.Map(func(character rune) rune {
		switch {
		case character >= 'A' && character <= 'Z':
			return character + ('a' - 'A')
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			return character
		default:
			return -1
		}
	}, name)
	if compact == "key" {
		return true
	}
	for _, marker := range []string{
		"apikey",
		"accesskey",
		"privatekey",
		"token",
		"secret",
		"password",
		"credential",
		"signature",
		"bearer",
	} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	return deniedCredentialHeaders[strings.ToLower(strings.TrimSpace(name))]
}

func validateHTTPBindings(bindings []HTTPArgumentBinding, body bool) error {
	targets := make(map[string]bool, len(bindings))
	for i := range bindings {
		binding := &bindings[i]
		if err := validateJSONPointer(binding.ArgumentPointer); err != nil {
			return fmt.Errorf("binding %d argument_pointer: %w", i, err)
		}
		binding.Target = strings.TrimSpace(binding.Target)
		if binding.Target == "" {
			return fmt.Errorf("binding %d target is required", i)
		}
		if body {
			if err := validateJSONPointer(binding.Target); err != nil {
				return fmt.Errorf("binding %d target: %w", i, err)
			}
		} else if strings.ContainsAny(binding.Target, "&=?#") {
			return fmt.Errorf("binding %d query target is invalid", i)
		}
		if targets[binding.Target] {
			return fmt.Errorf("binding %d duplicates target %q", i, binding.Target)
		}
		targets[binding.Target] = true
	}
	return nil
}

func validateJSONPointer(pointer string) error {
	if pointer == "" {
		return nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return fmt.Errorf("must be an RFC 6901 JSON Pointer")
	}
	for i := 0; i < len(pointer); i++ {
		if pointer[i] != '~' {
			continue
		}
		if i+1 >= len(pointer) || (pointer[i+1] != '0' && pointer[i+1] != '1') {
			return fmt.Errorf("contains an invalid escape")
		}
		i++
	}
	return nil
}

func normalizeHTTPAuth(auth *HTTPAuth, requireDirectSecrets bool) error {
	auth.Method = strings.TrimSpace(auth.Method)
	auth.BearerToken = normalizedStringPtr(auth.BearerToken)
	auth.Header = normalizedStringPtr(auth.Header)
	auth.APIKey = normalizedStringPtr(auth.APIKey)
	auth.Credential = normalizedStringPtr(auth.Credential)
	auth.Region = normalizedStringPtr(auth.Region)
	auth.Service = normalizedStringPtr(auth.Service)
	auth.Action = normalizedStringPtr(auth.Action)
	auth.Version = normalizedStringPtr(auth.Version)
	switch auth.Method {
	case "none":
		if httpAuthFieldCount(*auth) != 0 {
			return fmt.Errorf("%w: auth method none forbids additional fields", ErrInvalidTool)
		}
	case "bearer":
		count := httpAuthFieldCount(*auth)
		if (auth.BearerToken == nil && count != 0) || (auth.BearerToken != nil && count != 1) ||
			(requireDirectSecrets && auth.BearerToken == nil) {
			return fmt.Errorf("%w: bearer auth requires only bearer_token", ErrInvalidTool)
		}
	case "header_api_key":
		count := httpAuthFieldCount(*auth)
		if auth.Header == nil || (auth.APIKey == nil && count != 1) || (auth.APIKey != nil && count != 2) ||
			(requireDirectSecrets && auth.APIKey == nil) {
			return fmt.Errorf("%w: header_api_key auth requires only header and api_key", ErrInvalidTool)
		}
		header := http.CanonicalHeaderKey(*auth.Header)
		if !validHeaderName(header) || deniedCredentialHeaders[strings.ToLower(header)] {
			return fmt.Errorf("%w: credential header %q is denied", ErrInvalidTool, *auth.Header)
		}
		*auth.Header = header
	case "volc_ark", "volc_search", "aliyun_app_code":
		if auth.Credential == nil || httpAuthFieldCount(*auth) != 1 {
			return fmt.Errorf("%w: auth method %s requires only credential", ErrInvalidTool, auth.Method)
		}
	case "volc_openapi":
		if auth.Credential == nil || auth.Region == nil || auth.Service == nil || httpAuthFieldCount(*auth) != 3 {
			return fmt.Errorf("%w: volc_openapi auth requires only credential, region, and service", ErrInvalidTool)
		}
	case "aliyun_openapi_v3":
		if auth.Credential == nil || auth.Action == nil || auth.Version == nil || httpAuthFieldCount(*auth) != 3 {
			return fmt.Errorf("%w: aliyun_openapi_v3 auth requires only credential, action, and version", ErrInvalidTool)
		}
	default:
		return fmt.Errorf("%w: unsupported HTTP auth method %q", ErrInvalidTool, auth.Method)
	}
	return nil
}

func httpAuthFieldCount(auth HTTPAuth) int {
	fields := []*string{auth.BearerToken, auth.Header, auth.APIKey, auth.Credential, auth.Region, auth.Service, auth.Action, auth.Version}
	count := 0
	for _, field := range fields {
		if field != nil {
			count++
		}
	}
	return count
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={} \t", r) || r <= 31 || r == 127 {
			return false
		}
	}
	return true
}

func validateTriggers(triggers []ToolTrigger) error {
	for i, trigger := range triggers {
		if strings.TrimSpace(trigger.Name) == "" {
			return fmt.Errorf("%w: triggers[%d].name is required", ErrInvalidTool, i)
		}
		if len(trigger.Metadata) > 0 && !json.Valid(trigger.Metadata) {
			return fmt.Errorf("%w: triggers[%d].metadata must be valid JSON", ErrInvalidTool, i)
		}
		for j, example := range trigger.Examples {
			if strings.TrimSpace(example.Input) == "" {
				return fmt.Errorf("%w: triggers[%d].examples[%d].input is required", ErrInvalidTool, i, j)
			}
			if len(example.Args) > 0 && !json.Valid(example.Args) {
				return fmt.Errorf("%w: triggers[%d].examples[%d].args must be valid JSON", ErrInvalidTool, i, j)
			}
		}
	}
	return nil
}

func normalizedStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}
