//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerCredentialRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	credentialList, err := env.peer.ListCredentials(env.ctx, "credential.list.shared", rpcapi.CredentialListRequest{})
	if err != nil {
		t.Fatalf("credential.list shared: %v", err)
	}
	if len(credentialList.Items) == 0 {
		t.Fatalf("credential.list returned no items")
	}
	sharedCredentialObject, err := env.peer.GetCredential(env.ctx, "credential.get.shared", rpcapi.CredentialGetRequest{Name: sharedCredential})
	if err != nil {
		t.Fatalf("credential.get shared: %v", err)
	}
	if sharedCredentialObject.Name != sharedCredential {
		t.Fatalf("credential.get shared name = %q", sharedCredentialObject.Name)
	}
	_, _ = env.peer.DeleteCredential(env.ctx, "credential.delete.preclean", rpcapi.CredentialDeleteRequest{Name: mutationCredential})
	credential, err := env.peer.CreateCredential(env.ctx, "credential.create", rpcCredential(mutationCredential, "sk-created"))
	if err != nil {
		t.Fatalf("credential.create: %v", err)
	}
	if credential.Name != mutationCredential {
		t.Fatalf("credential.create name = %q", credential.Name)
	}
	credential, err = env.peer.PutCredential(env.ctx, "credential.put", rpcapi.CredentialPutRequest{
		Name: mutationCredential,
		Body: rpcCredential(mutationCredential, "sk-updated"),
	})
	if err != nil {
		t.Fatalf("credential.put: %v", err)
	}
	if testRPCCredentialBodyString(credential.Body, "api_key") != "sk-updated" {
		t.Fatalf("credential.put body = %#v", credential.Body)
	}
	credential, err = env.peer.GetCredential(env.ctx, "credential.get.updated", rpcapi.CredentialGetRequest{Name: mutationCredential})
	if err != nil {
		t.Fatalf("credential.get updated: %v", err)
	}
	if testRPCCredentialBodyString(credential.Body, "api_key") != "sk-updated" {
		t.Fatalf("credential.get updated body = %#v", credential.Body)
	}
	assertCredentialPagination(t, env.ctx, env.peer, sharedCredential, mutationCredential)
	if _, err := env.peer.DeleteCredential(env.ctx, "credential.delete", rpcapi.CredentialDeleteRequest{Name: mutationCredential}); err != nil {
		t.Fatalf("credential.delete: %v", err)
	}
}
