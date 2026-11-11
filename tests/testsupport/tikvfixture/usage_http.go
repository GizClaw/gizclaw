package tikvfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// usageHTTP holds trusted origins installed by Go tests, never by a control
// request. A nil receiver selects the fixed private Docker fixture topology.
// Keep origins separate from request paths so inbound URLs never reach a dial.
type usageHTTP struct {
	edges    []string
	controls []string
}

var usageHTTPClient = &http.Client{
	Transport: func() *http.Transport {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		return transport
	}(),
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (g *UsageGate) http() *usageHTTP {
	if services := g.services.Load(); services != nil {
		return services.http
	}
	return nil
}

func (h *usageHTTP) origin(endpoint string) (origin string, control bool, err error) {
	if h == nil {
		// These origins match docker-compose.multi-server.yaml. Return trusted
		// literals rather than passing even a validated inbound URL to net/http.
		switch endpoint {
		case "http://server-a:9791":
			return "http://server-a:9791", true, nil
		case "http://server-b:9791":
			return "http://server-b:9791", true, nil
		case "http://edge-a:9821":
			return "http://edge-a:9821", false, nil
		case "http://edge-b:9821":
			return "http://edge-b:9821", false, nil
		}
	} else {
		for _, origin := range h.controls {
			if endpoint == origin {
				return origin, true, nil
			}
		}
		for _, origin := range h.edges {
			if endpoint == origin {
				return origin, false, nil
			}
		}
	}
	return "", false, errors.New("HTTP endpoint must be a trusted TiKV usage fixture origin")
}

func usageHTTPPath(path string, control bool) error {
	if control {
		switch path {
		case "/arm", "/invoke", "/after", "/race", "/wait", "/release":
			return nil
		default:
			return errors.New("HTTP path must be a TiKV usage coordination operation")
		}
	}
	// The seven usage documents exercise these product resources. Accept only
	// literal, unreserved path segments: no authority, query, fragment, encoded
	// separator, double decoding or traversal can change the destination/path.
	parts := strings.Split(path, "/")
	if len(parts) < 4 || len(parts) > 6 || parts[0] != "" || parts[1] != "gizclaw" || parts[2] != "v1" {
		return errors.New("HTTP path must be a fixture product resource")
	}
	for _, part := range parts[3:] {
		if part == "" || part == "." || part == ".." {
			return errors.New("HTTP path requires literal resource segments")
		}
		for _, char := range part {
			if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' || char == '~' {
				continue
			}
			return errors.New("HTTP path requires unescaped resource segments")
		}
	}
	switch parts[3] {
	case "api-keys", "friends":
		if len(parts) <= 5 {
			return nil
		}
	case "friend-groups":
		if len(parts) == 5 || len(parts) == 6 && parts[5] == "invite-token" {
			return nil
		}
	}
	return errors.New("HTTP path must be a fixture product resource")
}

func (h *usageHTTP) request(ctx context.Context, endpoint, method, path string, body io.Reader, controlOnly bool) (*http.Request, error) {
	origin, control, err := h.origin(endpoint)
	if err != nil {
		return nil, err
	}
	if method == "" || control && method != http.MethodPost || controlOnly && !control {
		return nil, errors.New("HTTP method and fixture destination do not match")
	}
	if err := usageHTTPPath(path, control); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, origin, body)
	if err != nil {
		return nil, err
	}
	req.URL.Path = path
	return req, nil
}

// forward is a test-owned envelope for a real HTTP response. It is used when a
// race may legitimately return different 4xx codes. Status/body are preserved,
// including redirects, which must never be followed to another destination.
func (h *usageHTTP) forward(ctx context.Context, action UsageAction) (any, error) {
	req, err := h.request(ctx, action.Endpoint, action.Method, action.Path, nil, false)
	if err != nil {
		return nil, err
	}
	if action.Body != nil {
		raw, err := json.Marshal(action.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
		req.ContentLength = int64(len(raw))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	}
	for name, value := range action.Headers {
		req.Header.Set(name, value)
	}
	if action.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := usageHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, errors.New("HTTP response exceeds 4 MiB")
	}
	var value any
	if len(raw) != 0 && json.Unmarshal(raw, &value) != nil {
		value = string(raw)
	}
	return map[string]any{"status": response.StatusCode, "body": value}, nil
}

// control uses a trusted private fixture origin without forwarding incoming
// cookies, credentials or headers. Redirects remain actual non-200 failures.
func (h *usageHTTP) control(ctx context.Context, endpoint, path string, input, output any) error {
	req, err := h.request(ctx, endpoint, http.MethodPost, path, nil, true)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.ContentLength = int64(len(raw))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	req.Header.Set("Content-Type", "application/json")
	response, err := usageHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("fixture %s: status %d: %s", path, response.StatusCode, text)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}
