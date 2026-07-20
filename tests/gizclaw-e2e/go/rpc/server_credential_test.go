//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerCredentialRPCMethodsAreAbsent(t *testing.T) {
	for _, method := range []rpcapi.RPCMethod{
		"server.credential.list",
		"server.credential.get",
		"server.credential.create",
		"server.credential.put",
		"server.credential.delete",
	} {
		if method.Valid() {
			t.Fatalf("removed Peer RPC method %q is still registered", method)
		}
	}
}
