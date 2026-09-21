import assert from "node:assert/strict";
import test from "node:test";
import { chacha20poly1305 } from "@noble/ciphers/chacha.js";
import { x25519 } from "@noble/curves/ed25519.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { sha256 } from "@noble/hashes/sha2.js";
import {
  base58Decode,
  encodeAdmissionCredential,
  prepareEncryptedGiznetWebRTCOffer,
  GIZNET_MAX_CREDENTIAL_VALUE_BYTES,
} from "./signaling.ts";

import { registrationTokenCredential } from "./index.ts";

const clientPrivateKey = new Uint8Array(32).fill(1);
const serverPrivateKey = new Uint8Array(32).fill(2);
const identity = {
  clientPrivateKey,
  serverPublicKey: x25519.getPublicKey(serverPrivateKey),
};

test("encrypted offer carries the shared admission vector and keeps legacy SDP", async () => {
  for (const credential of [
    undefined,
    registrationTokenCredential("token"),
    { version: 1, type: "x", value: "x".repeat(512) },
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
    if (credential == null) {
      assert.equal(new TextDecoder().decode(plain), "v=0\r\n");
    } else if (credential.value === "token") {
      assert.equal(
        Buffer.from(plain).toString("hex"),
        "475a4f460100290801121e67697a636c61772e636f6d2f726567697374726174696f6e5f746f6b656e1a05746f6b656e763d300d0a",
      );
    } else {
      assert.equal(
        new DataView(plain.buffer, plain.byteOffset).getUint16(5),
        520,
      );
      assert.deepEqual(
        plain.slice(7, -5),
        encodeAdmissionCredential(credential),
      );
    }
    encrypted[8] ^= 1;
    assert.throws(() => chacha20poly1305(key, nonce, aad).decrypt(encrypted));
  }
});

test("oversized credential is rejected without including its contents", async () => {
  await assert.rejects(
    prepareEncryptedGiznetWebRTCOffer(identity, "v=0", {
      version: 1,
      type: "example.com/test",
      value: "x".repeat(GIZNET_MAX_CREDENTIAL_VALUE_BYTES + 1),
    }),
    /invalid admission credential length/,
  );
});

test("generated credential encodings match Go, Dart and nanopb", () => {
  for (const [credential, hex] of [
    [
      registrationTokenCredential("token"),
      "0801121e67697a636c61772e636f6d2f726567697374726174696f6e5f746f6b656e1a05746f6b656e",
    ],
    [
      { version: 2, type: "custom", value: "令牌" },
      "08021206637573746f6d1a06e4bba4e7898c",
    ],
    [{ version: 0, type: "x", value: "" }, "120178"],
  ] as const) {
    assert.equal(
      Buffer.from(encodeAdmissionCredential(credential)!).toString("hex"),
      hex,
    );
  }
  for (const credential of [
    { version: 0, type: "", value: "" },
    { version: 1, type: "x", value: "x".repeat(513) },
    { version: 1, type: "x".repeat(129), value: "" },
    { version: 1, type: "令".repeat(43), value: "" },
  ]) {
    assert.throws(
      () => encodeAdmissionCredential(credential),
      /invalid admission credential length/,
    );
  }
});

test("registration token helper validates the UTF-8 byte boundary at construction", () => {
  for (const value of ["x".repeat(512), "é".repeat(256)]) {
    assert.equal(registrationTokenCredential(value).value, value);
    assert.throws(
      () => registrationTokenCredential(value + "x"),
      /invalid admission credential length/,
    );
  }
});

test("oversized SDP is rejected before envelope allocation", async () => {
  for (const credential of [undefined, registrationTokenCredential("token")]) {
    for (const sdp of ["x".repeat(258026), "é".repeat(129013)]) {
      await assert.rejects(
        prepareEncryptedGiznetWebRTCOffer(identity, sdp, credential),
        /offer SDP exceeds signaling limit/,
      );
    }
  }
});
