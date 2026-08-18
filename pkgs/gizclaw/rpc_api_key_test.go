package gizclaw

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRPCAPIKeyCreateRequiresRegistrationAndReturnsSecretOnce(t *testing.T) {
	profiles, _ := registrationServerAndToken(t, "profile-api-key")
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := &rpcServer{
		registrations:   profiles,
		apiKeys:         apikey.NewServer(kv.NewMemory(nil)),
		callerPublicKey: keyPair.Public,
		validateAPIKeyOwner: func(context.Context, giznet.PublicKey) error {
			return nil
		},
	}
	request := newRPCRequest("api-key", rpcapi.RPCMethodServerAPIKeyCreate, mustRPCParams(
		rpcapi.APIKeyCreateRequest{DisplayName: "phone", ManageAPIKeys: true},
		(*rpcapi.RPCPayload).FromAPIKeyCreateRequest,
	))

	response, err := server.dispatch(context.Background(), request)
	if err != nil || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeForbidden {
		t.Fatalf("unregistered dispatch = %#v, %v", response, err)
	}
	if err := profiles.BindOwnerProfile(t.Context(), keyPair.Public.String(), "profile-api-key"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		err  error
		code rpcapi.RPCErrorCode
	}{
		{name: "non-client", err: errAPIKeyOwnerUnavailable, code: rpcapi.RPCErrorCodeForbidden},
		{name: "pending deletion", err: peer.ErrPeerPendingDeletion, code: rpcapi.RPCErrorCodeConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server.validateAPIKeyOwner = func(context.Context, giznet.PublicKey) error { return tc.err }
			response, err := server.dispatch(t.Context(), request)
			if err != nil || response.Error == nil || response.Error.Code != tc.code {
				t.Fatalf("dispatch = %#v, %v; want code %v", response, err, tc.code)
			}
		})
	}
	server.validateAPIKeyOwner = func(context.Context, giznet.PublicKey) error { return nil }
	response, err = server.dispatch(context.Background(), request)
	if err != nil || response == nil || response.Error != nil || response.Result == nil {
		if response == nil {
			t.Fatalf("registered dispatch failed: err=%v nil response", err)
		}
		t.Fatalf("registered dispatch failed: err=%v rpc_error=%v result_present=%t", err, response.Error, response.Result != nil)
	}
	created, err := response.Result.AsAPIKeyCreateResponse()
	if err != nil {
		t.Fatal(err)
	}
	if created.Value == nil || !created.Value.ManageAPIKeys || !strings.HasPrefix(created.APIKey, "gizclaw_sk_v1_") {
		t.Fatal("created response returned invalid metadata or secret format")
	}
}
