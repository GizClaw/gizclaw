package resourcemanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
)

// ResolveAssetOwner loads a complete declarative Resource and enumerates every
// canonical AssetRef in its current structure.
func (m *Manager) ResolveAssetOwner(ctx context.Context, owner asset.Owner) (asset.OwnerSnapshot, error) {
	resource, exists, err := m.assetOwnerResource(ctx, owner)
	if err != nil || !exists {
		return asset.OwnerSnapshot{Exists: exists}, err
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return asset.OwnerSnapshot{}, fmt.Errorf("encode resource owner %s: %w", owner.ID, err)
	}
	return asset.OwnerSnapshot{Exists: true, Refs: assetRefsInJSON(data)}, nil
}

// ResourceHasDisplayAsset reports whether the current Resource displays
// structure contains ref. It deliberately excludes other string fields so the
// generic asset download RPC cannot expose domain-internal assets.
func (m *Manager) ResourceHasDisplayAsset(ctx context.Context, owner asset.Owner, ref asset.Ref) (bool, error) {
	resource, exists, err := m.assetOwnerResource(ctx, owner)
	if err != nil || !exists {
		return false, err
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return false, fmt.Errorf("encode resource owner %s: %w", owner.ID, err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return false, fmt.Errorf("decode resource owner %s: %w", owner.ID, err)
	}
	displays, ok := value["displays"]
	if !ok {
		return false, nil
	}
	for _, candidate := range assetRefsInJSON(displays) {
		if candidate == ref {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) assetOwnerResource(ctx context.Context, owner asset.Owner) (apitypes.Resource, bool, error) {
	if owner.Kind != asset.OwnerKindResource {
		return apitypes.Resource{}, false, fmt.Errorf("unsupported resource owner kind %q", owner.Kind)
	}
	kindValue, name, ok := strings.Cut(owner.ID, "/")
	kind := apitypes.ResourceKind(kindValue)
	if !ok || !kind.Valid() || name == "" {
		return apitypes.Resource{}, false, fmt.Errorf("invalid resource owner id %q", owner.ID)
	}
	resource, err := m.Get(ctx, kind, name)
	if err != nil {
		var resourceErr *Error
		if errors.As(err, &resourceErr) && resourceErr.StatusCode == 404 {
			return apitypes.Resource{}, false, nil
		}
		return apitypes.Resource{}, false, err
	}
	return resource, true, nil
}

func assetRefsInJSON(data []byte) []asset.Ref {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	refs := make([]asset.Ref, 0)
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case string:
			if ref, err := asset.ParseRef(typed); err == nil {
				refs = append(refs, ref)
			}
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			for _, item := range typed {
				visit(item)
			}
		}
	}
	visit(value)
	return refs
}
