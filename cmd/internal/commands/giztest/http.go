package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxHTTPStepBodyBytes = 4 << 20

type httpStepResult struct {
	body     any
	evidence map[string]any
}

// httpBaseURL turns a client access point (host:port or URL) into the Public
// HTTP origin the Server or Edge serves /gizclaw/v1 on.
func httpBaseURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid access point %q", endpoint)
	}
	parsed.Path, parsed.RawQuery, parsed.Fragment = "", "", ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func invokeHTTP(ctx context.Context, endpoint string, step giztest.Step, vars *giztest.Variables) (httpStepResult, error) {
	base, err := httpBaseURL(endpoint)
	if err != nil {
		return httpStepResult{}, err
	}
	pathValue, err := vars.Resolve(step.HTTP.Path)
	if err != nil {
		return httpStepResult{}, err
	}
	path, ok := pathValue.(string)
	if !ok || !strings.HasPrefix(path, "/") {
		return httpStepResult{}, fmt.Errorf("http path must resolve to an absolute path")
	}
	var body io.Reader
	if step.HTTP.Body != nil {
		resolved, err := vars.Resolve(step.HTTP.Body)
		if err != nil {
			return httpStepResult{}, err
		}
		encoded, err := json.Marshal(resolved)
		if err != nil {
			return httpStepResult{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, step.HTTP.Method, base+path, body)
	if err != nil {
		return httpStepResult{}, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, raw := range step.HTTP.Headers {
		resolved, err := vars.Resolve(raw)
		if err != nil {
			return httpStepResult{}, fmt.Errorf("header %s: %w", name, err)
		}
		value, ok := resolved.(string)
		if !ok {
			return httpStepResult{}, fmt.Errorf("header %s must resolve to string", name)
		}
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return httpStepResult{}, err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPStepBodyBytes+1))
	if err != nil {
		return httpStepResult{}, err
	}
	if len(raw) > maxHTTPStepBodyBytes {
		return httpStepResult{}, fmt.Errorf("http response body exceeds %d bytes", maxHTTPStepBodyBytes)
	}
	result := httpStepResult{evidence: map[string]any{"method": step.HTTP.Method, "path": path, "status": response.StatusCode}}
	if len(bytes.TrimSpace(raw)) > 0 {
		var decoded any
		if json.Unmarshal(raw, &decoded) == nil {
			result.body = decoded
		} else {
			result.body = string(raw)
		}
	}
	if step.HTTP.Status != 0 && response.StatusCode != step.HTTP.Status {
		return result, giztest.NewAssertionError(fmt.Errorf("http status = %d, want %d", response.StatusCode, step.HTTP.Status))
	}
	if step.HTTP.Status == 0 && response.StatusCode >= http.StatusBadRequest {
		return result, giztest.NewAssertionError(fmt.Errorf("http status = %d", response.StatusCode))
	}
	return result, nil
}
