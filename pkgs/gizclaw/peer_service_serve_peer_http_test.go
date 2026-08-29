package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPeerHTTPAPIKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	ownerKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := kv.NewMemory(nil)
	keys := apikey.NewServer(store)
	managerKey, err := keys.Create(ctx, ownerKey.Public.String(), "manager", true)
	if err != nil {
		t.Fatal(err)
	}
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	manager := NewManager(peers)
	if _, err := peers.SavePeer(ctx, apitypes.Peer{
		PublicKey: ownerKey.Public.String(), Role: apitypes.PeerRoleClient,
		Status: apitypes.PeerRegistrationStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	profiles, _ := registrationServerAndToken(t, "profile-peer-http-key")
	if err := profiles.BindOwnerProfile(ctx, ownerKey.Public.String(), "profile-peer-http-key"); err != nil {
		t.Fatal(err)
	}
	manager.RuntimeProfiles = profiles
	service := &PeerService{
		apiKeys: keys,
		manager: manager,
		public:  &peerHTTP{PeerHTTPService: peers, APIKeys: keys},
	}
	handler := service.publicHTTPHandler(keys)

	body := bytes.NewBufferString(`{"display_name":"phone","manage_api_keys":false}`)
	request := httptest.NewRequest(http.MethodPost, "/gizclaw/v1/api-keys", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+managerKey.Secret)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST status = %d body=%s", response.Code, response.Body.String())
	}
	var created peerhttp.APIKeyCreateResult
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ApiKey == "" || created.Value.DisplayName != "phone" || created.Value.ApiKey != created.ApiKey {
		t.Fatal("created response returned invalid metadata or secret")
	}

	request = httptest.NewRequest(http.MethodGet, "/gizclaw/v1/api-keys/self", nil)
	request.Header.Set("Authorization", "Bearer "+created.ApiKey)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET self status = %d body=%s", response.Code, response.Body.String())
	}
	var self peerhttp.APIKey
	if err := json.NewDecoder(response.Body).Decode(&self); err != nil {
		t.Fatal(err)
	}
	if self.ApiKey != created.ApiKey {
		t.Fatalf("GET self api_key = %q, want complete key", self.ApiKey)
	}

	request = httptest.NewRequest(http.MethodGet, "/gizclaw/v1/api-keys", nil)
	request.Header.Set("Authorization", "Bearer "+created.ApiKey)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("GET list with ordinary key status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateAPIKeyOwnerRequiresActiveClientAndBinding(t *testing.T) {
	ctx := t.Context()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	profiles, _ := registrationServerAndToken(t, "profile-api-key-owner")
	manager := NewManager(peers)
	manager.RuntimeProfiles = profiles
	service := &PeerService{manager: manager}

	if _, err := peers.SavePeer(ctx, apitypes.Peer{
		PublicKey: keyPair.Public.String(), Role: apitypes.PeerRoleServer,
		Status: apitypes.PeerRegistrationStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.validateAPIKeyOwner(ctx, keyPair.Public); !errors.Is(err, errAPIKeyOwnerUnavailable) {
		t.Fatalf("server-role validation error = %v", err)
	}
	if _, err := peers.SavePeer(ctx, apitypes.Peer{
		PublicKey: keyPair.Public.String(), Role: apitypes.PeerRoleClient,
		Status: apitypes.PeerRegistrationStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.validateAPIKeyOwner(ctx, keyPair.Public); !errors.Is(err, errAPIKeyOwnerUnavailable) {
		t.Fatalf("unbound validation error = %v", err)
	}
	if err := profiles.BindOwnerProfile(ctx, keyPair.Public.String(), "profile-api-key-owner"); err != nil {
		t.Fatal(err)
	}
	if err := service.validateAPIKeyOwner(ctx, keyPair.Public); err != nil {
		t.Fatalf("active bound Client validation error = %v", err)
	}
}

func TestPeerHTTPRejectsLegacyRoutesAndSupportsCORS(t *testing.T) {
	service := &PeerService{public: &peerHTTP{PeerHTTPService: &peer.Server{}, APIKeys: apikey.NewServer(kv.NewMemory(nil))}}
	handler := service.publicHTTPHandler(service.public.APIKeys)

	for _, path := range []string{"/login", "/me", "/side-control/sessions"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodOptions, "/gizclaw/v1/api-keys", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-request-id")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") || strings.Contains(got, "X-Public-Key") {
		t.Fatalf("Access-Control-Allow-Headers = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); got != "GET,POST,DELETE,OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods = %q", got)
	}
	if got := response.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q", got)
	}
}

func TestPeerHTTPCORSAppendsOriginToVary(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		ctx.Set(fiber.HeaderVary, fiber.HeaderAcceptEncoding)
		setPeerHTTPCORSHeaders(ctx)
		return ctx.SendStatus(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/server-info", nil)
	request.Header.Set("Origin", "https://app.example.com")
	response := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(response, request)
	if got := response.Header().Get("Vary"); got != "Accept-Encoding, Origin" {
		t.Fatalf("Vary = %q", got)
	}
}
