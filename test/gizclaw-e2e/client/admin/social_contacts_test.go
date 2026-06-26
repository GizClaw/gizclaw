//go:build gizclaw_e2e

package admin_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
)

func TestAdminAPIContactsListGetCreatePutDelete(t *testing.T) {
	env := newAdminAPIHarness(t)
	contactID := mutationName("contact")
	phone := fmt.Sprintf("+1555%d", time.Now().UnixNano()%1000000000)

	created, err := env.api.CreateContactWithResponse(env.ctx, adminservice.AdminContactCreateRequest{
		OwnerPublicKey: env.adminKey,
		Id:             ptr(contactID),
		DisplayName:    ptr("Admin API Contact"),
		PhoneNumber:    ptr(phone),
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	requireStatusOK(t, created, created.Body)
	if created.JSON200 == nil || created.JSON200.OwnerPublicKey != env.adminKey || created.JSON200.Id != contactID {
		t.Fatalf("created contact = %#v", created.JSON200)
	}
	t.Cleanup(func() { _, _ = env.api.DeleteContactWithResponse(env.ctx, env.adminKey, contactID) })

	duplicate, err := env.api.CreateContactWithResponse(env.ctx, adminservice.AdminContactCreateRequest{
		OwnerPublicKey: env.adminKey,
		Id:             ptr(mutationName("contact-dup")),
		PhoneNumber:    ptr(phone),
	})
	if err != nil {
		t.Fatalf("create duplicate phone contact: %v", err)
	}
	if duplicate.StatusCode() == 200 {
		t.Fatalf("duplicate phone status = 200, body=%s", string(duplicate.Body))
	}

	got, err := env.api.GetContactWithResponse(env.ctx, env.adminKey, contactID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}
	requireStatusOK(t, got, got.Body)
	if got.JSON200 == nil || got.JSON200.PhoneNumber == nil || *got.JSON200.PhoneNumber != phone {
		t.Fatalf("got contact = %#v", got.JSON200)
	}

	ownerRows := collectAdminPagesInt(t, 1, func(cursor *string, limit int) ([]adminservice.AdminContactObject, bool, *string) {
		resp, err := env.api.ListContactsWithResponse(env.ctx, &adminservice.ListContactsParams{OwnerPublicKey: ptr(env.adminKey), Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list owner contacts: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list owner contacts missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, ownerRows, contactID, func(item adminservice.AdminContactObject) string {
		return item.Id
	})

	allRows := collectAdminPagesInt(t, 2, func(cursor *string, limit int) ([]adminservice.AdminContactObject, bool, *string) {
		resp, err := env.api.ListContactsWithResponse(env.ctx, &adminservice.ListContactsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			t.Fatalf("list contacts: %v", err)
		}
		requireStatusOK(t, resp, resp.Body)
		if resp.JSON200 == nil {
			t.Fatalf("list contacts missing JSON200")
		}
		return resp.JSON200.Items, resp.JSON200.HasNext, resp.JSON200.NextCursor
	})
	requireName(t, allRows, contactID, func(item adminservice.AdminContactObject) string {
		if item.OwnerPublicKey == env.adminKey {
			return item.Id
		}
		return ""
	})

	updated, err := env.api.PutContactWithResponse(env.ctx, env.adminKey, contactID, adminservice.AdminContactPutRequest{
		DisplayName: ptr("Renamed Contact"),
		PhoneNumber: ptr(phone + "1"),
	})
	if err != nil {
		t.Fatalf("put contact: %v", err)
	}
	requireStatusOK(t, updated, updated.Body)
	if updated.JSON200 == nil || updated.JSON200.DisplayName == nil || *updated.JSON200.DisplayName != "Renamed Contact" {
		t.Fatalf("updated contact = %#v", updated.JSON200)
	}

	deleted, err := env.api.DeleteContactWithResponse(env.ctx, env.adminKey, contactID)
	if err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	requireStatusOK(t, deleted, deleted.Body)
	missing, err := env.api.GetContactWithResponse(env.ctx, env.adminKey, contactID)
	if err != nil {
		t.Fatalf("get deleted contact: %v", err)
	}
	if missing.StatusCode() != 404 {
		t.Fatalf("get deleted contact status = %d body=%s", missing.StatusCode(), string(missing.Body))
	}
}
