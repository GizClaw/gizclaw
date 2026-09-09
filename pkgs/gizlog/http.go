package gizlog

import (
	"log/slog"
	"net/http"
	"net/netip"
	"unicode/utf8"
)

// ClientIPHeader carries the socket-derived client IP from an authenticated
// Edge to Server. Public ingress must overwrite it rather than trust it.
const ClientIPHeader = "X-Gizclaw-Client-IP"

// HTTPMetadata contains bounded access-log fields. They are log-only values,
// never metric labels. RequestPath excludes the query string.
type HTTPMetadata struct {
	RequestPath string
	ClientIP    string
	UserAgent   string
}

// HTTPRequestMetadata records the socket peer unless trustedEdge is true.
// Only an authenticated Edge service may enable trustedEdge; other proxy
// headers, including X-Forwarded-For, are deliberately not consulted.
func HTTPRequestMetadata(request *http.Request, trustedEdge bool) HTTPMetadata {
	if request == nil {
		return HTTPMetadata{}
	}
	path := "/"
	if request.URL != nil && request.URL.EscapedPath() != "" {
		path = request.URL.EscapedPath()
	}
	address := request.RemoteAddr
	if trustedEdge {
		address = request.Header.Get(ClientIPHeader)
	}
	ip, err := netip.ParseAddr(address)
	if err != nil && !trustedEdge {
		if endpoint, parseErr := netip.ParseAddrPort(address); parseErr == nil {
			ip = endpoint.Addr()
		}
	}
	clientIP := ""
	if ip.IsValid() {
		clientIP = ip.Unmap().WithZone("").String()
	}
	return HTTPMetadata{RequestPath: boundedHTTPText(path, 1024), ClientIP: clientIP, UserAgent: boundedHTTPText(request.UserAgent(), 512)}
}

// Attrs returns access metadata suitable for an HTTP completion record.
func (m HTTPMetadata) Attrs() []slog.Attr {
	return []slog.Attr{slog.String("request_path", m.RequestPath), slog.String("client_ip", m.ClientIP), slog.String("user_agent", m.UserAgent)}
}

func boundedHTTPText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}
