package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxMem0ResponseBytes = 8 << 20

// HTTPClient is the transport boundary shared by remote memory providers.
type HTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

type mem0Client struct {
	endpoint string
	apiKey   string
	flavor   Mem0Flavor
	client   HTTPClient
}

func newMem0Client(endpoint, apiKey string, flavor Mem0Flavor, client HTTPClient) (*mem0Client, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: mem0 endpoint must be an absolute URL", ErrInvalidInput)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: mem0 endpoint scheme must be http or https", ErrInvalidInput)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: mem0 endpoint must not contain credentials, query, or fragment", ErrInvalidInput)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &mem0Client{endpoint: endpoint, apiKey: apiKey, flavor: flavor, client: client}, nil
}

func (c *mem0Client) do(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("mem0 encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return fmt.Errorf("mem0 create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		if c.flavor == Mem0SelfHosted {
			request.Header.Set("X-API-Key", c.apiKey)
		} else {
			request.Header.Set("Authorization", "Token "+c.apiKey)
		}
	}
	response, err := c.client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: mem0 request: %v", ErrUnavailable, redactSecret(err.Error(), c.apiKey))
	}
	if response == nil || response.Body == nil {
		return fmt.Errorf("%w: mem0 response has no body", ErrUnavailable)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxMem0ResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("%w: mem0 read response", ErrUnavailable)
	}
	if len(raw) > maxMem0ResponseBytes {
		return fmt.Errorf("%w: mem0 response exceeds %d bytes", ErrUnavailable, maxMem0ResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return mapMem0Status(response.StatusCode)
	}
	if responseBody == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, responseBody); err != nil {
		return fmt.Errorf("%w: mem0 decode response", ErrUnavailable)
	}
	return nil
}

func mapMem0Status(status int) error {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: mem0 returned HTTP %d", ErrInvalidInput, status)
	case http.StatusNotFound:
		return fmt.Errorf("%w: mem0 returned HTTP %d", ErrNotFound, status)
	case http.StatusConflict, http.StatusPreconditionFailed:
		return fmt.Errorf("%w: mem0 returned HTTP %d", ErrConflict, status)
	case http.StatusNotImplemented:
		return fmt.Errorf("%w: mem0 returned HTTP %d", ErrUnsupported, status)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: mem0 returned HTTP %d", ErrUnavailable, status)
	default:
		if status == http.StatusTooManyRequests || status >= 500 {
			return fmt.Errorf("%w: mem0 returned HTTP %d", ErrUnavailable, status)
		}
		return fmt.Errorf("mem0 returned HTTP %d", status)
	}
}

func redactSecret(value, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[redacted]")
}

func truncate(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[:length]
}
