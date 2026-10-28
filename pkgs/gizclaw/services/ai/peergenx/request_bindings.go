package peergenx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// ReportedKVGetter reads one durable device-owned snapshot. The binding
// resolver passes only the authenticated caller's public key.
type ReportedKVGetter interface {
	GetReportedKV(context.Context, giznet.PublicKey) (*apitypes.PeerReportedKV, error)
}

type requestBindingError struct {
	reason string
	index  int
}

func (e *requestBindingError) Error() string {
	return fmt.Sprintf("model request binding %d: %s", e.index, e.reason)
}

func (e *requestBindingError) PublicError() (string, string, bool) {
	return "MODEL_REQUEST_BINDING_" + e.reason, "Model request metadata could not be resolved.", e.reason == "UNAVAILABLE"
}

func modelRequestBindings(model apitypes.Model) (*apitypes.ModelRequestBindings, error) {
	if model.Provider.Kind != apitypes.ModelProviderKindOpenaiTenant {
		return nil, nil
	}
	raw, err := model.ProviderData.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	// Capability-only model descriptors can omit provider configuration.
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	data, err := model.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := apitypes.ValidateModelRequestBindings(model.Kind, model.Provider.Kind, data.RequestBindings); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return data.RequestBindings, nil
}

func (s *Service) requestHeaders(model apitypes.Model) (func(context.Context) (http.Header, error), error) {
	declarations, err := modelRequestBindings(model)
	if err != nil {
		return nil, err
	}
	if declarations == nil || len(*declarations) == 0 {
		return nil, nil
	}
	// Own the declaration, including optional scalar pointers. Neither the
	// construction context nor a device's values belong in this closure.
	raw, err := json.Marshal(declarations)
	if err != nil {
		return nil, err
	}
	var bindings apitypes.ModelRequestBindings
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return nil, err
	}
	paths := make([][]string, len(bindings))
	sources := make([]apitypes.ModelRequestPeerRuntimeKVSelector, len(bindings))
	constants := make([]*string, len(bindings))
	needsKV := false
	for i, binding := range bindings {
		raw, err := binding.Source.MarshalJSON()
		if err != nil {
			return nil, err
		}
		var fixed struct {
			Constant *string `json:"constant"`
		}
		if err := json.Unmarshal(raw, &fixed); err != nil {
			return nil, err
		}
		if fixed.Constant != nil {
			constants[i] = fixed.Constant
			continue
		}
		needsKV = true
		value, err := binding.Source.AsModelRequestPeerRuntimeKVSource()
		if err != nil {
			return nil, fmt.Errorf("%w: invalid request binding source", ErrInvalid)
		}
		sources[i] = value.PeerRuntimeKv
		paths[i], err = apitypes.ModelRequestPointerSegments(sources[i].Pointer)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid JSON pointer", ErrInvalid)
		}
	}
	getter := s.ReportedKV
	return func(ctx context.Context) (http.Header, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var snapshot *apitypes.PeerReportedKV
		var err error
		if needsKV {
			peer := ownership.Caller(ctx)
			if peer.IsZero() {
				return nil, &requestBindingError{reason: "CALLER_REQUIRED", index: -1}
			}
			if getter == nil {
				return nil, &requestBindingError{reason: "UNAVAILABLE", index: -1}
			}
			snapshot, err = getter.GetReportedKV(ctx, peer)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, &requestBindingError{reason: "UNAVAILABLE", index: -1}
			}
		}
		headers := make(http.Header, len(bindings))
		values := make(map[string]any)
		total := 0
		for i, binding := range bindings {
			source := sources[i]
			var entry apitypes.PeerKVEntry
			present := false
			if snapshot != nil {
				entry, present = (*snapshot)[source.Key]
			}
			var value any
			if constants[i] != nil {
				value, present = *constants[i], true
			} else if present {
				var decoded bool
				value, decoded = values[source.Key]
				if !decoded {
					value, err = decodeBindingJSON(entry.Value)
					if err != nil {
						return nil, &requestBindingError{reason: "INVALID", index: i}
					}
					values[source.Key] = value
				}
				value, present, err = selectBindingValue(value, paths[i])
				if err != nil {
					return nil, &requestBindingError{reason: "INVALID", index: i}
				}
			}
			if !present || value == nil {
				if binding.Required == nil || *binding.Required {
					reason := "MISSING"
					if present {
						reason = "NULL"
					}
					return nil, &requestBindingError{reason: reason, index: i}
				}
				continue
			}
			text, ok := value.(string)
			if !ok || apitypes.ValidateModelRequestHeader(binding.Target.Name, text) != nil {
				return nil, &requestBindingError{reason: "INVALID", index: i}
			}
			total += len(binding.Target.Name) + len(text) + 4
			if total > apitypes.MaxModelRequestHeadersBytes {
				return nil, &requestBindingError{reason: "TOO_LARGE", index: i}
			}
			headers.Set(binding.Target.Name, text)
		}
		return headers, nil
	}, nil
}

// Decode tokens rather than a map directly so duplicate members cannot give
// an identity field an ambiguous interpretation. Runtime still stores opaque JSON.
func decodeBindingJSON(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeBindingValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON")
	}
	return value, nil
}

func decodeBindingValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		for decoder.More() {
			name, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := name.(string)
			if !ok {
				return nil, errors.New("invalid JSON member")
			}
			if _, exists := object[key]; exists {
				return nil, errors.New("duplicate JSON member")
			}
			value, err := decodeBindingValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		_, err := decoder.Token()
		return object, err
	case json.Delim('['):
		var values []any
		for decoder.More() {
			value, err := decodeBindingValue(decoder)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		_, err := decoder.Token()
		return values, err
	}
	return token, nil
}

func selectBindingValue(value any, parts []string) (any, bool, error) {
	for _, part := range parts {
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[part]
			if !ok {
				return nil, false, nil
			}
		case []any:
			if part == "" || len(part) > 1 && part[0] == '0' {
				return nil, false, errors.New("invalid array index")
			}
			for i := range len(part) {
				if part[i] < '0' || part[i] > '9' {
					return nil, false, errors.New("invalid array index")
				}
			}
			index, err := strconv.ParseUint(part, 10, 64)
			if err != nil || index >= uint64(len(current)) {
				return nil, false, nil
			}
			value = current[index]
		default:
			return nil, false, errors.New("JSON pointer type mismatch")
		}
	}
	return value, true, nil
}
