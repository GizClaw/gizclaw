import { ed25519, x25519 } from "@noble/curves/ed25519.js";
import { bytesToNumberLE, concatBytes } from "@noble/curves/utils.js";
import { sha512 } from "@noble/hashes/sha2.js";
import { randomBytes } from "@noble/hashes/utils.js";
import { base58Decode, base58Encode } from "./signaling.ts";

const prefix = "gizclaw_sig_v1_";

function keyBytes(text: string, name: string): Uint8Array {
  // Check bounds before decoding, and never include a private key in errors.
  try {
    if (text.length < 32 || text.length > 44) throw new Error();
    const bytes = base58Decode(text);
    if (
      bytes.length !== 32 ||
      bytes.every((byte) => byte === 0) ||
      base58Encode(bytes) !== text
    )
      throw new Error();
    return bytes;
  } catch {
    throw new TypeError(`${name} must be a nonzero 32-byte Base58 Giznet key`);
  }
}

function base64url(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/g, "");
}

/**
 * Signs target-bound Peer HTTP Bearers locally with an existing Base58 Giznet
 * X25519 private key. No key is uploaded or issued. The Server rechecks the
 * caller's current authority on every request; signing grants no permissions.
 * Wire format is owned by api/http/peer.json's x-peer-http-signed-bearer.
 */
export function createPeerHTTPSigner(privateKey: string): {
  publicKey: string;
  sign(targetPublicKey: string, lifetimeSeconds?: number): string;
} {
  const bytes = keyBytes(privateKey, "privateKey");
  const publicKey = base58Encode(x25519.getPublicKey(bytes));
  bytes[0] &= 248;
  bytes[31] = (bytes[31] & 127) | 64;
  const { Fn, BASE } = ed25519.Point;
  const scalar = Fn.create(bytesToNumberLE(bytes));
  bytes.fill(0);
  const edPublicKey = BASE.multiply(scalar).toBytes();
  const a = Fn.cmov(scalar, Fn.neg(scalar), (edPublicKey[31] & 128) !== 0);
  edPublicKey[31] &= 127;

  return {
    publicKey,
    sign(targetPublicKey, lifetimeSeconds = 60) {
      keyBytes(targetPublicKey, "targetPublicKey");
      if (
        !Number.isInteger(lifetimeSeconds) ||
        lifetimeSeconds < 1 ||
        lifetimeSeconds > 300
      )
        throw new TypeError("Bearer lifetime must be 1 to 300 seconds");
      const now = Math.floor(Date.now() / 1000);
      if (!Number.isSafeInteger(now) || now <= 0)
        throw new TypeError("Bearer requires a valid current time");
      const payload = base64url(
        new TextEncoder().encode(
          JSON.stringify({
            iss: publicKey,
            aud: "gizclaw/peer-http/v1",
            target: targetPublicKey,
            iat: now,
            exp: now + lifetimeSeconds,
          }),
        ),
      );
      const message = `${prefix}${payload}`;
      const encoded = new TextEncoder().encode(message);
      // Signal XEdDSA §3: hash_1(a || M || Z), with fresh secret 64-byte Z.
      // https://signal.org/docs/specifications/xeddsa/#xeddsa
      const domain = new Uint8Array(32).fill(255);
      domain[0] = 254;
      const nonceHash = sha512
        .create()
        .update(domain)
        .update(Fn.toBytes(a))
        .update(encoded)
        .update(randomBytes(64))
        .digest();
      const r = Fn.create(bytesToNumberLE(nonceHash));
      const R =
        r === 0n ? ed25519.Point.ZERO.toBytes() : BASE.multiply(r).toBytes();
      const h = Fn.create(
        bytesToNumberLE(sha512(concatBytes(R, edPublicKey, encoded))),
      );
      const s = Fn.add(r, Fn.mul(h, a));
      return `${message}.${base64url(concatBytes(R, Fn.toBytes(s)))}`;
    },
  };
}
