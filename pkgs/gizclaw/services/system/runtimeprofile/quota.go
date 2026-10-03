package runtimeprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// CustomQuotaPolicy validates the binding and returns its custom HTTP policy.
// Omitted, null and explicit unlimited bindings return nil without dependencies.
func CustomQuotaPolicy(binding *apitypes.RuntimeProfileQuota) (*apitypes.RuntimeProfileQuotaCustom, error) {
	normalized, err := normalizeQuota(binding)
	if err != nil || normalized == nil {
		return nil, err
	}
	kind, err := normalized.Discriminator()
	if err != nil || kind == "unlimited" {
		return nil, err
	}
	policy, err := normalized.AsRuntimeProfileQuotaCustom()
	return &policy, err
}

func normalizeQuota(binding *apitypes.RuntimeProfileQuota) (*apitypes.RuntimeProfileQuota, error) {
	if binding == nil {
		return nil, nil
	}
	data, err := json.Marshal(binding)
	if err != nil {
		return nil, fmt.Errorf("quota: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, errors.New("quota must be an object or null")
	}
	if fields == nil {
		return nil, nil
	}
	kind, err := binding.Discriminator()
	if err != nil {
		return nil, errors.New("quota.type must be unlimited or custom")
	}
	result := &apitypes.RuntimeProfileQuota{}
	switch kind {
	case "unlimited":
		if len(fields) != 1 {
			return nil, errors.New("quota unlimited only permits type")
		}
		err = result.FromRuntimeProfileQuotaUnlimited(apitypes.RuntimeProfileQuotaUnlimited{Type: apitypes.RuntimeProfileQuotaUnlimitedTypeUnlimited})
	case "custom":
		if len(fields) != 3 || fields["endpoint"] == nil || fields["api_key"] == nil {
			return nil, errors.New("quota custom requires only type, endpoint and api_key")
		}
		policy, decodeErr := binding.AsRuntimeProfileQuotaCustom()
		if decodeErr != nil {
			return nil, errors.New("quota custom fields must be strings")
		}
		policy.Endpoint = strings.TrimSpace(policy.Endpoint)
		policy.ApiKey = strings.TrimSpace(policy.ApiKey)
		if err := validateMemoryEndpoint(policy.Endpoint); err != nil {
			return nil, fmt.Errorf("quota.endpoint: %w", err)
		}
		if policy.ApiKey == "" || strings.ContainsAny(policy.ApiKey, "\r\n") {
			return nil, errors.New("quota.api_key must be non-empty and contain no newlines")
		}
		err = result.FromRuntimeProfileQuotaCustom(policy)
	default:
		return nil, errors.New("quota.type must be unlimited or custom")
	}
	return result, err
}
