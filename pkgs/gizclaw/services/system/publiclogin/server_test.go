package publiclogin

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestLoginAssertionAndSession(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}

	now := time.Now().Add(time.Minute).Truncate(time.Second)
	assertion, err := newLoginAssertionAt(deviceKey, serverKey.Public, now, time.Minute, strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatalf("newLoginAssertionAt error = %v", err)
	}
	manager := NewSessionManager(kv.NewMemory(nil))
	manager.now = func() time.Time { return now.Add(10 * time.Second) }

	login, err := manager.login(context.Background(), serverKey, deviceKey.Public, assertion)
	if err != nil {
		t.Fatalf("login error = %v", err)
	}
	if login.TokenType != "Bearer" || login.AccessToken == "" || login.ExpiresAt == 0 {
		t.Fatalf("login response = %+v", login)
	}
	publicKey, err := manager.Authenticate("Bearer " + login.AccessToken)
	if err != nil {
		t.Fatalf("Authenticate error = %v", err)
	}
	if publicKey != deviceKey.Public {
		t.Fatalf("Authenticate public key = %s, want %s", publicKey, deviceKey.Public)
	}

	reloaded := NewSessionManager(manager.Store)
	reloaded.now = manager.now
	publicKey, err = reloaded.Authenticate("Bearer " + login.AccessToken)
	if err != nil {
		t.Fatalf("reloaded Authenticate error = %v", err)
	}
	if publicKey != deviceKey.Public {
		t.Fatalf("reloaded Authenticate public key = %s, want %s", publicKey, deviceKey.Public)
	}

	if _, err := reloaded.login(context.Background(), serverKey, deviceKey.Public, assertion); err == nil {
		t.Fatal("replayed assertion should fail")
	}
}

func TestLoginAssertionRejectsWrongAudience(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	otherServerKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(other server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}

	assertion, err := NewLoginAssertion(deviceKey, otherServerKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	if _, err := NewSessionManager(kv.NewMemory(nil)).login(context.Background(), serverKey, deviceKey.Public, assertion); err == nil {
		t.Fatal("wrong audience assertion should fail")
	}
}

func TestLoginAssertionRejectsExpiredAndMalformedTokens(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}

	assertion, err := newLoginAssertionAt(deviceKey, serverKey.Public, time.Unix(1_700_000_000, 0), time.Minute, strings.NewReader(strings.Repeat("b", 16)))
	if err != nil {
		t.Fatalf("newLoginAssertionAt error = %v", err)
	}
	manager := NewSessionManager(kv.NewMemory(nil))
	manager.now = func() time.Time { return time.Unix(1_700_000_120, 0) }
	if _, err := manager.login(context.Background(), serverKey, deviceKey.Public, assertion); err == nil {
		t.Fatal("expired assertion should fail")
	}
	if _, err := manager.login(context.Background(), serverKey, deviceKey.Public, "not-a-token"); err == nil {
		t.Fatal("malformed assertion should fail")
	}
}

func TestSessionAuthenticateRejectsExpiredAndMissingBearer(t *testing.T) {
	manager := NewSessionManager(kv.NewMemory(nil))
	expiresAt := time.Now().Add(20 * time.Millisecond)
	body := []byte(fmt.Sprintf(`{"public_key":"server","expires_at":%d}`, expiresAt.UnixMilli()))
	if err := manager.Store.BatchSet(context.Background(), []kv.Entry{
		{Key: sessionKey("expired"), Value: body, Deadline: expiresAt},
	}); err != nil {
		t.Fatalf("seed expired session: %v", err)
	}

	if _, err := manager.Authenticate("expired"); err == nil {
		t.Fatal("missing bearer prefix should fail")
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := manager.Authenticate("Bearer expired"); err == nil {
		t.Fatal("expired session should fail")
	}
}

func TestServerLoginHandler(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}
	assertion, err := NewLoginAssertion(deviceKey, serverKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}

	server := NewServer(serverKey, kv.NewMemory(nil))
	manager := server.SessionManager()
	if manager == nil || manager.Store != server.Store {
		t.Fatalf("SessionManager = %+v, want store-backed manager", manager)
	}
	server.Store = kv.NewMemory(nil)
	if refreshed := server.SessionManager(); refreshed == manager || refreshed.Store != server.Store {
		t.Fatalf("SessionManager did not refresh after store change")
	}

	resp, err := server.Login(context.Background(), peerhttp.LoginRequestObject{
		Params: peerhttp.LoginParams{
			XPublicKey:    deviceKey.Public.String(),
			Authorization: "Bearer " + assertion,
		},
	})
	if err != nil {
		t.Fatalf("Login error = %v", err)
	}
	ok, isOK := resp.(peerhttp.Login200JSONResponse)
	if !isOK {
		t.Fatalf("Login response type = %T", resp)
	}
	if ok.AccessToken == "" || ok.TokenType != peerhttp.Bearer || ok.ExpiresAt == 0 {
		t.Fatalf("Login response = %+v", ok)
	}
}

func TestServerLoginHandlerRejectsInvalidRequests(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}
	otherKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(other) error = %v", err)
	}
	assertion, err := NewLoginAssertion(deviceKey, serverKey.Public, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}

	tests := []struct {
		name    string
		server  *Server
		params  peerhttp.LoginParams
		wantErr string
	}{
		{
			name:    "unsupported",
			server:  &Server{},
			wantErr: "UNSUPPORTED_LOGIN",
		},
		{
			name:   "invalid public key",
			server: NewServer(serverKey, kv.NewMemory(nil)),
			params: peerhttp.LoginParams{
				XPublicKey:    "not-a-key",
				Authorization: "Bearer " + assertion,
			},
			wantErr: "INVALID_PUBLIC_KEY",
		},
		{
			name:   "missing assertion",
			server: NewServer(serverKey, kv.NewMemory(nil)),
			params: peerhttp.LoginParams{
				XPublicKey:    deviceKey.Public.String(),
				Authorization: assertion,
			},
			wantErr: "MISSING_ASSERTION",
		},
		{
			name:   "issuer mismatch",
			server: NewServer(serverKey, kv.NewMemory(nil)),
			params: peerhttp.LoginParams{
				XPublicKey:    otherKey.Public.String(),
				Authorization: "Bearer " + assertion,
			},
			wantErr: "INVALID_ASSERTION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.server.Login(context.Background(), peerhttp.LoginRequestObject{Params: tt.params})
			if err != nil {
				t.Fatalf("Login error = %v", err)
			}
			unauthorized, ok := resp.(peerhttp.Login401JSONResponse)
			if !ok {
				t.Fatalf("Login response type = %T", resp)
			}
			if unauthorized.Error.Code != tt.wantErr {
				t.Fatalf("Login error code = %q, want %q", unauthorized.Error.Code, tt.wantErr)
			}
		})
	}
}

func TestLoginAssertionConstructionErrors(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}

	if _, err := newLoginAssertionAt(nil, serverKey.Public, time.Now(), time.Minute, strings.NewReader("seed")); err == nil {
		t.Fatal("nil key pair should fail")
	}
	if _, err := newLoginAssertionAt(deviceKey, serverKey.Public, time.Now(), time.Minute, errReader{}); err == nil {
		t.Fatal("random reader error should fail")
	}
	assertion, err := newLoginAssertionAt(deviceKey, serverKey.Public, time.Now(), 0, strings.NewReader(strings.Repeat("z", 16)))
	if err != nil {
		t.Fatalf("zero ttl should default: %v", err)
	}
	if assertion == "" {
		t.Fatal("zero ttl assertion is empty")
	}
}

func TestAuthenticateRejectsInvalidStoredSession(t *testing.T) {
	manager := NewSessionManager(kv.NewMemory(nil))
	if err := manager.Store.Set(context.Background(), sessionKey("bad-json"), []byte("{")); err != nil {
		t.Fatalf("seed bad json: %v", err)
	}
	if _, err := manager.Authenticate("Bearer bad-json"); err == nil {
		t.Fatal("bad session JSON should fail")
	}

	expiresAt := time.Now().Add(time.Minute)
	body := []byte(fmt.Sprintf(`{"public_key":"","expires_at":%d}`, expiresAt.UnixMilli()))
	if err := manager.Store.Set(context.Background(), sessionKey("empty-public-key"), body); err != nil {
		t.Fatalf("seed empty public key: %v", err)
	}
	if _, err := manager.Authenticate("Bearer empty-public-key"); err == nil {
		t.Fatal("empty public key session should fail")
	}
	if _, err := (*SessionManager)(nil).Authenticate("Bearer token"); err == nil {
		t.Fatal("nil manager should fail")
	}
}

func TestVerifyLoginAssertionRejectsBoundaryClaims(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}
	now := time.Now().Truncate(time.Second)
	shared, err := deviceKey.DH(serverKey.Public)
	if err != nil {
		t.Fatalf("DH error = %v", err)
	}
	header := loginAssertionHeader{Alg: loginAssertionAlg, Typ: loginAssertionTyp}

	cases := []struct {
		name   string
		header loginAssertionHeader
		claims LoginAssertionClaims
	}{
		{
			name:   "unsupported algorithm",
			header: loginAssertionHeader{Alg: "none", Typ: loginAssertionTyp},
			claims: LoginAssertionClaims{Iss: deviceKey.Public.String(), Aud: serverKey.Public.String(), Iat: now.Unix(), Exp: now.Add(time.Minute).Unix(), Nonce: "n"},
		},
		{
			name:   "empty nonce",
			header: header,
			claims: LoginAssertionClaims{Iss: deviceKey.Public.String(), Aud: serverKey.Public.String(), Iat: now.Unix(), Exp: now.Add(time.Minute).Unix()},
		},
		{
			name:   "future issued at",
			header: header,
			claims: LoginAssertionClaims{Iss: deviceKey.Public.String(), Aud: serverKey.Public.String(), Iat: now.Add(2 * time.Minute).Unix(), Exp: now.Add(3 * time.Minute).Unix(), Nonce: "n"},
		},
		{
			name:   "too long ttl",
			header: header,
			claims: LoginAssertionClaims{Iss: deviceKey.Public.String(), Aud: serverKey.Public.String(), Iat: now.Unix(), Exp: now.Add(maxLoginAssertionTTL + time.Second).Unix(), Nonce: "n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertion, err := encodeLoginAssertion(tc.header, tc.claims, shared[:])
			if err != nil {
				t.Fatalf("encodeLoginAssertion error = %v", err)
			}
			if _, err := verifyLoginAssertion(serverKey, deviceKey.Public, assertion, now); err == nil {
				t.Fatal("verifyLoginAssertion should fail")
			}
		})
	}
}

func TestParseLoginAssertionRejectsMalformedParts(t *testing.T) {
	tokens := []string{
		"too.few",
		"bad!base64.claims.sig",
		base64Segment("{}") + ".bad!base64.sig",
		base64Segment("{}") + "." + base64Segment("{}") + ".bad!base64",
		base64Segment("{") + "." + base64Segment("{}") + "." + base64Segment("sig"),
		base64Segment("{}") + "." + base64Segment("{") + "." + base64Segment("sig"),
	}
	for _, token := range tokens {
		if _, _, _, _, err := parseLoginAssertion(token); err == nil {
			t.Fatalf("parseLoginAssertion(%q) should fail", token)
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func base64Segment(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
