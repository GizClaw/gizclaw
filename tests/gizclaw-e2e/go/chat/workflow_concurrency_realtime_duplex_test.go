//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeDuplexWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeDuplexWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeDuplexWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeDuplexWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}
