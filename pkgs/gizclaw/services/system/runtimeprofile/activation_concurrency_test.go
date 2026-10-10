package runtimeprofile

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
)

func activationPostgres(t *testing.T) func() *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is required")
	}
	open := openProfilePostgresSchema(t, dsn)
	return func() *sqlx.DB {
		db := open()
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
}

func activationBackendPID(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var pid int
	if err := db.GetContext(t.Context(), &pid, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	return pid
}

// Observe the actual database dependency before releasing a writer. No sleep
// is used to guess whether the competing RegisterOwner has reached its lock.
func awaitActivationBlock(t *testing.T, db *sqlx.DB, pid int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := db.GetContext(ctx, &blocked, "SELECT cardinality(pg_blocking_pids($1))>0", pid); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("backend %d never reached the competing lock", pid)
		case <-ticker.C:
		}
	}
}

func startActivation(ctx context.Context, s *Server, owner string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := s.RegisterOwner(ctx, owner, "value", nil)
		done <- err
	}()
	return done
}

func finishActivation(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("registration did not make progress")
		return nil
	}
}

func TestPostgreSQLRegistrationIndependentProgress(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		limited   bool
		sameOwner bool
	}{
		{name: "different owners share unlimited token"},
		{name: "limited token serializes", limited: true},
		{name: "same owner remains idempotent", sameOwner: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			open := activationPostgres(t)
			s := seedActivation(t, open())
			other := &Server{DB: open(), Now: s.Now}
			observer, holder := open(), open()
			if scenario.limited {
				if _, err := s.DB.Exec("UPDATE registration_tokens SET max_activations=1"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.DB.Exec(`INSERT INTO runtime_profile_owners VALUES ('held',NULL,'before',NULL)`); err != nil {
				t.Fatal(err)
			}
			tx, err := holder.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec("UPDATE runtime_profile_owners SET binding_id=binding_id WHERE owner_public_key='held'"); err != nil {
				t.Fatal(err)
			}
			pid, otherPID := activationBackendPID(t, s.DB), activationBackendPID(t, other.DB)
			first := startActivation(t.Context(), s, "held")
			awaitActivationBlock(t, observer, pid)
			owner := "independent"
			if scenario.sameOwner {
				owner = "held"
			}
			second := startActivation(t.Context(), other, owner)
			secondPending := true
			if scenario.limited || scenario.sameOwner {
				awaitActivationBlock(t, observer, otherPID)
			} else {
				select {
				case err := <-second:
					secondPending = false
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					// Release and join both writers before reporting the failure,
					// so the regression also cleans up against the old algorithm.
					t.Error("independent owner registration is blocked by another owner")
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := finishActivation(t, first); err != nil {
				t.Fatal(err)
			}
			if secondPending {
				err := finishActivation(t, second)
				if scenario.limited && !errors.Is(err, ErrRegistrationDenied) || !scenario.limited && err != nil {
					t.Fatal(err)
				}
			}
			item, _, err := getRegistrationTokenSQL(t.Context(), observer, "token")
			want := int64(2)
			if scenario.sameOwner || scenario.limited {
				want = 1
			}
			if err != nil || item.ActivationCount != want {
				t.Fatalf("count=%d, want %d: %v", item.ActivationCount, want, err)
			}
		})
	}
}

func TestPostgreSQLRegistrationSeesConcurrentTokenChange(t *testing.T) {
	for _, change := range []string{
		"enabled=FALSE", "expires_at='2020-01-01T00:00:00Z'", "max_activations=0",
		"token='replaced'", "runtime_profile_id='other'", "firmware_id='other'", "delete",
	} {
		t.Run(change, func(t *testing.T) {
			open := activationPostgres(t)
			s := seedActivation(t, open())
			observer, writer := open(), open()
			pid := activationBackendPID(t, s.DB)
			tx, err := writer.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			query := "UPDATE registration_tokens SET " + change + " WHERE id='token'"
			if change == "delete" {
				query = "DELETE FROM registration_tokens WHERE id='token'"
			}
			if _, err := tx.Exec(query); err != nil {
				t.Fatal(err)
			}
			done := startActivation(t.Context(), s, "new-owner")
			awaitActivationBlock(t, observer, pid)
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := finishActivation(t, done); !errors.Is(err, ErrRegistrationDenied) && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("changed token accepted: %v", err)
			}
			assertActivationCounts(t, observer, 0, 0)
		})
	}
}

func TestPostgreSQLRegistrationPinsTokenUntilCommit(t *testing.T) {
	for _, deletion := range []bool{false, true} {
		t.Run(map[bool]string{false: "update", true: "delete"}[deletion], func(t *testing.T) {
			open := activationPostgres(t)
			s := seedActivation(t, open())
			observer, holder, writer := open(), open(), open()
			if _, err := s.DB.Exec(`INSERT INTO runtime_profile_owners VALUES ('held',NULL,'before',NULL)`); err != nil {
				t.Fatal(err)
			}
			tx, err := holder.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec("UPDATE runtime_profile_owners SET binding_id=binding_id WHERE owner_public_key='held'"); err != nil {
				t.Fatal(err)
			}
			pid, writerPID := activationBackendPID(t, s.DB), activationBackendPID(t, writer)
			done := startActivation(t.Context(), s, "held")
			awaitActivationBlock(t, observer, pid)
			item, version, err := getRegistrationTokenSQL(t.Context(), observer, "token")
			if err != nil {
				t.Fatal(err)
			}
			changed := make(chan error, 1)
			go func() {
				var err error
				if deletion {
					_, _, err = deleteRegistrationTokenSQL(t.Context(), writer, item.Id, version)
				} else {
					item.Enabled = false
					_, _, err = updateRegistrationTokenSQL(t.Context(), writer, item, version)
				}
				changed <- err
			}()
			awaitActivationBlock(t, observer, writerPID)
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := finishActivation(t, done); err != nil {
				t.Fatal(err)
			}
			if err := finishActivation(t, changed); err != nil {
				t.Fatal(err)
			}
			want := 1
			if deletion {
				want = 0
			}
			assertActivationCounts(t, observer, want, 1)
			if profile, err := s.ResolveOwnerProfile(t.Context(), "held"); err != nil || profile.Id != "profile" {
				t.Fatalf("committed binding: %s %v", profile.Id, err)
			}
		})
	}
}

func TestPostgreSQLRegistrationLimitAddedWhileWaiting(t *testing.T) {
	open := activationPostgres(t)
	s := seedActivation(t, open())
	other := &Server{DB: open(), Now: s.Now}
	observer, writer := open(), open()
	pid, otherPID := activationBackendPID(t, s.DB), activationBackendPID(t, other.DB)
	tx, err := writer.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE registration_tokens SET max_activations=1"); err != nil {
		t.Fatal(err)
	}
	first, second := startActivation(t.Context(), s, "a"), startActivation(t.Context(), other, "b")
	awaitActivationBlock(t, observer, pid)
	awaitActivationBlock(t, observer, otherPID)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	a, b := finishActivation(t, first), finishActivation(t, second)
	if !(a == nil && errors.Is(b, ErrRegistrationDenied) || b == nil && errors.Is(a, ErrRegistrationDenied)) {
		t.Fatalf("last slot: %v / %v", a, b)
	}
	assertActivationCounts(t, observer, 1, 1)
}

func TestPostgreSQLRegistrationCancellationRollsBack(t *testing.T) {
	open := activationPostgres(t)
	s := seedActivation(t, open())
	observer, holder := open(), open()
	if _, err := s.DB.Exec(`INSERT INTO runtime_profile_owners VALUES ('held',NULL,'before',NULL)`); err != nil {
		t.Fatal(err)
	}
	tx, err := holder.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE runtime_profile_owners SET binding_id=binding_id WHERE owner_public_key='held'"); err != nil {
		t.Fatal(err)
	}
	pid := activationBackendPID(t, s.DB)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	published := make(chan Registration, 1)
	done := make(chan error, 1)
	go func() {
		_, err := s.RegisterOwner(ctx, "held", "value", func(r Registration) { published <- r })
		done <- err
	}()
	awaitActivationBlock(t, observer, pid)
	cancel()
	if err := finishActivation(t, done); err == nil {
		t.Fatal("canceled registration succeeded")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertActivationCounts(t, observer, 0, 1)
	select {
	case <-published:
		t.Fatal("published a rolled-back registration")
	default:
	}
	var profile *string
	if err := observer.GetContext(t.Context(), &profile, "SELECT runtime_profile_id FROM runtime_profile_owners WHERE owner_public_key='held'"); err != nil || profile != nil {
		t.Fatalf("canceled registration changed binding: %v %v", profile, err)
	}
}

func assertActivationCounts(t *testing.T, db *sqlx.DB, activations, owners int) {
	t.Helper()
	var counts struct{ Activations, Owners int }
	if err := db.GetContext(t.Context(), &counts, `SELECT (SELECT COUNT(*) FROM registration_token_activations) AS activations,
		(SELECT COUNT(*) FROM runtime_profile_owners) AS owners`); err != nil {
		t.Fatal(err)
	}
	if counts.Activations != activations || counts.Owners != owners {
		t.Fatalf("activations/owners=%d/%d, want %d/%d", counts.Activations, counts.Owners, activations, owners)
	}
}

func TestPostgreSQLRegistrationConfiguredIsolation(t *testing.T) {
	open := activationPostgres(t)
	testActivationLastSlot(t, func() *sqlx.DB {
		db := open()
		if _, err := db.Exec("SET default_transaction_isolation='repeatable read'"); err != nil {
			t.Fatal(err)
		}
		return db
	})
}

func TestRegistrationProfileAndFirmwareSelection(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) { testActivationSelection(t, seedActivation(t, profileSQLTestDB(t))) })
	t.Run("postgres", func(t *testing.T) { testActivationSelection(t, seedActivation(t, activationPostgres(t)())) })
}

func testActivationSelection(t *testing.T, s *Server) {
	t.Helper()
	ctx := t.Context()
	s.ResolveResource = func(_ context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
		var resource apitypes.Resource
		if kind != apitypes.ResourceKindFirmware || id != "firmware" {
			return resource, sql.ErrNoRows
		}
		err := resource.FromFirmwareResource(apitypes.FirmwareResource{Metadata: apitypes.ResourceMetadata{Id: id}})
		return resource, err
	}
	item, version, err := getRegistrationTokenSQL(ctx, s.DB, "token")
	if err != nil {
		t.Fatal(err)
	}
	item.FirmwareId = new("firmware")
	_, version, err = updateRegistrationTokenSQL(ctx, s.DB, item, version)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := s.RegisterOwner(ctx, "owner", "value", nil)
	if err != nil || registration.RuntimeProfile.Id != "profile" || registration.FirmwareID == nil || *registration.FirmwareID != "firmware" {
		t.Fatalf("initial selection: %#v %v", registration, err)
	}
	if _, err := insertRuntimeProfileSQL(ctx, s.DB, apitypes.RuntimeProfile{Id: "other", Revision: "current", CreatedAt: s.now(), UpdatedAt: s.now()}); err != nil {
		t.Fatal(err)
	}
	item.RuntimeProfileId, item.FirmwareId, item.Enabled, item.MaxActivations = "other", nil, false, new(int64(0))
	_, _, err = updateRegistrationTokenSQL(ctx, s.DB, item, version)
	if err != nil {
		t.Fatal(err)
	}
	var published Registration
	registration, err = s.RegisterOwner(ctx, "owner", "value", func(r Registration) { published = r })
	if err != nil || registration.RuntimeProfile.Id != "other" || registration.RuntimeProfile.Revision != "current" || published.RuntimeProfile.Id != "other" || registration.FirmwareID != nil {
		t.Fatalf("idempotent selection: %#v %#v %v", registration, published, err)
	}
	firmware, err := s.ResolveOwnerFirmware(ctx, "owner")
	if err != nil || firmware == nil || *firmware != "firmware" {
		t.Fatalf("nil selection lost previous firmware: %v %v", firmware, err)
	}
	profile, err := s.ResolveOwnerProfile(ctx, "owner")
	if err != nil || profile.Id != "other" {
		t.Fatalf("owner profile: %#v %v", profile, err)
	}
	assertActivationCounts(t, s.DB, 1, 1)
}
