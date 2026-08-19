//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeWorkflowConcurrency1(t *testing.T) {
	runWorkflowConcurrency(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyConversation, 1)
}

func TestRealtimeWorkflowConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeWorkflowConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestRealtimeWorkflowConcurrency20(t *testing.T) {
	runWorkflowConcurrency20(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeWorkflowConcurrencyInterrupt20(t *testing.T) {
	runWorkflowConcurrency20(t, realtimeWorkflowConcurrencySpec, workflowConcurrencyInterrupt)
}

func TestRealtimeWorkflowRealtimeConcurrency1(t *testing.T) {
	runWorkflowConcurrency(t, realtimeWorkflowRealtimeConcurrencySpec, workflowConcurrencyConversation, 1)
}

func TestRealtimeWorkflowRealtimeConcurrency10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowRealtimeConcurrencySpec, workflowConcurrencyConversation)
}

func TestRealtimeWorkflowRealtimeConcurrencyInterrupt10(t *testing.T) {
	runWorkflowConcurrency10(t, realtimeWorkflowRealtimeConcurrencySpec, workflowConcurrencyInterrupt)
}
