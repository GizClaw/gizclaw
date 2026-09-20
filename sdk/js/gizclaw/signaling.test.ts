import assert from "node:assert/strict";
import test from "node:test";
import { chacha20poly1305 } from "@noble/ciphers/chacha.js";
import { x25519 } from "@noble/curves/ed25519.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { sha256 } from "@noble/hashes/sha2.js";
import {
  base58Decode,
  prepareEncryptedGiznetWebRTCOffer,
  GIZNET_MAX_CREDENTIAL_BYTES,
} from "./signaling.ts";

const clientPrivateKey = new Uint8Array(32).fill(1);
const serverPrivateKey = new Uint8Array(32).fill(2);
const identity = {
  clientPrivateKey,
  serverPublicKey: x25519.getPublicKey(serverPrivateKey),
};

test("encrypted offer carries the shared admission vector and keeps legacy SDP", async () => {
  for (const credential of [
    undefined,
    new Uint8Array(),
    new Uint8Array([0, 255, 1]),
    new Uint8Array(GIZNET_MAX_CREDENTIAL_BYTES).fill(9),
  ]) {
    const prepared = await prepareEncryptedGiznetWebRTCOffer(
      identity,
      "v=0\r\n",
      credential,
    );
    const shared = x25519.getSharedSecret(
      serverPrivateKey,
      base58Decode(prepared.clientPublicKey),
    );
    const salt = Buffer.concat([
      Buffer.from(prepared.nonce, "base64url"),
      Buffer.from(String(prepared.timestamp)),
    ]);
    const key = hkdf(
      sha256,
      shared,
      salt,
      new TextEncoder().encode("giznet/gizwebrtc/http-signaling/v1 c2s"),
      32,
    );
    const nonce = hkdf(
      sha256,
      shared,
      salt,
      new TextEncoder().encode("giznet/gizwebrtc/http-signaling/v1 c2s nonce"),
      12,
    );
    const aad = new TextEncoder().encode(
      `POST\n/webrtc/v1/offer\n${prepared.clientPublicKey}\n${prepared.timestamp}\n${prepared.nonce}`,
    );
    const encrypted = new Uint8Array(await prepared.body.arrayBuffer());
    const plain = chacha20poly1305(key, nonce, aad).decrypt(encrypted);
    if ((credential?.length ?? 0) === 0) {
      assert.equal(new TextDecoder().decode(plain), "v=0\r\n");
    } else if (credential?.length === 3) {
      assert.equal(
        Buffer.from(plain).toString("hex"),
        "475a4f4601000300ff01763d300d0a",
      );
    } else {
      assert.equal(
        new DataView(plain.buffer, plain.byteOffset).getUint16(5),
        GIZNET_MAX_CREDENTIAL_BYTES,
      );
      assert.deepEqual(plain.slice(7, -5), credential);
    }
    encrypted[8] ^= 1;
    assert.throws(() => chacha20poly1305(key, nonce, aad).decrypt(encrypted));
  }
});

test("oversized credential is rejected without including its contents", async () => {
  await assert.rejects(
    prepareEncryptedGiznetWebRTCOffer(
      identity,
      "v=0",
      new Uint8Array(GIZNET_MAX_CREDENTIAL_BYTES + 1),
    ),
    /invalid admission credential length/,
  );
});
