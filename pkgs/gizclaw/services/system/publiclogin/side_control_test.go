package publiclogin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestSideControlDeviceTokenLoginAndRevocation(t *testing.T) {
	serverKey := generateTestKeyPair(t)
	targetKey := generateTestKeyPair(t)
	controllerKey := generateTestKeyPair(t)
	store := kv.NewMemory(nil)
	server := NewServer(serverKey, store)
	manager := server.SessionManager()

	deviceToken, err := manager.CreateSideControlDeviceToken(context.Background(), targetKey.Public)
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	for entry, err := range store.List(context.Background(), nil) {
		if err != nil {
			t.Fatalf("List store error = %v", err)
		}
		if bytes.Contains([]byte(entry.Key.String()), []byte(deviceToken.Token)) || bytes.Contains(entry.Value, []byte(deviceToken.Token)) {
			t.Fatal("raw device token was persisted")
		}
	}

	login := loginSideController(t, server, controllerKey, deviceToken.Token)
	principal, err := manager.AuthenticatePrincipal("Bearer " + login.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticatePrincipal error = %v", err)
	}
	if principal.Kind != SessionKindSideControl || principal.PublicKey != controllerKey.Public || principal.TargetPublicKey != targetKey.Public || principal.SessionID == "" {
		t.Fatalf("principal = %+v", principal)
	}

	items, err := manager.ListSideControlSessions(context.Background(), targetKey.Public)
	if err != nil {
		t.Fatalf("ListSideControlSessions error = %v", err)
	}
	if len(items) != 1 || items[0].Id != principal.SessionID || items[0].ControllerPublicKey != controllerKey.Public.String() {
		t.Fatalf("sessions = %+v", items)
	}

	assertion, err := NewLoginAssertion(controllerKey, serverKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	replay, err := server.Login(context.Background(), peerhttp.LoginRequestObject{
		Params: peerhttp.LoginParams{XPublicKey: controllerKey.Public.String(), Authorization: "Bearer " + assertion},
		Body:   &peerhttp.LoginJSONRequestBody{GrantType: peerhttp.SideControl, DeviceToken: deviceToken.Token},
	})
	if err != nil {
		t.Fatalf("replay Login error = %v", err)
	}
	if _, ok := replay.(peerhttp.Login401JSONResponse); !ok {
		t.Fatalf("replay response = %T, want 401", replay)
	}

	if err := manager.RevokeSideControlSession(context.Background(), targetKey.Public, principal.SessionID); err != nil {
		t.Fatalf("RevokeSideControlSession error = %v", err)
	}
	if _, err := manager.AuthenticatePrincipal("Bearer " + login.AccessToken); err == nil {
		t.Fatal("revoked side-control bearer still authenticates")
	}
	if err := manager.RevokeSideControlSession(context.Background(), targetKey.Public, principal.SessionID); !errors.Is(err, errSideControlSessionNotFound) {
		t.Fatalf("second revoke error = %v", err)
	}
}

func TestSideControlDeviceTokenCanBeRevokedBeforeUse(t *testing.T) {
	manager := NewSessionManager(kv.NewMemory(nil))
	target := generateTestKeyPair(t).Public
	otherTarget := generateTestKeyPair(t).Public
	token, err := manager.CreateSideControlDeviceToken(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	if err := manager.RevokeSideControlDeviceToken(context.Background(), otherTarget, token.Id); !errors.Is(err, errDeviceTokenNotFound) {
		t.Fatalf("wrong-owner revoke error = %v", err)
	}
	if err := manager.RevokeSideControlDeviceToken(context.Background(), target, token.Id); err != nil {
		t.Fatalf("RevokeSideControlDeviceToken error = %v", err)
	}
	if err := manager.RevokeSideControlDeviceToken(context.Background(), target, token.Id); !errors.Is(err, errDeviceTokenNotFound) {
		t.Fatalf("second revoke error = %v", err)
	}
}

func TestSideControlManagementRejectsMalformedIDs(t *testing.T) {
	manager := NewSessionManager(kv.NewMemory(nil))
	target := generateTestKeyPair(t).Public

	if err := manager.RevokeSideControlDeviceToken(context.Background(), target, "a:b"); !errors.Is(err, errDeviceTokenNotFound) {
		t.Fatalf("malformed device token ID error = %v", err)
	}
	if err := manager.RevokeSideControlSession(context.Background(), target, "a:b"); !errors.Is(err, errSideControlSessionNotFound) {
		t.Fatalf("malformed session ID error = %v", err)
	}
}

func TestSideControlDeviceTokenExpires(t *testing.T) {
	manager := NewSessionManager(kv.NewMemory(nil))
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	target := generateTestKeyPair(t).Public
	token, err := manager.CreateSideControlDeviceToken(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	now = now.Add(deviceTokenTTL + time.Second)
	if err := manager.RevokeSideControlDeviceToken(context.Background(), target, token.Id); !errors.Is(err, errDeviceTokenNotFound) {
		t.Fatalf("expired token revoke error = %v", err)
	}
}

func TestSideControlDeviceTokenConcurrentConsumeHasOneWinner(t *testing.T) {
	serverKey := generateTestKeyPair(t)
	target := generateTestKeyPair(t).Public
	manager := NewSessionManager(kv.NewMemory(nil))
	token, err := manager.CreateSideControlDeviceToken(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	controllers := []*giznet.KeyPair{generateTestKeyPair(t), generateTestKeyPair(t)}
	assertions := make([]string, len(controllers))
	for i, controller := range controllers {
		assertions[i], err = NewLoginAssertion(controller, serverKey.Public, time.Minute)
		if err != nil {
			t.Fatalf("NewLoginAssertion error = %v", err)
		}
	}

	errorsByController := make([]error, len(controllers))
	var wait sync.WaitGroup
	for i, controller := range controllers {
		wait.Add(1)
		go func(i int, controller *giznet.KeyPair) {
			defer wait.Done()
			_, errorsByController[i] = manager.loginSideControl(context.Background(), serverKey, controller.Public, assertions[i], token.Token)
		}(i, controller)
	}
	wait.Wait()
	winners := 0
	consumed := 0
	for _, err := range errorsByController {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, errDeviceTokenNotFound):
			consumed++
		default:
			t.Fatalf("concurrent login error = %v", err)
		}
	}
	if winners != 1 || consumed != 1 {
		t.Fatalf("winners = %d consumed failures = %d", winners, consumed)
	}
}

func TestSideControlSessionExpires(t *testing.T) {
	serverKey := generateTestKeyPair(t)
	target := generateTestKeyPair(t).Public
	controller := generateTestKeyPair(t)
	manager := NewSessionManager(kv.NewMemory(nil))
	now := time.Now().UTC().Truncate(time.Second)
	manager.now = func() time.Time { return now }
	token, err := manager.CreateSideControlDeviceToken(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	assertion, err := newLoginAssertionAt(controller, serverKey.Public, now, time.Minute, strings.NewReader(strings.Repeat("s", 16)))
	if err != nil {
		t.Fatalf("newLoginAssertionAt error = %v", err)
	}
	login, err := manager.loginSideControl(context.Background(), serverKey, controller.Public, assertion, token.Token)
	if err != nil {
		t.Fatalf("loginSideControl error = %v", err)
	}
	now = now.Add(defaultSessionTTL + time.Second)
	if _, err := manager.AuthenticatePrincipal("Bearer " + login.AccessToken); err == nil {
		t.Fatal("expired side-control session still authenticates")
	}
	sessions, err := manager.ListSideControlSessions(context.Background(), target)
	if err != nil {
		t.Fatalf("ListSideControlSessions error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expired sessions = %+v", sessions)
	}
}

func TestPrimarySessionRemainsTyped(t *testing.T) {
	serverKey := generateTestKeyPair(t)
	deviceKey := generateTestKeyPair(t)
	manager := NewSessionManager(kv.NewMemory(nil))
	assertion, err := NewLoginAssertion(deviceKey, serverKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	login, err := manager.login(context.Background(), serverKey, deviceKey.Public, assertion, nil)
	if err != nil {
		t.Fatalf("login error = %v", err)
	}
	principal, err := manager.AuthenticatePrincipal("Bearer " + login.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticatePrincipal error = %v", err)
	}
	if principal.Kind != SessionKindPrimary || principal.PublicKey != deviceKey.Public || !principal.TargetPublicKey.IsZero() {
		t.Fatalf("principal = %+v", principal)
	}
}

func TestDeviceTokenManagementRequiresPrimaryPrincipal(t *testing.T) {
	server := NewServer(generateTestKeyPair(t), kv.NewMemory(nil))
	target := generateTestKeyPair(t).Public
	primaryContext := WithPrincipal(context.Background(), Principal{Kind: SessionKindPrimary, PublicKey: target})
	response, err := server.CreateSideControlDeviceToken(primaryContext, peerhttp.CreateSideControlDeviceTokenRequestObject{})
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken error = %v", err)
	}
	if _, ok := response.(peerhttp.CreateSideControlDeviceToken201JSONResponse); !ok {
		t.Fatalf("primary response = %T", response)
	}

	sideContext := WithPrincipal(context.Background(), Principal{Kind: SessionKindSideControl, PublicKey: generateTestKeyPair(t).Public, TargetPublicKey: target})
	response, err = server.CreateSideControlDeviceToken(sideContext, peerhttp.CreateSideControlDeviceTokenRequestObject{})
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken side error = %v", err)
	}
	if _, ok := response.(peerhttp.CreateSideControlDeviceToken403JSONResponse); !ok {
		t.Fatalf("side response = %T, want 403", response)
	}

	callerOnlyContext := peerhttp.WithCallerPublicKey(context.Background(), target)
	response, err = server.CreateSideControlDeviceToken(callerOnlyContext, peerhttp.CreateSideControlDeviceTokenRequestObject{})
	if err != nil {
		t.Fatalf("CreateSideControlDeviceToken caller-only error = %v", err)
	}
	if _, ok := response.(peerhttp.CreateSideControlDeviceToken403JSONResponse); !ok {
		t.Fatalf("caller-only response = %T, want 403", response)
	}
}

func loginSideController(t *testing.T, server *Server, controller *giznet.KeyPair, deviceToken string) peerhttp.Login200JSONResponse {
	t.Helper()
	assertion, err := NewLoginAssertion(controller, server.KeyPair.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	response, err := server.Login(context.Background(), peerhttp.LoginRequestObject{
		Params: peerhttp.LoginParams{XPublicKey: controller.Public.String(), Authorization: "Bearer " + assertion},
		Body:   &peerhttp.LoginJSONRequestBody{GrantType: peerhttp.SideControl, DeviceToken: deviceToken},
	})
	if err != nil {
		t.Fatalf("Login error = %v", err)
	}
	login, ok := response.(peerhttp.Login200JSONResponse)
	if !ok {
		t.Fatalf("Login response = %T", response)
	}
	return login
}

func generateTestKeyPair(t *testing.T) *giznet.KeyPair {
	t.Helper()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	return keyPair
}
