package publiclogin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// SessionKind identifies the authorization scope carried by a bearer session.
type SessionKind string

const (
	// SessionKindPrimary represents the device's authoritative identity.
	SessionKindPrimary SessionKind = "primary"
	// SessionKindSideControl represents a controller authorized for one target.
	SessionKindSideControl SessionKind = "side_control"
)

var (
	errDeviceTokenNotFound        = errors.New("publiclogin: side-control device token not found")
	errSideControlSessionNotFound = errors.New("publiclogin: side-control session not found")
	errWrongSessionKind           = errors.New("publiclogin: session is not authorized for this route")
)

// Principal is the typed identity and optional target bound to a bearer session.
type Principal struct {
	Kind            SessionKind
	PublicKey       giznet.PublicKey
	TargetPublicKey giznet.PublicKey
	SessionID       string
	IssuedAt        time.Time
	ExpiresAt       time.Time
}

type principalContextKey struct{}
type bodylessLoginContextKey struct{}

// WithPrincipal attaches an authenticated typed principal to a request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated typed principal, if present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// WithBodylessLogin marks the legacy bodyless primary login transport form.
func WithBodylessLogin(ctx context.Context) context.Context {
	return context.WithValue(ctx, bodylessLoginContextKey{}, true)
}

func isBodylessLogin(ctx context.Context) bool {
	value, _ := ctx.Value(bodylessLoginContextKey{}).(bool)
	return value
}

type deviceTokenRecord struct {
	ID              string `json:"id"`
	TargetPublicKey string `json:"target_public_key"`
	ExpiresAt       int64  `json:"expires_at"`
	Consumed        bool   `json:"consumed"`
}

type sideSessionIndex struct {
	AccessToken string  `json:"access_token"`
	Session     session `json:"session"`
}

func (s *Server) CreateSideControlDeviceToken(ctx context.Context, _ peerhttp.CreateSideControlDeviceTokenRequestObject) (peerhttp.CreateSideControlDeviceTokenResponseObject, error) {
	target, err := primaryTarget(ctx)
	if err != nil {
		return createDeviceTokenForbidden(err), nil
	}
	result, err := s.SessionManager().CreateSideControlDeviceToken(ctx, target)
	if err != nil {
		return peerhttp.CreateSideControlDeviceToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))}, nil
	}
	return peerhttp.CreateSideControlDeviceToken201JSONResponse(result), nil
}

func (s *Server) RevokeSideControlDeviceToken(ctx context.Context, request peerhttp.RevokeSideControlDeviceTokenRequestObject) (peerhttp.RevokeSideControlDeviceTokenResponseObject, error) {
	target, err := primaryTarget(ctx)
	if err != nil {
		return peerhttp.RevokeSideControlDeviceToken403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apitypes.NewErrorResponse("PRIMARY_SESSION_REQUIRED", err.Error()))}, nil
	}
	err = s.SessionManager().RevokeSideControlDeviceToken(ctx, target, request.TokenId)
	if errors.Is(err, errDeviceTokenNotFound) {
		return peerhttp.RevokeSideControlDeviceToken404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apitypes.NewErrorResponse("DEVICE_TOKEN_NOT_FOUND", err.Error()))}, nil
	}
	if err != nil {
		return peerhttp.RevokeSideControlDeviceToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))}, nil
	}
	return peerhttp.RevokeSideControlDeviceToken204Response{}, nil
}

func (s *Server) ListSideControlSessions(ctx context.Context, _ peerhttp.ListSideControlSessionsRequestObject) (peerhttp.ListSideControlSessionsResponseObject, error) {
	target, err := primaryTarget(ctx)
	if err != nil {
		return peerhttp.ListSideControlSessions403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apitypes.NewErrorResponse("PRIMARY_SESSION_REQUIRED", err.Error()))}, nil
	}
	items, err := s.SessionManager().ListSideControlSessions(ctx, target)
	if err != nil {
		return peerhttp.ListSideControlSessions500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))}, nil
	}
	return peerhttp.ListSideControlSessions200JSONResponse{Items: items}, nil
}

func (s *Server) RevokeSideControlSession(ctx context.Context, request peerhttp.RevokeSideControlSessionRequestObject) (peerhttp.RevokeSideControlSessionResponseObject, error) {
	target, err := primaryTarget(ctx)
	if err != nil {
		return peerhttp.RevokeSideControlSession403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apitypes.NewErrorResponse("PRIMARY_SESSION_REQUIRED", err.Error()))}, nil
	}
	err = s.SessionManager().RevokeSideControlSession(ctx, target, request.SessionId)
	if errors.Is(err, errSideControlSessionNotFound) {
		return peerhttp.RevokeSideControlSession404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apitypes.NewErrorResponse("SIDE_CONTROL_SESSION_NOT_FOUND", err.Error()))}, nil
	}
	if err != nil {
		return peerhttp.RevokeSideControlSession500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))}, nil
	}
	return peerhttp.RevokeSideControlSession204Response{}, nil
}

func createDeviceTokenForbidden(err error) peerhttp.CreateSideControlDeviceToken403JSONResponse {
	return peerhttp.CreateSideControlDeviceToken403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apitypes.NewErrorResponse("PRIMARY_SESSION_REQUIRED", err.Error()))}
}

func primaryTarget(ctx context.Context) (giznet.PublicKey, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal.Kind != SessionKindPrimary || principal.PublicKey.IsZero() {
		return giznet.PublicKey{}, errWrongSessionKind
	}
	return principal.PublicKey, nil
}

// SideControlPrincipal requires and returns a target-bound side-control principal.
func SideControlPrincipal(ctx context.Context) (Principal, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal.Kind != SessionKindSideControl || principal.PublicKey.IsZero() || principal.TargetPublicKey.IsZero() {
		return Principal{}, errWrongSessionKind
	}
	return principal, nil
}

// CreateSideControlDeviceToken creates a short-lived single-use token for target.
func (m *SessionManager) CreateSideControlDeviceToken(ctx context.Context, target giznet.PublicKey) (peerhttp.SideControlDeviceToken, error) {
	if m == nil || m.Store == nil || target.IsZero() {
		return peerhttp.SideControlDeviceToken{}, errInvalidSession
	}
	id, err := randomToken(rand.Reader, 16)
	if err != nil {
		return peerhttp.SideControlDeviceToken{}, err
	}
	token, err := randomToken(rand.Reader, 32)
	if err != nil {
		return peerhttp.SideControlDeviceToken{}, err
	}
	expiresAt := m.nowOrDefault().Add(deviceTokenTTL)
	record := deviceTokenRecord{ID: id, TargetPublicKey: target.String(), ExpiresAt: expiresAt.UnixMilli()}
	body, err := json.Marshal(record)
	if err != nil {
		return peerhttp.SideControlDeviceToken{}, err
	}
	hash := deviceTokenHash(token)
	if err := m.Store.BatchSet(ctx, []kv.Entry{
		{Key: deviceTokenHashKey(hash), Value: body, Deadline: expiresAt},
		{Key: deviceTokenOwnerKey(target, id), Value: []byte(hash), Deadline: expiresAt},
	}); err != nil {
		return peerhttp.SideControlDeviceToken{}, err
	}
	return peerhttp.SideControlDeviceToken{Id: id, Token: token, ExpiresAt: expiresAt.UnixMilli()}, nil
}

// RevokeSideControlDeviceToken revokes an unconsumed token owned by target.
func (m *SessionManager) RevokeSideControlDeviceToken(ctx context.Context, target giznet.PublicKey, id string) error {
	if m == nil || m.Store == nil || target.IsZero() || strings.TrimSpace(id) == "" {
		return errDeviceTokenNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	indexKey := deviceTokenOwnerKey(target, id)
	hash, err := m.Store.Get(ctx, indexKey)
	if errors.Is(err, kv.ErrNotFound) {
		return errDeviceTokenNotFound
	}
	if err != nil {
		return err
	}
	recordKey := deviceTokenHashKey(string(hash))
	data, err := m.Store.Get(ctx, recordKey)
	if errors.Is(err, kv.ErrNotFound) {
		_ = m.Store.Delete(ctx, indexKey)
		return errDeviceTokenNotFound
	}
	if err != nil {
		return err
	}
	var record deviceTokenRecord
	if json.Unmarshal(data, &record) != nil || record.TargetPublicKey != target.String() || record.ID != id || record.Consumed || !time.UnixMilli(record.ExpiresAt).After(m.nowOrDefault()) {
		return errDeviceTokenNotFound
	}
	return m.Store.BatchDelete(ctx, []kv.Key{recordKey, indexKey})
}

func (m *SessionManager) loginSideControl(ctx context.Context, serverKeyPair *giznet.KeyPair, controller giznet.PublicKey, assertion, deviceToken string) (peerhttp.LoginResult, error) {
	if m == nil || m.Store == nil || serverKeyPair == nil {
		return peerhttp.LoginResult{}, errInvalidSession
	}
	now := m.nowOrDefault()
	claims, err := verifyLoginAssertion(serverKeyPair, controller, assertion, now)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.Store.Get(ctx, assertionKey(claims)); err == nil {
		return peerhttp.LoginResult{}, fmt.Errorf("%w: replayed assertion", errInvalidLoginAssertion)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return peerhttp.LoginResult{}, err
	}
	recordKey := deviceTokenHashKey(deviceTokenHash(deviceToken))
	data, err := m.Store.Get(ctx, recordKey)
	if errors.Is(err, kv.ErrNotFound) {
		return peerhttp.LoginResult{}, errDeviceTokenNotFound
	}
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	var record deviceTokenRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return peerhttp.LoginResult{}, errDeviceTokenNotFound
	}
	if record.Consumed || !time.UnixMilli(record.ExpiresAt).After(now) {
		return peerhttp.LoginResult{}, errDeviceTokenNotFound
	}
	var target giznet.PublicKey
	if err := target.UnmarshalText([]byte(record.TargetPublicKey)); err != nil || target.IsZero() {
		return peerhttp.LoginResult{}, errDeviceTokenNotFound
	}
	accessToken, err := randomToken(rand.Reader, 32)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	sessionID, err := randomToken(rand.Reader, 16)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	expiresAt := now.Add(defaultSessionTTL)
	sess := session{
		Kind:            SessionKindSideControl,
		PublicKey:       controller.String(),
		TargetPublicKey: target.String(),
		SessionID:       sessionID,
		IssuedAt:        now.UnixMilli(),
		ExpiresAt:       expiresAt.UnixMilli(),
	}
	sessionBody, err := json.Marshal(sess)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	indexBody, err := json.Marshal(sideSessionIndex{AccessToken: accessToken, Session: sess})
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	record.Consumed = true
	recordBody, err := json.Marshal(record)
	if err != nil {
		return peerhttp.LoginResult{}, err
	}
	if err := m.Store.BatchSet(ctx, []kv.Entry{
		{Key: assertionKey(claims), Value: []byte("used"), Deadline: time.Unix(claims.Exp, 0)},
		{Key: recordKey, Value: recordBody, Deadline: time.UnixMilli(record.ExpiresAt)},
		{Key: sessionKey(accessToken), Value: sessionBody, Deadline: expiresAt},
		{Key: sideSessionKey(target, sessionID), Value: indexBody, Deadline: expiresAt},
	}); err != nil {
		return peerhttp.LoginResult{}, err
	}
	return peerhttp.LoginResult{AccessToken: accessToken, TokenType: peerhttp.Bearer, ExpiresAt: expiresAt.UnixMilli()}, nil
}

// ListSideControlSessions lists active target-bound side-control sessions.
func (m *SessionManager) ListSideControlSessions(ctx context.Context, target giznet.PublicKey) ([]peerhttp.SideControlSession, error) {
	if m == nil || m.Store == nil || target.IsZero() {
		return nil, errInvalidSession
	}
	now := m.nowOrDefault()
	items := make([]peerhttp.SideControlSession, 0)
	for entry, err := range m.Store.List(ctx, sideSessionPrefix(target)) {
		if err != nil {
			return nil, err
		}
		var index sideSessionIndex
		if err := json.Unmarshal(entry.Value, &index); err != nil {
			return nil, err
		}
		expiresAt := time.UnixMilli(index.Session.ExpiresAt)
		if index.Session.Kind != SessionKindSideControl || !expiresAt.After(now) {
			_ = m.Store.BatchDelete(ctx, []kv.Key{entry.Key, sessionKey(index.AccessToken)})
			continue
		}
		items = append(items, peerhttp.SideControlSession{
			Id:                  index.Session.SessionID,
			ControllerPublicKey: index.Session.PublicKey,
			IssuedAt:            index.Session.IssuedAt,
			ExpiresAt:           index.Session.ExpiresAt,
		})
	}
	return items, nil
}

// RevokeSideControlSession revokes a target-owned side-control bearer session.
func (m *SessionManager) RevokeSideControlSession(ctx context.Context, target giznet.PublicKey, id string) error {
	if m == nil || m.Store == nil || target.IsZero() || strings.TrimSpace(id) == "" {
		return errSideControlSessionNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	indexKey := sideSessionKey(target, id)
	data, err := m.Store.Get(ctx, indexKey)
	if errors.Is(err, kv.ErrNotFound) {
		return errSideControlSessionNotFound
	}
	if err != nil {
		return err
	}
	var index sideSessionIndex
	if err := json.Unmarshal(data, &index); err != nil || index.Session.TargetPublicKey != target.String() || index.Session.SessionID != id {
		return errSideControlSessionNotFound
	}
	return m.Store.BatchDelete(ctx, []kv.Key{indexKey, sessionKey(index.AccessToken)})
}

func deviceTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func deviceTokenHashKey(hash string) kv.Key {
	return kv.Key{"side-control", "device-tokens", "by-hash", hash}
}

func deviceTokenOwnerKey(target giznet.PublicKey, id string) kv.Key {
	return kv.Key{"side-control", "device-tokens", "by-owner", target.String(), id}
}

func sideSessionPrefix(target giznet.PublicKey) kv.Key {
	return kv.Key{"side-control", "sessions", target.String()}
}

func sideSessionKey(target giznet.PublicKey, id string) kv.Key {
	return append(sideSessionPrefix(target), id)
}
