CREATE TABLE IF NOT EXISTS acl_roles (
	name TEXT PRIMARY KEY,
	permissions_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS acl_views (
	name TEXT PRIMARY KEY,
	description TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS acl_policy_bindings (
	id TEXT PRIMARY KEY,
	display_order REAL NOT NULL DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS acl_binding_permissions (
	binding_id TEXT NOT NULL,
	subject_kind TEXT NOT NULL,
	subject_id TEXT NOT NULL,
	resource_kind TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	permission TEXT NOT NULL,
	not_before TEXT,
	expires_at TEXT,
	PRIMARY KEY (binding_id, permission)
);
