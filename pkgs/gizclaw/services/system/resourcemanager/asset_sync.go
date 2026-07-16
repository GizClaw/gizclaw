package resourcemanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
)

// Put writes a Resource and coordinates its reverse asset bindings.
func (m *Manager) Put(ctx context.Context, resource apitypes.Resource) (apitypes.Resource, error) {
	finish, err := m.prepareAssetWrite(ctx, resource)
	if err != nil {
		return apitypes.Resource{}, err
	}
	stored, writeErr := m.put(ctx, resource)
	if writeErr != nil {
		if rollbackErr := finish(ctx, false); rollbackErr != nil {
			return apitypes.Resource{}, errors.Join(writeErr, rollbackErr)
		}
		return apitypes.Resource{}, writeErr
	}
	if err := finish(ctx, true); err != nil {
		return stored, err
	}
	return stored, nil
}

// Apply creates, updates, or leaves unchanged a Resource and coordinates its
// reverse asset bindings around the owner write.
func (m *Manager) Apply(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	finish, err := m.prepareAssetWrite(ctx, resource)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	result, writeErr := m.apply(ctx, resource)
	if writeErr != nil {
		if rollbackErr := finish(ctx, false); rollbackErr != nil {
			return apitypes.ApplyResult{}, errors.Join(writeErr, rollbackErr)
		}
		return apitypes.ApplyResult{}, writeErr
	}
	if err := finish(ctx, true); err != nil {
		return result, err
	}
	return result, nil
}

// Delete removes a Resource and then clears all reverse bindings owned by it.
func (m *Manager) Delete(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	if m != nil && m.services.Assets != nil {
		m.assetWriteMu.Lock()
		defer m.assetWriteMu.Unlock()
	}
	deleted, err := m.delete(ctx, kind, name)
	if err != nil || m == nil || m.services.Assets == nil {
		return deleted, err
	}
	owner := asset.Owner{Kind: asset.OwnerKindResource, ID: string(kind) + "/" + name}
	if err := m.services.Assets.UnbindOwner(ctx, owner); err != nil {
		return deleted, applyError(500, "ASSET_BINDING_FAILED", err.Error())
	}
	return deleted, nil
}

type finishAssetWrite func(context.Context, bool) error

func (m *Manager) prepareAssetWrite(ctx context.Context, resource apitypes.Resource) (finishAssetWrite, error) {
	noop := func(context.Context, bool) error { return nil }
	if m == nil || m.services.Assets == nil {
		return noop, nil
	}
	kind, name, err := resourceIdentity(resource)
	if err != nil {
		return noop, nil // normal resource validation owns malformed headers
	}
	if kind == apitypes.ResourceKindResourceList {
		return noop, nil
	}
	m.assetWriteMu.Lock()
	handedOff := false
	defer func() {
		if !handedOff {
			m.assetWriteMu.Unlock()
		}
	}()
	owner := asset.Owner{Kind: asset.OwnerKindResource, ID: string(kind) + "/" + name}
	oldRefs := make(map[asset.Ref]struct{})
	existing, err := m.Get(ctx, kind, name)
	if err == nil {
		oldRefs, err = resourceAssetRefs(existing)
		if err != nil {
			return nil, applyError(500, "ASSET_BINDING_FAILED", err.Error())
		}
	} else if !resourceNotFound(err) {
		return nil, err
	}
	newRefs, err := resourceAssetRefs(resource)
	if err != nil {
		return nil, applyError(400, "INVALID_ASSET_REF", err.Error())
	}
	added := refDifference(newRefs, oldRefs)
	removed := refDifference(oldRefs, newRefs)
	current := refDifference(newRefs, nil)
	binding := asset.Binding{Owner: owner}
	protected := make([]asset.Ref, 0, len(added))
	for _, ref := range added {
		if err := m.services.Assets.Protect(ctx, ref, binding); err != nil {
			for _, rollbackRef := range protected {
				_ = m.services.Assets.Unbind(ctx, rollbackRef, binding)
			}
			return nil, applyError(400, "INVALID_ASSET_REF", err.Error())
		}
		protected = append(protected, ref)
	}
	finish := func(ctx context.Context, committed bool) error {
		defer m.assetWriteMu.Unlock()
		if !committed {
			var errs []error
			for _, ref := range protected {
				errs = append(errs, m.services.Assets.Unbind(ctx, ref, binding))
			}
			return errors.Join(errs...)
		}
		var errs []error
		for _, ref := range current {
			if err := m.services.Assets.Activate(ctx, ref, binding); err != nil {
				errs = append(errs, fmt.Errorf("activate %s: %w", ref, err))
			}
		}
		for _, ref := range removed {
			if err := m.services.Assets.Unbind(ctx, ref, binding); err != nil {
				errs = append(errs, fmt.Errorf("unbind %s: %w", ref, err))
			}
		}
		if err := errors.Join(errs...); err != nil {
			return applyError(500, "ASSET_BINDING_FAILED", err.Error())
		}
		return nil
	}
	handedOff = true
	return finish, nil
}

func resourceIdentity(resource apitypes.Resource) (apitypes.ResourceKind, string, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return "", "", err
	}
	var header struct {
		Kind     apitypes.ResourceKind `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return "", "", err
	}
	if !header.Kind.Valid() || header.Metadata.Name == "" {
		return "", "", fmt.Errorf("invalid resource identity")
	}
	return header.Kind, header.Metadata.Name, nil
}

func resourceAssetRefs(resource apitypes.Resource) (map[asset.Ref]struct{}, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	refs := make(map[asset.Ref]struct{})
	for _, ref := range assetRefsInJSON(data) {
		refs[ref] = struct{}{}
	}
	return refs, nil
}

func refDifference(left, right map[asset.Ref]struct{}) []asset.Ref {
	refs := make([]asset.Ref, 0)
	for ref := range left {
		if _, exists := right[ref]; !exists {
			refs = append(refs, ref)
		}
	}
	return refs
}

func resourceNotFound(err error) bool {
	var resourceErr *Error
	return errors.As(err, &resourceErr) && resourceErr.StatusCode == 404
}
