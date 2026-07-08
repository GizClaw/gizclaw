import { readFile } from "node:fs/promises";
import path from "node:path";
import { x25519 } from "@noble/curves/ed25519.js";
import wrtc from "@roamhq/wrtc";
import { GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL, connectGiznetWebRTCFromEndpoint } from "@gizclaw/gizclaw";
import { base58Decode, base58Encode } from "@gizclaw/gizclaw/signaling";

export const repoRoot = path.resolve(import.meta.dirname, "../../../..");

export type Identity = {
  clientPrivateKey: Uint8Array;
  endpoint: string;
  publicKey: string;
};

export async function connectSetupPeer(identityDir: string): Promise<wrtc.RTCPeerConnection> {
  const identity = await loadIdentity(identityDir);
  const pc = new wrtc.RTCPeerConnection();
  await connectGiznetWebRTCFromEndpoint({
    clientPrivateKey: identity.clientPrivateKey,
    endpoint: identity.endpoint,
    pc: pc as unknown as RTCPeerConnection,
  });
  await new Promise((resolve) => setTimeout(resolve, 100));
  return pc;
}

export type SetupPeerWithPacketChannel = {
  packetChannel: RTCDataChannel;
  pc: wrtc.RTCPeerConnection;
};

export async function connectSetupPeerWithPacketChannel(identityDir: string): Promise<SetupPeerWithPacketChannel> {
  const identity = await loadIdentity(identityDir);
  const pc = new wrtc.RTCPeerConnection();
  const packetChannel = pc.createDataChannel(GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL, {
    maxRetransmits: 0,
    ordered: false,
  }) as unknown as RTCDataChannel;
  await connectGiznetWebRTCFromEndpoint({
    clientPrivateKey: identity.clientPrivateKey,
    createPacketDataChannel: false,
    endpoint: identity.endpoint,
    pc: pc as unknown as RTCPeerConnection,
  });
  await waitForDataChannelOpen(packetChannel);
  return { packetChannel, pc };
}

export async function loadIdentity(dir: string): Promise<Identity> {
  const config = await readFile(path.join(dir, "config.yaml"), "utf8");
  const privateKey = base58Decode(matchConfig(config, /private-key:\s*"?([^"\s]+)"?/));
  if (privateKey.length !== 32) {
    throw new Error(`identity.private-key length = ${privateKey.length}, want 32`);
  }
  return {
    clientPrivateKey: privateKey,
    endpoint: matchConfig(config, /endpoint:\s*([^\s]+)/),
    publicKey: base58Encode(x25519.getPublicKey(privateKey)),
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
      `gizclaw e2e setup server is required at ${endpoint}; start the Docker e2e stack before this JS e2e test`,
      { cause: err },
    );
  }
}

export function closePeerConnection(pc: wrtc.RTCPeerConnection): void {
  pc.close();
}

function waitForDataChannelOpen(channel: RTCDataChannel): Promise<void> {
  if (channel.readyState === "open") {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup();
      reject(new Error(`packet data channel readyState is ${channel.readyState}, want open`));
    }, 10_000);
    const onOpen = (): void => {
      cleanup();
      resolve();
    };
    const onClose = (): void => {
      cleanup();
      reject(new Error("packet data channel closed before opening"));
    };
    const cleanup = (): void => {
      clearTimeout(timer);
      channel.removeEventListener("open", onOpen);
      channel.removeEventListener("close", onClose);
    };
    channel.addEventListener("open", onOpen);
    channel.addEventListener("close", onClose);
  });
}

function matchConfig(config: string, pattern: RegExp): string {
  const match = config.match(pattern);
  if (match?.[1] == null) {
    throw new Error(`missing config field matching ${pattern}`);
  }
  return match[1].trim();
}
