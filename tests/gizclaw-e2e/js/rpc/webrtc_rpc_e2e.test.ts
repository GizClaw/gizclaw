import assert from "node:assert/strict";
import { createCipheriv, createDecipheriv, createPrivateKey, createPublicKey, diffieHellman, hkdfSync } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";

import wrtc from "@roamhq/wrtc";
import { WebRTCRPCClient, connectGiznetWebRTC, sendGiznetWebRTCOffer } from "@gizclaw/webrtc";

const repoRoot = path.resolve(import.meta.dirname, "../../../..");
const identityDir = process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");
const signalingPath = "/webrtc/v1/offer";

test("Node WebRTC SDK connects to setup server and runs all.ping", async (t) => {
  const identity = await loadIdentity(identityDir);
  if (!(await serverAvailable(identity.endpoint))) {
    t.skip(`e2e setup server is not available at ${identity.endpoint}`);
    return;
  }

  const pc = new wrtc.RTCPeerConnection();
  try {
    await connectGiznetWebRTC({
      pc: pc as unknown as RTCPeerConnection,
      prepareOffer: async (offerSDP) => prepareEncryptedOffer(identity, offerSDP),
      sendOffer: (offer, signal) => sendGiznetWebRTCOffer(offer, { baseUrl: `http://${identity.endpoint}`, signal }),
    });

    const rpc = new WebRTCRPCClient(pc as unknown as RTCPeerConnection, {
      createID: () => "js.all.ping",
      requestTimeoutMs: 10_000,
    });
    const result = await rpc.call<{ server_time: number }>("all.ping", {
      client_send_time: Date.now(),
    });

    assert.equal(typeof result.server_time, "number");
    assert.ok(result.server_time > 0);
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
});

type Identity = {
  clientPrivateKey: Buffer;
  clientPublicKey: Buffer;
  endpoint: string;
  serverPublicKey: Buffer;
};

async function loadIdentity(dir: string): Promise<Identity> {
  const [config, privateKey] = await Promise.all([
    readFile(path.join(dir, "config.yaml"), "utf8"),
    readFile(path.join(dir, "identity.key")),
  ]);
  if (privateKey.length !== 32) {
    throw new Error(`identity.key length = ${privateKey.length}, want 32`);
  }
  const endpoint = matchConfig(config, /endpoint:\s*([^\s]+)/);
  const serverPublicKey = base58Decode(matchConfig(config, /public-key:\s*"?([^"\s]+)"?/));
  const clientPublicKey = x25519PublicKey(privateKey);
  return {
    clientPrivateKey: Buffer.from(privateKey),
    clientPublicKey,
    endpoint,
    serverPublicKey,
  };
}

async function serverAvailable(endpoint: string): Promise<boolean> {
  try {
    const response = await fetch(`http://${endpoint}/api/public/server-info`, { signal: AbortSignal.timeout(1000) });
    return response.ok;
  } catch {
    return false;
  }
}

async function prepareEncryptedOffer(identity: Identity, offerSDP: string) {
  const nonceBytes = crypto.getRandomValues(new Uint8Array(16));
  const nonce = Buffer.from(nonceBytes).toString("base64url");
  const timestamp = Math.floor(Date.now() / 1000);
  const keys = deriveSignalingKeys(identity, nonce, timestamp);
  const aad = signalingAAD(identity.clientPublicKey, timestamp, nonce, false);
  const body = sealChaCha20Poly1305(keys.requestKey, keys.requestNonce, Buffer.from(offerSDP), aad);
  return {
    body: new Blob([body]),
    clientPublicKey: base58Encode(identity.clientPublicKey),
    nonce,
    openAnswer: async (encryptedAnswer: Blob) => {
      const answer = openChaCha20Poly1305(
        keys.responseKey,
        keys.responseNonce,
        Buffer.from(await encryptedAnswer.arrayBuffer()),
        signalingAAD(identity.clientPublicKey, timestamp, nonce, true),
      );
      return answer.toString("utf8");
    },
    timestamp,
  };
}

function deriveSignalingKeys(identity: Identity, nonce: string, timestamp: number) {
  const shared = x25519DH(identity.clientPrivateKey, identity.serverPublicKey);
  const salt = Buffer.concat([Buffer.from(nonce, "base64url"), Buffer.from(String(timestamp))]);
  return {
    requestKey: hkdf(shared, salt, "giznet/gizwebrtc/http-signaling/v1 c2s", 32),
    requestNonce: hkdf(shared, salt, "giznet/gizwebrtc/http-signaling/v1 c2s nonce", 12),
    responseKey: hkdf(shared, salt, "giznet/gizwebrtc/http-signaling/v1 s2c", 32),
    responseNonce: hkdf(shared, salt, "giznet/gizwebrtc/http-signaling/v1 s2c nonce", 12),
  };
}

function x25519DH(privateKey: Buffer, publicKey: Buffer): Buffer {
  return diffieHellman({
    privateKey: x25519PrivateKeyObject(privateKey),
    publicKey: x25519PublicKeyObject(publicKey),
  });
}

function x25519PublicKey(privateKey: Buffer): Buffer {
  const spki = createPublicKey(x25519PrivateKeyObject(privateKey)).export({ format: "der", type: "spki" });
  return Buffer.from(spki).subarray(-32);
}

function x25519PrivateKeyObject(privateKey: Buffer) {
  return createPrivateKey({
    format: "der",
    key: Buffer.concat([Buffer.from("302e020100300506032b656e04220420", "hex"), privateKey]),
    type: "pkcs8",
  });
}

function x25519PublicKeyObject(publicKey: Buffer) {
  return createPublicKey({
    format: "der",
    key: Buffer.concat([Buffer.from("302a300506032b656e032100", "hex"), publicKey]),
    type: "spki",
  });
}

function hkdf(shared: Buffer, salt: Buffer, info: string, length: number): Buffer {
  return Buffer.from(hkdfSync("sha256", shared, salt, Buffer.from(info), length));
}

function sealChaCha20Poly1305(key: Buffer, nonce: Buffer, plaintext: Buffer, aad: Buffer): Buffer {
  const cipher = createCipheriv("chacha20-poly1305", key, nonce, { authTagLength: 16 });
  cipher.setAAD(aad);
  return Buffer.concat([cipher.update(plaintext), cipher.final(), cipher.getAuthTag()]);
}

function openChaCha20Poly1305(key: Buffer, nonce: Buffer, ciphertext: Buffer, aad: Buffer): Buffer {
  const tag = ciphertext.subarray(ciphertext.length - 16);
  const body = ciphertext.subarray(0, ciphertext.length - 16);
  const decipher = createDecipheriv("chacha20-poly1305", key, nonce, { authTagLength: 16 });
  decipher.setAAD(aad);
  decipher.setAuthTag(tag);
  return Buffer.concat([decipher.update(body), decipher.final()]);
}

function signalingAAD(clientPublicKey: Buffer, timestamp: number, nonce: string, answer: boolean): Buffer {
  const parts = ["POST", signalingPath, base58Encode(clientPublicKey), String(timestamp), nonce];
  if (answer) {
    parts.push("answer");
  }
  return Buffer.from(parts.join("\n"));
}

function matchConfig(config: string, pattern: RegExp): string {
  const match = config.match(pattern);
  if (match?.[1] == null) {
    throw new Error(`missing config field matching ${pattern}`);
  }
  return match[1].trim();
}

function closePeerConnection(pc: wrtc.RTCPeerConnection): void {
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

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";
const base58Map = new Map([...base58Alphabet].map((char, index) => [char, index]));

function base58Decode(text: string): Buffer {
  let value = 0n;
  for (const char of text) {
    const digit = base58Map.get(char);
    if (digit == null) {
      throw new Error(`invalid base58 character ${char}`);
    }
    value = value * 58n + BigInt(digit);
  }
  const bytes: number[] = [];
  while (value > 0n) {
    bytes.push(Number(value & 0xffn));
    value >>= 8n;
  }
  for (const char of text) {
    if (char !== "1") {
      break;
    }
    bytes.push(0);
  }
  return Buffer.from(bytes.reverse());
}

function base58Encode(bytes: Uint8Array): string {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) + BigInt(byte);
  }
  let text = "";
  while (value > 0n) {
    const mod = Number(value % 58n);
    text = base58Alphabet[mod] + text;
    value /= 58n;
  }
  for (const byte of bytes) {
    if (byte !== 0) {
      break;
    }
    text = "1" + text;
  }
  return text || "1";
}
