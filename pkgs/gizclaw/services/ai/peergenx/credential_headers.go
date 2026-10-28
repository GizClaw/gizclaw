package peergenx

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func credentialRequestHeaders(model apitypes.Model, declared *map[string]string, resolve func(context.Context) (http.Header, error)) (func(context.Context) (http.Header, error), error) {
	if declared == nil || len(*declared) == 0 {
		return resolve, nil
	}
	if err := apitypes.ValidateOpenAICredentialHeaders(declared); err != nil {
		return nil, fmt.Errorf("%w: invalid credential headers", ErrInvalid)
	}
	private := make(http.Header, len(*declared))
	for name, value := range *declared {
		private.Set(name, value)
	}
	bindings, err := modelRequestBindings(model)
	if err != nil {
		return nil, err
	}
	if bindings != nil {
		for _, binding := range *bindings {
			if len(private.Values(binding.Target.Name)) != 0 {
				return nil, fmt.Errorf("%w: credential and Model headers overlap", ErrInvalid)
			}
		}
	}
	return func(ctx context.Context) (http.Header, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		headers := private.Clone()
		if resolve != nil {
			bound, err := resolve(ctx)
			if err != nil {
				return nil, err
			}
			for name, values := range bound {
				if len(headers.Values(name)) != 0 {
					return nil, fmt.Errorf("%w: credential and Model headers overlap", ErrInvalid)
				}
				headers[name] = append([]string(nil), values...)
			}
		}
		if len(headers) > apitypes.MaxModelRequestHeaders {
			return nil, &requestBindingError{reason: "TOO_LARGE", index: -1}
		}
		total := 0
		for name, values := range headers {
			if len(values) != 1 || apitypes.ValidateModelRequestHeader(name, values[0]) != nil {
				return nil, &requestBindingError{reason: "INVALID", index: -1}
			}
			total += len(name) + len(values[0]) + 4
		}
		if total > apitypes.MaxModelRequestHeadersBytes {
			return nil, &requestBindingError{reason: "TOO_LARGE", index: -1}
		}
		return headers, nil
	}, nil
}
