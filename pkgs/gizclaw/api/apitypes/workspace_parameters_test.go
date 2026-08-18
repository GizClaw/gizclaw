package apitypes

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceParametersChatRoomBranch(t *testing.T) {
	mode := ChatRoomModeGroup
	input := WorkspaceInputModePushToTalk
	ttl := "168h"
	asr := "e2e-asr"
	transcriptEnabled := true
	var params WorkspaceParameters
	if err := params.FromChatRoomWorkspaceParameters(ChatRoomWorkspaceParameters{
		Input: &input,
		Mode:  &mode,
		History: &ChatRoomWorkspaceHistoryParameters{
			Ttl: &ttl,
		},
		Transcript: &ChatRoomWorkspaceTranscriptParameters{
			Enabled:  &transcriptEnabled,
			AsrModel: &asr,
		},
	}); err != nil {
		t.Fatalf("FromChatRoomWorkspaceParameters() error = %v", err)
	}
	if got, err := params.Discriminator(); err != nil || got != "chatroom" {
		t.Fatalf("Discriminator() = %q, %v; want chatroom", got, err)
	}
	value, err := params.ValueByDiscriminator()
	if err != nil {
		t.Fatalf("ValueByDiscriminator() error = %v", err)
	}
	typed, ok := value.(ChatRoomWorkspaceParameters)
	if !ok {
		t.Fatalf("ValueByDiscriminator() = %T, want ChatRoomWorkspaceParameters", value)
	}
	if typed.AgentType != ChatRoomWorkspaceParametersAgentTypeChatroom {
		t.Fatalf("agent_type = %q", typed.AgentType)
	}
	if typed.History == nil || typed.History.Ttl == nil || *typed.History.Ttl != ttl {
		t.Fatalf("history = %#v", typed.History)
	}
	if typed.Transcript == nil || typed.Transcript.AsrModel == nil || *typed.Transcript.AsrModel != asr {
		t.Fatalf("transcript = %#v", typed.Transcript)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("MarshalJSON() produced invalid JSON: %s", raw)
	}
}

func TestWorkspaceParametersNewWorkflowBranches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want string
		set  func(*WorkspaceParameters) error
	}{
		{
			name: "dashscope realtime",
			want: "dashscope-realtime",
			set: func(parameters *WorkspaceParameters) error {
				return parameters.FromDashScopeRealtimeWorkspaceParameters(DashScopeRealtimeWorkspaceParameters{
					AgentType: DashScopeRealtimeWorkspaceParametersAgentTypeDashscopeRealtime,
				})
			},
		},
		{
			name: "doubao realtime duplex",
			want: "doubao-realtime-duplex",
			set: func(parameters *WorkspaceParameters) error {
				return parameters.FromDoubaoRealtimeDuplexWorkspaceParameters(DoubaoRealtimeDuplexWorkspaceParameters{
					AgentType: DoubaoRealtimeDuplexWorkspaceParametersAgentTypeDoubaoRealtimeDuplex,
				})
			},
		},
		{
			name: "eino",
			want: "eino",
			set: func(parameters *WorkspaceParameters) error {
				mode := WorkspaceInputModeRealtime
				return parameters.FromEinoWorkspaceParameters(EinoWorkspaceParameters{
					AgentType: EinoWorkspaceParametersAgentTypeEino,
					Input:     &mode,
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var parameters WorkspaceParameters
			if err := test.set(&parameters); err != nil {
				t.Fatal(err)
			}
			if got, err := parameters.Discriminator(); err != nil || got != test.want {
				t.Fatalf("Discriminator() = %q, %v; want %q", got, err, test.want)
			}
			raw, err := json.Marshal(parameters)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip WorkspaceParameters
			if err := json.Unmarshal(raw, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if got, err := roundTrip.Discriminator(); err != nil || got != test.want {
				t.Fatalf("round-trip Discriminator() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
