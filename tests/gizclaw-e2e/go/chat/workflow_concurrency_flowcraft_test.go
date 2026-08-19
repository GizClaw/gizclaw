//go:build gizclaw_e2e

package chat

import "testing"

func TestFlowcraftWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, flowcraftWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestFlowcraftWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, flowcraftWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestFlowcraftWorkflowConcurrency20(t *testing.T) {
	runWorkflowConcurrency20(t, flowcraftWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestFlowcraftWorkflowConcurrencyInterrupt20(t *testing.T) {
	runWorkflowConcurrency20(t, flowcraftWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestFlowcraftWorkflowRealtimeConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, flowcraftWorkflowRealtimeConcurrencySpec, workflowConcurrencyConversation)
}

func TestFlowcraftWorkflowRealtimeConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, flowcraftWorkflowRealtimeConcurrencySpec, workflowConcurrencyInterrupt)
}
