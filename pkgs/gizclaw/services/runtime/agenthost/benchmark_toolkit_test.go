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
	echo := putAgentHostTool(t, server, agentHostClientTool("giztest_echo"))
	resolver := ServiceResolver{ToolBuilder: &toolkit.Builder{Tools: server}}
	ctx := toolTestContext(t, map[string]string{"giztest-echo": echo.ID}, nil)
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
			if err != nil {
				t.Fatal(err)
			}
			definitions, err := invoker.ResolveTools(ctx)
			if err != nil || len(definitions) != 0 {
				t.Fatalf("benchmark tools = %v, error = %v", definitions, err)
			}
			// Omission still inherits the RuntimeProfile, including client tools.
			resource.Spec.Toolkit = nil
			inherited, err := resolver.resolveToolkit(ctx, apitypes.Workspace{}, apitypes.Workflow{Spec: resource.Spec})
			if err != nil {
				t.Fatal(err)
			}
			definitions, err = inherited.ResolveTools(ctx)
			if err != nil || len(definitions) != 1 || definitions[0].Name != "giztest_echo" {
				t.Fatalf("inherited tools = %v, error = %v", definitions, err)
			}
		})
	}
}
