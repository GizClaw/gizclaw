import { readFile } from "node:fs/promises";
import path from "node:path";
import wrtc from "@roamhq/wrtc";
import { connectGiznetWebRTC, sendGiznetWebRTCOffer } from "@gizclaw/gizclaw";
import { prepareEncryptedGiznetWebRTCOffer } from "@gizclaw/gizclaw/signaling";

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
  await new Promise((resolve) => setTimeout(resolve, 100));
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

export async function assertSetupServerAvailable(endpoint: string): Promise<void> {
  try {
    const response = await fetch(`http://${endpoint}/server-info`, { signal: AbortSignal.timeout(1000) });
    if (!response.ok) {
      throw new Error(`server-info returned HTTP ${response.status}`);
    }
  } catch (err) {
    throw new Error(
      `gizclaw e2e setup server is required at ${endpoint}; run tests/gizclaw-e2e/setup/start-server.sh before this JS e2e test`,
      { cause: err },
    );
  }
}

export function closePeerConnection(pc: wrtc.RTCPeerConnection): void {
  pc.close();
}

function matchConfig(config: string, pattern: RegExp): string {
  const match = config.match(pattern);
  if (match?.[1] == null) {
    throw new Error(`missing config field matching ${pattern}`);
  }
  return match[1].trim();
}
