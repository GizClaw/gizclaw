package connection

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizwebrtc"
)

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
	serverPK, err := cliCtx.ServerPublicKey()
	if err != nil {
		return nil, giznet.PublicKey{}, "", fmt.Errorf("invalid server public key: %w", err)
	}
	return &gizcli.Client{
		KeyPair: cliCtx.KeyPair,
		DialTransport: func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			if cliCtx.Config.Server.Transport == "webrtc" {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				l, conn, err := gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
					SignalingURL:   cliCtx.Config.Server.SignalingURL(),
					CipherMode:     gizwebrtc.CipherMode(cliCtx.Config.Server.CipherMode),
					SecurityPolicy: securityPolicy,
				})
				if err != nil {
					return nil, nil, err
				}
				return l, conn, nil
			}
			l, err := (&giznoise.ListenConfig{
				Addr:           ":0",
				CipherMode:     cliCtx.Config.Server.CipherMode,
				SecurityPolicy: securityPolicy,
			}).Listen(key)
			if err != nil {
				return nil, nil, err
			}
			udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
			if err != nil {
				_ = l.Close()
				return nil, nil, err
			}
			conn, err := l.Dial(serverPK, udpAddr)
			if err != nil {
				_ = l.Close()
				return nil, nil, err
			}
			return l, conn, nil
		},
	}, serverPK, cliCtx.Config.Server.NoiseUDPAddr(), nil
}

var dialFromContext = DialFromContext
var dialClient = func(c *gizcli.Client, serverPK giznet.PublicKey, serverAddr string) error {
	return c.Dial(serverPK, serverAddr)
}
var serveClient = func(c *gizcli.Client) error {
	return c.Serve()
}
var probeReady = probeServerPublicReady
var connectReadyTimeout = 5 * time.Second
var connectPollInterval = 10 * time.Millisecond

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
