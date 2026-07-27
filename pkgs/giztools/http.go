// Package giztools provides stateless execution helpers for Tool backends.
package giztools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	MaxRequestBytes      = 1 << 20
	MaxHTTPResponseBytes = 4 << 20
	MaxHTTPTimeout       = 60 * time.Second
)

type HTTPBinding struct {
	ArgumentPointer string
	Target          string
	Required        bool
}

type HTTPOperation struct {
	URL                string
	Method             string
	Headers            map[string]string
	Query              []HTTPBinding
	Body               []HTTPBinding
	ResponsePointer    *string
	SuccessStatusCodes []int
	Timeout            time.Duration
	MaxResponseBytes   int64
}

type HTTPAuthorizer interface {
	Authorize(context.Context, *http.Request) error
}

type HTTPAuthorizerFunc func(context.Context, *http.Request) error

func (f HTTPAuthorizerFunc) Authorize(ctx context.Context, request *http.Request) error {
	return f(ctx, request)
}

type EgressPolicy struct {
	AllowedHosts   map[string]bool
	DeniedPrefixes []netip.Prefix
}

type HTTPExecutor struct {
	Transport http.RoundTripper
	Policy    EgressPolicy
	Resolver  *net.Resolver
}

func (e HTTPExecutor) Invoke(
	ctx context.Context,
	operation HTTPOperation,
	args json.RawMessage,
	authorizer HTTPAuthorizer,
) (json.RawMessage, error) {
	if operation.Timeout <= 0 || operation.Timeout > MaxHTTPTimeout {
		return nil, fmt.Errorf("giztools: HTTP timeout must be within (0, %s]", MaxHTTPTimeout)
	}
	if operation.MaxResponseBytes <= 0 || operation.MaxResponseBytes > MaxHTTPResponseBytes {
		return nil, fmt.Errorf(
			"giztools: HTTP response limit must be within (0, %d]",
			MaxHTTPResponseBytes,
		)
	}
	request, err := BuildHTTPRequest(ctx, operation, args)
	if err != nil {
		return nil, err
	}
	if authorizer != nil {
		if err := authorizer.Authorize(ctx, request); err != nil {
			return nil, fmt.Errorf("giztools: authorize HTTP request: %w", err)
		}
	}
	if err := validateHTTPSURL(request.URL); err != nil {
		return nil, err
	}
	transport := e.Transport
	if transport == nil {
		transport = e.secureTransport()
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   operation.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("giztools: HTTP redirects are disabled")
		},
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("giztools: HTTP request timeout: %w", context.DeadlineExceeded)
		}
		return nil, fmt.Errorf("giztools: execute HTTP request: %w", err)
	}
	defer response.Body.Close()
	if !acceptedStatus(operation.SuccessStatusCodes, response.StatusCode) {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("giztools: HTTP response status %d is not accepted", response.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return nil, fmt.Errorf("giztools: HTTP response content type must be application/json")
	}
	limit := operation.MaxResponseBytes
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("giztools: read HTTP response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("giztools: HTTP response exceeds %d bytes", limit)
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, fmt.Errorf("giztools: HTTP response is invalid JSON: %w", err)
	}
	if operation.ResponsePointer != nil {
		value, err = valueAtPointer(value, *operation.ResponsePointer)
		if err != nil {
			return nil, fmt.Errorf("giztools: select HTTP response: %w", err)
		}
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("giztools: encode HTTP result: %w", err)
	}
	return json.RawMessage(result), nil
}

func BuildHTTPRequest(ctx context.Context, operation HTTPOperation, args json.RawMessage) (*http.Request, error) {
	switch operation.Method {
	case http.MethodGet, http.MethodPost:
	default:
		return nil, errors.New("giztools: HTTP method must be GET or POST")
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if len(args) > MaxRequestBytes {
		return nil, fmt.Errorf("giztools: Tool arguments exceed %d bytes", MaxRequestBytes)
	}
	var values any
	if err := json.Unmarshal(args, &values); err != nil {
		return nil, fmt.Errorf("giztools: decode Tool arguments: %w", err)
	}
	if _, ok := values.(map[string]any); !ok {
		return nil, errors.New("giztools: Tool arguments must be a JSON object")
	}
	targetURL, err := url.Parse(operation.URL)
	if err != nil {
		return nil, fmt.Errorf("giztools: parse HTTP URL: %w", err)
	}
	if err := validateHTTPSURL(targetURL); err != nil {
		return nil, err
	}
	query := targetURL.Query()
	for _, binding := range operation.Query {
		value, found, err := optionalValueAtPointer(values, binding.ArgumentPointer)
		if err != nil {
			return nil, fmt.Errorf("giztools: query binding %q: %w", binding.Target, err)
		}
		if !found {
			if binding.Required {
				return nil, fmt.Errorf("giztools: required argument %q is missing", binding.ArgumentPointer)
			}
			continue
		}
		scalar, err := queryScalar(value)
		if err != nil {
			return nil, fmt.Errorf("giztools: query binding %q: %w", binding.Target, err)
		}
		query.Set(binding.Target, scalar)
	}
	targetURL.RawQuery = query.Encode()
	if len(targetURL.RequestURI()) > MaxRequestBytes {
		return nil, fmt.Errorf("giztools: HTTP request target exceeds %d bytes", MaxRequestBytes)
	}
	var requestBody io.Reader
	if operation.Method == http.MethodPost {
		body := map[string]any{}
		for _, binding := range operation.Body {
			value, found, err := optionalValueAtPointer(values, binding.ArgumentPointer)
			if err != nil {
				return nil, fmt.Errorf("giztools: body binding %q: %w", binding.Target, err)
			}
			if !found {
				if binding.Required {
					return nil, fmt.Errorf("giztools: required argument %q is missing", binding.ArgumentPointer)
				}
				continue
			}
			if err := setValueAtPointer(body, binding.Target, value); err != nil {
				return nil, fmt.Errorf("giztools: body binding %q: %w", binding.Target, err)
			}
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("giztools: encode HTTP request body: %w", err)
		}
		if len(encoded) > MaxRequestBytes {
			return nil, fmt.Errorf("giztools: HTTP request exceeds %d bytes", MaxRequestBytes)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, operation.Method, targetURL.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("giztools: create HTTP request: %w", err)
	}
	for name, value := range operation.Headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Accept", "application/json")
	if operation.Method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}

func validateHTTPSURL(target *url.URL) error {
	if target == nil || target.Scheme != "https" || target.Hostname() == "" ||
		target.User != nil || target.Fragment != "" {
		return errors.New("giztools: HTTP URL must be absolute HTTPS without userinfo or fragment")
	}
	return nil
}

func (e HTTPExecutor) secureTransport() *http.Transport {
	resolver := e.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	policy := e.Policy
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("giztools: parse destination: %w", err)
			}
			ips, err := resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("giztools: resolve destination: %w", err)
			}
			if len(ips) == 0 {
				return nil, errors.New("giztools: destination has no IP addresses")
			}
			for _, ip := range ips {
				if err := policy.allow(host, ip); err != nil {
					return nil, err
				}
			}
			var lastErr error
			for _, ip := range ips {
				connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return connection, nil
				}
				lastErr = err
			}
			return nil, fmt.Errorf("giztools: connect destination: %w", lastErr)
		},
		ForceAttemptHTTP2: true,
	}
}

func (p EgressPolicy) allow(host string, ip netip.Addr) error {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	ip = ip.Unmap()
	if len(p.AllowedHosts) > 0 {
		allowed := false
		for configuredHost, configured := range p.AllowedHosts {
			if configured && strings.EqualFold(strings.TrimSuffix(configuredHost, "."), host) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("giztools: destination host %q is not allowed", host)
		}
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || carrierGradeNAT.Contains(ip) {
		return fmt.Errorf("giztools: destination address is denied")
	}
	for _, prefix := range p.DeniedPrefixes {
		if prefix.Contains(ip) {
			return fmt.Errorf("giztools: destination address is denied")
		}
	}
	return nil
}

var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

func acceptedStatus(configured []int, status int) bool {
	if len(configured) == 0 {
		return status == http.StatusOK
	}
	return slices.Contains(configured, status)
}

func queryScalar(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case nil:
		return "null", nil
	default:
		return "", errors.New("query value must be a string, number, boolean, or null")
	}
}

func optionalValueAtPointer(root any, pointer string) (any, bool, error) {
	if pointer == "" {
		return root, true, nil
	}
	segments, err := pointerSegments(pointer)
	if err != nil {
		return nil, false, err
	}
	current := root
	for _, segment := range segments {
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[segment]
			if !ok {
				return nil, false, nil
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(value) {
				return nil, false, nil
			}
			current = value[index]
		default:
			return nil, false, nil
		}
	}
	return current, true, nil
}

func valueAtPointer(root any, pointer string) (any, error) {
	value, found, err := optionalValueAtPointer(root, pointer)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("JSON Pointer %q was not found", pointer)
	}
	return value, nil
}

func setValueAtPointer(root map[string]any, pointer string, value any) error {
	segments, err := pointerSegments(pointer)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		return errors.New("body target must not be the document root")
	}
	current := root
	for _, segment := range segments[:len(segments)-1] {
		next, exists := current[segment]
		if !exists {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("target path collides at %q", segment)
		}
		current = child
	}
	last := segments[len(segments)-1]
	if _, exists := current[last]; exists {
		return fmt.Errorf("target %q is duplicated", pointer)
	}
	current[last] = value
	return nil
}

func pointerSegments(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("invalid RFC 6901 JSON Pointer")
	}
	raw := strings.Split(pointer[1:], "/")
	out := make([]string, len(raw))
	for i, segment := range raw {
		var builder strings.Builder
		for j := 0; j < len(segment); j++ {
			if segment[j] != '~' {
				builder.WriteByte(segment[j])
				continue
			}
			if j+1 >= len(segment) || (segment[j+1] != '0' && segment[j+1] != '1') {
				return nil, errors.New("invalid RFC 6901 escape")
			}
			if segment[j+1] == '0' {
				builder.WriteByte('~')
			} else {
				builder.WriteByte('/')
			}
			j++
		}
		out[i] = builder.String()
	}
	return out, nil
}
