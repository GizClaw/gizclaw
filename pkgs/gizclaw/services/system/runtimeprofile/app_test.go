package runtimeprofile

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAppBindingsRequireExistingApp(t *testing.T) {
	base := workspaceRewardResourceResolverForTest(t, "", nil)
	exists := false
	s := &Server{ResolveResource: func(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
		if kind != apitypes.ResourceKindApp {
			return base(ctx, kind, id)
		}
		if !exists {
			return apitypes.Resource{}, sql.ErrNoRows
		}
		var resource apitypes.Resource
		err := resource.FromAppResource(apitypes.AppResource{ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1, Kind: apitypes.AppResourceKindApp, Metadata: apitypes.ResourceMetadata{Id: id}})
		return resource, err
	}}
	spec := apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{System: runtimeProfileTestSystemWorkflows()}, Resources: apitypes.RuntimeProfileResources{Apps: new(map[string]apitypes.RuntimeProfileBinding{"clock": runtimeProfileTestBinding("clock-package")})}}
	if err := s.validateResources(t.Context(), spec); err == nil || !strings.Contains(err.Error(), "resources.apps.clock") {
		t.Fatalf("missing App: %v", err)
	}
	exists = true
	if err := s.validateResources(t.Context(), spec); err != nil {
		t.Fatalf("existing App: %v", err)
	}
}
