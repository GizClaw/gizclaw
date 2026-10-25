import assert from "node:assert/strict";
import { verify } from "node:crypto";
import { readFileSync } from "node:fs";
import test from "node:test";
import { createPeerHTTPSigner } from "./peerhttp.ts";

// Synthetic keys covering both Edwards sign bits, fixed time and fixed entropy. Produced independently with
// go.step.sm/crypto/x25519.Sign, also verified by Go's signed Bearer tests.
const vectors = JSON.parse(
  readFileSync(
    new URL("./testdata/signed-bearer.json", import.meta.url),
    "utf8",
  ),
) as {
  privateKey: string;
  publicKey: string;
  target: string;
  edPublicKey: string;
  random: string;
  now: number;
  token: string;
}[];

test("local signing matches Go XEdDSA vectors and native Ed25519 verification", (t) => {
  for (const v of vectors) {
    t.mock.method(Date, "now", () => v.now * 1000);
    const entropy = Buffer.from(v.random, "hex");
    t.mock.method(crypto, "getRandomValues", (bytes: Uint8Array) => {
      for (let i = 0; i < bytes.length; i++) bytes[i] = entropy[i % 64];
      return bytes;
    });
    const signer = createPeerHTTPSigner(v.privateKey);
    assert.equal(signer.publicKey, v.publicKey);
    const token = signer.sign(v.target);
    assert.equal(token, v.token);
    const [message, signature] = token.split(".");
    assert.ok(
      verify(
        null,
        Buffer.from(message),
        {
          key: Buffer.from(`302a300506032b6570032100${v.edPublicKey}`, "hex"),
          format: "der",
          type: "spki",
        },
        Buffer.from(signature, "base64url"),
      ),
    );
    t.mock.restoreAll();
  }
});

test("signing uses fresh randomness, current time and the requested target", (t) => {
  const v = vectors[0];
  let now = v.now;
  t.mock.method(Date, "now", () => now * 1000);
  const signer = createPeerHTTPSigner(v.privateKey);
  const first = signer.sign(v.target);
  const second = signer.sign(v.target);
  assert.notEqual(first, second);
  now += 3600;
  const fresh = signer.sign(v.publicKey, 300);
  const claims = JSON.parse(
    Buffer.from(
      fresh.split(".")[0].slice("gizclaw_sig_v1_".length),
      "base64url",
    ).toString(),
  );
  assert.deepEqual(claims, {
    iss: v.publicKey,
    aud: "gizclaw/peer-http/v1",
    target: v.publicKey,
    iat: now,
    exp: now + 300,
  });
  assert.ok(!fresh.includes(v.privateKey));
});

test("invalid keys, targets, clocks and unbounded lifetimes fail locally without disclosing the key", (t) => {
  for (const key of ["", "secret-with-0", "1".repeat(32), "2".repeat(10000)]) {
    assert.throws(() => createPeerHTTPSigner(key), {
      message: "privateKey must be a nonzero 32-byte Base58 Giznet key",
    });
  }
  const signer = createPeerHTTPSigner(vectors[0].privateKey);
  for (const target of [
    "",
    "1".repeat(32),
    "wrong-target",
    vectors[0].target + " ",
  ])
    assert.throws(() => signer.sign(target), /targetPublicKey/);
  for (const lifetime of [0, -1, 0.5, 301, NaN, Infinity])
    assert.throws(() => signer.sign(vectors[0].target, lifetime), /lifetime/);
  t.mock.method(Date, "now", () => 0);
  assert.throws(() => signer.sign(vectors[0].target), /current time/);
});
