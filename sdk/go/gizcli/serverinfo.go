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

// FetchServerInfo fetches http://<endpoint>/server-info, decodes it as
// apitypes.ServerInfo, and validates the metadata needed to dial the Server.
// endpoint must be host[:port] without a scheme. Transient failures are
// classified by IsRetryableServerInfoError.
func FetchServerInfo(ctx context.Context, endpoint string) (ServerInfoMetadata, error) {
	endpoint, err := normalizeServerInfoEndpoint(endpoint)
	if err != nil {
		return ServerInfoMetadata{}, fmt.Errorf("server-info invalid endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/server-info", nil)
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
	transportPK := serverPK
	signalingEndpoint := endpoint
	signalingPath := strings.TrimSpace(body.SignalingPath)
	iceServers := serverInfoICEServers(body)
	if body.Transport != nil {
		if body.Transport.Mode != apitypes.ServerInfoTransportModeEdgeGateway {
			return ServerInfoMetadata{}, fmt.Errorf("server-info unsupported transport mode %q", string(body.Transport.Mode))
		}
		var err error
		signalingEndpoint, err = normalizeServerInfoEndpoint(body.Transport.Endpoint)
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
	signalingURL := url.URL{Scheme: "http", Host: signalingEndpoint, Path: signalingPath}
	return ServerInfoMetadata{
		PublicKey:          serverPK,
		TransportPublicKey: transportPK,
		SignalingURL:       signalingURL.String(),
		ICEServers:         iceServers,
	}, nil
}

func normalizeServerInfoEndpoint(endpoint string) (string, error) {
	value := strings.TrimSpace(endpoint)
	if value == "" {
		return "", errors.New("empty endpoint")
	}
	if strings.Contains(value, "://") {
		return "", errors.New("endpoint must be host[:port]")
	}
	parsed, err := url.Parse("http://" + value)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("endpoint must be host[:port]")
	}
	return parsed.Host, nil
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
