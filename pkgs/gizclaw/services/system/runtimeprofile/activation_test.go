package runtimeprofile

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func seedActivation(t *testing.T, db *sqlx.DB) *Server {
	t.Helper()
	s := &Server{DB: db, Now: func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := insertRuntimeProfileSQL(t.Context(), db, apitypes.RuntimeProfile{Id: "profile", CreatedAt: s.now(), UpdatedAt: s.now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshMemoryIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := insertRegistrationTokenSQL(t.Context(), db, apitypes.RegistrationToken{Id: "token", Token: "value", RuntimeProfileId: "profile", Enabled: true, CreatedAt: s.now(), UpdatedAt: s.now()}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRegistrationActivationLifecycle(t *testing.T) {
	s := seedActivation(t, profileSQLTestDB(t))
	testActivationLifecycle(t, s)
}

func testActivationLifecycle(t *testing.T, s *Server) {
	t.Helper()
	ctx := t.Context()
	update := func(enabled bool, expiry *time.Time, limit *int64) {
		t.Helper()
		response, err := s.PutRegistrationToken(ctx, adminhttp.PutRegistrationTokenRequestObject{Id: "token", Body: &adminhttp.RegistrationTokenUpsert{Id: "token", Token: "value", RuntimeProfileId: "profile", Enabled: &enabled, ExpiresAt: expiry, MaxActivations: limit}})
		if _, ok := response.(adminhttp.PutRegistrationToken200JSONResponse); err != nil || !ok {
			t.Fatalf("update: %T, %v", response, err)
		}
	}
	deny := func() {
		t.Helper()
		if err := s.PreflightRegistration(ctx, "value"); !errors.Is(err, ErrRegistrationDenied) {
			t.Fatalf("preflight = %v", err)
		}
		if _, err := s.RegisterOwner(ctx, "new-peer", "value", nil); !errors.Is(err, ErrRegistrationDenied) {
			t.Fatalf("register = %v", err)
		}
		if _, err := s.ResolveOwnerProfile(ctx, "new-peer"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("denial left owner binding: %v", err)
		}
	}
	update(false, nil, nil)
	deny()
	update(true, new(s.now()), nil)
	deny()
	update(true, new(s.now().Add(time.Hour)), new(int64(1)))
	if err := s.PreflightRegistration(ctx, "value"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterOwner(ctx, "first-peer", "value", nil); err != nil {
		t.Fatal(err)
	}
	deny()
	// Lowering below usage is valid and never revokes the existing binding.
	update(false, new(s.now().Add(-time.Hour)), new(int64(0)))
	deny()
	for range 3 {
		if _, err := s.RegisterOwner(ctx, "first-peer", "value", nil); err != nil {
			t.Fatalf("idempotent registration: %v", err)
		}
	}
	item, _, err := getRegistrationTokenSQL(ctx, s.DB, "token")
	if err != nil || item.ActivationCount != 1 {
		t.Fatalf("count = %d, %v", item.ActivationCount, err)
	}
	if profile, err := s.ResolveOwnerProfile(ctx, "first-peer"); err != nil || profile.Id != "profile" {
		t.Fatalf("existing binding lost: %v", err)
	}
	update(true, new(s.now().Add(time.Hour)), new(int64(2)))
	if err := s.PreflightRegistration(ctx, "value"); err != nil {
		t.Fatalf("extended token: %v", err)
	}
	if _, err := s.RegisterOwner(ctx, "second-peer", "value", nil); err != nil {
		t.Fatal(err)
	}
	item, version, err := getRegistrationTokenSQL(ctx, s.DB, "token")
	if err != nil || item.ActivationCount != 2 {
		t.Fatalf("count = %d, %v", item.ActivationCount, err)
	}
	if _, _, err := deleteRegistrationTokenSQL(ctx, s.DB, "token", version); err != nil {
		t.Fatal(err)
	}
	if _, err := insertRegistrationTokenSQL(ctx, s.DB, item); err != nil {
		t.Fatal(err)
	}
	item, _, err = getRegistrationTokenSQL(ctx, s.DB, "token")
	if err != nil || item.ActivationCount != 0 {
		t.Fatalf("recreated token retained activations: %d, %v", item.ActivationCount, err)
	}
}

func TestRegistrationActivationConcurrentSQLite(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "race.sqlite") + "?_pragma=busy_timeout(5000)"
	open := func() *sqlx.DB {
		db, err := sqlx.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	testActivationLastSlot(t, open)
}

func testActivationLastSlot(t *testing.T, open func() *sqlx.DB) {
	t.Helper()
	s := seedActivation(t, open())
	defer s.DB.Close()
	other := &Server{DB: open(), Now: s.Now}
	defer other.DB.Close()
	item, version, err := getRegistrationTokenSQL(t.Context(), s.DB, "token")
	if err != nil {
		t.Fatal(err)
	}
	item.MaxActivations = new(int64(1))
	if _, _, err := updateRegistrationTokenSQL(t.Context(), s.DB, item, version); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for i, server := range []*Server{s, other} {
		go func() {
			ready.Done()
			<-start
			_, err := server.RegisterOwner(t.Context(), []string{"a", "b"}[i], "value", nil)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	accepted, rejected := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			accepted++
		case errors.Is(err, ErrRegistrationDenied):
			rejected++
		default:
			t.Fatalf("concurrent registration: %v", err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted %d, denied %d", accepted, rejected)
	}
	var activations, owners int
	if err := s.DB.GetContext(t.Context(), &activations, "SELECT COUNT(*) FROM registration_token_activations"); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.GetContext(t.Context(), &owners, "SELECT COUNT(*) FROM runtime_profile_owners"); err != nil {
		t.Fatal(err)
	}
	if activations != 1 || owners != 1 {
		t.Fatalf("partial binding: activations=%d owners=%d", activations, owners)
	}
}

func TestRegistrationTokenLifecycleMigration(t *testing.T) {
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "old.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTokenLifecycleMigration(t, db)
}

func testTokenLifecycleMigration(t *testing.T, db *sqlx.DB) {
	t.Helper()
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `CREATE TABLE registration_tokens(id TEXT PRIMARY KEY,token TEXT NOT NULL,runtime_profile_id TEXT NOT NULL,firmware_id TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,row_version BIGINT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO registration_tokens VALUES ('old','value','profile',NULL,'2020-01-01T00:00:00Z','2020-01-01T00:00:00Z','incarnation',7)`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := initializeProfileSQL(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	item, version, err := getRegistrationTokenSQL(ctx, db, "old")
	if err != nil || !item.Enabled || item.ExpiresAt != nil || item.MaxActivations != nil || item.ActivationCount != 0 || version.incarnation != "incarnation" || version.revision != 7 {
		t.Fatalf("migration: %#v, %#v, %v", item, version, err)
	}
}

func TestPostgreSQLRegistrationTokenLifecycle(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	t.Run("migration", func(t *testing.T) {
		db := openProfilePostgresSchema(t, dsn)()
		defer db.Close()
		testTokenLifecycleMigration(t, db)
	})
	t.Run("lifecycle", func(t *testing.T) {
		db := openProfilePostgresSchema(t, dsn)()
		defer db.Close()
		testActivationLifecycle(t, seedActivation(t, db))
	})
	t.Run("concurrent last slot", func(t *testing.T) { testActivationLastSlot(t, openProfilePostgresSchema(t, dsn)) })
}

func TestRegistrationTokenAdminValueByteLimit(t *testing.T) {
	s := seedActivation(t, profileSQLTestDB(t))
	for _, value := range []string{strings.Repeat("x", 512), strings.Repeat("é", 256)} {
		body := &adminhttp.RegistrationTokenUpsert{Id: "boundary", Token: value, RuntimeProfileId: "profile"}
		created, err := s.CreateRegistrationToken(t.Context(), adminhttp.CreateRegistrationTokenRequestObject{Body: body})
		if _, ok := created.(adminhttp.CreateRegistrationToken200JSONResponse); err != nil || !ok {
			t.Fatalf("create 512 bytes: %T %v", created, err)
		}
		body.Token = value + "x"
		updated, err := s.PutRegistrationToken(t.Context(), adminhttp.PutRegistrationTokenRequestObject{Id: "boundary", Body: body})
		if _, ok := updated.(adminhttp.PutRegistrationToken400JSONResponse); err != nil || !ok {
			t.Fatalf("update 513 bytes: %T %v", updated, err)
		}
		body.Id = "oversized"
		created, err = s.CreateRegistrationToken(t.Context(), adminhttp.CreateRegistrationTokenRequestObject{Body: body})
		if _, ok := created.(adminhttp.CreateRegistrationToken400JSONResponse); err != nil || !ok {
			t.Fatalf("create 513 bytes: %T %v", created, err)
		}
		item, version, err := getRegistrationTokenSQL(t.Context(), s.DB, "boundary")
		if err != nil || item.Token != value {
			t.Fatalf("failed update changed token: %v", err)
		}
		if _, _, err := deleteRegistrationTokenSQL(t.Context(), s.DB, "boundary", version); err != nil {
			t.Fatal(err)
		}
	}
}
