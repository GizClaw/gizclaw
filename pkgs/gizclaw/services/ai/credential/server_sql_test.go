package credential

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestSQLCredentialUpdatePreservesConcurrentRotation(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("recreate=%v", recreate), func(t *testing.T) {
			first := newTestServer(t)
			second := &Server{DB: first.DB}
			ctx := t.Context()
			original := mustCredentialUpsert(t, `{"id":"primary","provider":"openai","body":{"api_key":"old"}}`)
			response, err := first.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &original})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := response.(adminhttp.CreateCredential200JSONResponse); !ok {
				t.Fatalf("create type=%T", response)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var enterOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			first.Now = func() time.Time { enterOnce.Do(func() { close(entered); <-release }); return time.Now() }
			desired := mustCredentialUpsert(t, `{"id":"primary","provider":"openai","description":"updated"}`)
			done := make(chan adminhttp.PutCredentialResponseObject, 1)
			failed := make(chan error, 1)
			go func() {
				response, err := first.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Id: "primary", Body: &desired})
				done <- response
				failed <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("update did not reach write boundary")
			}
			rotated := mustCredentialUpsert(t, `{"id":"primary","provider":"openai","body":{"api_key":"new"}}`)
			if recreate {
				response, err := second.DeleteCredential(ctx, adminhttp.DeleteCredentialRequestObject{Id: "primary"})
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := response.(adminhttp.DeleteCredential200JSONResponse); !ok {
					t.Fatalf("delete type=%T", response)
				}
				response2, err := second.CreateCredential(ctx, adminhttp.CreateCredentialRequestObject{Body: &rotated})
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := response2.(adminhttp.CreateCredential200JSONResponse); !ok {
					t.Fatalf("recreate type=%T", response2)
				}
			} else {
				response, err := second.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Id: "primary", Body: &rotated})
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := response.(adminhttp.PutCredential200JSONResponse); !ok {
					t.Fatalf("rotation type=%T", response)
				}
			}
			unblock()
			select {
			case response := <-done:
				if err := <-failed; err != nil {
					t.Fatal(err)
				}
				if recreate {
					failure, ok := response.(adminhttp.PutCredential500JSONResponse)
					if !ok || failure.Error.Code != "CREDENTIAL_CONFLICT" {
						t.Fatalf("stale update type=%T", response)
					}
				} else {
					if _, ok := response.(adminhttp.PutCredential200JSONResponse); !ok {
						t.Fatalf("update type=%T", response)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("update did not complete")
			}
			record, err := getCredentialRecord(ctx, first.DB, "primary")
			if err != nil {
				t.Fatal(err)
			}
			body, err := record.Body.AsOpenAICredentialBody()
			if err != nil {
				t.Fatal(err)
			}
			if body.ApiKey == nil || *body.ApiKey != "new" {
				t.Fatal("stale update overwrote rotated secret")
			}
			if recreate && record.Description != nil {
				t.Fatal("stale update modified recreated credential")
			}
		})
	}
}

func TestSQLCredentialProviderPageUsesIndex(t *testing.T) {
	s := newTestServer(t)
	ctx := t.Context()
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range 2000 {
		provider, body := "volc", "invalid"
		if i >= 1000 && i <= 1002 {
			provider, body = "openai", `{"api_key":"fixture"}`
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO credentials(id,provider,body_json,created_at,updated_at,revision,incarnation) VALUES (?,?,?,'2026-09-06T00:00:00Z','2026-09-06T00:00:00Z',1,'fixture')`, fmt.Sprintf("credential-%04d", i), provider, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, more, cursor, err := listCredentialsPage(ctx, s.DB, "openai", "credential-0999", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Id != "credential-1000" || page[1].Id != "credential-1001" || !more || cursor == nil || *cursor != "credential-1001" {
		t.Fatalf("page length=%d more=%v cursor=%v", len(page), more, cursor)
	}
	rows, err := s.DB.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT `+credentialColumns+` FROM credentials WHERE id>? AND provider=? ORDER BY id LIMIT ?`, "credential-0999", "openai", 3)
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
	if !strings.Contains(plan.String(), "SEARCH credentials USING INDEX credentials_provider_id") || strings.Contains(plan.String(), "USE TEMP B-TREE") {
		t.Fatalf("provider page query plan: %s", plan.String())
	}
}
