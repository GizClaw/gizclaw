package gizlog

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHTTPRequestMetadataTrustBoundary(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com/missing?token=secret", nil)
	req.RemoteAddr = "[2001:db8::1]:443"
	req.Header.Set("User-Agent", "example-client/1.0")
	req.Header.Set("X-Forwarded-For", "192.0.2.99")
	req.Header.Set(ClientIPHeader, "192.0.2.10")
	metadata := HTTPRequestMetadata(req, false)
	if metadata.ClientIP != "2001:db8::1" || metadata.RequestPath != "/missing" || metadata.UserAgent != "example-client/1.0" {
		t.Fatalf("metadata: %+v", metadata)
	}
	if metadata = HTTPRequestMetadata(req, true); metadata.ClientIP != "192.0.2.10" {
		t.Fatalf("trusted Edge: %+v", metadata)
	}
	req.Header.Set(ClientIPHeader, "forged, 192.0.2.10")
	if metadata = HTTPRequestMetadata(req, true); metadata.ClientIP != "" {
		t.Fatalf("invalid forwarded IP accepted: %+v", metadata)
	}
}

func TestHTTPMetadataBoundsClientInput(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/"+strings.Repeat("a", 2000)+"?secret=hidden", nil)
	req.Header.Set("User-Agent", strings.Repeat("中", 1000))
	metadata := HTTPRequestMetadata(req, false)
	if len(metadata.RequestPath) > 1024 || len(metadata.UserAgent) > 512 || !utf8.ValidString(metadata.UserAgent) || strings.Contains(metadata.RequestPath, "secret") {
		t.Fatalf("unbounded metadata")
	}
}
