package runtimeprofile

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func controlAccessBinding(access *apitypes.RuntimeProfileBindingControlAccess) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{
		ResourceId: "res-1",
		I18n: map[string]apitypes.RuntimeProfileI18nText{
			"en": {DisplayName: "Timer"}, "zh-CN": {DisplayName: "计时"},
		},
		ControlAccess: access,
	}
}

func controlAccessUpsert(spec apitypes.RuntimeProfileSpec) adminhttp.RuntimeProfileUpsert {
	if spec.Workflows.Collections == nil {
		spec.Workflows.Collections = apitypes.RuntimeProfileWorkflowCollections{}
	}
	return adminhttp.RuntimeProfileUpsert{Id: "test-profile", Spec: spec}
}

func TestNormalizeProfileKeepsToolControlAccess(t *testing.T) {
	t.Parallel()
	owner := apitypes.RuntimeProfileBindingControlAccessOwner
	tools := map[string]apitypes.RuntimeProfileBinding{"timer": controlAccessBinding(&owner)}
	normalized, err := normalizeProfile(controlAccessUpsert(apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{Tools: &tools},
	}), "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	got := (*normalized.Spec.Resources.Tools)["timer"].ControlAccess
	if got == nil || *got != owner {
		t.Fatalf("control_access = %v, want owner", got)
	}
}

func TestNormalizeProfileRejectsMisplacedControlAccess(t *testing.T) {
	t.Parallel()
	owner := apitypes.RuntimeProfileBindingControlAccessOwner
	unknown := apitypes.RuntimeProfileBindingControlAccess("guardian")
	models := map[string]apitypes.RuntimeProfileBinding{"chat": controlAccessBinding(&owner)}
	tools := map[string]apitypes.RuntimeProfileBinding{"timer": controlAccessBinding(&unknown)}
	cases := []struct {
		name string
		spec apitypes.RuntimeProfileSpec
		want string
	}{
		{name: "model", spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &models}}, want: "only valid under resources.tools"},
		{name: "workflow", spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			Collections: apitypes.RuntimeProfileWorkflowCollections{"main": {"chat": controlAccessBinding(&owner)}},
		}}, want: "only valid under resources.tools"},
		{name: "unknown level", spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Tools: &tools}}, want: "unknown control_access"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeProfile(controlAccessUpsert(tc.spec), "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("normalizeProfile() error = %v, want %q", err, tc.want)
			}
		})
	}
}
