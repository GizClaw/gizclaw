package resourcemanager

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app"
)

func (m *Manager) applyApp(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Apps == nil {
		return apitypes.ApplyResult{}, missingService("apps")
	}
	item, err := resource.AsAppResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_APP_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, err
	}
	previous, err := m.services.Apps.Get(ctx, item.Metadata.Id)
	if err != nil && !errors.Is(err, app.ErrNotFound) {
		return apitypes.ApplyResult{}, err
	}
	action := apitypes.ApplyActionCreated
	if err == nil {
		action = apitypes.ApplyActionUpdated
		same, err := semanticEqual(apitypes.AppSpec{Package: previous.Package}, item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindApp, item.Metadata.Id), nil
		}
	}
	value, err := m.services.Apps.Put(ctx, item.Metadata.Id, item.Spec, true)
	if err != nil {
		return apitypes.ApplyResult{}, appServiceError(err)
	}
	return applyResult(action, apitypes.ResourceKindApp, value.Id), nil
}

func (m *Manager) getAppResource(ctx context.Context, id string) (apitypes.Resource, error) {
	if m.services.Apps == nil {
		return apitypes.Resource{}, missingService("apps")
	}
	value, err := m.services.Apps.Get(ctx, id)
	if err != nil {
		return apitypes.Resource{}, appServiceError(err)
	}
	return resourceFromApp(value)
}

func resourceFromApp(value apitypes.App) (apitypes.Resource, error) {
	return marshalResource(apitypes.AppResource{ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1, Kind: apitypes.AppResourceKindApp, Metadata: apitypes.ResourceMetadata{Id: value.Id}, Spec: apitypes.AppSpec{Package: value.Package}})
}

func appServiceError(err error) error {
	if errors.Is(err, app.ErrNotFound) {
		return applyError(404, "APP_NOT_FOUND", err.Error())
	}
	if errors.Is(err, app.ErrInvalid) {
		return applyError(400, "INVALID_APP", err.Error())
	}
	return err
}
