package genx

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"golang.org/x/net/http/httpguts"
)

func (g *OpenAIGenerator) requestOptions(ctx context.Context) ([]option.RequestOption, error) {
	if g.RequestHeaders == nil {
		return nil, nil
	}
	headers, err := g.RequestHeaders(ctx)
	if err != nil {
		return nil, err
	}
	// Freeze values before any goroutine, SDK retry or compatibility fallback.
	// Never modify Client.Options or retain the callback's mutable map.
	var options []option.RequestOption
	for name, values := range headers.Clone() {
		if !httpguts.ValidHeaderFieldName(name) || len(values) != 1 || !httpguts.ValidHeaderFieldValue(values[0]) {
			return nil, errors.New("invalid request metadata header")
		}
		options = append(options, option.WithHeader(name, values[0]))
	}
	return options, nil
}

// Upstreams can echo headers in error bodies. Preserve the typed cause for
// protocol handling while keeping error strings, public events and logs safe.
func (g *OpenAIGenerator) requestError(err error) error {
	if err == nil || g.RequestHeaders == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	status := 0
	if provider, ok := errors.AsType[*openai.Error](err); ok {
		status = provider.StatusCode
	}
	return &openAIRequestError{cause: err, status: status}
}

type openAIRequestError struct {
	cause  error
	status int
}

func (e *openAIRequestError) Error() string {
	return fmt.Sprintf("provider request failed (HTTP %d)", e.status)
}

func (e *openAIRequestError) Unwrap() error { return e.cause }

func (e *openAIRequestError) PublicError() (string, string, bool) {
	return "MODEL_PROVIDER_ERROR", "The model provider request failed.", e.status == 429 || e.status >= 500
}
