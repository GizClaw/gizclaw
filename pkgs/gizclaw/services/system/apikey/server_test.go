package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestCreateAuthenticateManageAndCleanup(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	server := NewServer(store)
	server.now = func() time.Time { return time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC) }
	owner := testOwner(t)

	manager, err := server.Create(ctx, owner, " manager ", true)
	if err != nil {
		t.Fatalf("Create(manager) error = %v", err)
	}
	if len(manager.Secret) != 57 || !strings.HasPrefix(manager.Secret, secretPrefix) || len(manager.Key.Name) != 26 {
		t.Fatal("Create(manager) returned invalid key metadata or secret format")
	}
	if !strings.Contains(string(mustGet(t, store, recordKey(manager.Key.Name))), manager.Secret) {
		t.Fatal("stored record does not contain the complete plaintext API key")
	}
	if got := string(mustGet(t, store, secretKey(manager.Secret))); got != manager.Key.Name {
		t.Fatalf("plaintext API key index = %q, want %q", got, manager.Key.Name)
	}
	managerPrincipal, err := server.Authenticate(ctx, manager.Secret)
	if err != nil {
		t.Fatalf("Authenticate(manager) error = %v", err)
	}

	ordinary, err := server.Create(ctx, owner, "manager", false)
	if err != nil {
		t.Fatalf("Create(ordinary) error = %v", err)
	}
	if ordinary.Key.Name == manager.Key.Name || ordinary.Key.DisplayName != manager.Key.DisplayName {
		t.Fatal("same-owner duplicate display names did not produce distinct key identities")
	}
	ordinaryPrincipal, err := server.Authenticate(ctx, ordinary.Secret)
	if err != nil {
		t.Fatalf("Authenticate(ordinary) error = %v", err)
	}
	if _, err := server.List(ctx, ordinaryPrincipal, "", 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("List(ordinary) error = %v, want forbidden", err)
	}
	page, err := server.List(ctx, managerPrincipal, "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("List(manager) = %#v, %v", page, err)
	}
	if _, err := server.List(ctx, managerPrincipal, "not-a-cursor", 1); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("List(invalid cursor) error = %v", err)
	}
	if err := server.RevokeSelf(ctx, ordinaryPrincipal); err != nil {
		t.Fatalf("RevokeSelf() error = %v", err)
	}
	if err := server.RevokeSelf(ctx, ordinaryPrincipal); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeSelf(replay) error = %v, want not found", err)
	}
	if _, err := server.Authenticate(ctx, ordinary.Secret); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("Authenticate(revoked) error = %v", err)
	}
	if err := server.CleanupPeer(ctx, owner); err != nil {
		t.Fatalf("CleanupPeer() error = %v", err)
	}
	if err := server.CleanupPeer(ctx, owner); err != nil {
		t.Fatalf("CleanupPeer(replay) error = %v", err)
	}
	if _, err := server.Authenticate(ctx, manager.Secret); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("Authenticate(cleaned) error = %v", err)
	}
	if _, err := server.Create(ctx, owner, "resurrected", true); !errors.Is(err, ErrOwnerRetired) {
		t.Fatalf("Create(retired owner) error = %v", err)
	}
	otherOwner := testOwner(t)
	other, err := server.Create(ctx, otherOwner, "other", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Authenticate(ctx, other.Secret); err != nil {
		t.Fatalf("Authenticate(other owner after cleanup) error = %v", err)
	}
}

func TestValidationAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	server := NewServer(kv.NewMemory(nil))
	ownerA := testOwner(t)
	keyPairB, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ownerB := keyPairB.Public.String()

	if _, err := server.Create(ctx, ownerA, strings.Repeat("x", 81), false); !errors.Is(err, ErrInvalidDisplayName) {
		t.Fatalf("Create(long display name) error = %v", err)
	}
	managerA, _ := server.Create(ctx, ownerA, "manager A", true)
	keyB, _ := server.Create(ctx, ownerB, "key B", false)
	principalA, _ := server.Authenticate(ctx, managerA.Secret)
	if _, err := server.Get(ctx, principalA, keyB.Key.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(cross-owner) error = %v", err)
	}
	if err := server.Revoke(ctx, principalA, keyB.Key.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Revoke(cross-owner) error = %v", err)
	}
}

func TestAPIKeyOwnerRootManagement(t *testing.T) {
	ctx := t.Context()
	server := NewServer(kv.NewMemory(nil))
	ownerA := testOwner(t)
	ownerB := testOwner(t)
	first, err := server.Create(ctx, ownerA, "ordinary-a", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.Create(ctx, ownerA, "ordinary-b", false)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := server.Create(ctx, ownerB, "foreign", true)
	if err != nil {
		t.Fatal(err)
	}

	page, err := server.ListOwner(ctx, ownerA, "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("ListOwner(first page) = %#v, %v", page, err)
	}
	next, err := server.ListOwner(ctx, ownerA, page.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.NextCursor != "" {
		t.Fatalf("ListOwner(second page) = %#v, %v", next, err)
	}
	wantAPIKeys := map[string]bool{first.Secret: true, second.Secret: true}
	for _, item := range append(page.Items, next.Items...) {
		if item.Owner != ownerA || !wantAPIKeys[item.APIKey] {
			t.Fatalf("ListOwner returned unexpected stored item %#v", item)
		}
	}
	if err := server.RevokeOwner(ctx, ownerA, foreign.Key.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeOwner(foreign) error = %v", err)
	}
	if err := server.RevokeOwner(ctx, ownerA, first.Key.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Authenticate(ctx, first.Secret); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("Authenticate(revoked) error = %v", err)
	}
	if _, err := server.Authenticate(ctx, second.Secret); err != nil {
		t.Fatalf("Authenticate(remaining) error = %v", err)
	}
}

func TestConcurrentCleanupDoesNotResurrectAPIKey(t *testing.T) {
	ctx := t.Context()
	server := NewServer(kv.NewMemory(nil))
	owner := testOwner(t)
	if _, err := server.Create(ctx, owner, "existing", true); err != nil {
		t.Fatal(err)
	}

	cleanupResult := make(chan error, 1)
	createResult := make(chan struct {
		created Created
		err     error
	}, 1)
	start := make(chan struct{})
	go func() {
		<-start
		cleanupResult <- server.CleanupPeer(ctx, owner)
	}()
	go func() {
		<-start
		created, err := server.Create(ctx, owner, "concurrent", false)
		createResult <- struct {
			created Created
			err     error
		}{created: created, err: err}
	}()
	close(start)
	if err := <-cleanupResult; err != nil {
		t.Fatalf("CleanupPeer() error = %v", err)
	}
	result := <-createResult
	if result.err != nil && !errors.Is(result.err, ErrOwnerRetired) {
		t.Fatalf("Create() error = %v", result.err)
	}
	if result.err == nil {
		if _, err := server.Authenticate(ctx, result.created.Secret); !errors.Is(err, ErrInvalidAPIKey) {
			t.Fatalf("Authenticate(concurrently created) error = %v", err)
		}
	}
}

func TestConcurrentListCreateAndRevokeIsConsistent(t *testing.T) {
	ctx := t.Context()
	server := NewServer(kv.NewMemory(nil))
	owner := testOwner(t)
	manager, err := server.Create(ctx, owner, "manager", true)
	if err != nil {
		t.Fatal(err)
	}
	managerPrincipal, err := server.Authenticate(ctx, manager.Secret)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := server.Create(ctx, owner, "removed", false)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	createResult := make(chan error, 1)
	revokeResult := make(chan error, 1)
	listResult := make(chan error, 1)
	go func() {
		<-start
		_, err := server.Create(ctx, owner, "created", false)
		createResult <- err
	}()
	go func() {
		<-start
		revokeResult <- server.Revoke(ctx, managerPrincipal, removed.Key.Name)
	}()
	go func() {
		<-start
		page, err := server.List(ctx, managerPrincipal, "", maxListLimit)
		if err == nil {
			seen := make(map[string]struct{}, len(page.Items))
			for _, item := range page.Items {
				if item.Owner != owner {
					err = errors.New("list returned a foreign owner")
					break
				}
				if _, exists := seen[item.Name]; exists {
					err = errors.New("list returned a duplicate key")
					break
				}
				seen[item.Name] = struct{}{}
			}
		}
		listResult <- err
	}()
	close(start)
	for name, result := range map[string]<-chan error{
		"create": createResult,
		"revoke": revokeResult,
		"list":   listResult,
	} {
		if err := <-result; err != nil {
			t.Fatalf("concurrent %s error = %v", name, err)
		}
	}
	page, err := server.List(ctx, managerPrincipal, "", maxListLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Name == removed.Key.Name {
			t.Fatal("revoked key remained in the owner list")
		}
	}
}

func TestConcurrentAuthenticateAndRevokeDoesNotResurrectAPIKey(t *testing.T) {
	ctx := t.Context()
	server := NewServer(kv.NewMemory(nil))
	owner := testOwner(t)
	created, err := server.Create(ctx, owner, "key", false)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := server.Authenticate(ctx, created.Secret)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	authResult := make(chan error, 1)
	revokeResult := make(chan error, 1)
	go func() {
		<-start
		_, err := server.Authenticate(ctx, created.Secret)
		authResult <- err
	}()
	go func() {
		<-start
		revokeResult <- server.RevokeSelf(ctx, principal)
	}()
	close(start)
	if err := <-authResult; err != nil && !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("Authenticate(concurrent) error = %v", err)
	}
	if err := <-revokeResult; err != nil {
		t.Fatalf("RevokeSelf(concurrent) error = %v", err)
	}
	if _, err := server.Authenticate(ctx, created.Secret); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("Authenticate(after revoke) error = %v", err)
	}
}

func testOwner(t *testing.T) string {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return keyPair.Public.String()
}

func mustGet(t *testing.T, store kv.Store, key kv.Key) []byte {
	t.Helper()
	value, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
