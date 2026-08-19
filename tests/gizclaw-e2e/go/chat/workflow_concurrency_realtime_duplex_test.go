//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeDuplexWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeDuplexWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeDuplexWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeDuplexWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestRealtimeDuplexWorkflowConcurrency20(t *testing.T) {
	runWorkflowConcurrency20(t, realtimeDuplexWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeDuplexWorkflowConcurrencyInterrupt20(t *testing.T) {
	runWorkflowConcurrency20(t, realtimeDuplexWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}
