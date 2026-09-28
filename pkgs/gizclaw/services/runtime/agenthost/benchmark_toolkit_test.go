package agenthost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"sigs.k8s.io/yaml"
)

func TestBenchmarkWorkflowsExcludeProfileTools(t *testing.T) {
	server := toolkittest.New(t)
	echo := putAgentHostTool(t, server, agentHostBoundHTTPTool("giztest_echo"))
	resolver := ServiceResolver{ToolBuilder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{"giztest-echo": echo.ID})
	for _, name := range []string{
		"05-flowcraft-basic.yaml",
		"20-flowcraft-latency-comparison.yaml",
		"21-eino-latency-comparison.yaml",
		"31-eino-concurrency.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../../../../tests/gizclaw-e2e/testdata/resources/04-workflows", name))
			if err != nil {
				t.Fatal(err)
			}
			var resource struct {
				Spec apitypes.WorkflowSpec `json:"spec"`
			}
			if err := yaml.Unmarshal(data, &resource); err != nil {
				t.Fatal(err)
			}
			invoker, err := resolver.resolveToolkit(ctx, apitypes.Workspace{}, apitypes.Workflow{Spec: resource.Spec})
			if err != nil || invoker != nil {
				t.Fatalf("benchmark ToolInvoker = %#v, error = %v", invoker, err)
			}
			// Omission is the same opt-out; it never inherits RuntimeProfile Tools.
			resource.Spec.Toolkit = nil
			invoker, err = resolver.resolveToolkit(ctx, apitypes.Workspace{}, apitypes.Workflow{Spec: resource.Spec})
			if err != nil || invoker != nil {
				t.Fatalf("omitted-policy ToolInvoker = %#v, error = %v", invoker, err)
			}
		})
	}
}
