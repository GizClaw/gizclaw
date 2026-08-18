//go:build gizclaw_e2e

package chat

import "testing"

func TestEinoWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, einoWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestEinoWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, einoWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}
