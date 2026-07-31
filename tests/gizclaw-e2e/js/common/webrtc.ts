import { readFile } from "node:fs/promises";
import path from "node:path";
import { x25519 } from "@noble/curves/ed25519.js";
import wrtc from "@roamhq/wrtc";
import {
  connectGiznetWebRTCFromEndpoint,
  getGiznetWebRTCPacketDataChannel,
  getGiznetWebRTCPeerEventDataChannel,
} from "@gizclaw/gizclaw";
import { base58Decode, base58Encode } from "@gizclaw/gizclaw/signaling";

export const repoRoot = path.resolve(import.meta.dirname, "../../../..");
const setupConnectTimeoutMs = 30_000;

export type Identity = {
  clientPrivateKey: Uint8Array;
  endpoint: string;
  publicKey: string;
};

export type ObservedDataChannel = {
  channel: RTCDataChannel;
  init?: RTCDataChannelInit;
  label: string;
};

export type SetupPeerTransports = {
  createdChannels: ObservedDataChannel[];
  eventChannel: RTCDataChannel;
  packetChannel: RTCDataChannel;
  pc: wrtc.RTCPeerConnection;
};

export async function connectSetupPeer(
  identityDir: string,
): Promise<wrtc.RTCPeerConnection> {
  const identity = await loadIdentity(identityDir);
  const pc = new wrtc.RTCPeerConnection();
  try {
    await connectGiznetWebRTCFromEndpoint({
      clientPrivateKey: identity.clientPrivateKey,
      endpoint: identity.endpoint,
      pc: pc as unknown as RTCPeerConnection,
      signal: AbortSignal.timeout(setupConnectTimeoutMs),
    });
    await new Promise((resolve) => setTimeout(resolve, 100));
    return pc;
  } catch (err) {
    const wrapped = setupConnectError(pc, err);
    closePeerConnection(pc);
    throw wrapped;
  }
}

export async function connectSetupPeerWithTransports(
  identityDir: string,
): Promise<SetupPeerTransports> {
  const identity = await loadIdentity(identityDir);
  const pc = new wrtc.RTCPeerConnection();
  const createdChannels: ObservedDataChannel[] = [];
  const createDataChannel = pc.createDataChannel.bind(pc);
  Object.defineProperty(pc, "createDataChannel", {
    configurable: true,
    value: (label: string, init?: RTCDataChannelInit): RTCDataChannel => {
      const channel = createDataChannel(
        label,
        init,
      ) as unknown as RTCDataChannel;
      createdChannels.push({ channel, init, label });
      return channel;
    },
  });
  try {
    await connectGiznetWebRTCFromEndpoint({
      clientPrivateKey: identity.clientPrivateKey,
      endpoint: identity.endpoint,
      pc: pc as unknown as RTCPeerConnection,
      signal: AbortSignal.timeout(setupConnectTimeoutMs),
    });
    const packetChannel = getGiznetWebRTCPacketDataChannel(
      pc as unknown as RTCPeerConnection,
    ) as RTCDataChannel | undefined;
    const eventChannel = getGiznetWebRTCPeerEventDataChannel(
      pc as unknown as RTCPeerConnection,
    ) as RTCDataChannel | undefined;
    if (packetChannel == null || eventChannel == null) {
      throw new Error("production SDK did not retain its mandatory channels");
    }
    await Promise.all([
      waitForDataChannelOpen(packetChannel),
      waitForDataChannelOpen(eventChannel),
    ]);
    return { createdChannels, eventChannel, packetChannel, pc };
  } catch (err) {
    const wrapped = setupConnectError(pc, err);
    closePeerConnection(pc);
    throw wrapped;
  }
}

export type SetupPeerWithPacketChannel = {
  packetChannel: RTCDataChannel;
  pc: wrtc.RTCPeerConnection;
};

export async function connectSetupPeerWithPacketChannel(
  identityDir: string,
): Promise<SetupPeerWithPacketChannel> {
  const pc = await connectSetupPeer(identityDir);
  try {
    const packetChannel = getGiznetWebRTCPacketDataChannel(
      pc as unknown as RTCPeerConnection,
    ) as RTCDataChannel | undefined;
    if (packetChannel == null) {
      throw new Error(
        "production SDK did not retain its mandatory packet channel",
      );
    }
    await waitForDataChannelOpen(packetChannel);
    return { packetChannel, pc };
  } catch (err) {
    const wrapped = setupConnectError(pc, err);
    closePeerConnection(pc);
    throw wrapped;
  }
}

export async function loadIdentity(dir: string): Promise<Identity> {
  const config = await readFile(path.join(dir, "config.yaml"), "utf8");
  const privateKey = base58Decode(
    matchConfig(config, /private-key:\s*"?([^"\s]+)"?/),
  );
  if (privateKey.length !== 32) {
    throw new Error(
      `identity.private-key length = ${privateKey.length}, want 32`,
    );
  }
  return {
    clientPrivateKey: privateKey,
    endpoint: matchConfig(config, /endpoint:\s*([^\s]+)/),
    publicKey: base58Encode(x25519.getPublicKey(privateKey)),
  };
}

export async function assertSetupServerAvailable(
  endpoint: string,
): Promise<void> {
  try {
    const response = await fetch(`http://${endpoint}/server-info`, {
      signal: AbortSignal.timeout(1000),
    });
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

function setupConnectError(pc: wrtc.RTCPeerConnection, cause: unknown): Error {
  const candidateCount =
    pc.localDescription?.sdp.match(/^a=candidate:/gm)?.length ?? 0;
  return new Error(
    `setup WebRTC connect failed: connection=${pc.connectionState} iceConnection=${pc.iceConnectionState} iceGathering=${pc.iceGatheringState} signaling=${pc.signalingState} localCandidates=${candidateCount}`,
    { cause },
  );
}

export function waitForDataChannelOpen(channel: RTCDataChannel): Promise<void> {
  if (channel.readyState === "open") {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup();
      reject(
        new Error(
          `data channel ${channel.label} readyState is ${channel.readyState}, want open`,
        ),
      );
    }, 10_000);
    const onOpen = (): void => {
      cleanup();
      resolve();
    };
    const onClose = (): void => {
      cleanup();
      reject(new Error(`data channel ${channel.label} closed before opening`));
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
