package agenthost

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

func TestServiceResolverToolkitIsOptIn(t *testing.T) {
	server := toolkittest.New(t)
	echo := putAgentHostTool(t, server, agentHostBoundHTTPTool("giztest_echo"))
	other := putAgentHostTool(t, server, agentHostBoundHTTPTool("giztest_other"))
	unbound := putAgentHostTool(t, server, agentHostBoundHTTPTool("giztest_unbound"))
	resolver := ServiceResolver{ToolBuilder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{"giztest-echo": echo.ID, "giztest-other": other.ID})
	policy := func(ids ...string) *apitypes.ToolkitPolicy {
		return &apitypes.ToolkitPolicy{ToolIds: &ids}
	}
	for _, tc := range []struct {
		name      string
		workflow  *apitypes.ToolkitPolicy
		workspace *apitypes.ToolkitPolicy
		want      []string
	}{
		{name: "omitted workflow policy", want: nil},
		{name: "omitted workflow tool_ids", workflow: &apitypes.ToolkitPolicy{}, want: nil},
		{name: "empty workflow list", workflow: policy(), want: nil},
		{name: "workspace cannot grant", workspace: policy(echo.ID, other.ID), want: nil},
		{name: "explicit workflow list", workflow: policy(echo.ID), want: []string{"giztest_echo"}},
		{name: "omitted workspace keeps workflow list", workflow: policy(echo.ID, other.ID), workspace: &apitypes.ToolkitPolicy{}, want: []string{"giztest_echo", "giztest_other"}},
		{name: "workspace narrows", workflow: policy(echo.ID, other.ID), workspace: policy(other.ID), want: []string{"giztest_other"}},
		{name: "workspace cannot widen", workflow: policy(echo.ID), workspace: policy(echo.ID, other.ID), want: []string{"giztest_echo"}},
		{name: "empty workspace list", workflow: policy(echo.ID, other.ID), workspace: policy(), want: nil},
		{name: "listed ID the profile does not bind", workflow: policy(echo.ID, unbound.ID), want: []string{"giztest_echo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invoker, err := resolver.resolveToolkit(ctx,
				apitypes.Workspace{Toolkit: tc.workspace},
				apitypes.Workflow{Spec: apitypes.WorkflowSpec{Toolkit: tc.workflow}},
			)
			if err != nil {
				t.Fatalf("resolveToolkit() error = %v", err)
			}
			if tc.want == nil {
				if invoker != nil {
					t.Fatalf("resolveToolkit() = %#v, want nil ToolInvoker", invoker)
				}
				return
			}
			if invoker == nil {
				t.Fatal("resolveToolkit() ToolInvoker = nil")
			}
			definitions, err := invoker.ResolveTools(ctx)
			if err != nil {
				t.Fatalf("ResolveTools() error = %v", err)
			}
			var names []string
			for _, definition := range definitions {
				names = append(names, definition.Name)
			}
			if !slices.Equal(names, tc.want) {
				t.Fatalf("ResolveTools() = %v, want %v", names, tc.want)
			}
		})
	}
}

func TestServiceResolverResolveWithoutToolsHasNilToolInvoker(t *testing.T) {
	owner := "owner-public-key"
	ws := systemWorkspace("demo", "workflow-1", nil)
	ws.OwnerPublicKey = &owner
	bindings := map[string]apitypes.RuntimeProfileBinding{"giztest-echo": {ResourceId: "giztest-toolkit-echo"}}
	resolver := ServiceResolver{
		Workspaces: fakeWorkspaceService{items: map[string]apitypes.Workspace{"demo": ws}},
		Workflows:  fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow-1": mustWorkflow(t, "workflow-1")}},
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) {
			return apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{
				Resources: apitypes.RuntimeProfileResources{Tools: &bindings},
			}}, nil
		},
	}
	// No ToolBuilder is needed when the Workflow lists no Tools, and the Spec
	// carries an untyped nil so transformers send no Tool declarations.
	spec, err := resolver.Resolve(t.Context(), "demo")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if spec.ToolInvoker != nil {
		t.Fatalf("Resolve() ToolInvoker = %#v, want nil", spec.ToolInvoker)
	}
	ids := []string{"giztest-toolkit-echo"}
	workflow := mustWorkflow(t, "workflow-1")
	workflow.Spec.Toolkit = &apitypes.ToolkitPolicy{ToolIds: &ids}
	resolver.Workflows = fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow-1": workflow}}
	if _, err := resolver.Resolve(t.Context(), "demo"); err == nil || !strings.Contains(err.Error(), "toolkit services are required") {
		t.Fatalf("Resolve() error = %v, want missing toolkit services", err)
	}
}
