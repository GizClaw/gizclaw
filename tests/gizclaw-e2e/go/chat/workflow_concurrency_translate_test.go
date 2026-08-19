//go:build gizclaw_e2e

package chat

import "testing"

func TestTranslateWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, translateWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestTranslateWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, translateWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestTranslateWorkflowConcurrency50(t *testing.T) {
	runWorkflowConcurrency50(t, translateWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestTranslateWorkflowConcurrencyInterrupt50(t *testing.T) {
	runWorkflowConcurrency50(t, translateWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestTranslateWorkflowRealtimeConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, translateWorkflowRealtimeConcurrencySpec, workflowConcurrencyConversation)
}

func TestTranslateWorkflowRealtimeConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, translateWorkflowRealtimeConcurrencySpec, workflowConcurrencyInterrupt)
}
