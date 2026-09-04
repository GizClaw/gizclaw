package apitypes

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceParametersRejectsChatroomAgentType(t *testing.T) {
	t.Parallel()
	var parameters WorkspaceParameters
	if err := json.Unmarshal([]byte(`{"agent_type":"chatroom","mode":"group"}`), &parameters); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, err := parameters.ValueByDiscriminator(); err == nil {
		t.Fatal("ValueByDiscriminator() error = nil, want unknown discriminator")
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
		{
			name: "pet",
			want: "pet",
			set: func(parameters *WorkspaceParameters) error {
				mode := WorkspaceInputModeRealtime
				return parameters.FromPetWorkspaceParameters(PetWorkspaceParameters{
					AgentType: PetWorkspaceParametersAgentTypePet,
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
