package giztools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestBuildHTTPRequestMapsQueryAndNestedJSONBody(t *testing.T) {
	t.Parallel()
	request, err := BuildHTTPRequest(context.Background(), HTTPOperation{
		URL:    "https://example.com/search?fixed=yes",
		Method: http.MethodPost,
		Headers: map[string]string{
			"X-Product": "kids",
		},
		Query: []HTTPBinding{{ArgumentPointer: "/query", Target: "q", Required: true}},
		Body: []HTTPBinding{
			{ArgumentPointer: "/age", Target: "/learner/age", Required: true},
			{ArgumentPointer: "/missing", Target: "/optional", Required: false},
		},
	}, json.RawMessage(`{"query":"天气 & news","age":7}`))
	if err != nil {
		t.Fatalf("BuildHTTPRequest(): %v", err)
	}
	if request.URL.RawQuery != "fixed=yes&q=%E5%A4%A9%E6%B0%94+%26+news" {
		t.Fatalf("RawQuery = %q", request.URL.RawQuery)
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"learner":{"age":7}}` {
		t.Fatalf("body = %s", body)
	}
	if request.Header.Get("X-Product") != "kids" || request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers = %#v", request.Header)
	}
}

func TestBuildHTTPRequestRejectsUnsafeOperationAndOversizedArguments(t *testing.T) {
	t.Parallel()
	base := HTTPOperation{URL: "https://example.com", Method: http.MethodGet}
	tests := []struct {
		name      string
		operation HTTPOperation
		args      json.RawMessage
	}{
		{name: "plain HTTP", operation: HTTPOperation{URL: "http://example.com", Method: http.MethodGet}, args: json.RawMessage(`{}`)},
		{name: "userinfo", operation: HTTPOperation{URL: "https://user@example.com", Method: http.MethodGet}, args: json.RawMessage(`{}`)},
		{name: "fragment", operation: HTTPOperation{URL: "https://example.com/path#fragment", Method: http.MethodGet}, args: json.RawMessage(`{}`)},
		{name: "unsupported method", operation: HTTPOperation{URL: "https://example.com", Method: http.MethodDelete}, args: json.RawMessage(`{}`)},
		{
			name: "oversized arguments", operation: base,
			args: json.RawMessage(`{"value":"` + strings.Repeat("x", MaxRequestBytes) + `"}`),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := BuildHTTPRequest(t.Context(), test.operation, test.args); err == nil {
				t.Fatal("BuildHTTPRequest() error = nil")
			}
		})
	}
}

func TestHTTPExecutorRevalidatesOperationAfterAuthorization(t *testing.T) {
	t.Parallel()
	operation := HTTPOperation{
		URL: "https://example.com", Method: http.MethodGet,
		Timeout: time.Second, MaxResponseBytes: 1024,
	}
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport called after authorizer changed URL to HTTP")
		return nil, nil
	})
	authorizer := HTTPAuthorizerFunc(func(_ context.Context, request *http.Request) error {
		request.URL.Scheme = "http"
		return nil
	})
	if _, err := (HTTPExecutor{Transport: transport}).Invoke(
		t.Context(), operation, json.RawMessage(`{}`), authorizer,
	); err == nil {
		t.Fatal("Invoke() error = nil")
	}
}

func TestHTTPExecutorRejectsUnsafeExecutionLimits(t *testing.T) {
	t.Parallel()
	base := HTTPOperation{
		URL: "https://example.com", Method: http.MethodGet,
		Timeout: time.Second, MaxResponseBytes: 1024,
	}
	tests := []HTTPOperation{base, base}
	tests[0].Timeout = MaxHTTPTimeout + time.Nanosecond
	tests[1].MaxResponseBytes = MaxHTTPResponseBytes + 1
	for _, operation := range tests {
		if _, err := (HTTPExecutor{}).Invoke(t.Context(), operation, json.RawMessage(`{}`), nil); err == nil {
			t.Fatal("Invoke() accepted an unsafe execution limit")
		}
	}
}

func TestHTTPExecutorAuthorizesExtractsAndBoundsResponse(t *testing.T) {
	t.Parallel()
	executor := HTTPExecutor{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":{"temperature":21}}`)),
		}, nil
	})}
	pointer := "/data"
	result, err := executor.Invoke(context.Background(), HTTPOperation{
		URL:                "https://example.com/weather",
		Method:             http.MethodGet,
		ResponsePointer:    &pointer,
		SuccessStatusCodes: []int{http.StatusOK},
		Timeout:            time.Second,
		MaxResponseBytes:   128,
	}, nil, HTTPAuthorizerFunc(func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer secret")
		return nil
	}))
	if err != nil {
		t.Fatalf("Invoke(): %v", err)
	}
	if string(result) != `{"temperature":21}` {
		t.Fatalf("result = %s", result)
	}

	executor.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"too":"large"}`)),
		}, nil
	})
	if _, err := executor.Invoke(context.Background(), HTTPOperation{
		URL:              "https://example.com",
		Method:           http.MethodGet,
		Timeout:          time.Second,
		MaxResponseBytes: 4,
	}, nil, nil); err == nil {
		t.Fatal("oversized response succeeded")
	}
}

func TestHTTPExecutorRejectsStatusContentTypeInvalidJSONAndMissingPointer(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		status  int
		content string
		body    string
		pointer *string
	}{
		{name: "status", status: 503, content: "application/json", body: `{"secret":"must not leak"}`},
		{name: "content type", status: 200, content: "text/plain", body: `{}`},
		{name: "invalid JSON", status: 200, content: "application/json", body: `{`},
		{name: "missing pointer", status: 200, content: "application/json", body: `{}`, pointer: new("/missing")},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			executor := HTTPExecutor{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: testCase.status,
					Header:     http.Header{"Content-Type": []string{testCase.content}},
					Body:       io.NopCloser(strings.NewReader(testCase.body)),
				}, nil
			})}
			_, err := executor.Invoke(context.Background(), HTTPOperation{
				URL:              "https://example.com",
				Method:           http.MethodGet,
				ResponsePointer:  testCase.pointer,
				Timeout:          time.Second,
				MaxResponseBytes: 1024,
			}, nil, nil)
			if err == nil {
				t.Fatal("Invoke() succeeded")
			}
			if strings.Contains(err.Error(), "must not leak") {
				t.Fatalf("response body leaked in error: %v", err)
			}
		})
	}
}

func TestBuildHTTPRequestRejectsMissingAndNonScalarQuery(t *testing.T) {
	t.Parallel()
	operation := HTTPOperation{
		URL:    "https://example.com",
		Method: http.MethodGet,
		Query:  []HTTPBinding{{ArgumentPointer: "/query", Target: "q", Required: true}},
	}
	for _, args := range []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{"query":{"nested":true}}`)} {
		if _, err := BuildHTTPRequest(context.Background(), operation, args); err == nil {
			t.Fatalf("BuildHTTPRequest(%s) succeeded", args)
		}
	}
}

func TestHTTPExecutorPreservesCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor := HTTPExecutor{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}
	_, err := executor.Invoke(ctx, HTTPOperation{
		URL:              "https://example.com",
		Method:           http.MethodGet,
		Timeout:          time.Second,
		MaxResponseBytes: 1024,
	}, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func TestEgressPolicyRejectsNonPublicAddresses(t *testing.T) {
	t.Parallel()
	policy := EgressPolicy{}
	for _, address := range []string{
		"127.0.0.1",
		"10.0.0.1",
		"169.254.1.1",
		"224.0.0.1",
		"100.64.0.1",
		"::1",
		"::ffff:127.0.0.1",
		"::ffff:10.0.0.1",
	} {
		ip := netip.MustParseAddr(address)
		if err := policy.allow("example.com", ip); err == nil {
			t.Fatalf("allow(%s) succeeded", address)
		}
	}
	if err := policy.allow("example.com", netip.MustParseAddr("8.8.8.8")); err != nil {
		t.Fatalf("allow(public) = %v", err)
	}
	if err := policy.allow("example.com", netip.MustParseAddr("::ffff:8.8.8.8")); err != nil {
		t.Fatalf("allow(mapped public) = %v", err)
	}
}
