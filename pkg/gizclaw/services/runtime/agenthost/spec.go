package agenthost

import (
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workspace"
)

const workspaceAgentTypeParameter = "agent_type"

// Spec is the fully resolved configuration used to construct one agent.
type Spec struct {
	Workspace apitypes.Workspace
	Workflow  apitypes.WorkflowDocument
	AgentType string
	Runtime   workspace.Runtime
}

func resolveAgentType(workspace apitypes.Workspace, workflow apitypes.WorkflowDocument) (string, error) {
	if workspace.Parameters != nil {
		agentType, err := workspace.Parameters.Discriminator()
		if err != nil {
			return "", fmt.Errorf("agenthost: decode workspace parameters: %w", err)
		}
		agentType = strings.TrimSpace(agentType)
		if agentType == "" {
			return "", fmt.Errorf("agenthost: workspace parameter %q is empty", workspaceAgentTypeParameter)
		}
		return agentType, nil
	}
	return agentTypeFromWorkflow(workflow)
}

func agentTypeFromWorkflow(workflow apitypes.WorkflowDocument) (string, error) {
	driver := strings.TrimSpace(string(workflow.Spec.Driver))
	if driver == "" {
		return "", fmt.Errorf("agenthost: workflow spec.driver is required")
	}
	if !workflow.Spec.Driver.Valid() {
		return "", fmt.Errorf("agenthost: unsupported workflow spec.driver %q", workflow.Spec.Driver)
	}
	return driver, nil
}
