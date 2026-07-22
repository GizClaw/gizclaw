import assert from "node:assert/strict";
import test from "node:test";

import {
  parsePixa,
  pixaClipFrameIndex,
  renderPixaFrameRGBA,
  selectPixaClip,
  validatePixa,
  PixaParseError,
} from "./index.ts";

test("parsePixa reads a valid PIXA header, clips, and frames", () => {
  const asset = parsePixa(
    makePixa({ clips: ["idle", "feed"], height: 96, width: 120 }),
  );

  assert.equal(asset.version, 1);
  assert.deepEqual(asset.canvas, {
    height: 96,
    pixelCount: 11520,
    rgb565ByteCount: 23040,
    width: 120,
  });
  assert.equal(asset.colorCount, 1);
  assert.equal(asset.clipCount, 2);
  assert.equal(asset.frameCount, 1);
  assert.equal(asset.payloadLength, 4);
  assert.deepEqual(
    asset.clips.map((clip) => ({
      frameCount: clip.frameCount,
      loop: clip.loop,
      name: clip.name,
      totalDurationMs: clip.totalDurationMs,
    })),
    [
      { frameCount: 1, loop: true, name: "idle", totalDurationMs: 120 },
      { frameCount: 1, loop: true, name: "feed", totalDurationMs: 120 },
    ],
  );
  assert.deepEqual(asset.frames, [
    {
      durationMs: 120,
      payloadLength: 4,
      payloadOffset: 0,
      type: "key",
      typeCode: 0,
    },
  ]);
});

test("parsePixa accepts typed array views with byte offsets", () => {
  const pixa = makePixa({ clips: ["idle"], height: 16, width: 16 });
  const padded = new Uint8Array(pixa.byteLength + 8);
  padded.set(new Uint8Array(pixa), 4);

  const asset = parsePixa(padded.subarray(4, 4 + pixa.byteLength));

  assert.equal(asset.canvas.width, 16);
  assert.equal(asset.clips[0]?.name, "idle");
});

test("parsePixa rejects invalid files", () => {
  assert.throws(() => parsePixa(new Uint8Array()), PixaParseError);
  assert.throws(
    () => parsePixa(new TextEncoder().encode("ZPET")),
    PixaParseError,
  );
  assert.throws(
    () =>
      parsePixa(
        makePixa({
          clips: ["idle"],
          mutate(data) {
            new DataView(data).setUint16(4, 2, true);
          },
        }),
      ),
    /Unsupported PIXA version/,
  );
  assert.throws(
    () =>
      parsePixa(
        makePixa({
          clips: ["idle"],
          mutate(data) {
            new DataView(data).setUint32(36, 9999, true);
          },
        }),
      ),
    /payload range/,
  );
});

test("validatePixa enforces PetDef and BadgeDef clip contracts", () => {
  assert.equal(
    validatePixa(makePixa({ clips: ["idle", "feed"] }), "petdef").clips.length,
    2,
  );
  assert.equal(
    validatePixa(makePixa({ clips: ["icon"] }), "badgedef").clips[0]?.name,
    "icon",
  );
  assert.throws(
    () => validatePixa(makePixa({ clips: ["feed"] }), "petdef"),
    /idle/,
  );
  assert.throws(
    () => validatePixa(makePixa({ clips: ["idle"] }), "badgedef"),
    /icon/,
  );
});

test("selectPixaClip and pixaClipFrameIndex choose stable animation frames", () => {
  const asset = parsePixa(
    makePixa({ clips: ["idle", "bath"], height: 1, width: 2 }),
  );

  assert.equal(selectPixaClip(asset, "bath")?.name, "bath");
  assert.equal(selectPixaClip(asset, "missing")?.name, "idle");
  const clip = selectPixaClip(asset, "bath");
  assert.ok(clip);
  assert.equal(pixaClipFrameIndex(clip, 0), 0);
  assert.equal(pixaClipFrameIndex(clip, 250), 0);
});

test("renderPixaFrameRGBA decodes RGB565 key frames", () => {
  const asset = parsePixa(makePixa({ clips: ["idle"], height: 1, width: 2 }));
  const frame = renderPixaFrameRGBA(asset, 0);

  assert.equal(frame.width, 2);
  assert.equal(frame.height, 1);
  assert.deepEqual(Array.from(frame.data), [255, 0, 0, 255, 0, 255, 0, 255]);
});

type MakePixaOptions = {
  clips: string[];
  height?: number;
  mutate?: (data: ArrayBuffer) => void;
  width?: number;
};

function makePixa({
  clips,
  height = 16,
  mutate,
  width = 16,
}: MakePixaOptions): ArrayBuffer {
  const headerSize = 40;
  const clipEntrySize = 56;
  const frameEntrySize = 16;
  const paletteOffset = headerSize;
  const clipOffset = paletteOffset + 2;
  const frameOffset = clipOffset + clips.length * clipEntrySize;
  const payloadOffset = frameOffset + frameEntrySize;
  const payloadLength = 4;
  const data = new ArrayBuffer(payloadOffset + payloadLength);
  const bytes = new Uint8Array(data);
  const view = new DataView(data);

  bytes.set(new TextEncoder().encode("PIXA"), 0);
  view.setUint16(4, 1, true);
  view.setUint16(6, headerSize, true);
  view.setUint16(8, width, true);
  view.setUint16(10, height, true);
  view.setUint16(12, 1, true);
  view.setUint16(14, clips.length, true);
  view.setUint32(16, 1, true);
  view.setUint32(20, paletteOffset, true);
  view.setUint32(24, clipOffset, true);
  view.setUint32(28, frameOffset, true);
  view.setUint32(32, payloadOffset, true);
  view.setUint32(36, payloadLength, true);

  for (let i = 0; i < clips.length; i += 1) {
    const base = clipOffset + i * clipEntrySize;
    bytes.set(new TextEncoder().encode(clips[i] ?? ""), base);
    view.setUint32(base + 40, 1, true);
    view.setUint32(base + 44, 120, true);
    view.setUint16(base + 48, 1, true);
  }

  view.setUint16(frameOffset, 120, true);
  view.setUint8(frameOffset + 2, 0);
  view.setUint32(frameOffset + 4, 0, true);
  view.setUint32(frameOffset + 8, payloadLength, true);
  bytes.set([0x00, 0xf8, 0xe0, 0x07], payloadOffset);
  mutate?.(data);
  return data;
}
