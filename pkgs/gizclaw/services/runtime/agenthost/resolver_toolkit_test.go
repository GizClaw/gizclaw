package agenthost

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"slices"
	"testing"
)

func TestToolVerificationFollowsUniqueWorkflowBinding(t *testing.T) {
	profile := apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"assistant": {ResourceId: "workflow", Toolkit: &apitypes.RuntimeProfileToolSelection{VerificationModel: new("checker")}}}}}
	workspace := apitypes.Workspace{WorkflowId: "workflow"}
	if model, err := resolveToolVerificationModel(profile, workspace); err != nil || model != "checker" {
		t.Fatalf("missing label disabled configured verification: model=%q err=%v", model, err)
	}
	profile.Spec.Workflows["other"] = profile.Spec.Workflows["assistant"]
	if _, err := resolveToolVerificationModel(profile, workspace); err == nil {
		t.Fatal("ambiguous binding silently selected verification settings")
	}
	workspace.Labels = &map[string]string{"workflow_name": "assistant"}
	if model, err := resolveToolVerificationModel(profile, workspace); err != nil || model != "checker" {
		t.Fatalf("explicit binding: model=%q err=%v", model, err)
	}
}

func TestServiceResolverToolkitIsProfileOptIn(t *testing.T) {
	server := toolkittest.New(t)
	echo := putAgentHostTool(t, server, agentHostBoundHTTPTool("giztest_echo"))
	other := putAgentHostTool(t, server, agentHostBoundHTTPTool("giztest_other"))
	profile := apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Tools: &map[string]apitypes.RuntimeProfileToolBinding{"giztest-echo": {ResourceId: echo.ID}, "giztest-other": {ResourceId: other.ID}}}, Workflows: apitypes.RuntimeProfileWorkflows{"assistant": {ResourceId: "workflow"}}}}
	resolver := ServiceResolver{ToolCatalog: &toolcatalog.Catalog{Tools: server}, RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil }}
	for _, tc := range []struct {
		name      string
		selected  *[]string
		workspace *apitypes.ToolkitPolicy
		want      []string
	}{
		{name: "omitted binding"},
		{name: "empty binding", selected: &[]string{}},
		{name: "workspace cannot grant", workspace: &apitypes.ToolkitPolicy{ToolNames: &[]string{"giztest-echo"}}},
		{name: "binding injects", selected: &[]string{"giztest-echo", "giztest-other"}, want: []string{"giztest-echo", "giztest-other"}},
		{name: "workspace narrows", selected: &[]string{"giztest-echo", "giztest-other"}, workspace: &apitypes.ToolkitPolicy{ToolNames: &[]string{"giztest-other"}}, want: []string{"giztest-other"}},
		{name: "workspace cannot widen", selected: &[]string{"giztest-echo"}, workspace: &apitypes.ToolkitPolicy{ToolNames: &[]string{"giztest-echo", "giztest-other"}}, want: []string{"giztest-echo"}},
		{name: "workspace empty", selected: &[]string{"giztest-echo"}, workspace: &apitypes.ToolkitPolicy{ToolNames: &[]string{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := profile.Spec.Workflows["assistant"]
			binding.Toolkit = &apitypes.RuntimeProfileToolSelection{ToolNames: tc.selected}
			profile.Spec.Workflows["assistant"] = binding
			invoker, err := resolver.resolveToolkit(t.Context(), apitypes.Workspace{OwnerPublicKey: new("owner"), WorkflowId: "workflow", Toolkit: tc.workspace}, apitypes.Workflow{Id: "workflow", Spec: apitypes.WorkflowSpec{Toolkit: &apitypes.ToolkitPolicy{ToolIds: &[]string{"unrelated-fixed-id"}}}})
			if err != nil {
				t.Fatal(err)
			}
			definitions, err := invoker.ResolveTools(WithResourceAccess(t.Context(), "owner", nil, nil))
			if err != nil {
				t.Fatal(err)
			}
			names := []string{}
			for _, definition := range definitions {
				names = append(names, definition.Name)
			}
			if !slices.Equal(names, tc.want) {
				t.Fatalf("names=%v want=%v", names, tc.want)
			}
		})
	}
}
