package peerhttp

import (
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
