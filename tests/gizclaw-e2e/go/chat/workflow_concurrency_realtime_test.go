//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestRealtimeWorkflowRealtimeConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowRealtimeConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeWorkflowRealtimeConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowRealtimeConcurrencySpec, workflowConcurrencyInterrupt)
}
