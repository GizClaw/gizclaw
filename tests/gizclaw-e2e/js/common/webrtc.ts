import { readFile } from "node:fs/promises";
import path from "node:path";
import wrtc from "@roamhq/wrtc";
import { connectGiznetWebRTC, sendGiznetWebRTCOffer } from "@gizclaw/webrtc";
import { prepareEncryptedGiznetWebRTCOffer } from "@gizclaw/webrtc/signaling";

export const repoRoot = path.resolve(import.meta.dirname, "../../../..");

export type Identity = {
  clientPrivateKey: Uint8Array;
  endpoint: string;
  serverPublicKey: string;
};

export async function connectSetupPeer(identityDir: string): Promise<wrtc.RTCPeerConnection> {
  const identity = await loadIdentity(identityDir);
  const pc = new wrtc.RTCPeerConnection();
  await connectGiznetWebRTC({
    pc: pc as unknown as RTCPeerConnection,
    prepareOffer: (offerSDP) =>
      prepareEncryptedGiznetWebRTCOffer(
        {
          clientPrivateKey: identity.clientPrivateKey,
          serverPublicKey: identity.serverPublicKey,
        },
        offerSDP,
      ),
    sendOffer: (offer, signal) => sendGiznetWebRTCOffer(offer, { baseUrl: `http://${identity.endpoint}`, signal }),
  });
  return pc;
}

export async function loadIdentity(dir: string): Promise<Identity> {
  const [config, privateKey] = await Promise.all([
    readFile(path.join(dir, "config.yaml"), "utf8"),
    readFile(path.join(dir, "identity.key")),
  ]);
  if (privateKey.length !== 32) {
    throw new Error(`identity.key length = ${privateKey.length}, want 32`);
  }
  return {
    clientPrivateKey: privateKey,
    endpoint: matchConfig(config, /endpoint:\s*([^\s]+)/),
    serverPublicKey: matchConfig(config, /public-key:\s*"?([^"\s]+)"?/),
  };
}

export async function serverAvailable(endpoint: string): Promise<boolean> {
  try {
    const response = await fetch(`http://${endpoint}/server-info`, { signal: AbortSignal.timeout(1000) });
    return response.ok;
  } catch {
    return false;
  }
}

export function closePeerConnection(pc: wrtc.RTCPeerConnection): void {
  for (const sender of pc.getSenders?.() ?? []) {
    sender.track?.stop();
  }
  for (const transceiver of pc.getTransceivers?.() ?? []) {
    try {
      transceiver.stop();
    } catch {
      // Some runtimes throw if a transceiver has already stopped.
    }
  }
  pc.close();
}

function matchConfig(config: string, pattern: RegExp): string {
  const match = config.match(pattern);
  if (match?.[1] == null) {
    throw new Error(`missing config field matching ${pattern}`);
  }
  return match[1].trim();
}
