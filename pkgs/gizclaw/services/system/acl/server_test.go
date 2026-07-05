package acl

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	_ "modernc.org/sqlite"
)

func TestAuthorizeValidation(t *testing.T) {
	if err := (*Server)(nil).Authorize(context.Background(), AuthorizeRequest{}); err == nil {
		t.Fatal("nil server Authorize() error = nil")
	}
	server := &Server{}
	if err := server.Authorize(context.Background(), AuthorizeRequest{}); err == nil {
		t.Fatal("missing db Authorize() error = nil")
	}
	db := openTestDB(t)
	server.DB = db
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	request := AuthorizeRequest{
		Subject:    PublicKeySubject("z6Mk"),
		Resource:   WorkspaceResource("demo"),
		Permission: "use",
	}
	if err := server.Authorize(context.Background(), request); !errors.Is(err, ErrDenied) {
		t.Fatalf("Authorize() error = %v, want %v", err, ErrDenied)
	}
	request.Permission = ""
	if err := server.Authorize(context.Background(), request); err == nil || !errors.Is(err, ErrDenied) && err.Error() != "acl: permission is required" {
		t.Fatalf("empty permission error = %v", err)
	}
	request.Permission = "use"
	request.Subject = apitypes.ACLSubject{}
	if err := server.Authorize(context.Background(), request); err == nil {
		t.Fatal("invalid subject Authorize() error = nil")
	}
	request.Subject = PublicKeySubject("z6Mk")
	request.Resource = apitypes.ACLResource{}
	if err := server.Authorize(context.Background(), request); err == nil {
		t.Fatal("invalid resource Authorize() error = nil")
	}
}

func TestAuthorizeFallsBackToAllPeersSubject(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if _, err := server.CreatePolicyBinding(ctx, "all-peers-reader", 0, apitypes.ACLPolicy{
		Subject:  AllPeersSubject(),
		Resource: WorkspaceResource("demo"),
		Role:     "workspace-reader",
	}); err != nil {
		t.Fatalf("CreatePolicyBinding() error = %v", err)
	}
	if err := server.Authorize(ctx, AuthorizeRequest{
		Subject:    PublicKeySubject("subject-a"),
		Resource:   WorkspaceResource("demo"),
		Permission: "read",
	}); err != nil {
		t.Fatalf("Authorize(peer via all_peers) error = %v", err)
	}
}

func TestCleanupQueueAndRun(t *testing.T) {
	server := &Server{}
	server.EnqueueExpiredPolicyBinding("")
	server.EnqueueExpiredPolicyBinding(" expired ")

	select {
	case got := <-server.cleanupCh:
		if got != "expired" {
			t.Fatalf("cleanup item = %q, want expired", got)
		}
	default:
		t.Fatal("cleanup item was not queued")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRunDeletesQueuedPolicyBinding(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	if _, err := server.CreateRole(ctx, "workspace-reader", apitypes.ACLPermissionList{"read"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if _, err := server.CreatePolicyBinding(ctx, "binding-a", 0, apitypes.ACLPolicy{
		Subject:  PublicKeySubject("subject-a"),
		Resource: WorkspaceResource("workspace-a"),
		Role:     "workspace-reader",
	}); err != nil {
		t.Fatalf("CreatePolicyBinding() error = %v", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(runCtx)
	}()
	server.EnqueueExpiredPolicyBinding("binding-a")
	for i := 0; i < 100; i++ {
		if _, err := server.GetPolicyBinding(ctx, "binding-a"); errors.Is(err, ErrPolicyBindingNotFound) {
			cancel()
			if err := <-errCh; !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want %v", err, context.Canceled)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("queued policy binding was not deleted")
}

func TestCleanupExpiredValidation(t *testing.T) {
	if _, err := (*Server)(nil).CleanupExpired(context.Background(), 1); err == nil {
		t.Fatal("nil server CleanupExpired() error = nil")
	}
	server := &Server{}
	if _, err := server.CleanupExpired(context.Background(), 1); err == nil {
		t.Fatal("missing db CleanupExpired() error = nil")
	}
	server.DB = openTestDB(t)
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	n, err := server.CleanupExpired(context.Background(), 0)
	if err != nil {
		t.Fatalf("CleanupExpired(limit=0) error = %v", err)
	}
	if n != 0 {
		t.Fatalf("CleanupExpired(limit=0) = %d, want 0", n)
	}
	if err := server.deletePolicyBinding(context.Background(), "missing"); err != nil {
		t.Fatalf("deletePolicyBinding() error = %v", err)
	}
}

func TestServerClockAndLogger(t *testing.T) {
	want := time.Date(2026, 5, 14, 1, 2, 3, 0, time.UTC)
	server := &Server{Now: func() time.Time { return want }}
	if got := server.now(); !got.Equal(want) {
		t.Fatalf("now() = %v, want %v", got, want)
	}
	if got := (&Server{}).now(); got.IsZero() {
		t.Fatal("default now() returned zero time")
	}
	if server.logger() == nil {
		t.Fatal("logger() returned nil")
	}
	customLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if got := (&Server{Logger: customLogger}).logger(); got != customLogger {
		t.Fatal("logger() did not return custom logger")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func migratedTestServer(t *testing.T) *Server {
	t.Helper()
	server := &Server{DB: openTestDB(t)}
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	return server
}

func sqliteObjectExists(t *testing.T, db *sql.DB, kind, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = ? AND name = ?`, kind, name).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	return count > 0
}

func sqliteColumnExists(t *testing.T, db *sql.DB, tableName, columnName string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		t.Fatalf("query table info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info: %v", err)
		}
		if name == columnName {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info: %v", err)
	}
	return false
}
