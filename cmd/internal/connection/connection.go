package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const serverInfoTimeout = 5 * time.Second

type serverInfoMetadata struct {
	PublicKey    giznet.PublicKey
	SignalingURL string
}

func DialFromContext(name string) (*gizcli.Client, giznet.PublicKey, string, error) {
	store, err := clicontext.DefaultStore()
	if err != nil {
		return nil, giznet.PublicKey{}, "", err
	}
	var cliCtx *clicontext.CLIContext
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

	ctx, cancel := context.WithTimeout(context.Background(), serverInfoTimeout)
	defer cancel()
	info, err := fetchServerInfo(ctx, cliCtx.Config.Server.Endpoint)
	if err != nil {
		return nil, giznet.PublicKey{}, "", err
	}
	return &gizcli.Client{
		KeyPair: cliCtx.KeyPair,
		DialTransport: func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			_ = serverAddr
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			l, conn, err := gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
				SignalingURL:   info.SignalingURL,
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
var fetchServerInfo = fetchServerPublicInfo
var dialClient = func(c *gizcli.Client, serverPK giznet.PublicKey, serverAddr string) error {
	return c.Dial(serverPK, serverAddr)
}
var serveClient = func(c *gizcli.Client) error {
	return c.Serve()
}
var probeReady = probeServerPublicReady
var connectReadyTimeout = 5 * time.Second
var connectPollInterval = 10 * time.Millisecond

func fetchServerPublicInfo(ctx context.Context, endpoint string) (serverInfoMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/server-info", nil)
	if err != nil {
		return serverInfoMetadata{}, fmt.Errorf("server-info request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return serverInfoMetadata{}, fmt.Errorf("server-info fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return serverInfoMetadata{}, fmt.Errorf("server-info status: %s", resp.Status)
	}
	var body struct {
		PublicKey     string `json:"public_key"`
		Protocol      string `json:"protocol"`
		SignalingPath string `json:"signaling_path"`
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
	signalingPath := strings.TrimSpace(body.SignalingPath)
	if signalingPath == "" {
		signalingPath = gizwebrtc.SignalingPath
	}
	if !strings.HasPrefix(signalingPath, "/") || strings.HasPrefix(signalingPath, "//") {
		return serverInfoMetadata{}, fmt.Errorf("server-info invalid signaling_path %q", signalingPath)
	}
	signalingURL := url.URL{Scheme: "http", Host: endpoint, Path: signalingPath}
	return serverInfoMetadata{PublicKey: serverPK, SignalingURL: signalingURL.String()}, nil
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
	go func() {
		errCh <- serveClient(c)
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

func probeServerPublicReady(c *gizcli.Client) error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if c.PeerConn() == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	api, err := c.ServerPublicClient()
	if err != nil {
		return err
	}
	_, err = api.GetServerInfoWithResponse(ctx)
	return err
}
