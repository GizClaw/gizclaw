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
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestCRUDUsesDirectFieldsAndPerPeerScope(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

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

func TestDuplicatePhoneConstraintAcrossCatalog(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)
	nextID := 0
	s.NewID = func() string {
		nextID++
		return fmt.Sprintf("contact-%03d", nextID)
	}

	var lastPhone string
	for i := range PeerContactLimit - 1 {
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
		t.Fatal("CreateContact duplicate phone error = nil")
	}
}

func TestCreateContactEnforcesPeerLimit(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	for i := range PeerContactLimit {
		if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{Name: fmt.Sprintf("person-%d", i), PhoneNumber: new(fmt.Sprintf("+1 555 01%02d", i))}); err != nil {
			t.Fatalf("CreateContact %d: %v", i+1, err)
		}
	}
	before := contactNames(t, s, "peer-a")
	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{Name: "overflow", DisplayName: new("Overflow")}); !errors.Is(err, ErrPeerContactLimit) {
		t.Fatalf("CreateContact over limit error = %v, want %v", err, ErrPeerContactLimit)
	}
	if _, err := s.AdminCreateContact(ctx, adminhttp.AdminContactCreateRequest{Id: "admin-overflow", OwnerPublicKey: "peer-a", Name: "admin-overflow", DisplayName: new("Overflow")}); !errors.Is(err, ErrPeerContactLimit) {
		t.Fatalf("AdminCreateContact over limit error = %v, want %v", err, ErrPeerContactLimit)
	}
	if after := contactNames(t, s, "peer-a"); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("contacts after rejected create = %v, want %v", after, before)
	}
	if _, err := s.CreateContact(ctx, "peer-b", rpcapi.ContactCreateRequest{Name: "other", DisplayName: new("Other")}); err != nil {
		t.Fatalf("CreateContact for another owner: %v", err)
	}

	if _, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{Name: "person-0", DisplayName: new("Updated")}); err != nil {
		t.Fatalf("PutContact at limit: %v", err)
	}
	if _, err := s.DeleteContact(ctx, "peer-a", rpcapi.ContactDeleteRequest{Name: "person-0"}); err != nil {
		t.Fatalf("DeleteContact at limit: %v", err)
	}
	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{Name: "replacement", DisplayName: new("Replacement")}); err != nil {
		t.Fatalf("CreateContact after delete: %v", err)
	}
	if got := len(contactNames(t, s, "peer-a")); got != PeerContactLimit {
		t.Fatalf("contacts = %d, want %d", got, PeerContactLimit)
	}
}

func TestConcurrentCreateContactRespectsPeerLimit(t *testing.T) {
	s := newTestServer(t)
	const attempts = PeerContactLimit * 3
	start := make(chan struct{})
	results := make(chan error, attempts)
	for i := range attempts {
		go func() {
			<-start
			_, err := s.AdminCreateContact(t.Context(), adminhttp.AdminContactCreateRequest{Id: fmt.Sprintf("contact-%02d", i), OwnerPublicKey: "owner", Name: fmt.Sprintf("person-%02d", i), DisplayName: new("Person")})
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range attempts {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPeerContactLimit):
		default:
			t.Fatal(err)
		}
	}
	if got := len(contactNames(t, s, "owner")); successes != PeerContactLimit || got != PeerContactLimit {
		t.Fatalf("successes = %d, stored = %d, want %d", successes, got, PeerContactLimit)
	}
}

func TestCreateContactKeepsContactsAboveLimit(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	for i := range PeerContactLimit + 2 {
		id := fmt.Sprintf("legacy-%02d", i)
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO contacts(id,owner_public_key,name,display_name,created_at,updated_at,incarnation) VALUES (?,?,?,?,?,?,?)`, id, "peer-a", id, "Legacy", "2026-09-06T00:00:00Z", "2026-09-06T00:00:00Z", id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateContact(ctx, "peer-a", rpcapi.ContactCreateRequest{Name: "overflow", DisplayName: new("Overflow")}); !errors.Is(err, ErrPeerContactLimit) {
		t.Fatalf("CreateContact above limit error = %v, want %v", err, ErrPeerContactLimit)
	}
	if got := len(contactNames(t, s, "peer-a")); got != PeerContactLimit+2 {
		t.Fatalf("contacts = %d, want existing %d kept", got, PeerContactLimit+2)
	}
	if _, err := s.PutContact(ctx, "peer-a", rpcapi.ContactPutRequest{Name: "legacy-00", DisplayName: new("Updated")}); err != nil {
		t.Fatalf("PutContact above limit: %v", err)
	}
	if _, err := s.DeleteContact(ctx, "peer-a", rpcapi.ContactDeleteRequest{Name: "legacy-00"}); err != nil {
		t.Fatalf("DeleteContact above limit: %v", err)
	}
}

func contactNames(t *testing.T, s *Server, owner string) []string {
	t.Helper()
	list, err := s.ListContacts(t.Context(), owner, rpcapi.ContactListRequest{Limit: new(socialutil.MaxListLimit)})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.Name)
	}
	return names
}

func TestAdminContactCRUDAndPagination(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t)

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
	s := newTestServer(t)

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

func newTestServer(t *testing.T) *Server {
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	nextID := 0
	return &Server{
		DB:  newTestDB(t),
		Now: func() time.Time { return now },
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
	s := newTestServer(t)
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
	if _, err := s.AdminGetContactByID(t.Context(), first.Id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retired Contact error = %v", err)
	}
	if got, err := s.AdminGetContactByID(t.Context(), foreign.Id); err != nil || got.OwnerPublicKey != "peer-b" {
		t.Fatalf("foreign Contact = %#v, %v", got, err)
	}
}

func TestPutAndDeleteContactAreSerialized(t *testing.T) {
	s := newTestServer(t)
	created, err := s.CreateContact(t.Context(), "peer-a", rpcapi.ContactCreateRequest{
		Name:        "alice001",
		DisplayName: new("Alice"),
	})
	if err != nil {
		t.Fatalf("CreateContact() error = %v", err)
	}
	blocked := &blockingContactAvailability{
		owner:   "peer-a",
		reached: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	s.PeerAvailability = blocked.Check
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
	if _, err := s.GetContact(t.Context(), "peer-a", rpcapi.ContactGetRequest{Name: created.Name}); err != ErrNotFound {
		t.Fatalf("GetContact() after delete error = %v, want ErrNotFound", err)
	}
}

func TestPutContactDoesNotBlockIndependentOwner(t *testing.T) {
	s := newTestServer(t)
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
	blocked := &blockingContactAvailability{
		owner:   "peer-a",
		reached: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	s.PeerAvailability = blocked.Check

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
		t.Fatal("independent Contact owner could not complete Put while first owner admission was blocked")
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
		DB:    newTestDB(t),
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

type blockingContactAvailability struct {
	owner   string
	reached chan struct{}
	release chan struct{}
}

func (s *blockingContactAvailability) Check(ctx context.Context, owner string) error {
	if owner == s.owner {
		select {
		case s.reached <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.release:
		}
	}
	return nil
}
func newTestDB(t testing.TB) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := (&Server{DB: db}).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLPhoneUniquenessAcrossServiceInstances(t *testing.T) {
	db := newTestDB(t)
	servers := []*Server{{DB: db}, {DB: db}}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, server := range servers {
		go func() {
			<-start
			_, err := server.AdminCreateContact(t.Context(), adminhttp.AdminContactCreateRequest{Id: fmt.Sprintf("contact-%d", i), OwnerPublicKey: "owner", Name: fmt.Sprintf("person-%d", i), PhoneNumber: new([]string{"+1 (555) 0100", "15550100"}[i])})
			results <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, socialutil.ErrResourceAlreadyExists):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("claims = %d successes/%d conflicts", successes, conflicts)
	}
}

func TestRetirementSnapshotDoesNotDeleteRecreatedContact(t *testing.T) {
	server := newTestServer(t)
	request := adminhttp.AdminContactCreateRequest{Id: "same-id", OwnerPublicKey: "owner", Name: "alice", DisplayName: new("Alice")}
	if _, err := server.AdminCreateContact(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.SnapshotPeerContacts(t.Context(), "owner")
	if err != nil || len(snapshot) != 1 {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
	if _, err := server.AdminDeleteContactByID(t.Context(), request.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := server.AdminCreateContact(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if err := server.RetirePeerContact(t.Context(), snapshot[0]); err == nil {
		t.Fatal("old snapshot accepted a new creation instance")
	}
	if _, err := server.AdminGetContactByID(t.Context(), request.Id); err != nil {
		t.Fatalf("replacement removed: %v", err)
	}
}
