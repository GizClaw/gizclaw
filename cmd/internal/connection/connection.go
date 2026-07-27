package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const serverInfoRetryAttempts = 2

var (
	serverInfoAttemptTimeout = 5 * time.Second
	serverInfoRetryDelay     = 100 * time.Millisecond
)

type serverInfoMetadata struct {
	PublicKey          giznet.PublicKey
	TransportPublicKey giznet.PublicKey
	SignalingURL       string
	ICEServers         []gizwebrtc.ICEServer
}

func DialFromContext(name string) (*gizcli.Client, giznet.PublicKey, string, error) {
	store, err := clicontext.DefaultStore()
	if err != nil {
		return nil, giznet.PublicKey{}, "", err
	}
	var cliCtx *contextstore.Context
	if name != "" {
		cliCtx, err = store.LoadByName(name)
	} else {
		cliCtx, err = store.Current()
	}
	if err != nil {
		return nil, giznet.PublicKey{}, "", err
	}
	if cliCtx == nil {
		return nil, giznet.PublicKey{}, "", fmt.Errorf("no active context; run 'gizclaw context create' first")
	}

	info, err := fetchServerInfoWithRetry(cliCtx.Config.Server.Endpoint)
	if err != nil {
		return nil, giznet.PublicKey{}, "", err
	}
	return &gizcli.Client{
		KeyPair: cliCtx.KeyPair,
		DialTransport: func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			_ = serverPK
			_ = serverAddr
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			l, conn, err := gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{
				SignalingURL:   info.SignalingURL,
				ICEServers:     info.ICEServers,
				SecurityPolicy: securityPolicy,
			})
			if err != nil {
				return nil, nil, err
			}
			return l, conn, nil
		},
	}, info.PublicKey, cliCtx.Config.Server.Endpoint, nil
}

var dialFromContext = DialFromContext
var fetchServerInfo = fetchPeerHTTPInfo
var dialClient = func(c *gizcli.Client, serverPK giznet.PublicKey, serverAddr string) error {
	return c.Dial(serverPK, serverAddr)
}
var serveClient = func(c *gizcli.Client) error {
	return c.Serve()
}
var probeReady = probePeerHTTPReady
var connectReadyTimeout = 5 * time.Second
var connectPollInterval = 10 * time.Millisecond

type retryableServerInfoError struct {
	err error
}

func (e *retryableServerInfoError) Error() string {
	return e.err.Error()
}

func (e *retryableServerInfoError) Unwrap() error {
	return e.err
}

func fetchServerInfoWithRetry(endpoint string) (serverInfoMetadata, error) {
	var lastErr error
	for attempt := range serverInfoRetryAttempts {
		ctx, cancel := context.WithTimeout(context.Background(), serverInfoAttemptTimeout)
		info, err := fetchServerInfo(ctx, endpoint)
		cancel()
		if err == nil {
			return info, nil
		}
		lastErr = err

		var retryable *retryableServerInfoError
		if !errors.As(err, &retryable) || attempt+1 == serverInfoRetryAttempts {
			return serverInfoMetadata{}, err
		}
		time.Sleep(serverInfoRetryDelay)
	}
	return serverInfoMetadata{}, lastErr
}

func fetchPeerHTTPInfo(ctx context.Context, endpoint string) (serverInfoMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/server-info", nil)
	if err != nil {
		return serverInfoMetadata{}, fmt.Errorf("server-info request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return serverInfoMetadata{}, &retryableServerInfoError{err: fmt.Errorf("server-info fetch: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("server-info status: %s", resp.Status)
		if resp.StatusCode >= http.StatusInternalServerError {
			return serverInfoMetadata{}, &retryableServerInfoError{err: err}
		}
		return serverInfoMetadata{}, err
	}
	var body struct {
		PublicKey     string `json:"public_key"`
		Protocol      string `json:"protocol"`
		SignalingPath string `json:"signaling_path"`
		ICEServers    []struct {
			URLs       []string `json:"urls"`
			Username   string   `json:"username"`
			Credential string   `json:"credential"`
		} `json:"ice_servers"`
		Transport *struct {
			Mode          string `json:"mode"`
			Endpoint      string `json:"endpoint"`
			PublicKey     string `json:"public_key"`
			SignalingPath string `json:"signaling_path"`
		} `json:"transport"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return serverInfoMetadata{}, fmt.Errorf("server-info decode: %w", err)
	}
	if body.Protocol != "" && body.Protocol != "gizclaw-webrtc" {
		return serverInfoMetadata{}, fmt.Errorf("server-info protocol = %q, want gizclaw-webrtc", body.Protocol)
	}
	var serverPK giznet.PublicKey
	if strings.TrimSpace(body.PublicKey) == "" {
		return serverInfoMetadata{}, fmt.Errorf("server-info missing public_key")
	}
	if err := serverPK.UnmarshalText([]byte(strings.TrimSpace(body.PublicKey))); err != nil {
		return serverInfoMetadata{}, fmt.Errorf("server-info invalid public_key: %w", err)
	}
	if serverPK.IsZero() {
		return serverInfoMetadata{}, fmt.Errorf("server-info invalid public_key: zero key")
	}
	transportPK := serverPK
	signalingEndpoint := endpoint
	signalingPath := strings.TrimSpace(body.SignalingPath)
	iceServers := toWebRTCICEServers(body.ICEServers)
	if body.Transport != nil {
		if body.Transport.Mode != "edge-gateway" {
			return serverInfoMetadata{}, fmt.Errorf("server-info unsupported transport mode %q", body.Transport.Mode)
		}
		var err error
		signalingEndpoint, err = normalizeServerInfoEndpoint(body.Transport.Endpoint)
		if err != nil {
			return serverInfoMetadata{}, fmt.Errorf("server-info invalid transport.endpoint: %w", err)
		}
		if strings.TrimSpace(body.Transport.PublicKey) == "" {
			return serverInfoMetadata{}, fmt.Errorf("server-info missing transport.public_key")
		}
		if err := transportPK.UnmarshalText([]byte(strings.TrimSpace(body.Transport.PublicKey))); err != nil {
			return serverInfoMetadata{}, fmt.Errorf("server-info invalid transport.public_key: %w", err)
		}
		if transportPK.IsZero() {
			return serverInfoMetadata{}, fmt.Errorf("server-info invalid transport.public_key: zero key")
		}
		if transportPK.Equal(serverPK) {
			return serverInfoMetadata{}, fmt.Errorf("server-info transport.public_key conflicts with authoritative public_key")
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
		return serverInfoMetadata{}, fmt.Errorf("server-info invalid signaling_path %q", signalingPath)
	}
	signalingURL := url.URL{Scheme: "http", Host: signalingEndpoint, Path: signalingPath}
	return serverInfoMetadata{
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

func toWebRTCICEServers(in []struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}) []gizwebrtc.ICEServer {
	if len(in) == 0 {
		return nil
	}
	out := make([]gizwebrtc.ICEServer, 0, len(in))
	for _, server := range in {
		out = append(out, gizwebrtc.ICEServer{
			URLs:       server.URLs,
			Username:   server.Username,
			Credential: server.Credential,
		})
	}
	return out
}

func ConnectFromContext(name string) (*gizcli.Client, error) {
	c, serverPK, serverAddr, err := dialFromContext(name)
	if err != nil {
		return nil, err
	}
	if err := dialClient(c, serverPK, serverAddr); err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
	serve := serveClient
	go func() {
		errCh <- serve(c)
	}()
	deadline := time.Now().Add(connectReadyTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			_ = c.Close()
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("gizclaw: client stopped before ready")
		default:
		}
		if err := probeReady(c); err == nil {
			return c, nil
		}
		time.Sleep(connectPollInterval)
	}
	_ = c.Close()
	return nil, fmt.Errorf("gizclaw: timeout waiting for client readiness")
}

func probePeerHTTPReady(c *gizcli.Client) error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if c.PeerConn() == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	api, err := c.PeerHTTPClient()
	if err != nil {
		return err
	}
	_, err = api.GetServerInfoWithResponse(ctx)
	return err
}
