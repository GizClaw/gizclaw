package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var (
	_ socialutil.PingDelivery        = (*Manager)(nil)
	_ socialutil.RoomPresenceService = (*Manager)(nil)
)

// socialPingTimeout bounds one client.social.ping push. A device that does not
// acknowledge in time counts as not reached; the push is never retried.
const socialPingTimeout = SocialPingDeliveryTimeout

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

// DeliverSocialPing prefers the active local Peer, otherwise uses one trusted
// connected Edge to route to the owning Server. Success requires a device ACK.
func (m *Manager) DeliverSocialPing(ctx context.Context, publicKey string, request rpcapi.ClientSocialPingRequest) error {
	key, err := parseSocialPingPeer(publicKey)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, socialPingTimeout)
	defer cancel()
	if _, ok := m.Peer(key); ok {
		return m.deliverLocalSocialPing(callCtx, key, request)
	}
	deadline, _ := callCtx.Deadline()
	delivery := &rpcpb.SocialPingDeliveryRequest{
		TargetPeerPublicKey: key.String(),
		Ping: &rpcpb.ClientSocialPingRequest{FromPeerPublicKey: request.FromPeerPublicKey,
			FromDisplayName: request.FromDisplayName, FriendGroupName: request.FriendGroupName},
		DeadlineUnixMs: deadline.UnixMilli(),
	}
	for _, edge := range m.socialPingRelays() {
		if !m.allowActivePeerRole(callCtx, edge.PublicKey(), apitypes.PeerRoleEdgeNode) {
			continue
		}
		// Never try another relay after a request may have reached a device.
		err := CallSocialPingDelivery(callCtx, edge, rpcapi.RPCMethodEdgeSocialPingRelay, delivery)
		slog.DebugContext(callCtx, "social ping relayed", "target_peer_public_key", publicKey,
			"edge_public_key", edge.PublicKey().String(), "acknowledged", err == nil)
		return err
	}
	return ErrDeviceOffline
}

// deliverLocalSocialPing is terminal: an Edge-delivered request cannot use
// another Edge when the target is offline or the assignment is stale.
func (m *Manager) deliverLocalSocialPing(ctx context.Context, key giznet.PublicKey, request rpcapi.ClientSocialPingRequest) error {
	peer, ok := m.Peer(key)
	if !ok {
		return ErrDeviceOffline
	}
	stream, err := dialSocialPingStream(ctx, peer, ServicePeerRPC)
	if err != nil {
		return err
	}
	defer stream.Close()
	stop := context.AfterFunc(ctx, func() { _ = stream.Close() })
	defer stop()
	_, err = (&rpcClient{}).PingSocial(ctx, stream, "client.social.ping", request)
	return err
}

func (m *Manager) socialPingRelays() []giznet.Conn {
	m.mu.RLock()
	var relays []giznet.Conn
	for _, state := range m.peers {
		if state.deleting {
			continue
		}
		for conn := range state.edgeTransports {
			relays = append(relays, conn)
		}
	}
	m.mu.RUnlock()
	relays = slices.DeleteFunc(relays, func(conn giznet.Conn) bool {
		info := conn.PeerInfo()
		return info != nil && info.State == giznet.PeerStateOffline
	})
	slices.SortFunc(relays, func(a, b giznet.Conn) int {
		return strings.Compare(a.PublicKey().String(), b.PublicKey().String())
	})
	return relays
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
