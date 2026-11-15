// Package mcp provides a remote MCP client and shareable resource snapshots.
// Protocol types belong to github.com/modelcontextprotocol/go-sdk/mcp.
package mcp

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
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/net/http/httpguts"
)

var (
	// ErrClosed reports an operation on a closed remote client.
	ErrClosed = errors.New("mcp: client closed")
	// ErrUnsupported reports a capability not advertised by the server.
	ErrUnsupported = errors.New("mcp: unsupported capability")
	// ErrResponseTooLarge reports a response exceeding the configured limit.
	ErrResponseTooLarge = errors.New("mcp: response too large")
)

// Config configures one endpoint and authentication context. Each Client owns
// its session; share the Client and its ResourceCache only within that context.
type Config struct {
	Endpoint string
	// Headers are copied and applied to every request, including session teardown.
	// MCP protocol headers cannot be overridden. Redirects are disabled.
	Headers http.Header
	// HTTPClient is copied; its Transport remains caller-owned. A zero Timeout
	// allows the SDK's long-lived notification stream to stay open.
	HTTPClient *http.Client
	// RequestTimeout bounds initialization and each complete operation, including
	// pagination. Zero selects thirty seconds.
	RequestTimeout time.Duration
	// MaxResponseBytes bounds JSON responses, individual SSE events, and complete
	// paginated lists. Zero selects four MiB.
	MaxResponseBytes int64
	// MaxPages bounds list pagination. Zero selects 128 pages.
	MaxPages int
}

// Client is safe for concurrent use. Tool calls and resource reads use the
// official SDK; resource caching is explicitly owned by ResourceCache.
type Client struct {
	session   *mcp.ClientSession
	timeout   time.Duration
	maxBytes  int64
	maxPages  int
	tools     bool
	resources bool
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
}

// Dial connects to a Streamable HTTP endpoint and negotiates its capabilities.
// The caller must Close the returned Client. Canceling ctx after Dial returns
// does not close the session; individual operations use their own contexts.
func Dial(ctx context.Context, config Config) (*Client, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint == nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, errors.New("mcp: endpoint must be an absolute HTTP(S) URL without userinfo or fragment")
	}
	if config.RequestTimeout < 0 || config.MaxResponseBytes < 0 || config.MaxPages < 0 {
		return nil, errors.New("mcp: timeout and limits must not be negative")
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = 4 << 20
	}
	if config.MaxResponseBytes > int64(^uint(0)>>1)-1 {
		return nil, errors.New("mcp: response limit exceeds platform integer range")
	}
	if config.MaxPages == 0 {
		config.MaxPages = 128
	}
	headers := config.Headers.Clone()
	for name, values := range headers {
		lower := strings.ToLower(name)
		if !httpguts.ValidHeaderFieldName(name) || lower == "host" || lower == "content-type" || lower == "accept" || strings.HasPrefix(lower, "mcp-") {
			return nil, errors.New("mcp: invalid or reserved HTTP header")
		}
		for _, value := range values {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, errors.New("mcp: invalid HTTP header value")
			}
		}
	}
	httpClient := http.Client{}
	if config.HTTPClient != nil {
		httpClient = *config.HTTPClient
	}
	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	httpClient.Transport = &httpTransport{base: transport, headers: headers, maxBytes: config.MaxResponseBytes}
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	life, cancel := context.WithCancel(context.WithoutCancel(ctx))
	client := &Client{timeout: config.RequestTimeout, maxBytes: config.MaxResponseBytes, maxPages: config.MaxPages, ctx: life, cancel: cancel}
	requestCtx, stop := client.operationContext(ctx)
	defer stop()
	sdk := mcp.NewClient(&mcp.Implementation{Name: "gizclaw", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := sdk.Connect(requestCtx, &mcp.StreamableClientTransport{
		Endpoint: config.Endpoint, HTTPClient: &httpClient, MaxEventSize: int(config.MaxResponseBytes),
	}, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("mcp: connect: %w", err)
	}
	client.session = session
	if capabilities := session.InitializeResult().Capabilities; capabilities != nil {
		client.tools = capabilities.Tools != nil
		client.resources = capabilities.Resources != nil
	}
	return client, nil
}

// Close cancels outstanding operations and closes the MCP session exactly once.
// Caller-owned HTTP transports and previously acquired snapshots remain usable.
func (c *Client) Close() error {
	c.closeOnce.Do(func() { c.cancel(); c.closeErr = c.session.Close() })
	return c.closeErr
}

func (c *Client) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	stop := context.AfterFunc(c.ctx, cancel)
	return requestCtx, func() { stop(); cancel() }
}

func (c *Client) check(ctx context.Context, supported bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.ctx.Err() != nil {
		return ErrClosed
	}
	if !supported {
		return ErrUnsupported
	}
	return nil
}

// ListTools returns every page of tool declarations. It does not cache tool
// execution results; MCP server cache hints are handled by the SDK.
func (c *Client) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	if err := c.check(ctx, c.tools); err != nil {
		return nil, err
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	return collectPages(ctx, c.maxPages, c.maxBytes, func(cursor string) ([]*mcp.Tool, string, error) {
		result, err := c.session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, "", fmt.Errorf("mcp: list tools: %w", err)
		}
		return result.Tools, result.NextCursor, nil
	})
}

// CallTool invokes a tool once. Arguments must be a JSON object; empty arguments
// mean {}. A tool execution failure remains a CallToolResult with IsError=true;
// transport and protocol failures are returned as Go errors. Calls are not retried
// by this package, because they may have side effects.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*mcp.CallToolResult, error) {
	if err := c.check(ctx, c.tools); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("mcp: tool name is required")
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if int64(len(arguments)) > c.maxBytes {
		return nil, errors.New("mcp: tool arguments exceed the response byte limit")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil || object == nil {
		return nil, errors.New("mcp: tool arguments must be a JSON object")
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("mcp: call tool %q: %w", name, err)
	}
	return result, nil
}

// ListResources returns every page of resource metadata. For local reads, use a
// shared ResourceCache. SDK-negotiated server cache hints remain effective.
func (c *Client) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	if err := c.check(ctx, c.resources); err != nil {
		return nil, err
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	return collectPages(ctx, c.maxPages, c.maxBytes, func(cursor string) ([]*mcp.Resource, string, error) {
		result, err := c.session.ListResources(ctx, &mcp.ListResourcesParams{Cursor: cursor})
		if err != nil {
			return nil, "", fmt.Errorf("mcp: list resources: %w", err)
		}
		return result.Resources, result.NextCursor, nil
	})
}

// ListResourceTemplates returns every page of parameterized resource templates.
func (c *Client) ListResourceTemplates(ctx context.Context) ([]*mcp.ResourceTemplate, error) {
	if err := c.check(ctx, c.resources); err != nil {
		return nil, err
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	return collectPages(ctx, c.maxPages, c.maxBytes, func(cursor string) ([]*mcp.ResourceTemplate, string, error) {
		result, err := c.session.ListResourceTemplates(ctx, &mcp.ListResourceTemplatesParams{Cursor: cursor})
		if err != nil {
			return nil, "", fmt.Errorf("mcp: list resource templates: %w", err)
		}
		return result.ResourceTemplates, result.NextCursor, nil
	})
}

// ReadResource reads an opaque absolute resource URI through MCP. It never
// fetches the URI directly. Text, binary contents and metadata retain SDK types.
func (c *Client) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	if err := c.check(ctx, c.resources); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(uri)
	if err != nil || !parsed.IsAbs() {
		return nil, errors.New("mcp: resource URI must be absolute")
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	result, err := c.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("mcp: read resource %q: %w", uri, err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("mcp: encode resource response: %w", err)
	}
	if int64(len(data)) > c.maxBytes {
		return nil, ErrResponseTooLarge
	}
	return decode[*mcp.ReadResourceResult](data)
}

func collectPages[T any](ctx context.Context, maxPages int, maxBytes int64, fetch func(string) ([]*T, string, error)) ([]*T, error) {
	items := make([]*T, 0)
	cursor := ""
	seen := map[string]bool{"": true}
	var size int64
	for range maxPages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, next, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(page)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode list page: %w", err)
		}
		size += int64(len(encoded))
		if size > maxBytes {
			return nil, ErrResponseTooLarge
		}
		for _, item := range page {
			if item == nil {
				return nil, errors.New("mcp: null list entry")
			}
		}
		// The SDK may retain page pointers for server-directed caching.
		owned, err := decode[[]*T](encoded)
		if err != nil {
			return nil, err
		}
		items = append(items, owned...)
		if next == "" {
			return items, nil
		}
		if seen[next] {
			return nil, errors.New("mcp: repeated pagination cursor")
		}
		seen[next] = true
		cursor = next
	}
	return nil, errors.New("mcp: pagination page limit exceeded")
}

type httpTransport struct {
	base     http.RoundTripper
	headers  http.Header
	maxBytes int64
}

func (t *httpTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	for name, values := range t.headers {
		request.Header[name] = append([]string(nil), values...)
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	// SSE is bounded per event by StreamableClientTransport, not over the
	// lifetime of the notification stream.
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		if response.ContentLength > t.maxBytes {
			_ = response.Body.Close()
			return nil, ErrResponseTooLarge
		}
		// Check finite JSON responses before handing them to the SDK's async
		// reader. A size error then fails only this request, preserving both
		// the sentinel and the session for subsequent operations.
		body, readErr := io.ReadAll(io.LimitReader(response.Body, t.maxBytes+1))
		closeErr := response.Body.Close()
		if int64(len(body)) > t.maxBytes {
			return nil, ErrResponseTooLarge
		}
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, err
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
	}
	return response, nil
}
