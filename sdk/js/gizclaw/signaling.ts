import { create, toBinary } from "@bufbuild/protobuf";
import {
  AdmissionCredentialSchema,
  type AdmissionCredential as AdmissionCredentialMessage,
} from "./generated/giznet/admission_pb.js";

import { chacha20poly1305 } from "@noble/ciphers/chacha.js";
import { x25519 } from "@noble/curves/ed25519.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { sha256 } from "@noble/hashes/sha2.js";

/** Structured Giznet credential; only the receiving policy interprets fields. */
export type AdmissionCredential = Pick<
  AdmissionCredentialMessage,
  "version" | "type" | "value"
>;

/** Maximum encoded protobuf credential bytes inside an encrypted offer. */
export const GIZNET_MAX_CREDENTIAL_BYTES = 4096;
/** Maximum UTF-8 bytes in the opaque credential value. */
export const GIZNET_MAX_CREDENTIAL_VALUE_BYTES = 512;
/** Leaves room within the 256 KiB signaling body for credentials and AEAD. */
export const GIZNET_MAX_OFFER_SDP_BYTES =
  256 * 1024 - GIZNET_MAX_CREDENTIAL_BYTES - 7 - 16;

const signalingPath = "/webrtc/v1/offer";
const base58Alphabet =
  "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";
const base58Map = new Map(
  [...base58Alphabet].map((char, index) => [char, index]),
);

export type GiznetSignalingIdentity = {
  clientPrivateKey: Uint8Array;
  clientPublicKey?: Uint8Array | string;
  serverPublicKey: Uint8Array | string;
};

export type PreparedGiznetWebRTCOffer = {
  body: Blob | File;
  clientPublicKey: string;
  nonce: string;
  openAnswer: (encryptedAnswer: Blob) => Promise<string>;
  timestamp: number;
};

export async function prepareEncryptedGiznetWebRTCOffer(
  identity: GiznetSignalingIdentity,
  offerSDP: string,
  credential?: AdmissionCredential,
): Promise<PreparedGiznetWebRTCOffer> {
  if (offerSDP.length > GIZNET_MAX_OFFER_SDP_BYTES)
    throw new Error("offer SDP exceeds signaling limit");
  const sdp = new TextEncoder().encode(offerSDP);
  if (sdp.byteLength > GIZNET_MAX_OFFER_SDP_BYTES)
    throw new Error("offer SDP exceeds signaling limit");
  const encoded = encodeAdmissionCredential(credential);
  let plaintext: Uint8Array = sdp;
  if (encoded != null) {
    plaintext = concatBytes([
      new Uint8Array([
        0x47,
        0x5a,
        0x4f,
        0x46,
        1,
        encoded.byteLength >> 8,
        encoded.byteLength & 0xff,
      ]),
      encoded,
      sdp,
    ]);
  }
  const clientPrivateKey = expectKeyBytes(
    identity.clientPrivateKey,
    "client private key",
  );
  const clientPublicKey =
    typeof identity.clientPublicKey === "string"
      ? base58Decode(identity.clientPublicKey)
      : identity.clientPublicKey != null
        ? expectKeyBytes(identity.clientPublicKey, "client public key")
        : x25519.getPublicKey(clientPrivateKey);
  const serverPublicKey =
    typeof identity.serverPublicKey === "string"
      ? base58Decode(identity.serverPublicKey)
      : expectKeyBytes(identity.serverPublicKey, "server public key");
  const nonce = randomNonce();
  const timestamp = Math.floor(Date.now() / 1000);
  const keys = deriveSignalingKeys(
    clientPrivateKey,
    serverPublicKey,
    nonce,
    timestamp,
  );
  const requestAAD = signalingAAD(clientPublicKey, timestamp, nonce, false);
  const body = chacha20poly1305(
    keys.requestKey,
    keys.requestNonce,
    requestAAD,
  ).encrypt(plaintext);

  return {
    body: new Blob([arrayBufferFromBytes(body)]),
    clientPublicKey: base58Encode(clientPublicKey),
    nonce,
    openAnswer: async (encryptedAnswer: Blob) => {
      const encrypted = new Uint8Array(await encryptedAnswer.arrayBuffer());
      const responseAAD = signalingAAD(clientPublicKey, timestamp, nonce, true);
      const answer = chacha20poly1305(
        keys.responseKey,
        keys.responseNonce,
        responseAAD,
      ).decrypt(encrypted);
      return new TextDecoder().decode(answer);
    },
    timestamp,
  };
}

/** Encodes and bounds a credential before discovery, ICE or signaling I/O. */
export function encodeAdmissionCredential(
  credential?: AdmissionCredential,
): Uint8Array | undefined {
  if (credential == null) return undefined;
  const utf8 = new TextEncoder();
  if (
    utf8.encode(credential.type).length > 128 ||
    utf8.encode(credential.value).length > GIZNET_MAX_CREDENTIAL_VALUE_BYTES
  ) {
    throw new Error("invalid admission credential length");
  }
  const encoded = toBinary(
    AdmissionCredentialSchema,
    create(AdmissionCredentialSchema, credential),
  );
  if (encoded.length === 0 || encoded.length > GIZNET_MAX_CREDENTIAL_BYTES) {
    throw new Error("invalid admission credential length");
  }
  return encoded;
}

function deriveSignalingKeys(
  clientPrivateKey: Uint8Array,
  serverPublicKey: Uint8Array,
  nonce: string,
  timestamp: number,
) {
  const shared = x25519.getSharedSecret(clientPrivateKey, serverPublicKey);
  const salt = concatBytes([
    base64URLDecode(nonce),
    new TextEncoder().encode(String(timestamp)),
  ]);
  return {
    requestKey: hkdf(
      sha256,
      shared,
      salt,
      new TextEncoder().encode("giznet/gizwebrtc/http-signaling/v1 c2s"),
      32,
    ),
    requestNonce: hkdf(
      sha256,
      shared,
      salt,
      new TextEncoder().encode("giznet/gizwebrtc/http-signaling/v1 c2s nonce"),
      12,
    ),
    responseKey: hkdf(
      sha256,
      shared,
      salt,
      new TextEncoder().encode("giznet/gizwebrtc/http-signaling/v1 s2c"),
      32,
    ),
    responseNonce: hkdf(
      sha256,
      shared,
      salt,
      new TextEncoder().encode("giznet/gizwebrtc/http-signaling/v1 s2c nonce"),
      12,
    ),
  };
}

function signalingAAD(
  clientPublicKey: Uint8Array,
  timestamp: number,
  nonce: string,
  answer: boolean,
): Uint8Array {
  const parts = [
    "POST",
    signalingPath,
    base58Encode(clientPublicKey),
    String(timestamp),
    nonce,
  ];
  if (answer) {
    parts.push("answer");
  }
  return new TextEncoder().encode(parts.join("\n"));
}

function randomNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return base64URLEncode(bytes);
}

export function base58Decode(text: string): Uint8Array {
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
  return new Uint8Array(bytes.reverse());
}

export function base58Encode(bytes: Uint8Array): string {
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

export function base64Decode(text: string): Uint8Array {
  const binary = atob(text);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

function base64URLDecode(text: string): Uint8Array {
  const padded = text
    .replace(/-/g, "+")
    .replace(/_/g, "/")
    .padEnd(Math.ceil(text.length / 4) * 4, "=");
  return base64Decode(padded);
}

function base64URLEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/g, "");
}

function expectKeyBytes(bytes: Uint8Array, name: string): Uint8Array {
  if (bytes.byteLength !== 32) {
    throw new Error(`invalid ${name} length: ${bytes.byteLength}`);
  }
  return bytes;
}

function concatBytes(parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function arrayBufferFromBytes(data: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(data.byteLength);
  new Uint8Array(out).set(data);
  return out;
}
