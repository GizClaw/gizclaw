package eino

import (
	"testing"

	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestMapInitiativePrecedence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		workflow  *apitypes.EinoConversationInitiative
		workspace *apitypes.ConversationParameters
		want      genxeino.InitiativePolicy
	}{
		{name: "default peer", want: genxeino.InitiativeDisabled},
		{name: "workflow peer", workflow: new(apitypes.EinoConversationInitiativePeer), want: genxeino.InitiativeDisabled},
		{name: "workflow agent", workflow: new(apitypes.EinoConversationInitiativeAgent), want: genxeino.InitiativeOnReload},
		{
			name: "workspace peer overrides agent", workflow: new(apitypes.EinoConversationInitiativeAgent),
			workspace: &apitypes.ConversationParameters{Initiative: new(apitypes.ConversationParametersInitiativePeer)}, want: genxeino.InitiativeDisabled,
		},
		{
			name: "workspace agent overrides peer", workflow: new(apitypes.EinoConversationInitiativePeer),
			workspace: &apitypes.ConversationParameters{Initiative: new(apitypes.ConversationParametersInitiativeAgent)}, want: genxeino.InitiativeOnReload,
		},
		{
			name:      "workspace agent without workflow default",
			workspace: &apitypes.ConversationParameters{Initiative: new(apitypes.ConversationParametersInitiativeAgent)}, want: genxeino.InitiativeOnReload,
		},
		{
			name: "workspace policy uses workflow agent", workflow: new(apitypes.EinoConversationInitiativeAgent),
			workspace: &apitypes.ConversationParameters{AgentInitiativePolicy: new(apitypes.ConversationParametersAgentInitiativePolicyOnceWhenEmpty)}, want: genxeino.InitiativeOnceWhenEmpty,
		},
		{
			name:      "policy alone does not enable opening",
			workspace: &apitypes.ConversationParameters{AgentInitiativePolicy: new(apitypes.ConversationParametersAgentInitiativePolicyOnceWhenEmpty)}, want: genxeino.InitiativeDisabled,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var conversation *apitypes.EinoConversation
			if test.workflow != nil {
				conversation = &apitypes.EinoConversation{Initiative: test.workflow}
			}
			var parameters *apitypes.WorkspaceParameters
			if test.workspace != nil {
				parameters = new(apitypes.WorkspaceParameters)
				if err := parameters.FromEinoWorkspaceParameters(apitypes.EinoWorkspaceParameters{AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino, Conversation: test.workspace}); err != nil {
					t.Fatal(err)
				}
			}
			if got := mapInitiative(conversation, parameters); got != test.want {
				t.Fatalf("mapInitiative() = %q, want %q", got, test.want)
			}
		})
	}
}
