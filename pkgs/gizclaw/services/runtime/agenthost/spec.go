package agenthost

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

const workspaceAgentTypeParameter = "agent_type"

// Spec is the fully resolved configuration used to construct one agent.
type Spec struct {
	Workspace      apitypes.Workspace
	Workflow       apitypes.Workflow
	AgentType      string
	OwnerPublicKey string
	Runtime        workspace.Runtime
	Toolkit        *ToolkitContext
}

// RuntimeScope identifies the peer-owned workflow configuration behind one
// workspace runtime. It is safe to use as an opaque provider namespace.
func (s Spec) RuntimeScope() string {
	identity := fmt.Sprintf("%d:%s|%d:%s|%d:%s|%d:%s",
		len(s.OwnerPublicKey), s.OwnerPublicKey,
		len(s.Workspace.Name), s.Workspace.Name,
		len(s.Workflow.Name), s.Workflow.Name,
		len(s.AgentType), s.AgentType,
	)
	return fmt.Sprintf("gizclaw-runtime-%x", sha256.Sum256([]byte(identity)))
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
