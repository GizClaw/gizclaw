package gizclaw

import (
	"context"
	"net/netip"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const peerAddressRecordTimeout = 2 * time.Second

// peerSourceAddr excludes transport placeholders and normalizes IP endpoints.
func peerSourceAddr(info *giznet.PeerInfo) string {
	if info == nil || info.Endpoint == nil {
		return ""
	}
	endpoint, err := netip.ParseAddrPort(info.Endpoint.String())
	if err != nil || endpoint.Port() == 0 {
		return ""
	}
	return netip.AddrPortFrom(endpoint.Addr().Unmap().WithZone(""), endpoint.Port()).String()
}

// recordPeerAddress runs under the per-Peer connection publication lock, never
// the Manager state lock. Only accepted new connections write their address;
// disconnect and retirement keep the last accepted address unchanged.
func (m *Manager) recordPeerAddress(ctx context.Context, publicKey giznet.PublicKey, conn giznet.Conn) error {
	if m.PeerRun == nil || conn == nil {
		return nil
	}
	address := peerSourceAddr(conn.PeerInfo())
	if address == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, peerAddressRecordTimeout)
	defer cancel()
	return m.PeerRun.RecordLastAddr(ctx, publicKey, address)
}
