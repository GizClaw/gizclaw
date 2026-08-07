package publiclogin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestCleanupPeerRevokesOwnedTargetAndControllerCredentials(t *testing.T) {
	ctx := t.Context()
	serverKey := generateTestKeyPair(t)
	retiring := generateTestKeyPair(t)
	foreign := generateTestKeyPair(t)
	other := generateTestKeyPair(t)
	manager := NewSessionManager(kv.NewMemory(nil))

	retiringPrimary := loginPrimaryForCleanup(t, manager, serverKey, retiring)
	foreignPrimary := loginPrimaryForCleanup(t, manager, serverKey, foreign)

	foreignTargetToken, err := manager.CreateSideControlDeviceToken(ctx, foreign.Public)
	if err != nil {
		t.Fatal(err)
	}
	retiringController := loginSideControlForCleanup(t, manager, serverKey, retiring, foreignTargetToken.Token)

	retiringTargetToken, err := manager.CreateSideControlDeviceToken(ctx, retiring.Public)
	if err != nil {
		t.Fatal(err)
	}
	foreignController := loginSideControlForCleanup(t, manager, serverKey, other, retiringTargetToken.Token)
	if _, err := manager.CreateSideControlDeviceToken(ctx, retiring.Public); err != nil {
		t.Fatal(err)
	}
	preservedToken, err := manager.CreateSideControlDeviceToken(ctx, foreign.Public)
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.CleanupPeer(ctx, retiring.Public.String()); err != nil {
		t.Fatal(err)
	}
	if err := manager.CleanupPeer(ctx, retiring.Public.String()); err != nil {
		t.Fatalf("replay cleanup: %v", err)
	}
	for _, token := range []string{retiringPrimary.AccessToken, retiringController.AccessToken, foreignController.AccessToken} {
		if _, err := manager.AuthenticatePrincipal("Bearer " + token); err == nil {
			t.Fatalf("retiring credential %q still authenticates", token)
		}
	}
	if _, err := manager.AuthenticatePrincipal("Bearer " + foreignPrimary.AccessToken); err != nil {
		t.Fatalf("foreign primary session removed: %v", err)
	}
	if err := manager.RevokeSideControlDeviceToken(ctx, foreign.Public, preservedToken.Id); err != nil {
		t.Fatalf("foreign device token removed: %v", err)
	}
}

func TestCleanupPeerFailsBeforeMutationOnCrossOwnedIndex(t *testing.T) {
	ctx := t.Context()
	serverKey := generateTestKeyPair(t)
	retiring := generateTestKeyPair(t)
	foreign := generateTestKeyPair(t)
	manager := NewSessionManager(kv.NewMemory(nil))
	login := loginPrimaryForCleanup(t, manager, serverKey, retiring)
	if err := manager.Store.Set(ctx, primarySessionOwnerKey(retiring.Public, "foreign"), []byte{1}); err != nil {
		t.Fatal(err)
	}
	foreignBody := []byte(`{"kind":"primary","public_key":"` + foreign.Public.String() + `","expires_at":4102444800000}`)
	if err := manager.Store.Set(ctx, sessionKey("foreign"), foreignBody); err != nil {
		t.Fatal(err)
	}
	if err := manager.CleanupPeer(ctx, retiring.Public.String()); err == nil {
		t.Fatal("CleanupPeer() error = nil")
	}
	if _, err := manager.AuthenticatePrincipal("Bearer " + login.AccessToken); err != nil {
		t.Fatalf("cleanup partially mutated before validation failure: %v", err)
	}
}

func TestSessionAvailabilityFencesExistingAndSideControlUse(t *testing.T) {
	ctx := t.Context()
	serverKey := generateTestKeyPair(t)
	retiring := generateTestKeyPair(t)
	manager := NewSessionManager(kv.NewMemory(nil))
	login := loginPrimaryForCleanup(t, manager, serverKey, retiring)
	denied := errors.New("peer deleted")
	manager.Authorizer = func(_ context.Context, key giznet.PublicKey) error {
		if key == retiring.Public {
			return denied
		}
		return nil
	}
	if _, err := manager.AuthenticatePrincipal("Bearer " + login.AccessToken); !errors.Is(err, denied) {
		t.Fatalf("AuthenticatePrincipal() error = %v", err)
	}
	if _, err := manager.CreateSideControlDeviceToken(ctx, retiring.Public); !errors.Is(err, denied) {
		t.Fatalf("CreateSideControlDeviceToken() error = %v", err)
	}
}

func loginPrimaryForCleanup(t *testing.T, manager *SessionManager, server, owner *giznet.KeyPair) peerhttp.LoginResult {
	t.Helper()
	assertion, err := NewLoginAssertion(owner, server.Public, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.login(t.Context(), server, owner.Public, assertion, manager.Authorizer)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func loginSideControlForCleanup(t *testing.T, manager *SessionManager, server, controller *giznet.KeyPair, token string) peerhttp.LoginResult {
	t.Helper()
	assertion, err := NewLoginAssertion(controller, server.Public, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.loginSideControl(t.Context(), server, controller.Public, assertion, token)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
