package peerhttp

import (
	"errors"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"net/http"
	"strings"
)

// IsIdentifierLookup identifies the anonymous device directory routes.
func IsIdentifierLookup(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	return strings.HasPrefix(path, "/gizclaw/v1/peers/@findBySn/") ||
		strings.HasPrefix(path, "/gizclaw/v1/peers/@findByImei/")
}

// IsDebugDataPath identifies the device and contact surfaces accepting debug access.
// Credentials and authentication management are outside this surface.
func IsDebugDataPath(path string) bool {
	return path == "/gizclaw/v1/device" || strings.HasPrefix(path, "/gizclaw/v1/device/") ||
		path == "/gizclaw/v1/contacts" || strings.HasPrefix(path, "/gizclaw/v1/contacts/")
}

// DebugPublicKey parses a tagged public-key bearer without interpreting API keys.
// Public key text must use the canonical Base58 representation.
func DebugPublicKey(authorization string) (giznet.PublicKey, bool, error) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(authorization), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(token, "gizclaw_pk_") {
		return giznet.PublicKey{}, false, nil
	}
	text := strings.TrimPrefix(token, "gizclaw_pk_")
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(text)); err != nil || key.IsZero() || key.String() != text {
		return giznet.PublicKey{}, true, errors.New("invalid public-key bearer")
	}
	return key, true, nil
}
