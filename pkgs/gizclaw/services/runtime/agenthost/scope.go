package agenthost

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// WorkspaceAgentScope returns the stable owner/Workspace/Agent history namespace.
func WorkspaceAgentScope(owner, workspaceID, agentID string) string {
	parts := make([]string, 0, 6)
	if owner = strings.TrimSpace(owner); owner != "" {
		parts = append(parts, "o", scopeToken(owner))
	}
	return strings.Join(append(parts, "w", scopeToken(workspaceID), "a", scopeToken(agentID)), "/")
}

func scopeToken(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return fmt.Sprintf("%x", digest[:8])
}
