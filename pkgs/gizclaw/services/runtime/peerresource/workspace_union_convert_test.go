package peerresource

import (
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestWorkspaceParametersNewDriversRoundTrip(t *testing.T) {
	t.Parallel()

	boolValue := true
	intValue := 24_000
	floatValue := float32(0.7)
	stringValue := "value"
	modalities := []string{"text", "audio"}

	tests := []struct {
		name  string
		input func(t *testing.T) rpcapi.WorkspaceParameters
	}{
		{
			name: "dashscope realtime",
			input: func(t *testing.T) rpcapi.WorkspaceParameters {
				t.Helper()
				var parameters rpcapi.WorkspaceParameters
				if err := parameters.FromDashScopeRealtimeWorkspaceParameters(rpcapi.DashScopeRealtimeWorkspaceParameters{
					AgentType:         rpcapi.DashScopeRealtimeWorkspaceParametersAgentTypeDashscopeRealtime,
					AsrModel:          &stringValue,
					E2e:               &boolValue,
					EnableAsr:         &boolValue,
					InputAudioFormat:  &stringValue,
					Instructions:      &stringValue,
					MaxOutputTokens:   &intValue,
					Modalities:        &modalities,
					Model:             &stringValue,
					OutputAudioFormat: &stringValue,
					Temperature:       &floatValue,
					Vad:               &stringValue,
					Voice:             &stringValue,
				}); err != nil {
					t.Fatalf("build RPC parameters: %v", err)
				}
				return parameters
			},
		},
		{
			name: "doubao realtime duplex",
			input: func(t *testing.T) rpcapi.WorkspaceParameters {
				t.Helper()
				var parameters rpcapi.WorkspaceParameters
				if err := parameters.FromDoubaoRealtimeDuplexWorkspaceParameters(rpcapi.DoubaoRealtimeDuplexWorkspaceParameters{
					AgentType:       rpcapi.DoubaoRealtimeDuplexWorkspaceParametersAgentTypeDoubaoRealtimeDuplex,
					E2e:             &boolValue,
					Format:          &stringValue,
					InputChannels:   &intValue,
					InputFormat:     &stringValue,
					InputSampleRate: &intValue,
					InputTranscode:  &boolValue,
					Instructions:    &stringValue,
					Model:           &stringValue,
					OutputLoudness:  &intValue,
					OutputSpeed:     &intValue,
					SampleRate:      &intValue,
					Voice:           &stringValue,
				}); err != nil {
					t.Fatalf("build RPC parameters: %v", err)
				}
				return parameters
			},
		},
		{
			name: "eino",
			input: func(t *testing.T) rpcapi.WorkspaceParameters {
				t.Helper()
				var parameters rpcapi.WorkspaceParameters
				if err := parameters.FromEinoWorkspaceParameters(rpcapi.EinoWorkspaceParameters{
					AgentType: rpcapi.EinoWorkspaceParametersAgentTypeEino,
					E2e:       &boolValue,
				}); err != nil {
					t.Fatalf("build RPC parameters: %v", err)
				}
				return parameters
			},
		},
		{
			name: "pet",
			input: func(t *testing.T) rpcapi.WorkspaceParameters {
				t.Helper()
				input := rpcapi.WorkspaceInputModeRealtime
				var parameters rpcapi.WorkspaceParameters
				if err := parameters.FromPetWorkspaceParameters(rpcapi.PetWorkspaceParameters{
					AgentType: rpcapi.PetWorkspaceParametersAgentTypePet,
					Input:     &input,
				}); err != nil {
					t.Fatalf("build RPC parameters: %v", err)
				}
				return parameters
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := test.input(t)
			apiParameters, err := rpcWorkspaceParametersToAPI(input)
			if err != nil {
				t.Fatalf("convert RPC parameters to API: %v", err)
			}
			output, err := apiWorkspaceParametersToRPC(apiParameters)
			if err != nil {
				t.Fatalf("convert API parameters to RPC: %v", err)
			}
			if !reflect.DeepEqual(output, input) {
				t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", output, input)
			}
		})
	}
}
