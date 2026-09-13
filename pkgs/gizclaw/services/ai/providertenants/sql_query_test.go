package providertenants

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestTenantSQLPaginationRestrictsProviderAndRange(t *testing.T) {
	db := tenantTestDB(t)
	ctx := t.Context()
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 2000 {
		id := fmt.Sprintf("tenant-%04d", i)
		for _, kind := range []string{"openai", "deepseek"} {
			config := `{}`
			if kind == "deepseek" || i < 1000 || i > 1010 {
				config = `invalid json`
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO provider_tenants(provider_kind,id,credential_id,created_at,updated_at,config_json,incarnation) VALUES (?,?,?,'2026-09-06T00:00:00Z','2026-09-06T00:00:00Z',?,'initial')`, kind, id, "credential", config); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	items, more, cursor, err := listSQLTenants[apitypes.OpenAITenant](ctx, db, "openai", "tenant-0999", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 || !more || cursor == nil || *cursor != "tenant-1009" || items[0].Id != "tenant-1000" {
		t.Fatalf("page = %v, %v, %v", items, more, cursor)
	}
	rows, err := db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT `+tenantColumns+` FROM provider_tenants WHERE provider_kind=? AND id>? ORDER BY id LIMIT ?`, "openai", "tenant-0999", 11)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "SEARCH") || !strings.Contains(plan, "provider_kind=? AND id>?") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("query plan = %s", plan)
	}
}
