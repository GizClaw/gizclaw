package gizcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

// MaxServerInfoBytes caps the accepted /server-info response body size.
const MaxServerInfoBytes = 1 << 20

// ServerInfoMetadata is the validated dial metadata derived from a Server's
// public /server-info document. When the Server advertises an edge-gateway
// transport, TransportPublicKey and SignalingURL point at the Edge while
// PublicKey remains the authoritative Server identity.
type ServerInfoMetadata struct {
	PublicKey          giznet.PublicKey
	TransportPublicKey giznet.PublicKey
	SignalingURL       string
	ICEServers         []gizwebrtc.ICEServer
	// ICEEndpoint is the host[:port] UDP endpoint advertised by /server-info.
	// The HTTP entry point may terminate TLS on a port that carries no ICE, so
	// this is the address WebRTC media uses, not the server URL authority.
	ICEEndpoint string
}

type retryableServerInfoError struct {
	err error
}

func (e *retryableServerInfoError) Error() string {
	return e.err.Error()
}

func (e *retryableServerInfoError) Unwrap() error {
	return e.err
}

// IsRetryableServerInfoError reports whether err is a transient
// FetchServerInfo failure (network error or 5xx status) that may succeed on
// retry. Malformed or invalid documents are never retryable.
func IsRetryableServerInfoError(err error) bool {
	var retryable *retryableServerInfoError
	return errors.As(err, &retryable)
}

// FetchServerInfo fetches <serverURL>/server-info, decodes it as
// apitypes.ServerInfo, and validates the metadata needed to dial the Server.
// serverURL is an http or https base URL such as "http://127.0.0.1:9820" or
// "https://ap.gizclaw.com"; a path prefix is preserved and a trailing slash is
// ignored. A bare host[:port] keeps working and defaults to http. Transient
// failures are classified by IsRetryableServerInfoError.
func FetchServerInfo(ctx context.Context, serverURL string) (ServerInfoMetadata, error) {
	baseURL, err := normalizeServerBaseURL(serverURL)
	if err != nil {
		return ServerInfoMetadata{}, fmt.Errorf("server-info invalid server URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/server-info", nil)
	if err != nil {
		return ServerInfoMetadata{}, fmt.Errorf("server-info request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ServerInfoMetadata{}, &retryableServerInfoError{err: fmt.Errorf("server-info fetch: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("server-info status: %s", resp.Status)
		if resp.StatusCode >= http.StatusInternalServerError {
			return ServerInfoMetadata{}, &retryableServerInfoError{err: err}
		}
		return ServerInfoMetadata{}, err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxServerInfoBytes+1))
	if err != nil {
		return ServerInfoMetadata{}, &retryableServerInfoError{err: fmt.Errorf("server-info read: %w", err)}
	}
	if len(data) > MaxServerInfoBytes {
		return ServerInfoMetadata{}, fmt.Errorf("server-info exceeds %d bytes", MaxServerInfoBytes)
	}
	var body apitypes.ServerInfo
	if err := json.Unmarshal(data, &body); err != nil {
		return ServerInfoMetadata{}, fmt.Errorf("server-info decode: %w", err)
	}
	if body.Protocol != "" && body.Protocol != "gizclaw-webrtc" {
		return ServerInfoMetadata{}, fmt.Errorf("server-info protocol = %q, want gizclaw-webrtc", body.Protocol)
	}
	var serverPK giznet.PublicKey
	if strings.TrimSpace(body.PublicKey) == "" {
		return ServerInfoMetadata{}, fmt.Errorf("server-info missing public_key")
	}
	if err := serverPK.UnmarshalText([]byte(strings.TrimSpace(body.PublicKey))); err != nil {
		return ServerInfoMetadata{}, fmt.Errorf("server-info invalid public_key: %w", err)
	}
	if serverPK.IsZero() {
		return ServerInfoMetadata{}, fmt.Errorf("server-info invalid public_key: zero key")
	}
	iceEndpoint := strings.TrimSpace(body.Endpoint)
	if iceEndpoint != "" {
		if err := validateHostPort(iceEndpoint); err != nil {
			return ServerInfoMetadata{}, fmt.Errorf("server-info invalid endpoint: %w", err)
		}
	}
	transportPK := serverPK
	signalingBase := baseURL
	signalingPath := strings.TrimSpace(body.SignalingPath)
	iceServers := serverInfoICEServers(body)
	if body.Transport != nil {
		if body.Transport.Mode != apitypes.ServerInfoTransportModeEdgeGateway {
			return ServerInfoMetadata{}, fmt.Errorf("server-info unsupported transport mode %q", string(body.Transport.Mode))
		}
		var err error
		signalingBase, err = normalizeTransportBaseURL(baseURL, body.Transport.Endpoint)
		if err != nil {
			return ServerInfoMetadata{}, fmt.Errorf("server-info invalid transport.endpoint: %w", err)
		}
		if strings.TrimSpace(body.Transport.PublicKey) == "" {
			return ServerInfoMetadata{}, fmt.Errorf("server-info missing transport.public_key")
		}
		if err := transportPK.UnmarshalText([]byte(strings.TrimSpace(body.Transport.PublicKey))); err != nil {
			return ServerInfoMetadata{}, fmt.Errorf("server-info invalid transport.public_key: %w", err)
		}
		if transportPK.IsZero() {
			return ServerInfoMetadata{}, fmt.Errorf("server-info invalid transport.public_key: zero key")
		}
		if transportPK.Equal(serverPK) {
			return ServerInfoMetadata{}, fmt.Errorf("server-info transport.public_key conflicts with authoritative public_key")
		}
		signalingPath = strings.TrimSpace(body.Transport.SignalingPath)
		// Authoritative Server ICE metadata is not valid for an Edge
		// transport. The Edge answer advertises its shared ICE mux.
		iceServers = nil
	}
	if signalingPath == "" {
		signalingPath = gizwebrtc.SignalingPath
	}
	if !strings.HasPrefix(signalingPath, "/") || strings.HasPrefix(signalingPath, "//") {
		return ServerInfoMetadata{}, fmt.Errorf("server-info invalid signaling_path %q", signalingPath)
	}
	return ServerInfoMetadata{
		PublicKey:          serverPK,
		TransportPublicKey: transportPK,
		SignalingURL:       signalingBase + signalingPath,
		ICEServers:         iceServers,
		ICEEndpoint:        iceEndpoint,
	}, nil
}

// normalizeServerBaseURL validates an absolute http or https base URL and
// returns it without a trailing slash.
// validateHostPort accepts the bare host[:port] form the /server-info endpoint
// field advertises. A URL, path, query, userinfo, empty port, or non-numeric
// port is rejected rather than silently reinterpreted.
func validateHostPort(endpoint string) error {
	value := strings.TrimSpace(endpoint)
	if value == "" {
		return errors.New("empty host")
	}
	if strings.ContainsAny(value, "/?#@\\ \t\r\n") {
		return errors.New("must be host[:port]")
	}
	host := value
	if strings.HasPrefix(value, "[") {
		end := strings.Index(value, "]")
		if end <= 1 {
			return errors.New("must be host[:port]")
		}
		host = value[:end+1]
	} else if prefix, _, found := strings.Cut(value, ":"); found {
		host = prefix
		if strings.ContainsAny(host, "[]") {
			return errors.New("must be host[:port]")
		}
	}
	if host == "" || host == "[]" {
		return errors.New("must be host[:port]")
	}
	rest := value[len(host):]
	if rest == "" {
		return nil
	}
	port, ok := strings.CutPrefix(rest, ":")
	if !ok || port == "" || len(port) > 5 {
		return errors.New("must be host[:port]")
	}
	for _, digit := range port {
		if digit < '0' || digit > '9' {
			return errors.New("port must be numeric")
		}
	}
	return nil
}

func normalizeServerBaseURL(serverURL string) (string, error) {
	value := strings.TrimSpace(serverURL)
	if value == "" {
		return "", errors.New("empty server URL")
	}
	if !strings.Contains(value, "://") {
		if err := validateHostPort(value); err != nil {
			return "", fmt.Errorf("server URL must be http://host[:port] or https://host[:port]: %w", err)
		}
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", errors.New("server URL must be http://host[:port] or https://host[:port]")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("server URL must use the http or https scheme")
	}
	if parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("server URL must be http://host[:port] or https://host[:port]")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.Contains(path, "//") {
		return "", errors.New("server URL path must not contain empty segments")
	}
	base := url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: path}
	return base.String(), nil
}

// normalizeTransportBaseURL accepts either a bare host[:port], which inherits
// the scheme of the configured server URL, or an absolute http or https URL.
func normalizeTransportBaseURL(serverBaseURL, endpoint string) (string, error) {
	value := strings.TrimSpace(endpoint)
	if strings.Contains(value, "://") {
		return normalizeServerBaseURL(value)
	}
	if err := validateHostPort(value); err != nil {
		return "", err
	}
	parsed, err := url.Parse(serverBaseURL)
	if err != nil {
		return "", err
	}
	base := url.URL{Scheme: parsed.Scheme, Host: value}
	return base.String(), nil
}

func serverInfoICEServers(body apitypes.ServerInfo) []gizwebrtc.ICEServer {
	if body.IceServers == nil || len(*body.IceServers) == 0 {
		return nil
	}
	out := make([]gizwebrtc.ICEServer, 0, len(*body.IceServers))
	for _, server := range *body.IceServers {
		converted := gizwebrtc.ICEServer{URLs: server.Urls}
		if server.Username != nil {
			converted.Username = *server.Username
		}
		if server.Credential != nil {
			converted.Credential = *server.Credential
		}
		out = append(out, converted)
	}
	return out
}
