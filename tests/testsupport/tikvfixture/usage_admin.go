package tikvfixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// adminFriendCreate forwards a real Admin HTTP operation over a real accepted
// fixture Admin WebRTC connection. Only this isolated Docker mode receives the
// published synthetic admin key; no private credentials or files are read.
func (s *UsageServices) adminFriendCreate(ctx context.Context, action UsageAction) (any, error) {
	const keyEnv = "GIZCLAW_E2E_TIKV_USAGE_ADMIN_PRIVATE_KEY"
	if s.AdminEndpoint == "" || os.Getenv(keyEnv) == "" {
		return nil, errors.New("fixture Admin endpoint and synthetic key are required")
	}
	var private giznet.Key
	if err := private.UnmarshalText([]byte(os.Getenv(keyEnv))); err != nil {
		return nil, fmt.Errorf("parse fixture Admin key: %w", err)
	}
	key, err := giznet.NewKeyPair(private)
	if err != nil {
		return nil, err
	}
	info, err := gizcli.FetchServerInfo(ctx, s.AdminEndpoint)
	if err != nil {
		return nil, err
	}
	client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		dial, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return gizwebrtc.Dial(dial, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
	}}
	if err := client.Dial(info.PublicKey, s.AdminEndpoint); err != nil {
		_ = client.Close()
		return nil, err
	}
	served := make(chan error, 1)
	go func() { served <- client.Serve() }()
	defer func() { _ = client.Close(); <-served }()
	if _, err := client.Ping(ctx, "tikv-usage-admin"); err != nil {
		return nil, err
	}
	api, err := client.ServerAdminClient()
	if err != nil {
		return nil, err
	}
	response, err := api.CreateFriend(ctx, adminhttp.CreateFriendJSONRequestBody{Id: usageRelation(action.Owner, action.Peer), OwnerPublicKey: action.Owner, PeerPublicKey: action.Peer})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("Admin Friend response exceeds 1 MiB")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode actual Admin Friend response: %w", err)
	}
	return map[string]any{"status": response.StatusCode, "body": value}, nil
}
