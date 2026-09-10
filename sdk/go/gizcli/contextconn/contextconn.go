// Package contextconn connects a gizcli.Client from a local GizClaw CLI
// context.
//
// A context is a directory under the GizClaw config root holding one
// config.yaml with a Giznet identity and a Server endpoint. The package is
// pure Go and does not depend on the CLI command tree, cgo, or embedded web
// assets, so it can be used by tools that run on hosts without a native CLI
// build.
package contextconn

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

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

// Options selects the context used for a connection.
type Options struct {
	// Context is the context name. Empty selects the current context.
	Context string
	// ConfigDir is the context root. Empty selects ConfigDir().
	ConfigDir string
	// Endpoint, when non-empty, replaces the context's server.endpoint. It
	// must satisfy ValidateEndpoint.
	Endpoint string
}

// ConfigDir returns the GizClaw configuration root directory.
//
// On Unix-like systems it is $XDG_CONFIG_HOME/gizclaw when XDG_CONFIG_HOME is
// set and ~/.config/gizclaw otherwise. On Windows it is %AppData%\gizclaw.
func ConfigDir() (string, error) {
	if runtime.GOOS != "windows" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "gizclaw"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "gizclaw"), nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gizclaw"), nil
}

// ValidateEndpoint validates an explicit Server endpoint override and returns
// its canonical form. The override must be https://host[:port] without
// userinfo, path, query, or fragment; a single trailing slash is removed.
func ValidateEndpoint(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("contextconn: invalid endpoint: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Opaque != "" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("contextconn: endpoint must be https://host[:port] without userinfo, path, query, or fragment")
	}
	return (&url.URL{Scheme: "https", Host: parsed.Host}).String(), nil
}

// LoadContext loads the selected context and applies the endpoint override.
func LoadContext(opts Options) (*contextstore.Context, error) {
	root := opts.ConfigDir
	if root == "" {
		var err error
		root, err = ConfigDir()
		if err != nil {
			return nil, fmt.Errorf("contextconn: config dir: %w", err)
		}
	}
	store := &contextstore.Store{Root: root}
	var cliCtx *contextstore.Context
	var err error
	if opts.Context != "" {
		cliCtx, err = store.LoadByName(opts.Context)
	} else {
		cliCtx, err = store.Current()
	}
	if err != nil {
		return nil, err
	}
	if cliCtx == nil {
		return nil, fmt.Errorf("no active context; run 'gizclaw context create' first")
	}
	if opts.Endpoint != "" {
		endpoint, err := ValidateEndpoint(opts.Endpoint)
		if err != nil {
			return nil, err
		}
		cliCtx.Config.Server.Endpoint = endpoint
	}
	return cliCtx, nil
}

// Dial loads the context, fetches the Server's server-info, and returns a
// client configured for the WebRTC transport together with the Server public
// key and base URL. The returned client is not connected yet.
func Dial(opts Options) (*gizcli.Client, giznet.PublicKey, string, error) {
	cliCtx, err := LoadContext(opts)
	if err != nil {
		return nil, giznet.PublicKey{}, "", err
	}

	info, err := fetchServerInfoWithRetry(cliCtx.Config.Server.BaseURL())
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
	}, info.PublicKey, cliCtx.Config.Server.BaseURL(), nil
}

var dialOptions = Dial
var fetchServerInfo = gizcli.FetchServerInfo
var dialClient = func(c *gizcli.Client, serverPK giznet.PublicKey, serverAddr string) error {
	return c.Dial(serverPK, serverAddr)
}
var serveClient = func(c *gizcli.Client) error {
	return c.Serve()
}
var probeReady = probePeerHTTPReady
var connectReadyTimeout = 5 * time.Second
var connectPollInterval = 10 * time.Millisecond

func fetchServerInfoWithRetry(serverURL string) (gizcli.ServerInfoMetadata, error) {
	var lastErr error
	for attempt := range serverInfoRetryAttempts {
		ctx, cancel := context.WithTimeout(context.Background(), serverInfoAttemptTimeout)
		info, err := fetchServerInfo(ctx, serverURL)
		cancel()
		if err == nil {
			return info, nil
		}
		lastErr = err

		if !gizcli.IsRetryableServerInfoError(err) || attempt+1 == serverInfoRetryAttempts {
			return gizcli.ServerInfoMetadata{}, err
		}
		time.Sleep(serverInfoRetryDelay)
	}
	return gizcli.ServerInfoMetadata{}, lastErr
}

// Connect dials the selected context, starts serving the client-side Peer
// services, and returns once the Peer HTTP service answers. The caller owns
// the returned client and must Close it.
func Connect(opts Options) (*gizcli.Client, error) {
	c, serverPK, serverAddr, err := dialOptions(opts)
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
