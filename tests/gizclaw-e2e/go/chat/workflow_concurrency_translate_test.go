//go:build gizclaw_e2e

package chat

import "testing"

func TestTranslateWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, translateWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestTranslateWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, translateWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}
