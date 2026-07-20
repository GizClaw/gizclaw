package publiclogin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	loginAssertionAlg = "X25519-HS256"
	loginAssertionTyp = "JWT"

	defaultSessionTTL       = 24 * time.Hour
	deviceTokenTTL          = 5 * time.Minute
	maxLoginAssertionTTL    = 5 * time.Minute
	loginAssertionClockSkew = time.Minute

	PublicKeyHeader         = "X-Public-Key"
	RegistrationTokenHeader = "X-Registration-Token"
)

var (
	errInvalidLoginAssertion = errors.New("publiclogin: invalid login assertion")
	errInvalidSession        = errors.New("publiclogin: invalid session")
	ErrPublicKeyMismatch     = errors.New("publiclogin: public key mismatch")
)

type loginAssertionHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type LoginAssertionClaims struct {
	Iss   string `json:"iss"`
	Aud   string `json:"aud"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	Nonce string `json:"nonce"`
}

type PeerHTTP interface {
	Login(context.Context, peerhttp.LoginRequestObject) (peerhttp.LoginResponseObject, error)
	CreateSideControlDeviceToken(context.Context, peerhttp.CreateSideControlDeviceTokenRequestObject) (peerhttp.CreateSideControlDeviceTokenResponseObject, error)
	RevokeSideControlDeviceToken(context.Context, peerhttp.RevokeSideControlDeviceTokenRequestObject) (peerhttp.RevokeSideControlDeviceTokenResponseObject, error)
	ListSideControlSessions(context.Context, peerhttp.ListSideControlSessionsRequestObject) (peerhttp.ListSideControlSessionsResponseObject, error)
	RevokeSideControlSession(context.Context, peerhttp.RevokeSideControlSessionRequestObject) (peerhttp.RevokeSideControlSessionResponseObject, error)
}

type SessionAuthorizer func(context.Context, giznet.PublicKey) error

type RegistrationResolver func(context.Context, string) (runtimeprofile.Registration, error)

type Server struct {
	KeyPair *giznet.KeyPair
	Store   kv.Store

	SessionAuthorizer    SessionAuthorizer
	RegistrationResolver RegistrationResolver

	mu       sync.Mutex
	sessions *SessionManager
}

var _ PeerHTTP = (*Server)(nil)

func NewServer(keyPair *giznet.KeyPair, store kv.Store) *Server {
	return &Server{
		KeyPair:  keyPair,
		Store:    store,
		sessions: NewSessionManager(store),
	}
}

func (s *Server) SessionManager() *SessionManager {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil || s.sessions.Store != s.Store {
		s.sessions = NewSessionManager(s.Store)
	}
	return s.sessions
}

func (s *Server) Login(ctx context.Context, request peerhttp.LoginRequestObject) (peerhttp.LoginResponseObject, error) {
	return s.login(ctx, request, s.SessionAuthorizer)
}

func (s *Server) LoginWithoutAuthorizer(ctx context.Context, request peerhttp.LoginRequestObject) (peerhttp.LoginResponseObject, error) {
	return s.login(ctx, request, nil)
}

func (s *Server) login(ctx context.Context, request peerhttp.LoginRequestObject, authorizer SessionAuthorizer) (peerhttp.LoginResponseObject, error) {
	if s == nil || s.KeyPair == nil || s.Store == nil {
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("UNSUPPORTED_LOGIN", "login is not configured")), nil
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(request.Params.XPublicKey)); err != nil {
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("INVALID_PUBLIC_KEY", err.Error())), nil
	}
	assertion := bearerToken(request.Params.Authorization)
	if assertion == "" {
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("MISSING_ASSERTION", "missing bearer assertion")), nil
	}
	var registration *runtimeprofile.Registration
	var result peerhttp.LoginResult
	var err error
	switch {
	case request.Body == nil || isBodylessLogin(ctx):
		if request.Params.XRegistrationToken != nil {
			rawToken := strings.TrimSpace(*request.Params.XRegistrationToken)
			if rawToken != "" {
				if s.RegistrationResolver == nil {
					return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("UNSUPPORTED_REGISTRATION", "registration is not configured")), nil
				}
				resolved, resolveErr := s.RegistrationResolver(ctx, rawToken)
				if resolveErr != nil {
					slog.WarnContext(ctx, "public HTTP registration rejected", "peer_public_key", publicKey.String(), "error", resolveErr)
					return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("INVALID_REGISTRATION_TOKEN", "registration token is invalid")), nil
				}
				registration = &resolved
			}
		}
		result, err = s.SessionManager().loginWithRegistration(ctx, s.KeyPair, publicKey, assertion, authorizer, registration)
	case request.Body.GrantType == peerhttp.SideControl && strings.TrimSpace(request.Body.DeviceToken) != "" && request.Params.XRegistrationToken == nil:
		result, err = s.SessionManager().loginSideControl(ctx, s.KeyPair, publicKey, assertion, request.Body.DeviceToken)
	default:
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("INVALID_GRANT", "unsupported login grant")), nil
	}
	if err != nil {
		return peerhttp.Login401JSONResponse(apitypes.NewErrorResponse("INVALID_ASSERTION", err.Error())), nil
	}
	if registration != nil {
		slog.InfoContext(ctx, "public HTTP registration accepted", "peer_public_key", publicKey.String(), "registration_token", registration.TokenName, "runtime_profile", registration.RuntimeProfile.Name)
	}
	return peerhttp.Login200JSONResponse(result), nil
}

type SessionManager struct {
	Store kv.Store

	mu  sync.Mutex
	now func() time.Time
}

type session struct {
	Kind            SessionKind                  `json:"kind,omitempty"`
	PublicKey       string                       `json:"public_key"`
	TargetPublicKey string                       `json:"target_public_key,omitempty"`
	SessionID       string                       `json:"session_id,omitempty"`
	IssuedAt        int64                        `json:"issued_at,omitempty"`
	ExpiresAt       int64                        `json:"expires_at"`
	Registration    *runtimeprofile.Registration `json:"registration,omitempty"`
}

type AuthenticatedSession struct {
	Principal
	Registration *runtimeprofile.Registration
}

func NewSessionManager(store kv.Store) *SessionManager {
	return &SessionManager{
		Store: store,
		now:   time.Now,
	}
}

func NewLoginAssertion(keyPair *giznet.KeyPair, serverPublicKey giznet.PublicKey, ttl time.Duration) (string, error) {
	return newLoginAssertionAt(keyPair, serverPublicKey, time.Now(), ttl, rand.Reader)
}

func newLoginAssertionAt(keyPair *giznet.KeyPair, serverPublicKey giznet.PublicKey, now time.Time, ttl time.Duration, random io.Reader) (string, error) {
	if keyPair == nil {
		return "", errors.New("publiclogin: nil key pair")
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	nonce, err := randomToken(random, 16)
	if err != nil {
		return "", err
	}
	claims := LoginAssertionClaims{
		Iss:   keyPair.Public.String(),
		Aud:   serverPublicKey.String(),
		Iat:   now.Unix(),
		Exp:   now.Add(ttl).Unix(),
		Nonce: nonce,
	}
	header := loginAssertionHeader{Alg: loginAssertionAlg, Typ: loginAssertionTyp}
	shared, err := keyPair.DH(serverPublicKey)
	if err != nil {
		return "", err
	}
	return encodeLoginAssertion(header, claims, shared[:])
}

func (m *SessionManager) login(ctx context.Context, serverKeyPair *giznet.KeyPair, publicKey giznet.PublicKey, assertion string, authorizer SessionAuthorizer) (peerhttp.LoginResult, error) {
	return m.loginWithRegistration(ctx, serverKeyPair, publicKey, assertion, authorizer, nil)
}

func (m *SessionManager) loginWithRegistration(ctx context.Context, serverKeyPair *giznet.KeyPair, publicKey giznet.PublicKey, assertion string, authorizer SessionAuthorizer, registration *runtimeprofile.Registration) (peerhttp.LoginResult, error) {
	if m == nil || m.Store == nil {
		return peerhttp.LoginResult{}, errInvalidSession
	}
	if serverKeyPair == nil {
		return peerhttp.LoginResult{}, errors.New("publiclogin: nil server key pair")
	}
	now := m.nowOrDefault()
	claims, err := verifyLoginAssertion(serverKeyPair, publicKey, assertion, now)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	if authorizer != nil {
		if err := authorizer(ctx, publicKey); err != nil {
			return peerhttp.LoginResult{}, err
		}
	}
	assertionDeadline := time.Unix(claims.Exp, 0)

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.Store.Get(ctx, assertionKey(claims)); err == nil {
		return peerhttp.LoginResult{}, fmt.Errorf("%w: replayed assertion", errInvalidLoginAssertion)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return peerhttp.LoginResult{}, err
	}

	token, err := randomToken(rand.Reader, 32)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	expiresAt := now.Add(defaultSessionTTL)
	body, err := json.Marshal(session{
		Kind:         SessionKindPrimary,
		PublicKey:    publicKey.String(),
		IssuedAt:     now.UnixMilli(),
		ExpiresAt:    expiresAt.UnixMilli(),
		Registration: registration,
	})
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	if err := m.Store.BatchSet(ctx, []kv.Entry{
		{
			Key:      assertionKey(claims),
			Value:    []byte("used"),
			Deadline: assertionDeadline,
		},
		{
			Key:      sessionKey(token),
			Value:    body,
			Deadline: expiresAt,
		},
	}); err != nil {
		return peerhttp.LoginResult{}, err
	}
	return peerhttp.LoginResult{
		AccessToken: token,
		TokenType:   peerhttp.Bearer,
		ExpiresAt:   expiresAt.UnixMilli(),
	}, nil
}

func (m *SessionManager) Authenticate(header string) (giznet.PublicKey, error) {
	authenticated, err := m.AuthenticateSession(header)
	return authenticated.PublicKey, err
}

func (m *SessionManager) AuthenticateSession(header string) (AuthenticatedSession, error) {
	token := bearerToken(header)
	if token == "" {
		return AuthenticatedSession{}, errInvalidSession
	}
	if m == nil || m.Store == nil {
		return AuthenticatedSession{}, errInvalidSession
	}
	now := m.nowOrDefault()

	data, err := m.Store.Get(context.Background(), sessionKey(token))
	if errors.Is(err, kv.ErrNotFound) {
		return AuthenticatedSession{}, errInvalidSession
	}
	if err != nil {
		return AuthenticatedSession{}, err
	}
	var sess session
	if err := json.Unmarshal(data, &sess); err != nil {
		return AuthenticatedSession{}, errInvalidSession
	}
	expiresAt := time.UnixMilli(sess.ExpiresAt)
	if sess.PublicKey == "" || !expiresAt.After(now) {
		_ = m.Store.Delete(context.Background(), sessionKey(token))
		return AuthenticatedSession{}, errInvalidSession
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(sess.PublicKey)); err != nil {
		return AuthenticatedSession{}, errInvalidSession
	}
	kind := sess.Kind
	if kind == "" {
		kind = SessionKindPrimary
	}
	principal := Principal{
		Kind:      kind,
		PublicKey: publicKey,
		SessionID: sess.SessionID,
		IssuedAt:  time.UnixMilli(sess.IssuedAt),
		ExpiresAt: expiresAt,
	}
	if sess.TargetPublicKey != "" {
		if err := principal.TargetPublicKey.UnmarshalText([]byte(sess.TargetPublicKey)); err != nil {
			return AuthenticatedSession{}, errInvalidSession
		}
	}
	return AuthenticatedSession{Principal: principal, Registration: sess.Registration}, nil
}

func (m *SessionManager) AuthenticateHeaders(authorization, publicKeyHeader string) (giznet.PublicKey, error) {
	authenticated, err := m.AuthenticateHeadersSession(authorization, publicKeyHeader)
	return authenticated.PublicKey, err
}

func (m *SessionManager) AuthenticateHeadersSession(authorization, publicKeyHeader string) (AuthenticatedSession, error) {
	authenticated, err := m.AuthenticateSession(authorization)
	if err != nil {
		return AuthenticatedSession{}, err
	}
	if publicKeyHeader != "" && publicKeyHeader != authenticated.PublicKey.String() {
		return AuthenticatedSession{}, ErrPublicKeyMismatch
	}
	return authenticated, nil
}

// AuthenticatePrincipal resolves a bearer token to its typed session principal.
func (m *SessionManager) AuthenticatePrincipal(header string) (Principal, error) {
	authenticated, err := m.AuthenticateSession(header)
	if err != nil {
		return Principal{}, err
	}
	return authenticated.Principal, nil
}

// AuthenticateHeadersPrincipal authenticates a bearer and verifies its optional public-key header.
func (m *SessionManager) AuthenticateHeadersPrincipal(authorization, publicKeyHeader string) (Principal, error) {
	authenticated, err := m.AuthenticateHeadersSession(authorization, publicKeyHeader)
	if err != nil {
		return Principal{}, err
	}
	return authenticated.Principal, nil
}

func (m *SessionManager) nowOrDefault() time.Time {
	if m != nil && m.now != nil {
		return m.now()
	}
	return time.Now()
}

func sessionKey(token string) kv.Key {
	return kv.Key{"sessions", token}
}

func assertionKey(claims LoginAssertionClaims) kv.Key {
	return kv.Key{"assertions", claims.Iss, claims.Nonce}
}

func verifyLoginAssertion(serverKeyPair *giznet.KeyPair, publicKey giznet.PublicKey, token string, now time.Time) (LoginAssertionClaims, error) {
	header, claims, signed, signature, err := parseLoginAssertion(token)
	if err != nil {
		return LoginAssertionClaims{}, err
	}
	if header.Alg != loginAssertionAlg || header.Typ != loginAssertionTyp {
		return LoginAssertionClaims{}, fmt.Errorf("%w: unsupported algorithm", errInvalidLoginAssertion)
	}
	if claims.Iss != publicKey.String() {
		return LoginAssertionClaims{}, fmt.Errorf("%w: issuer mismatch", errInvalidLoginAssertion)
	}
	if claims.Aud != serverKeyPair.Public.String() {
		return LoginAssertionClaims{}, fmt.Errorf("%w: audience mismatch", errInvalidLoginAssertion)
	}
	if claims.Nonce == "" {
		return LoginAssertionClaims{}, fmt.Errorf("%w: empty nonce", errInvalidLoginAssertion)
	}
	issuedAt := time.Unix(claims.Iat, 0)
	expiresAt := time.Unix(claims.Exp, 0)
	if issuedAt.After(now.Add(loginAssertionClockSkew)) || !expiresAt.After(now) || expiresAt.Sub(issuedAt) > maxLoginAssertionTTL {
		return LoginAssertionClaims{}, fmt.Errorf("%w: expired assertion", errInvalidLoginAssertion)
	}
	shared, err := serverKeyPair.DH(publicKey)
	if err != nil {
		return LoginAssertionClaims{}, err
	}
	expected := loginAssertionMAC(shared[:], signed)
	if !hmac.Equal(signature, expected) {
		return LoginAssertionClaims{}, fmt.Errorf("%w: bad mac", errInvalidLoginAssertion)
	}
	return claims, nil
}

func encodeLoginAssertion(header loginAssertionHeader, claims LoginAssertionClaims, secret []byte) (string, error) {
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := loginAssertionMAC(secret, signed)
	return signed + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseLoginAssertion(token string) (loginAssertionHeader, LoginAssertionClaims, string, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return loginAssertionHeader{}, LoginAssertionClaims{}, "", nil, errInvalidLoginAssertion
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return loginAssertionHeader{}, LoginAssertionClaims{}, "", nil, errInvalidLoginAssertion
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return loginAssertionHeader{}, LoginAssertionClaims{}, "", nil, errInvalidLoginAssertion
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return loginAssertionHeader{}, LoginAssertionClaims{}, "", nil, errInvalidLoginAssertion
	}
	var header loginAssertionHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return loginAssertionHeader{}, LoginAssertionClaims{}, "", nil, errInvalidLoginAssertion
	}
	var claims LoginAssertionClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return loginAssertionHeader{}, LoginAssertionClaims{}, "", nil, errInvalidLoginAssertion
	}
	return header, claims, parts[0] + "." + parts[1], signature, nil
}

func loginAssertionMAC(secret []byte, signed string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(signed))
	return mac.Sum(nil)
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func randomToken(random io.Reader, size int) (string, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(random, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
