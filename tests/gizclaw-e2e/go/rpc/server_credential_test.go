//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerCredentialRPC(t *testing.T) {
	env := newServerResourceHarness(t)
	pageCredential := mutationCredential + "-page"
	for _, name := range []string{mutationCredential, pageCredential} {
		_, _ = env.peer.DeleteCredential(env.ctx, "credential.delete.preclean", rpcapi.CredentialDeleteRequest{Name: name})
	}
	t.Cleanup(func() {
		for _, name := range []string{mutationCredential, pageCredential} {
			_, _ = env.peer.DeleteCredential(env.ctx, "credential.delete.cleanup", rpcapi.CredentialDeleteRequest{Name: name})
		}
	})

	credential, err := env.peer.CreateCredential(env.ctx, "credential.create", rpcCredential(mutationCredential, "sk-created"))
	if err != nil {
		t.Fatalf("credential.create: %v", err)
	}
	if credential.Name != mutationCredential {
		t.Fatalf("credential.create name = %q", credential.Name)
	}
	if _, err := env.peer.CreateCredential(env.ctx, "credential.create.page", rpcCredential(pageCredential, "sk-page")); err != nil {
		t.Fatalf("credential.create page item: %v", err)
	}

	credentialList, err := env.peer.ListCredentials(env.ctx, "credential.list.owned", rpcapi.CredentialListRequest{})
	if err != nil {
		t.Fatalf("credential.list owned: %v", err)
	}
	if len(credentialList.Items) != 2 {
		t.Fatalf("credential.list returned %#v, want two owned credentials", credentialList.Items)
	}
	ownedCredential, err := env.peer.GetCredential(env.ctx, "credential.get.owned", rpcapi.CredentialGetRequest{Name: mutationCredential})
	if err != nil {
		t.Fatalf("credential.get owned: %v", err)
	}
	if ownedCredential.Name != mutationCredential {
		t.Fatalf("credential.get owned name = %q", ownedCredential.Name)
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
	assertCredentialPagination(t, env.ctx, env.peer, mutationCredential, pageCredential)
	if _, err := env.peer.DeleteCredential(env.ctx, "credential.delete", rpcapi.CredentialDeleteRequest{Name: mutationCredential}); err != nil {
		t.Fatalf("credential.delete: %v", err)
	}
	if _, err := env.peer.DeleteCredential(env.ctx, "credential.delete.page", rpcapi.CredentialDeleteRequest{Name: pageCredential}); err != nil {
		t.Fatalf("credential.delete page item: %v", err)
	}
}
