// Package adminresource applies, reads, and deletes declarative GizClaw Admin
// resources through the Admin HTTP service of a connected gizcli.Client.
//
// It is the resource surface shared by the gizclaw CLI admin commands and the
// GizClaw Terraform provider. The package is pure Go: it does not depend on the
// CLI command tree, cgo, or embedded web assets.
package adminresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
)

// API is the subset of the generated Admin HTTP client used by Client.
type API interface {
	ApplyResourceWithResponse(ctx context.Context, body adminhttp.ApplyResourceJSONRequestBody, reqEditors ...adminhttp.RequestEditorFn) (*adminhttp.ApplyResourceResponse, error)
	DeleteResourceWithResponse(ctx context.Context, kind adminhttp.ResourceKind, id string, reqEditors ...adminhttp.RequestEditorFn) (*adminhttp.DeleteResourceResponse, error)
	GetResourceWithResponse(ctx context.Context, kind adminhttp.ResourceKind, id string, reqEditors ...adminhttp.RequestEditorFn) (*adminhttp.GetResourceResponse, error)
}

// Client performs Admin resource operations over one Admin API client.
type Client struct {
	api   API
	close func() error
}

// NewClient returns a Client over api. closeFn, when non-nil, is called by
// Close to release the underlying connection.
func NewClient(api API, closeFn func() error) *Client {
	return &Client{api: api, close: closeFn}
}

// Connect opens a ready connection for the selected CLI context and returns a
// Client that owns it. ctx bounds connection setup only. The caller must Close
// the Client.
func Connect(ctx context.Context, opts contextconn.Options) (*Client, error) {
	c, err := contextconn.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}
	api, err := c.ServerAdminClient()
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return NewClient(api, c.Close), nil
}

// ApplyResource creates or updates resource.
func (c *Client) ApplyResource(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	var writable adminhttp.ApplyResourceJSONRequestBody
	if err := json.Unmarshal(data, &writable); err != nil {
		return apitypes.ApplyResult{}, err
	}
	resp, err := c.api.ApplyResourceWithResponse(ctx, writable)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.ApplyResult{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500, resp.JSON501)
}

// DeleteResource deletes the resource addressed by kind and id and returns the
// deleted resource.
func (c *Client) DeleteResource(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
	resp, err := c.api.DeleteResourceWithResponse(ctx, kind, id)
	if err != nil {
		return apitypes.Resource{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Resource{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON409, resp.JSON500)
}

// GetResource reads the resource addressed by kind and id.
func (c *Client) GetResource(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
	resp, err := c.api.GetResourceWithResponse(ctx, kind, id)
	if err != nil {
		return apitypes.Resource{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Resource{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON501)
}

// GetResources reads refs over this Client with GetResources.
func (c *Client) GetResources(ctx context.Context, refs []Reference) []Result {
	return GetResources(ctx, c, refs)
}

// Close releases the connection owned by the Client, if any.
func (c *Client) Close() error {
	if c == nil || c.close == nil {
		return nil
	}
	return c.close()
}

// ResponseError is a non-success Admin API response.
type ResponseError struct {
	// StatusCode is the HTTP status, or 0 when no response was received.
	StatusCode int
	// Code and Message come from the structured error body when present.
	Code    string
	Message string
	// Body is the raw response body used when no structured error is present.
	Body string

	structured bool
}

func (e *ResponseError) Error() string {
	if e.structured {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	if e.Body != "" {
		return fmt.Sprintf("unexpected status %d: %s", e.StatusCode, e.Body)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("unexpected status %d", e.StatusCode)
	}
	return "unexpected empty response"
}

var notFoundCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_NOT_FOUND$`)

// IsNotFound reports whether err carries a structured Admin error code of the
// form <SCOPE>_NOT_FOUND. HTTP 404 responses without such a code, such as an
// unmatched route, are not treated as resource absence.
func IsNotFound(err error) bool {
	var responseErr *ResponseError
	return errors.As(err, &responseErr) && notFoundCodePattern.MatchString(responseErr.Code)
}

func responseError(status int, body []byte, errs ...*apitypes.ErrorResponse) error {
	for _, e := range errs {
		if e != nil {
			return &ResponseError{StatusCode: status, Code: e.Error.Code, Message: e.Error.Message, structured: true}
		}
	}
	return &ResponseError{StatusCode: status, Body: strings.TrimSpace(string(body))}
}
