package gizclaw

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRPCRegistrationReplacesSnapshotAndRejectedTokenPreservesIt(t *testing.T) {
	t.Parallel()
	registrations, tokenA := registrationServerAndToken(t, "profile-a")
	tokenB := createRegistrationToken(t, registrations, "profile-b")
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	server := &rpcServer{
		registrations:   registrations,
		callerPublicKey: giznet.PublicKey{1},
		onRegistration: func(registration runtimeprofile.Registration) {
			snapshot.Store(&registration)
		},
	}

	response := registerRPC(t, server, tokenA)
	if response.RuntimeProfileName != "profile-a" {
		t.Fatalf("first registration = %#v", response)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-a" {
		t.Fatalf("first snapshot = %#v", got)
	}

	rejected, err := server.dispatch(context.Background(), registrationRequest("invalid-token"))
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Error == nil || rejected.Error.Code != rpcapi.RPCErrorCodeForbidden {
		t.Fatalf("invalid registration response = %#v", rejected)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-a" {
		t.Fatalf("rejected registration replaced snapshot: %#v", got)
	}

	response = registerRPC(t, server, tokenB)
	if response.RuntimeProfileName != "profile-b" {
		t.Fatalf("second registration = %#v", response)
	}
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-b" {
		t.Fatalf("second snapshot = %#v", got)
	}
}

func TestRPCRegistrationSnapshotIsRaceSafe(t *testing.T) {
	registrations, tokenA := registrationServerAndToken(t, "profile-a")
	tokenB := createRegistrationToken(t, registrations, "profile-b")
	var snapshot atomic.Pointer[runtimeprofile.Registration]
	server := &rpcServer{registrations: registrations, onRegistration: func(registration runtimeprofile.Registration) {
		snapshot.Store(&registration)
	}}

	var wg sync.WaitGroup
	for i := range 32 {
		token := tokenA
		if i%2 == 1 {
			token = tokenB
		}
		wg.Go(func() {
			response, err := server.dispatch(context.Background(), registrationRequest(token))
			if err != nil || response.Error != nil {
				t.Errorf("concurrent registration = %#v, %v", response, err)
			}
		})
	}
	wg.Wait()

	registerRPC(t, server, tokenA)
	if got := snapshot.Load(); got == nil || got.RuntimeProfile.Name != "profile-a" {
		t.Fatalf("last successful registration snapshot = %#v", got)
	}
}

func registrationServerAndToken(t *testing.T, profileName string) (*runtimeprofile.Server, string) {
	t.Helper()
	server := &runtimeprofile.Server{Store: kv.NewMemory(nil)}
	return server, createRegistrationToken(t, server, profileName)
}

func createRegistrationToken(t *testing.T, server *runtimeprofile.Server, profileName string) string {
	t.Helper()
	ctx := context.Background()
	profileResponse, err := server.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{
		Name: profileName,
		Body: &adminhttp.RuntimeProfileUpsert{
			Name: profileName,
			Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := profileResponse.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("put RuntimeProfile = %#v", profileResponse)
	}
	tokenResponse, err := server.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{
		Body: &adminhttp.RegistrationTokenUpsert{
			Name:               "token-" + profileName,
			RuntimeProfileName: profileName,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := tokenResponse.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || strings.TrimSpace(created.Token) == "" {
		t.Fatalf("create RegistrationToken = %#v", tokenResponse)
	}
	return created.Token
}

func registerRPC(t *testing.T, server *rpcServer, token string) rpcapi.ServerRegisterResponse {
	t.Helper()
	response, err := server.dispatch(context.Background(), registrationRequest(token))
	if err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("register response = %#v", response)
	}
	value, err := response.Result.AsServerRegisterResponse()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func registrationRequest(token string) *rpcapi.RPCRequest {
	return newRPCRequest("register", rpcapi.RPCMethodServerRegister, mustRPCParams(
		rpcapi.ServerRegisterRequest{Token: token},
		(*rpcapi.RPCPayload).FromServerRegisterRequest,
	))
}
