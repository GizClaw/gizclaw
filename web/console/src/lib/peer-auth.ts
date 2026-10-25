import type { GizClawPeerMonitorIdentity } from "@gizclaw/gizclaw-control";

export type PeerReadAuth = string | GizClawPeerMonitorIdentity;

/** A device-specific credential wins over the configured shared identity. */
export function peerAuth(
  peer: { signedBearer?: string; identity?: GizClawPeerMonitorIdentity },
  identity = peer.identity,
): PeerReadAuth | undefined {
  return peer.signedBearer ?? identity;
}
