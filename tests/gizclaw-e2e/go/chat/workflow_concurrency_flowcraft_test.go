//go:build gizclaw_e2e

package chat

import "testing"

func TestFlowcraftWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, flowcraftWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestFlowcraftWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, flowcraftWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}
