package runtimeprofile

import (
	"errors"
	"testing"

	"database/sql"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestDeleteOwnerProfileBindingPreservesGlobalAndForeignState(t *testing.T) {
	ctx := t.Context()
	store := profileSQLTestDB(t)
	server := &Server{DB: store}
	retiring, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertRuntimeProfileSQL(ctx, store, apitypes.RuntimeProfile{Id: "profile-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := insertRegistrationTokenSQL(ctx, store, apitypes.RegistrationToken{Id: "token-a", Token: "token", RuntimeProfileId: "profile-a"}); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{retiring.Public.String(), foreign.Public.String()} {
		if err := server.BindOwnerProfile(ctx, owner, "profile-a"); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.DeleteOwnerProfileBinding(ctx, retiring.Public.String()); err != nil {
		t.Fatal(err)
	}
	if err := server.DeleteOwnerProfileBinding(ctx, retiring.Public.String()); err != nil {
		t.Fatalf("replay delete: %v", err)
	}
	if _, err := server.ResolveOwnerProfile(ctx, retiring.Public.String()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("retired owner = %v", err)
	}
	if _, err := server.ResolveOwnerProfile(ctx, foreign.Public.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := getProfileByID(ctx, store, "profile-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := getRegistrationTokenByID(ctx, store, "token-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveRegistrationSQL(ctx, store, "token"); err != nil {
		t.Fatal(err)
	}

}

func TestDeleteOwnerProfileBindingRejectsNonCanonicalKeyWithoutMutation(t *testing.T) {
	store := profileSQLTestDB(t)
	server := &Server{DB: store}
	if err := server.DeleteOwnerProfileBinding(t.Context(), " peer "); err == nil {
		t.Fatal("DeleteOwnerProfileBinding() error = nil")
	}
}
