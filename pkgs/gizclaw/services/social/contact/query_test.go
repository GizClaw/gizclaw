package contact

import (
	"fmt"
	"strings"
	"testing"
)

func TestSQLContactPageRestrictsOwnerAndReadRange(t *testing.T) {
	s := newTestServer(t)
	ctx := t.Context()
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 2000 {
		id := fmt.Sprintf("contact-%04d", i)
		if _, err := tx.ExecContext(ctx, `INSERT INTO contacts(id,owner_public_key,name,display_name,created_at,updated_at,incarnation) VALUES (?,?,?,?,?,?,?)`, id, "peer-a", id, "Contact", "2026-09-06T00:00:00Z", "2026-09-06T00:00:00Z", id); err != nil {
			t.Fatal(err)
		}
	}
	// Invalid timestamps reveal any application-side scan beyond the SQL page.
	if _, err := tx.ExecContext(ctx, `UPDATE contacts SET created_at='invalid' WHERE id<'contact-0999' OR id>'contact-1002'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO contacts(id,owner_public_key,name,display_name,created_at,updated_at,incarnation) VALUES ('other-owner','peer-b','other','Other','invalid','invalid','other')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, more, cursor, err := s.listContacts(ctx, "peer-a", "contact-0999", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].ID != "contact-1000" || page[1].ID != "contact-1001" || !more || cursor == nil || *cursor != "contact-1001" {
		t.Fatalf("page=%+v more=%v cursor=%v", page, more, cursor)
	}
	rows, err := s.DB.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT `+contactColumns+` FROM contacts WHERE owner_public_key=? AND id>? ORDER BY owner_public_key,id LIMIT ?`, "peer-a", "contact-0999", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "SEARCH contacts USING INDEX contacts_owner_id") || strings.Contains(plan.String(), "USE TEMP B-TREE") {
		t.Fatalf("page does not use ordered owner range: %s", plan.String())
	}
}
