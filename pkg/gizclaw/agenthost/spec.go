package agenthost

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

const workspaceAgentTypeParameter = "agent_type"

// Spec is the fully resolved configuration used to construct one agent.
type Spec struct {
	Workspace apitypes.Workspace
	Workflow  apitypes.WorkflowDocument
	AgentType string
	Runtime   WorkspaceRuntime
}

type workflowEnvelope struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
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
	data, err := json.Marshal(workflow)
	if err != nil {
		return "", fmt.Errorf("agenthost: encode workflow: %w", err)
	}
	var env workflowEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return "", fmt.Errorf("agenthost: decode workflow envelope: %w", err)
	}
	apiVersion := strings.TrimSpace(env.APIVersion)
	if apiVersion == "" {
		return "", errors.New("agenthost: workflow apiVersion is required")
	}
	group, _, ok := strings.Cut(apiVersion, "/")
	if !ok || strings.TrimSpace(group) == "" {
		return "", fmt.Errorf("agenthost: unsupported workflow apiVersion %q", apiVersion)
	}
	group = strings.TrimSpace(group)
	if strings.HasPrefix(group, "gizclaw.") {
		group = strings.TrimPrefix(group, "gizclaw.")
	}
	if group == "" {
		return "", fmt.Errorf("agenthost: unsupported workflow apiVersion %q", apiVersion)
	}
	return group, nil
}
