package contact

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func TestCRUDUsesDirectFieldsAndPerPeerScope(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	contact, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{
		DisplayName: strPtr("Alice"),
		PhoneNumber: strPtr("+1 (555) 0100"),
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if got := socialutil.StringValue(contact.DisplayName); got != "Alice" {
		t.Fatalf("display_name = %q", got)
	}
	if got := socialutil.StringValue(contact.PhoneNumber); got != "+1 (555) 0100" {
		t.Fatalf("phone_number = %q", got)
	}

	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{PhoneNumber: strPtr("15550100")}); err == nil {
		t.Fatal("CreateContact duplicate phone_number error = nil")
	}
	if _, err := s.CreateContact(ctx, "peer-b", rpcapi.ContactCreateRequest{PhoneNumber: strPtr("15550100")}); err != nil {
		t.Fatalf("CreateContact same phone for another peer: %v", err)
	}

	updated, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Id:          contactID(contact),
		DisplayName: strPtr("Alice Zhang"),
		PhoneNumber: strPtr("+1 555 0101"),
	})
	if err != nil {
		t.Fatalf("PutContact: %v", err)
	}
	if got := socialutil.StringValue(updated.DisplayName); got != "Alice Zhang" {
		t.Fatalf("updated display_name = %q", got)
	}
	phoneOnly, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Id:          contactID(contact),
		PhoneNumber: strPtr("+1 555 0102"),
	})
	if err != nil {
		t.Fatalf("PutContact phone only: %v", err)
	}
	if got := socialutil.StringValue(phoneOnly.DisplayName); got != "Alice Zhang" {
		t.Fatalf("phone-only PutContact display_name = %q, want previous value", got)
	}
	if _, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Id:          contactID(contact),
		DisplayName: strPtr(""),
		PhoneNumber: strPtr(""),
	}); err == nil {
		t.Fatal("PutContact clearing all fields error = nil")
	}

	got, err := s.GetContact(ctx, "peer-a", rpcapi.ContactGetRequest{Id: contactID(contact)})
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if socialutil.StringValue(got.Id) != contactID(contact) {
		t.Fatalf("GetContact id = %q, want %q", socialutil.StringValue(got.Id), contactID(contact))
	}
	list, err := s.ListContacts(ctx, "peer-a", rpcapi.ContactListRequest{})
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("ListContacts len = %d, want 1", len(list.Items))
	}
	deleted, err := s.DeleteContact(ctx, "peer-a", rpcapi.ContactDeleteRequest{Id: contactID(contact)})
	if err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	if socialutil.StringValue(deleted.Id) != contactID(contact) {
		t.Fatalf("DeleteContact id = %q, want %q", socialutil.StringValue(deleted.Id), contactID(contact))
	}
}

func TestDuplicatePhoneScansBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()
	nextID := 0
	s.NewID = func() string {
		nextID++
		return fmt.Sprintf("contact-%03d", nextID)
	}

	var lastPhone string
	for i := range socialutil.MaxListLimit + 1 {
		lastPhone = fmt.Sprintf("+1 555 9%03d", i)
		if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{
			DisplayName: strPtr(fmt.Sprintf("Contact %03d", i)),
			PhoneNumber: strPtr(lastPhone),
		}); err != nil {
			t.Fatalf("CreateContact %d: %v", i, err)
		}
	}
	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{PhoneNumber: strPtr(lastPhone)}); err == nil {
		t.Fatal("CreateContact duplicate phone beyond first page error = nil")
	}
}

func TestConfigurationErrors(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.ListContacts(ctx, "peer-a", rpcapi.ContactListRequest{}); err == nil {
		t.Fatal("ListContacts without store error = nil")
	}
	if _, err := empty.CreateContact(ctx, "", rpcapi.ContactCreateRequest{DisplayName: strPtr("Alice")}); err == nil {
		t.Fatal("CreateContact without store error = nil")
	}
}

func newTestServer() *Server {
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		Store: kv.NewMemory(nil),
		Now:   func() time.Time { return now },
		NewID: func() string {
			nextID++
			return "id-" + string(rune('a'+nextID-1))
		},
	}
}

func strPtr(v string) *string {
	return &v
}

func contactID(item rpcapi.ContactObject) string {
	return socialutil.StringValue(item.Id)
}
