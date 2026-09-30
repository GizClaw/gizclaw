package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var (
	_ socialutil.PingDelivery        = (*Manager)(nil)
	_ socialutil.RoomPresenceService = (*Manager)(nil)
)

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

// PeerRunningWorkspace reports whether publicKey's active connection on this
// Server is running workspaceName right now. A Peer that is offline, has no
// running runtime, or runs another Workspace answers false, as does a runtime
// that is starting, stopping or failed.
func (m *Manager) PeerRunningWorkspace(ctx context.Context, publicKey, workspaceName string) bool {
	workspaceName = strings.TrimSpace(workspaceName)
	key, err := parseSocialPingPeer(publicKey)
	if err != nil || workspaceName == "" {
		return false
	}
	conn, status := m.peerRunStatus(key)
	if status == nil {
		return false
	}
	run, err := status(ctx)
	if err != nil || run.State != apitypes.PeerRunStatusStateRunning || run.WorkspaceName == nil {
		return false
	}
	if strings.TrimSpace(*run.WorkspaceName) != workspaceName {
		return false
	}
	// The status was read outside m.mu. It only counts while the connection it
	// belongs to is still the active one: a disconnect or a replacement that
	// overlapped the read must not report the previous generation's runtime.
	current, _ := m.peerRunStatus(key)
	return current == conn
}

// peerRunStatus returns the active connection of publicKey together with its
// registered run-state reader, or nil values when the Peer is offline, is
// being deleted, or its connection has not registered a reader.
func (m *Manager) peerRunStatus(publicKey giznet.PublicKey) (giznet.Conn, func(context.Context) (apitypes.PeerRunStatus, error)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.peers[publicKey]
	if state == nil || state.conn == nil || state.deleting || state.runStatus == nil {
		return nil, nil
	}
	return state.conn, state.runStatus
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
