package contact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestCRUDUsesDirectFieldsAndPerPeerScope(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	contact, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{
		Name:        "alice001",
		DisplayName: new("Alice"),
		PhoneNumber: new("+1 (555) 0100"),
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

	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{Name: "alice002", PhoneNumber: new("15550100")}); err == nil {
		t.Fatal("CreateContact duplicate phone_number error = nil")
	}
	if _, err := s.CreateContact(ctx, "peer-b", rpcapi.ContactCreateRequest{Name: "alice001", PhoneNumber: new("15550100")}); err != nil {
		t.Fatalf("CreateContact same phone for another peer: %v", err)
	}

	updated, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Name:        contact.Name,
		DisplayName: new("Alice Zhang"),
		PhoneNumber: new("+1 555 0101"),
	})
	if err != nil {
		t.Fatalf("PutContact: %v", err)
	}
	if got := socialutil.StringValue(updated.DisplayName); got != "Alice Zhang" {
		t.Fatalf("updated display_name = %q", got)
	}
	phoneOnly, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Name:        contact.Name,
		PhoneNumber: new("+1 555 0102"),
	})
	if err != nil {
		t.Fatalf("PutContact phone only: %v", err)
	}
	if got := socialutil.StringValue(phoneOnly.DisplayName); got != "Alice Zhang" {
		t.Fatalf("phone-only PutContact display_name = %q, want previous value", got)
	}
	if _, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{
		Name:        contact.Name,
		DisplayName: new(""),
		PhoneNumber: new(""),
	}); err == nil {
		t.Fatal("PutContact clearing all fields error = nil")
	}

	got, err := s.GetContact(ctx, "peer-a", rpcapi.ContactGetRequest{Name: contact.Name})
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if got.Name != contact.Name {
		t.Fatalf("GetContact name = %q, want %q", got.Name, contact.Name)
	}
	list, err := s.ListContacts(ctx, "peer-a", rpcapi.ContactListRequest{})
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("ListContacts len = %d, want 1", len(list.Items))
	}
	deleted, err := s.DeleteContact(ctx, "peer-a", rpcapi.ContactDeleteRequest{Name: contact.Name})
	if err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	if deleted.Name != contact.Name {
		t.Fatalf("DeleteContact name = %q, want %q", deleted.Name, contact.Name)
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
			Name:        fmt.Sprintf("contact-%03d", i),
			DisplayName: new(fmt.Sprintf("Contact %03d", i)),
			PhoneNumber: new(lastPhone),
		}); err != nil {
			t.Fatalf("CreateContact %d: %v", i, err)
		}
	}
	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{Name: "duplicate-phone", PhoneNumber: new(lastPhone)}); err == nil {
		t.Fatal("CreateContact duplicate phone beyond first page error = nil")
	}
}

func TestAdminContactCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	first, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "id-a",
		OwnerPublicKey: "peer-a",
		Name:           "alice001",
		DisplayName:    new("Alice"),
		PhoneNumber:    new("+1 555 0100"),
	})
	if err != nil {
		t.Fatalf("AdminCreateContact: %v", err)
	}
	if first.OwnerPublicKey != "peer-a" || first.Id != "id-a" || first.Name != "alice001" {
		t.Fatalf("created contact = %+v", first)
	}
	if first.CreatedAt == nil || first.UpdatedAt == nil {
		t.Fatalf("created timestamps = created:%v updated:%v", first.CreatedAt, first.UpdatedAt)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             " padded-id ",
		OwnerPublicKey: "peer-a",
		Name:           "padded-id",
		DisplayName:    new("Padded ID"),
	}); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("AdminCreateContact(padded id) error = %v, want exact ID rejection", err)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "id-duplicate-name",
		OwnerPublicKey: "peer-a",
		Name:           "alice001",
		DisplayName:    new("Alice Again"),
	}); !errors.Is(err, socialutil.ErrResourceAlreadyExists) {
		t.Fatalf("AdminCreateContact duplicate name error = %v, want conflict", err)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "id-duplicate-phone",
		OwnerPublicKey: "peer-a",
		Name:           "alice-phone",
		PhoneNumber:    new("+1 (555) 0100"),
	}); !errors.Is(err, socialutil.ErrResourceAlreadyExists) {
		t.Fatalf("AdminCreateContact duplicate phone error = %v, want conflict", err)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "id-a",
		OwnerPublicKey: "peer-b",
		Name:           "other-alice",
		DisplayName:    new("Other Alice"),
	}); !errors.Is(err, socialutil.ErrResourceAlreadyExists) {
		t.Fatalf("AdminCreateContact duplicate global id error = %v, want conflict", err)
	}
	byID, err := s.AdminGetContactByID(ctx, "id-a")
	if err != nil {
		t.Fatalf("AdminGetContactByID after duplicate global id: %v", err)
	}
	if byID.OwnerPublicKey != "peer-a" || byID.Name != "alice001" {
		t.Fatalf("contact after duplicate global id = %+v, want original peer-a/alice001", byID)
	}

	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "id-b",
		OwnerPublicKey: "peer-a",
		Name:           "bob00001",
		DisplayName:    new("Bob"),
	}); err != nil {
		t.Fatalf("AdminCreateContact bob00001: %v", err)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "id-c",
		OwnerPublicKey: "peer-b",
		Name:           "carol001",
		DisplayName:    new("Carol"),
	}); err != nil {
		t.Fatalf("AdminCreateContact carol001: %v", err)
	}

	page, err := s.AdminListContacts(ctx, "peer-a", nil, new(1))
	if err != nil {
		t.Fatalf("AdminListContacts owner first page: %v", err)
	}
	if len(page.Items) != 1 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("owner page = %+v, want one item with next cursor", page)
	}
	nextPage, err := s.AdminListContacts(ctx, "peer-a", page.NextCursor, new(10))
	if err != nil {
		t.Fatalf("AdminListContacts owner next page: %v", err)
	}
	if len(nextPage.Items) != 1 || nextPage.HasNext {
		t.Fatalf("owner next page = %+v, want final item", nextPage)
	}
	global, err := s.AdminListContacts(ctx, "", nil, new(2))
	if err != nil {
		t.Fatalf("AdminListContacts global first page: %v", err)
	}
	if len(global.Items) != 2 || !global.HasNext || global.NextCursor == nil {
		t.Fatalf("global page = %+v, want two items with next cursor", global)
	}
	globalNext, err := s.AdminListContacts(ctx, "", global.NextCursor, new(10))
	if err != nil {
		t.Fatalf("AdminListContacts global next page: %v", err)
	}
	if len(globalNext.Items) != 1 || globalNext.Items[0].OwnerPublicKey != "peer-b" {
		t.Fatalf("global next page = %+v, want peer-b contact", globalNext)
	}

	updated, err := s.AdminPutContact(ctx, "peer-a", first.Id, adminhttp.AdminContactPutRequest{
		Id:          first.Id,
		DisplayName: new("Alice Zhang"),
		PhoneNumber: new("+1 555 0101"),
	})
	if err != nil {
		t.Fatalf("AdminPutContact: %v", err)
	}
	if socialutil.StringValue(updated.DisplayName) != "Alice Zhang" || socialutil.StringValue(updated.PhoneNumber) != "+1 555 0101" {
		t.Fatalf("updated contact = %+v", updated)
	}
	got, err := s.AdminGetContact(ctx, "peer-a", first.Id)
	if err != nil {
		t.Fatalf("AdminGetContact: %v", err)
	}
	if got.Id != first.Id || got.Name != "alice001" || got.OwnerPublicKey != "peer-a" {
		t.Fatalf("got contact = %+v", got)
	}
	if _, err := s.AdminGetContact(ctx, "peer-a", " "+first.Id+" "); err == nil {
		t.Fatal("AdminGetContact padded id error = nil")
	}
	if _, err := s.AdminPutContact(ctx, "peer-a", " "+first.Id+" ", adminhttp.AdminContactPutRequest{Id: first.Id}); err == nil {
		t.Fatal("AdminPutContact padded id error = nil")
	}
	if _, err := s.AdminDeleteContact(ctx, "peer-a", " "+first.Id+" "); err == nil {
		t.Fatal("AdminDeleteContact padded id error = nil")
	}
	deleted, err := s.AdminDeleteContact(ctx, "peer-a", first.Id)
	if err != nil {
		t.Fatalf("AdminDeleteContact: %v", err)
	}
	if deleted.Id != first.Id {
		t.Fatalf("deleted contact id = %q, want %q", deleted.Id, first.Id)
	}
	if _, err := s.AdminGetContact(ctx, "peer-a", first.Id); err == nil {
		t.Fatal("AdminGetContact deleted contact error = nil")
	}
}

func TestAdminContactAcceptsShortNameAndRejectsPaddedName(t *testing.T) {
	ctx := context.Background()
	s := newTestServer()

	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "contact-alice",
		OwnerPublicKey: "peer-a",
		Name:           "alice",
		DisplayName:    new("Alice"),
	}); err != nil {
		t.Fatalf("AdminCreateContact short Peer name: %v", err)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{
		Id:             "contact-padded",
		OwnerPublicKey: "peer-a",
		Name:           " alice001 ",
		DisplayName:    new("Alice"),
	}); err == nil {
		t.Fatal("AdminCreateContact accepted padded name")
	}
}

func TestConfigurationErrors(t *testing.T) {
	ctx := context.Background()
	empty := &Server{}
	if _, err := empty.ListContacts(ctx, "peer-a", rpcapi.ContactListRequest{}); err == nil {
		t.Fatal("ListContacts without store error = nil")
	}
	if _, err := empty.CreateContact(ctx, "", rpcapi.ContactCreateRequest{Name: "alice001", DisplayName: new("Alice")}); err == nil {
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

//go:fix inline
func strPtr(v string) *string {
	return new(v)
}

//go:fix inline
func intPtr(v int) *int {
	return new(v)
}

func TestPeerRetirementDeletesOnlyOwnedContactSnapshot(t *testing.T) {
	s := newTestServer()
	first, err := s.AdminCreateContact(t.Context(), adminhttp.AdminContactCreateRequest{
		Id: "contact-a", OwnerPublicKey: "peer-a", Name: "alice", DisplayName: new("Alice"),
	})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.AdminCreateContact(t.Context(), adminhttp.AdminContactCreateRequest{
		Id: "contact-b", OwnerPublicKey: "peer-b", Name: "bob", DisplayName: new("Bob"),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.SnapshotPeerContacts(t.Context(), "peer-a")
	if err != nil || len(snapshot) != 1 || snapshot[0].ID != first.Id {
		t.Fatalf("SnapshotPeerContacts() = %#v, %v", snapshot, err)
	}
	if err := s.RetirePeerContact(t.Context(), snapshot[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.RetirePeerContact(t.Context(), snapshot[0]); err != nil {
		t.Fatalf("replayed RetirePeerContact() error = %v", err)
	}
	if _, err := s.AdminGetContactByID(t.Context(), first.Id); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retired Contact error = %v", err)
	}
	if got, err := s.AdminGetContactByID(t.Context(), foreign.Id); err != nil || got.OwnerPublicKey != "peer-b" {
		t.Fatalf("foreign Contact = %#v, %v", got, err)
	}
}

func TestPutAndDeleteContactAreSerialized(t *testing.T) {
	s := newTestServer()
	created, err := s.CreateContact(t.Context(), "peer-a", rpcapi.ContactCreateRequest{
		Name:        "alice001",
		DisplayName: new("Alice"),
	})
	if err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	id, err := s.resolveContactName(t.Context(), "peer-a", created.Name)
	if err != nil {
		t.Fatalf("resolveContactName() error = %v", err)
	}
	blocked := &blockingContactGetStore{
		Store:   s.Store,
		key:     socialutil.ContactKey("peer-a", id).String(),
		reached: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	s.Store = blocked
	putDone := make(chan error, 1)
	go func() {
		_, putErr := s.PutContact(t.Context(), "peer-a", rpcapi.ContactPutRequest{
			Name:        created.Name,
			DisplayName: new("Alice Updated"),
		})
		putDone <- putErr
	}()
	<-blocked.reached

	deleteDone := make(chan error, 1)
	go func() {
		_, deleteErr := s.DeleteContact(t.Context(), "peer-a", rpcapi.ContactDeleteRequest{Name: created.Name})
		deleteDone <- deleteErr
	}()
	select {
	case deleteErr := <-deleteDone:
		t.Fatalf("DeleteContact() completed during PutContact() read: %v", deleteErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(blocked.release)
	if err := <-putDone; err != nil {
		t.Fatalf("PutContact() error = %v", err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("DeleteContact() error = %v", err)
	}
	if _, err := s.GetContact(t.Context(), "peer-a", rpcapi.ContactGetRequest{Name: created.Name}); err != kv.ErrNotFound {
		t.Fatalf("GetContact() after delete error = %v, want kv.ErrNotFound", err)
	}
}

func TestPutContactDoesNotBlockIndependentOwner(t *testing.T) {
	s := newTestServer()
	first, err := s.CreateContact(t.Context(), "peer-a", rpcapi.ContactCreateRequest{
		Name: "alice001", DisplayName: new("Alice"),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateContact(t.Context(), "peer-b", rpcapi.ContactCreateRequest{
		Name: "bob001", DisplayName: new("Bob"),
	})
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := s.resolveContactName(t.Context(), "peer-a", first.Name)
	if err != nil {
		t.Fatal(err)
	}
	blocked := &blockingContactGetStore{
		Store:   s.Store,
		key:     socialutil.ContactKey("peer-a", firstID).String(),
		reached: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	s.Store = blocked

	firstDone := make(chan error, 1)
	go func() {
		_, err := s.PutContact(t.Context(), "peer-a", rpcapi.ContactPutRequest{Name: first.Name, DisplayName: new("Alice Updated")})
		firstDone <- err
	}()
	<-blocked.reached
	sameDone := make(chan error, 1)
	go func() {
		_, err := s.DeleteContact(t.Context(), "peer-a", rpcapi.ContactDeleteRequest{Name: first.Name})
		sameDone <- err
	}()
	secondDone := make(chan error, 1)
	go func() {
		_, err := s.PutContact(t.Context(), "peer-b", rpcapi.ContactPutRequest{Name: second.Name, DisplayName: new("Bob Updated")})
		secondDone <- err
	}()

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("independent PutContact() error = %v", err)
		}
	case <-time.After(time.Second):
		close(blocked.release)
		t.Fatal("independent Contact owner could not complete Put while first owner Store.Get was blocked")
	}
	select {
	case <-blocked.reached:
		close(blocked.release)
		t.Fatal("same owner entered a second Contact mutation before first release")
	default:
	}

	close(blocked.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first PutContact() error = %v", err)
	}
	if err := <-sameDone; err != nil {
		t.Fatalf("same-owner DeleteContact() error = %v", err)
	}
}

func TestContactMutationsRejectUnavailableOwner(t *testing.T) {
	blocked := false
	s := &Server{
		Store: kv.NewMemory(nil),
		NewID: func() string { return "contact001" },
		PeerAvailability: func(context.Context, string) error {
			if blocked {
				return ErrPeerPendingDeletion
			}
			return nil
		},
	}
	created, err := s.CreateContact(t.Context(), "peer-a", rpcapi.ContactCreateRequest{
		Name: "alice001", DisplayName: new("Alice"),
	})
	if err != nil {
		t.Fatal(err)
	}
	blocked = true
	if _, err := s.CreateContact(t.Context(), "peer-a", rpcapi.ContactCreateRequest{Name: "alice002", DisplayName: new("Alice 2")}); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("CreateContact() error = %v, want pending deletion", err)
	}
	if _, err := s.PutContact(t.Context(), "peer-a", rpcapi.ContactPutRequest{Name: created.Name, DisplayName: new("Changed")}); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("PutContact() error = %v, want pending deletion", err)
	}
	if _, err := s.DeleteContact(t.Context(), "peer-a", rpcapi.ContactDeleteRequest{Name: created.Name}); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("DeleteContact() error = %v, want pending deletion", err)
	}
	if _, err := s.GetContact(t.Context(), "peer-a", rpcapi.ContactGetRequest{Name: created.Name}); err != nil {
		t.Fatalf("failed mutation changed retained Contact: %v", err)
	}
}

type blockingContactGetStore struct {
	kv.Store
	key     string
	reached chan struct{}
	release chan struct{}
}

func (s *blockingContactGetStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if key.String() == s.key {
		select {
		case s.reached <- struct{}{}:
		default:
		}
		<-s.release
	}
	return s.Store.Get(ctx, key)
}
