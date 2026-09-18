package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// speechRateWorkspaceResources resolves every Workspace selection to one Eino
// Workspace with the configured speech rate.
type speechRateWorkspaceResources struct {
	fakeRPCRunWorkspaceResources
	parameters *apitypes.WorkspaceParameters
}

func (f *speechRateWorkspaceResources) ResolveRunWorkspaceSelection(_ context.Context, name string) (apitypes.Workspace, *rpcapi.RPCStatus) {
	return apitypes.Workspace{Name: name, Parameters: f.parameters}, nil
}

func TestServerRunSayUsesActiveWorkspaceSpeechRate(t *testing.T) {
	publicKey := giznet.PublicKey{4, 5, 6}
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromEinoWorkspaceParameters(apitypes.EinoWorkspaceParameters{
		AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino, TtsSpeechRatePercent: new(70),
	}); err != nil {
		t.Fatal(err)
	}
	peerRun := peerruntest.New(t)
	genX := &fakeRPCServerGenXService{}
	server := &rpcServer{
		peerRun:         peerRun,
		serverResources: &speechRateWorkspaceResources{parameters: &parameters},
		serverGenX:      genX,
		callerPublicKey: publicKey,
	}
	say := func() {
		t.Helper()
		resp, err := server.dispatch(context.Background(), newRPCRequest("say", rpcapi.RPCMethodServerRunSay,
			mustRPCParams(rpcapi.ServerRunSayRequest{Text: "hello", VoiceName: "narrator"}, (*rpcapi.RPCPayload).FromServerRunSayRequest)))
		if err != nil || resp.Error != nil {
			t.Fatalf("server.run.say = %+v, %v", resp, err)
		}
	}

	say()
	if genX.lastSay.SpeechRatePercent != nil {
		t.Fatalf("say without an active Workspace rate = %v, want default", *genX.lastSay.SpeechRatePercent)
	}
	selection := apitypes.AgentSelection{WorkspaceName: "story"}
	if _, err := peerRun.SetRunAgent(context.Background(), publicKey, selection); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	say()
	if genX.lastSay.SpeechRatePercent != nil {
		t.Fatal("say used a pending Workspace rate before activation")
	}
	if _, err := peerRun.ActivateRunAgent(context.Background(), publicKey, selection); err != nil {
		t.Fatalf("ActivateRunAgent() error = %v", err)
	}
	say()
	if genX.lastSay.SpeechRatePercent == nil || *genX.lastSay.SpeechRatePercent != 70 {
		t.Fatalf("say rate = %v, want 70", genX.lastSay.SpeechRatePercent)
	}
}
