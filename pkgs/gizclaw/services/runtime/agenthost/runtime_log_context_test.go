package agenthost

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"testing"
)

func TestRuntimeLoggingRetainsConnectionWithoutReloadCredential(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{PublicKey: key.Public, SessionID: "connection"}
	request := gizlog.WithAPIKeyName(gizlog.WithRequestID(t.Context(), "reload"), "key-reload")
	runtime := service.runtimeLogContext(request)
	if gizlog.PeerPublicKey(runtime) != key.Public.String() || gizlog.SessionID(runtime) != "connection" || gizlog.RequestID(runtime) != "" || gizlog.APIKeyName(runtime) != "" {
		t.Fatal("runtime retained request identity or lost connection")
	}
	if gizlog.RequestID(request) != "reload" || gizlog.APIKeyName(request) != "key-reload" {
		t.Fatal("mutated request context")
	}
}
