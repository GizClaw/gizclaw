package peerhttp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"go.step.sm/crypto/x25519"
	"golang.org/x/crypto/curve25519"
)

const signedBearerPrefix = "gizclaw_sig_v1_"
const signedBearerAudience = "gizclaw/peer-http/v1"

// MaxSignedBearerLifetime bounds the validity of a locally signed HTTP token.
const MaxSignedBearerLifetime = 5 * time.Minute

// ErrInvalidSignedBearer never includes a credential or signature in its text.
var ErrInvalidSignedBearer = errors.New("invalid signed Peer HTTP bearer")

type signedBearerClaims struct {
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Target   string `json:"target"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

// IsSignedBearerToken identifies the versioned signature format, including
// malformed tokens that must fail authentication without a credential fallback.
func IsSignedBearerToken(token string) bool {
	return strings.HasPrefix(token, signedBearerPrefix)
}

// NewSignedBearerToken signs a short-lived Peer HTTP credential with the existing
// Giznet X25519 private key using XEdDSA. Only public claims and the signature are
// returned; no network request, API key issuance or private-key transfer occurs.
// A zero target means the caller itself. The Server determines current authority,
// so signing a token for another Peer does not grant access to it.
func NewSignedBearerToken(key *giznet.KeyPair, target giznet.PublicKey, lifetime time.Duration) (string, error) {
	if key == nil || key.Private.IsZero() || lifetime < time.Second || lifetime > MaxSignedBearerLifetime {
		return "", ErrInvalidSignedBearer
	}
	derived, err := giznet.NewKeyPair(key.Private)
	if err != nil || derived.Public != key.Public {
		return "", ErrInvalidSignedBearer
	}
	if target.IsZero() {
		target = key.Public
	}
	now := time.Now().Unix()
	claims := signedBearerClaims{Issuer: key.Public.String(), Audience: signedBearerAudience,
		Target: target.String(), IssuedAt: now, Expires: now + int64(lifetime/time.Second)}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	message := signedBearerPrefix + base64.RawURLEncoding.EncodeToString(payload)
	signature, err := x25519.Sign(rand.Reader, x25519.PrivateKey(key.Private[:]), []byte(message))
	if err != nil {
		return "", err
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// VerifySignedBearerToken verifies the signature, audience and bounded lifetime,
// returning the authenticated caller and signed resource target. It grants no
// role or resource permission; callers must recheck current Server authorization.
func VerifySignedBearerToken(token string, now time.Time) (caller, target giznet.PublicKey, err error) {
	invalid := func() (giznet.PublicKey, giznet.PublicKey, error) {
		return giznet.PublicKey{}, giznet.PublicKey{}, ErrInvalidSignedBearer
	}
	if !IsSignedBearerToken(token) || len(token) > 2048 {
		return invalid()
	}
	message, encodedSignature, ok := strings.Cut(token, ".")
	if !ok {
		return invalid()
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(encodedSignature)
	if err != nil || len(signature) != x25519.SignatureSize {
		return invalid()
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(message, signedBearerPrefix))
	if err != nil {
		return invalid()
	}
	var claims signedBearerClaims
	if json.Unmarshal(payload, &claims) != nil || claims.Audience != signedBearerAudience ||
		claims.IssuedAt <= 0 || claims.IssuedAt > now.Unix()+30 || claims.Expires <= now.Unix() ||
		claims.Expires <= claims.IssuedAt || claims.Expires-claims.IssuedAt > int64(MaxSignedBearerLifetime/time.Second) {
		return invalid()
	}
	for _, item := range []struct {
		text string
		key  *giznet.PublicKey
	}{{claims.Issuer, &caller}, {claims.Target, &target}} {
		text, key := item.text, item.key
		if len(text) < 32 || len(text) > 44 || key.UnmarshalText([]byte(text)) != nil || key.IsZero() || key.String() != text {
			return invalid()
		}
	}
	// Reject low-order caller points before signature verification. This uses a
	// fixed, non-secret test scalar and does not derive any authentication key.
	if _, err := curve25519.X25519(curve25519.Basepoint, caller[:]); err != nil {
		return invalid()
	}
	if !x25519.Verify(x25519.PublicKey(caller[:]), []byte(message), signature) {
		return invalid()
	}
	return caller, target, nil
}
