package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var _ socialutil.PingDelivery = (*Manager)(nil)

// socialPingTimeout bounds one client.social.ping push. A device that does not
// acknowledge in time counts as not reached; the push is never retried.
const socialPingTimeout = 3 * time.Second

// PeerOnline reports whether publicKey has an active connection on this
// Server. It is the same connection state Runtime.online reports.
func (m *Manager) PeerOnline(publicKey string) bool {
	key, err := parseSocialPingPeer(publicKey)
	if err != nil {
		return false
	}
	_, ok := m.Peer(key)
	return ok
}

// DeliverSocialPing pushes one client.social.ping over the Peer's active
// connection on this Server and returns once the device acknowledged it.
func (m *Manager) DeliverSocialPing(ctx context.Context, publicKey string, request rpcapi.ClientSocialPingRequest) error {
	key, err := parseSocialPingPeer(publicKey)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, socialPingTimeout)
	defer cancel()
	_, err = callPeerRPC(m, callCtx, key, func(client *rpcClient, conn net.Conn) (*rpcapi.ClientSocialPingResponse, error) {
		return client.PingSocial(callCtx, conn, "client.social.ping", request)
	})
	return err
}

func parseSocialPingPeer(publicKey string) (giznet.PublicKey, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(strings.TrimSpace(publicKey))); err != nil {
		return giznet.PublicKey{}, fmt.Errorf("gizclaw: social ping peer: %w", err)
	}
	if key.IsZero() {
		return giznet.PublicKey{}, errors.New("gizclaw: social ping peer is empty")
	}
	return key, nil
}
