package agenthost

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const workspaceAgentTypeParameter = "agent_type"

// Spec is the fully resolved configuration used to construct one agent.
type Spec struct {
	Workspace   apitypes.Workspace
	Workflow    apitypes.Workflow
	AgentType   string
	Runtime     workspace.Runtime
	ToolInvoker genx.ToolInvoker
	// Memory is the Workspace-bound provider-neutral Store selected through
	// the current RuntimeProfile. MemoryCloser belongs to this Agent generation.
	Memory       memory.Store
	MemoryKind   string
	MemoryCloser io.Closer
	// MemoryBinding and MemoryLayout are the owner RuntimeProfile snapshot
	// selected for Workflow.Spec.Memory. A peer-local factory turns them into
	// one Workspace-bound Store generation.
	MemoryBinding *apitypes.RuntimeProfileMemoryBinding
	MemoryLayout  *apitypes.MemoryLayout
	// MemoryName is the stable RuntimeProfile binding alias. It identifies the
	// selected physical store independently from the portable Layout policy.
	MemoryName string
	// MemoryProfileName and MemoryProfileRevision identify the immutable
	// RuntimeProfile snapshot that selected the binding.
	MemoryProfileName     string
	MemoryProfileRevision string
	// BoardInputs supplies product-owned transient values to drivers that
	// support a per-turn context board. The outer runtime also injects the same
	// values into the Transform context for every nested driver. They are never
	// persisted in the Workspace.
	BoardInputs func(context.Context) (map[string]any, error)
}

func resolveAgentType(workspace apitypes.Workspace, workflow apitypes.Workflow) (string, error) {
	if workspace.Parameters != nil {
		agentType, err := workspace.Parameters.Discriminator()
		if err != nil {
			return "", fmt.Errorf("agenthost: decode workspace parameters: %w", err)
		}
		agentType = strings.TrimSpace(agentType)
		if agentType == "" {
			return "", fmt.Errorf("agenthost: workspace parameter %q is empty", workspaceAgentTypeParameter)
		}
		workflowType, err := agentTypeFromWorkflow(workflow)
		if err != nil {
			return "", err
		}
		if agentType != workflowType {
			return "", fmt.Errorf("agenthost: workspace agent_type %q does not match workflow driver %q", agentType, workflowType)
		}
		return agentType, nil
	}
	return agentTypeFromWorkflow(workflow)
}

func agentTypeFromWorkflow(workflow apitypes.Workflow) (string, error) {
	driver := strings.TrimSpace(string(workflow.Spec.Driver))
	if driver == "" {
		return "", fmt.Errorf("agenthost: workflow spec.driver is required")
	}
	if !workflow.Spec.Driver.Valid() {
		return "", fmt.Errorf("agenthost: unsupported workflow spec.driver %q", workflow.Spec.Driver)
	}
	return driver, nil
}
