package gizclaw

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRPCAPIKeyCreateRequiresRegistrationAndReturnsRecoverableKey(t *testing.T) {
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
		rpcapi.APIKeyCreateRequest{DisplayName: "phone", ManageAPIKeys: false},
		(*rpcapi.RPCPayload).FromAPIKeyCreateRequest,
	))

	response, err := server.dispatch(context.Background(), request)
	if err != nil || response.Error == nil || response.Error.Code != rpcapi.StatusCodePermissionDenied {
		t.Fatalf("unregistered dispatch = %#v, %v", response, err)
	}
	if err := profiles.BindOwnerProfile(t.Context(), keyPair.Public.String(), "profile-api-key"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		err  error
		code rpcapi.StatusCode
	}{
		{name: "non-client", err: errAPIKeyOwnerUnavailable, code: rpcapi.StatusCodePermissionDenied},
		{name: "pending deletion", err: peer.ErrPeerPendingDeletion, code: rpcapi.StatusCodeFailedPrecondition},
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
	server.apiKeys = apikey.NewServer(&failingGetStore{Store: kv.NewMemory(nil), err: errors.New("sensitive store detail")})
	response, err = server.dispatch(context.Background(), request)
	if err != nil || response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal || strings.Contains(response.Error.Message, "sensitive") {
		t.Fatalf("store failure dispatch = %#v, %v", response, err)
	}
	server.apiKeys = apikey.NewServer(kv.NewMemory(nil))
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
	if created.Value == nil || created.Value.ManageAPIKeys || !strings.HasPrefix(created.APIKey, "gizclaw_sk_v1_") || created.Value.APIKey != created.APIKey {
		t.Fatal("created response returned invalid metadata or secret format")
	}
	if _, err := server.apiKeys.Create(t.Context(), keyPair.Public.String(), "tablet", false); err != nil {
		t.Fatal(err)
	}

	listRequest := newRPCRequest("api-key-list", rpcapi.RPCMethodServerAPIKeyList, mustRPCParams(
		rpcapi.APIKeyListRequest{Limit: 1}, (*rpcapi.RPCPayload).FromAPIKeyListRequest,
	))
	listResponse, err := server.dispatch(t.Context(), listRequest)
	if err != nil || listResponse.Error != nil || listResponse.Result == nil {
		t.Fatalf("list dispatch = %#v, %v", listResponse, err)
	}
	page, err := listResponse.Result.AsAPIKeyListResponse()
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("list result = %#v, %v", page, err)
	}
	listRequest = newRPCRequest("api-key-list-next", rpcapi.RPCMethodServerAPIKeyList, mustRPCParams(
		rpcapi.APIKeyListRequest{Cursor: page.NextCursor, Limit: 1}, (*rpcapi.RPCPayload).FromAPIKeyListRequest,
	))
	listResponse, err = server.dispatch(t.Context(), listRequest)
	if err != nil || listResponse.Error != nil || listResponse.Result == nil {
		t.Fatalf("next list dispatch = %#v, %v", listResponse, err)
	}
	nextPage, err := listResponse.Result.AsAPIKeyListResponse()
	if err != nil || len(nextPage.Items) != 1 || nextPage.NextCursor != "" {
		t.Fatalf("next list result = %#v, %v", nextPage, err)
	}
	for _, item := range append(page.Items, nextPage.Items...) {
		if !strings.HasPrefix(item.APIKey, "gizclaw_sk_v1_") {
			t.Fatalf("list did not return complete API key: %#v", item)
		}
	}
	invalidList := newRPCRequest("api-key-list-invalid", rpcapi.RPCMethodServerAPIKeyList, mustRPCParams(
		rpcapi.APIKeyListRequest{Cursor: "not-a-key"}, (*rpcapi.RPCPayload).FromAPIKeyListRequest,
	))
	invalidResponse, err := server.dispatch(t.Context(), invalidList)
	if err != nil || invalidResponse.Error == nil || invalidResponse.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("invalid list dispatch = %#v, %v", invalidResponse, err)
	}
	foreignPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := server.apiKeys.Create(t.Context(), foreignPair.Public.String(), "foreign", true)
	if err != nil {
		t.Fatal(err)
	}
	foreignRevoke := newRPCRequest("api-key-revoke-foreign", rpcapi.RPCMethodServerAPIKeyRevoke, mustRPCParams(
		rpcapi.APIKeyRevokeRequest{Name: foreign.Key.Name}, (*rpcapi.RPCPayload).FromAPIKeyRevokeRequest,
	))
	foreignResponse, err := server.dispatch(t.Context(), foreignRevoke)
	if err != nil || foreignResponse.Error == nil || foreignResponse.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("foreign revoke dispatch = %#v, %v", foreignResponse, err)
	}

	revokeRequest := newRPCRequest("api-key-revoke", rpcapi.RPCMethodServerAPIKeyRevoke, mustRPCParams(
		rpcapi.APIKeyRevokeRequest{Name: created.Value.Name}, (*rpcapi.RPCPayload).FromAPIKeyRevokeRequest,
	))
	revokeResponse, err := server.dispatch(t.Context(), revokeRequest)
	if err != nil || revokeResponse.Error != nil || revokeResponse.Result == nil {
		t.Fatalf("revoke dispatch = %#v, %v", revokeResponse, err)
	}
	if _, err := server.apiKeys.Authenticate(t.Context(), created.APIKey); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("Authenticate(revoked) error = %v", err)
	}
}
