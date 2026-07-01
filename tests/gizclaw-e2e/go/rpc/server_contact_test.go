//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerContactRPC(t *testing.T) {
	env := newSocialRPCHarness(t)

	aliceName := "Alice"
	alicePhone := "+1 555 0100"
	alice, err := env.a.CreateContact(env.ctx, "contact.create.alice", rpcapi.ContactCreateRequest{
		DisplayName: &aliceName,
		PhoneNumber: &alicePhone,
	})
	if err != nil {
		t.Fatalf("contact.create alice: %v", err)
	}
	if alice.Id == nil || *alice.Id == "" {
		t.Fatalf("contact.create alice id is empty: %#v", alice)
	}
	bobName := "Bob"
	bob, err := env.a.CreateContact(env.ctx, "contact.create.bob", rpcapi.ContactCreateRequest{DisplayName: &bobName})
	if err != nil {
		t.Fatalf("contact.create bob: %v", err)
	}

	got, err := env.a.GetContact(env.ctx, "contact.get", rpcapi.ContactGetRequest{Id: *alice.Id})
	if err != nil {
		t.Fatalf("contact.get: %v", err)
	}
	if got.DisplayName == nil || *got.DisplayName != aliceName {
		t.Fatalf("contact.get display_name = %#v", got.DisplayName)
	}
	updatedName := "Alice Zhang"
	updated, err := env.a.PutContact(env.ctx, "contact.put", rpcapi.ContactPutRequest{
		Id:          *alice.Id,
		DisplayName: &updatedName,
		PhoneNumber: &alicePhone,
	})
	if err != nil {
		t.Fatalf("contact.put: %v", err)
	}
	if updated.DisplayName == nil || *updated.DisplayName != updatedName {
		t.Fatalf("contact.put display_name = %#v", updated.DisplayName)
	}
	limit := 1
	first, err := env.a.ListContacts(env.ctx, "contact.list.page1", rpcapi.ContactListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("contact.list page1: %v", err)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("contact.list page1 = %#v", first)
	}
	second, err := env.a.ListContacts(env.ctx, "contact.list.page2", rpcapi.ContactListRequest{Limit: &limit, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("contact.list page2: %v", err)
	}
	if len(second.Items) != 1 || second.HasNext {
		t.Fatalf("contact.list page2 = %#v", second)
	}
	if _, err := env.b.GetContact(env.ctx, "contact.get.denied", rpcapi.ContactGetRequest{Id: *alice.Id}); err == nil {
		t.Fatal("peer-b unexpectedly read peer-a contact")
	}
	deleted, err := env.a.DeleteContact(env.ctx, "contact.delete", rpcapi.ContactDeleteRequest{Id: *bob.Id})
	if err != nil {
		t.Fatalf("contact.delete: %v", err)
	}
	if deleted.Id == nil || *deleted.Id != *bob.Id {
		t.Fatalf("contact.delete id = %#v, want %q", deleted.Id, *bob.Id)
	}
}
