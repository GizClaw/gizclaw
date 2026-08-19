//go:build gizclaw_e2e

package chat

import "testing"

func TestEinoWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, einoWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestEinoWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, einoWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestEinoWorkflowConcurrency50(t *testing.T) {
	runWorkflowConcurrency50(t, einoWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestEinoWorkflowConcurrencyInterrupt50(t *testing.T) {
	runWorkflowConcurrency50(t, einoWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestEinoWorkflowRealtimeConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, einoWorkflowRealtimeConcurrencySpec, workflowConcurrencyConversation)
}

func TestEinoWorkflowRealtimeConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, einoWorkflowRealtimeConcurrencySpec, workflowConcurrencyInterrupt)
}
