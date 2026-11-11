package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPeerAPIKeyLimitAndRevokeReleasesCapacity(t *testing.T) {
	server := NewServer(kv.NewMemory(nil))
	owner := testOwner(t)
	var first Created
	for i := range PeerAPIKeyLimit {
		created, err := server.Create(t.Context(), owner, "key", i == 0)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = created
		}
	}
	if _, err := server.Create(t.Context(), owner, "eleventh", false); !errors.Is(err, ErrPeerAPIKeyLimit) {
		t.Fatalf("Create(11th) = %v", err)
	}
	page, err := server.ListOwner(t.Context(), owner, "", maxListLimit)
	if err != nil || len(page.Items) != PeerAPIKeyLimit {
		t.Fatalf("ListOwner = %d keys, %v", len(page.Items), err)
	}
	if _, err := server.Create(t.Context(), testOwner(t), "other owner", false); err != nil {
		t.Fatalf("independent owner: %v", err)
	}
	if err := server.RevokeOwner(t.Context(), owner, first.Key.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Create(t.Context(), owner, "replacement", true); err != nil {
		t.Fatalf("released capacity: %v", err)
	}
	if err := server.CleanupPeer(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Create(t.Context(), owner, "retired", false); !errors.Is(err, ErrOwnerRetired) {
		t.Fatalf("retired owner: %v", err)
	}
}

type concurrentAPIKeyLimitStore struct {
	kv.Store
	entered chan struct{}
	release chan struct{}
}

func (s concurrentAPIKeyLimitStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.entered <- struct{}{}
	select {
	case <-s.release:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestPeerAPIKeyLimitAcrossConcurrentServers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store := kv.Prefixed(kv.NewMemory(nil), kv.Key{"api-keys"})
	owner := testOwner(t)
	seed := NewServer(store)
	for range PeerAPIKeyLimit - 1 {
		if _, err := seed.Create(t.Context(), owner, "existing", false); err != nil {
			t.Fatal(err)
		}
	}
	hook := concurrentAPIKeyLimitStore{Store: store, entered: make(chan struct{}, 2), release: make(chan struct{})}
	results := make(chan error, 2)
	for range 2 {
		server := NewServer(hook)
		go func() { _, err := server.Create(ctx, owner, "concurrent", false); results <- err }()
	}
	for range 2 {
		select {
		case <-hook.entered:
		case <-ctx.Done():
			t.Fatal("both Servers did not reach the shared capacity check")
		}
	}
	close(hook.release)
	winners, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrPeerAPIKeyLimit):
			rejected++
		default:
			t.Fatalf("Create = %v", err)
		}
	}
	if winners != 1 || rejected != 1 {
		t.Fatalf("winners=%d rejected=%d", winners, rejected)
	}
	page, err := seed.ListOwner(t.Context(), owner, "", maxListLimit)
	if err != nil || len(page.Items) != PeerAPIKeyLimit {
		t.Fatalf("concurrent total = %d, %v", len(page.Items), err)
	}
}

type laggingAPIKeyStore struct {
	kv.Store
	replica kv.Store
	reads   []bool
}

func (s *laggingAPIKeyStore) reader(ctx context.Context) kv.Store {
	primary := kv.WithPrimaryReads(ctx) == ctx
	s.reads = append(s.reads, primary)
	if primary {
		return s.Store
	}
	return s.replica
}

func (s *laggingAPIKeyStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	return s.reader(ctx).Get(ctx, key)
}

func (s *laggingAPIKeyStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	return s.reader(ctx).ListMembers(ctx, key)
}

func TestAPIKeyReadsHonorLearnerAndRetirementCAS(t *testing.T) {
	ctx := t.Context()
	writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	owner := testOwner(t)
	created, err := NewServer(writer).Create(ctx, owner, "key", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []kv.Key{recordKey(created.Key.Name), secretKey(created.Secret)} {
		data, err := writer.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if err := replica.Set(ctx, key, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := replica.AddMembers(ctx, ownerPrefix(owner), created.Key.Name); err != nil {
		t.Fatal(err)
	}
	store := &laggingAPIKeyStore{Store: writer, replica: replica}
	server := NewServer(store)
	if err := NewServer(writer).RevokeOwner(ctx, owner, created.Key.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Authenticate(ctx, created.Secret); err != nil {
		t.Fatalf("configured stale authentication = %v", err)
	}
	page, err := server.ListOwner(ctx, owner, "", 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("learner list = %+v, %v", page, err)
	}
	for _, primary := range store.reads {
		if primary {
			t.Fatal("ordinary auth/admin read bypassed learner")
		}
	}
	if err := writer.Set(ctx, retiredKey(owner), []byte("retired")); err != nil {
		t.Fatal(err)
	}
	store.reads = nil
	if _, err := server.Create(ctx, owner, "retired", false); !errors.Is(err, ErrOwnerRetired) {
		t.Fatalf("retirement condition = %v", err)
	}
	if len(store.reads) != 2 || store.reads[0] || !store.reads[1] {
		t.Fatalf("create reads = %v", store.reads)
	}
	// A failed create must not permanently change the server's read policy.
	store.reads = nil
	if _, err := server.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal(err)
	}
	for _, primary := range store.reads {
		if primary {
			t.Fatal("fallback pinned subsequent authentication to primary")
		}
	}
}

func TestAPIKeyCleanupConfirmsFreshRetirementFence(t *testing.T) {
	ctx := t.Context()
	writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	owner := testOwner(t)
	created, err := NewServer(writer).Create(ctx, owner, "key", false)
	if err != nil {
		t.Fatal(err)
	}
	// An earlier cleanup's marker must not make a stale empty owner index safe.
	if err := replica.Set(ctx, retiredKey(owner), []byte("old-retirement")); err != nil {
		t.Fatal(err)
	}
	store := &laggingAPIKeyStore{Store: writer, replica: replica}
	if err := NewServer(store).CleanupPeer(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if len(store.reads) != 3 || store.reads[0] || !store.reads[1] || !store.reads[2] {
		t.Fatalf("cleanup reads = %v", store.reads)
	}
	if _, err := NewServer(writer).Authenticate(ctx, created.Secret); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("cleanup left live credential = %v", err)
	}
}

type apiKeyReadErrorStore struct {
	kv.Store
	err error
}

func (s apiKeyReadErrorStore) Get(context.Context, kv.Key) ([]byte, error) {
	return nil, s.err
}

func TestAPIKeyReadFailuresDoNotRefreshPrimary(t *testing.T) {
	ctx := t.Context()
	writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	owner := testOwner(t)
	created, err := NewServer(writer).Create(ctx, owner, "key", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.AddMembers(ctx, ownerPrefix(owner), created.Key.Name); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("reader unavailable")
	for _, primary := range []bool{false, true} {
		ctx := ctx
		if primary {
			ctx = kv.WithPrimaryReads(ctx)
		}
		store := &laggingAPIKeyStore{Store: apiKeyReadErrorStore{Store: writer, err: failure}, replica: apiKeyReadErrorStore{Store: replica, err: failure}}
		if _, err := NewServer(store).Authenticate(ctx, created.Secret); !errors.Is(err, failure) || len(store.reads) != 1 || store.reads[0] != primary {
			t.Fatalf("authentication retried unavailable reader: %v reads=%v", err, store.reads)
		}
	}
	store := &laggingAPIKeyStore{Store: apiKeyReadErrorStore{Store: writer, err: errors.New("authority unavailable")}, replica: replica}
	server := NewServer(store)
	if _, err := server.Get(ctx, Principal{Key: created.Key}, created.Key.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing learner record was refreshed: %v", err)
	}
	page, err := server.ListOwner(ctx, owner, "", 10)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("missing learner list record was refreshed: %+v %v", page, err)
	}
	for _, primary := range store.reads {
		if primary {
			t.Fatalf("ordinary Get/List selected primary: %v", store.reads)
		}
	}
}

func TestAPIKeyRevokeRefreshesFreshRecordAtPrimary(t *testing.T) {
	for _, operation := range []string{"self", "manager", "owner"} {
		t.Run(operation, func(t *testing.T) {
			ctx := t.Context()
			writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			store := &laggingAPIKeyStore{Store: writer, replica: replica}
			server := NewServer(store)
			owner := testOwner(t)
			created, err := server.Create(ctx, owner, "new key", true)
			if err != nil {
				t.Fatal(err)
			}
			store.reads = nil
			principal := Principal{Key: created.Key}
			switch operation {
			case "self":
				err = server.RevokeSelf(ctx, principal)
			case "manager":
				err = server.Revoke(ctx, principal, created.Key.Name)
			case "owner":
				err = server.RevokeOwner(ctx, owner, created.Key.Name)
			}
			if err != nil || len(store.reads) != 2 || store.reads[0] || !store.reads[1] {
				t.Fatalf("fresh key revoke = %v reads=%v", err, store.reads)
			}
			for _, key := range []kv.Key{recordKey(created.Key.Name), secretKey(created.Secret)} {
				if _, err := writer.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("revoked record/index remained: %v", err)
				}
			}
			if names, err := writer.ListMembers(ctx, ownerPrefix(owner)); err != nil || len(names) != 0 {
				t.Fatalf("revoked owner index remained: %v %v", names, err)
			}
			store.reads = nil
			if err := server.RevokeOwner(ctx, owner, created.Key.Name); !errors.Is(err, ErrNotFound) || len(store.reads) != 2 {
				t.Fatalf("missing revoke refresh was unbounded: %v %v", err, store.reads)
			}
		})
	}
}

type apiKeyMutationHookStore struct {
	kv.Store
	before func()
	writes int
}

func (s *apiKeyMutationHookStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.writes++
	if before := s.before; before != nil {
		s.before = nil
		before()
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestAPIKeyRevokeRechecksOwnerAfterConditionalConflict(t *testing.T) {
	ctx := t.Context()
	writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	owner, foreignOwner := testOwner(t), testOwner(t)
	created, err := NewServer(writer).Create(ctx, owner, "key", true)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := writer.Get(ctx, recordKey(created.Key.Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, recordKey(created.Key.Name), raw); err != nil {
		t.Fatal(err)
	}
	store := &laggingAPIKeyStore{Store: writer, replica: replica}
	hook := &apiKeyMutationHookStore{Store: store, before: func() {
		replacement := created.Key
		replacement.Owner = foreignOwner
		data, err := json.Marshal(replacement)
		if err != nil {
			t.Fatal(err)
		}
		_, err = writer.ApplyMutation(ctx, kv.Mutation{
			Entries:       []kv.Entry{{Key: recordKey(created.Key.Name), Value: data}},
			AddMembers:    []kv.SetMembers{{Key: ownerPrefix(foreignOwner), Members: []string{created.Key.Name}}},
			RemoveMembers: []kv.SetMembers{{Key: ownerPrefix(owner), Members: []string{created.Key.Name}}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}}
	if err := NewServer(hook).RevokeOwner(ctx, owner, created.Key.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale revoke changed a replaced key: %v", err)
	}
	if hook.writes != 1 || len(store.reads) != 2 || store.reads[0] || !store.reads[1] {
		t.Fatalf("revoke did not recheck after conflict: writes=%d reads=%v", hook.writes, store.reads)
	}
	principal, err := NewServer(writer).Authenticate(ctx, created.Secret)
	if err != nil || principal.Key.Owner != foreignOwner {
		t.Fatalf("replacement credential was removed: %+v %v", principal, err)
	}
	if found, err := writer.HasMember(ctx, ownerPrefix(foreignOwner), created.Key.Name); err != nil || !found {
		t.Fatalf("replacement owner index was removed: %v %v", found, err)
	}
}

func TestAPIKeyAuthenticationPreservesLearnerMiss(t *testing.T) {
	for _, missing := range []string{"index", "record"} {
		t.Run(missing, func(t *testing.T) {
			ctx := t.Context()
			writer, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			created, err := NewServer(writer).Create(ctx, testOwner(t), "new credential", false)
			if err != nil {
				t.Fatal(err)
			}
			if missing == "record" {
				if err := replica.Set(ctx, secretKey(created.Secret), []byte(created.Key.Name)); err != nil {
					t.Fatal(err)
				}
			}
			for _, unavailable := range []bool{false, true} {
				authority := kv.Store(writer)
				if unavailable {
					authority = apiKeyReadErrorStore{Store: writer, err: errors.New("authority unavailable")}
				}
				store := &laggingAPIKeyStore{Store: authority, replica: replica}
				_, err := NewServer(store).Authenticate(ctx, created.Secret)
				wantReads := 1
				if missing == "record" {
					wantReads++
				}
				if !errors.Is(err, ErrInvalidAPIKey) || len(store.reads) != wantReads {
					t.Fatalf("missing auth (authority unavailable=%v) = %v, reads=%v", unavailable, err, store.reads)
				}
				for _, primary := range store.reads {
					if primary {
						t.Fatalf("ordinary authentication refreshed primary: %v", store.reads)
					}
				}
			}
		})
	}
}
