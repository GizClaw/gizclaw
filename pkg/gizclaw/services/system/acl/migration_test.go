package acl

import (
	"context"
	"testing"
)

func TestMigrationCreatesTablesAndIndexes(t *testing.T) {
	db := openTestDB(t)
	server := &Server{DB: db}
	for i := 0; i < 2; i++ {
		if err := server.Migration(context.Background()); err != nil {
			t.Fatalf("Migration(%d) error = %v", i, err)
		}
	}

	for _, name := range []string{
		"acl_roles",
		"acl_views",
		"acl_policy_bindings",
		"acl_binding_permissions",
	} {
		if !sqliteObjectExists(t, db, "table", name) {
			t.Fatalf("table %s was not created", name)
		}
	}
	for _, name := range []string{
		"idx_acl_policy_bindings_subject_resource_role",
		"idx_acl_policy_bindings_subject_resource",
		"idx_acl_policy_bindings_role",
		"idx_acl_policy_bindings_subject_display_order",
		"idx_acl_policy_bindings_resource_display_order",
		"idx_acl_policy_bindings_expires_at",
		"idx_acl_binding_permissions_subject_resource_permission",
		"idx_acl_binding_permissions_subject_resource",
		"idx_acl_binding_permissions_expires_at",
		"idx_acl_binding_permissions_not_before",
	} {
		if !sqliteObjectExists(t, db, "index", name) {
			t.Fatalf("index %s was not created", name)
		}
	}
}

func TestMigrationAddsDisplayOrderToExistingPolicyBindingTable(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`
CREATE TABLE acl_roles (
	name TEXT PRIMARY KEY,
	permissions_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE acl_policy_bindings (
	id TEXT PRIMARY KEY,
	subject_kind TEXT NOT NULL,
	subject_id TEXT NOT NULL,
	resource_kind TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	role TEXT NOT NULL,
	not_before TEXT,
	expires_at TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE acl_binding_permissions (
	binding_id TEXT NOT NULL,
	subject_kind TEXT NOT NULL,
	subject_id TEXT NOT NULL,
	resource_kind TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	permission TEXT NOT NULL,
	not_before TEXT,
	expires_at TEXT,
	PRIMARY KEY (binding_id, permission)
);`); err != nil {
		t.Fatalf("create old schema error = %v", err)
	}
	server := &Server{DB: db}
	if err := server.Migration(context.Background()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if !sqliteColumnExists(t, db, "acl_policy_bindings", "display_order") {
		t.Fatal("display_order column was not created")
	}
	if !sqliteObjectExists(t, db, "index", "idx_acl_policy_bindings_subject_display_order") {
		t.Fatal("display_order index was not created")
	}
}

func TestMigrationValidation(t *testing.T) {
	if err := (*Server)(nil).Migration(context.Background()); err == nil {
		t.Fatal("nil server Migration() error = nil")
	}
	if err := (&Server{}).Migration(context.Background()); err == nil {
		t.Fatal("missing db Migration() error = nil")
	}
}
