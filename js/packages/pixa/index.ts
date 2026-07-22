export const PIXA_MAGIC = "PIXA";
export const PIXA_HEADER_SIZE = 40;
export const PIXA_CLIP_ENTRY_SIZE = 56;
export const PIXA_FRAME_ENTRY_SIZE = 16;
export const PIXA_CLIP_NAME_SIZE = 32;
export const PIXA_RGB565_PIXEL_BYTES = 2;

export type PixaCanvas = {
  height: number;
  pixelCount: number;
  rgb565ByteCount: number;
  width: number;
};

export type PixaClip = {
  anchorX: number;
  anchorY: number;
  firstFrame: number;
  frameCount: number;
  loop: boolean;
  name: string;
  totalDurationMs: number;
};

export type PixaFrameType = "key" | "diff" | "unknown";

export type PixaFrame = {
  durationMs: number;
  payloadLength: number;
  payloadOffset: number;
  type: PixaFrameType;
  typeCode: number;
};

export type PixaAsset = {
  bytes: Uint8Array;
  canvas: PixaCanvas;
  clipCount: number;
  clipOffset: number;
  clips: PixaClip[];
  colorCount: number;
  frameCount: number;
  frameOffset: number;
  frames: PixaFrame[];
  paletteOffset: number;
  payloadLength: number;
  payloadOffset: number;
  version: number;
};

export type PixaValidationMode = "petdef" | "badgedef";

export type PixaFrameRGBA = {
  data: Uint8ClampedArray<ArrayBuffer>;
  height: number;
  width: number;
};

export class PixaParseError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PixaParseError";
  }
}

export function parsePixa(input: ArrayBuffer | ArrayBufferView): PixaAsset {
  const view = toDataView(input);
  if (view.byteLength < PIXA_HEADER_SIZE) {
    throw new PixaParseError("Invalid PIXA file: header is too short.");
  }
  if (readAscii(view, 0, 4) !== PIXA_MAGIC) {
    throw new PixaParseError("Invalid PIXA magic.");
  }

  const version = view.getUint16(4, true);
  if (version !== 1) {
    throw new PixaParseError(`Unsupported PIXA version ${version}.`);
  }
  const headerSize = view.getUint16(6, true);
  if (headerSize !== PIXA_HEADER_SIZE) {
    throw new PixaParseError(`Invalid PIXA header size ${headerSize}.`);
  }

  const width = view.getUint16(8, true);
  const height = view.getUint16(10, true);
  const colorCount = view.getUint16(12, true);
  const clipCount = view.getUint16(14, true);
  const frameCount = view.getUint32(16, true);
  const paletteOffset = view.getUint32(20, true);
  const clipOffset = view.getUint32(24, true);
  const frameOffset = view.getUint32(28, true);
  const payloadOffset = view.getUint32(32, true);
  const payloadLength = view.getUint32(36, true);

  requireRange(view, paletteOffset, colorCount * 2, "palette");
  requireRange(
    view,
    clipOffset,
    clipCount * PIXA_CLIP_ENTRY_SIZE,
    "clip table",
  );
  requireRange(
    view,
    frameOffset,
    frameCount * PIXA_FRAME_ENTRY_SIZE,
    "frame table",
  );
  requireRange(view, payloadOffset, payloadLength, "payload");

  const clips = parseClips(view, clipOffset, clipCount, frameCount);
  const frames = parseFrames(view, frameOffset, frameCount, payloadLength);
  const pixelCount = width * height;

  return {
    bytes: new Uint8Array(view.buffer, view.byteOffset, view.byteLength),
    canvas: {
      height,
      pixelCount,
      rgb565ByteCount: pixelCount * PIXA_RGB565_PIXEL_BYTES,
      width,
    },
    clipCount,
    clipOffset,
    clips,
    colorCount,
    frameCount,
    frameOffset,
    frames,
    paletteOffset,
    payloadLength,
    payloadOffset,
    version,
  };
}

export function validatePixa(
  input: ArrayBuffer | ArrayBufferView,
  mode?: PixaValidationMode,
): PixaAsset {
  const asset = parsePixa(input);
  if (mode === "petdef") {
    if (asset.clipCount === 0 || asset.frameCount === 0) {
      throw new PixaParseError(
        "PetDef PIXA must contain at least one clip and one frame.",
      );
    }
    if (findPixaClip(asset, "idle") == null) {
      throw new PixaParseError('PetDef PIXA must contain an "idle" clip.');
    }
  }
  if (mode === "badgedef") {
    const icon = findPixaClip(asset, "icon");
    if (icon == null) {
      throw new PixaParseError('BadgeDef PIXA must contain an "icon" clip.');
    }
    if (icon.frameCount !== 1) {
      throw new PixaParseError(
        "BadgeDef icon clip must contain exactly one frame.",
      );
    }
    const frame = asset.frames[icon.firstFrame];
    if (frame == null || frame.type !== "key") {
      throw new PixaParseError(
        "BadgeDef icon clip must reference a key frame.",
      );
    }
  }
  return asset;
}

export function findPixaClip(
  asset: PixaAsset,
  name: string,
): PixaClip | undefined {
  return asset.clips.find((clip) => clip.name === name);
}

export function selectPixaClip(
  asset: PixaAsset,
  preferred?: string,
): PixaClip | undefined {
  if (preferred != null && preferred.trim() !== "") {
    const selected = findPixaClip(asset, preferred.trim());
    if (selected != null) {
      return selected;
    }
  }
  return findPixaClip(asset, "idle") ?? asset.clips[0];
}

export function pixaClipFrameIndex(clip: PixaClip, elapsedMs: number): number {
  if (clip.frameCount <= 0) {
    return clip.firstFrame;
  }
  if (clip.totalDurationMs <= 0) {
    return clip.firstFrame;
  }
  const elapsed = clip.loop
    ? positiveModulo(elapsedMs, clip.totalDurationMs)
    : Math.min(Math.max(elapsedMs, 0), Math.max(clip.totalDurationMs - 1, 0));
  const averageFrameMs = clip.totalDurationMs / clip.frameCount;
  return (
    clip.firstFrame +
    Math.min(clip.frameCount - 1, Math.floor(elapsed / averageFrameMs))
  );
}

export function renderPixaFrameRGBA(
  asset: PixaAsset,
  frameIndex: number,
): PixaFrameRGBA {
  const frame = asset.frames[frameIndex];
  if (frame == null) {
    throw new PixaParseError(`PIXA frame ${frameIndex} does not exist.`);
  }
  if (frame.type !== "key") {
    throw new PixaParseError(
      `PIXA frame ${frameIndex} is ${frame.type}; only key frames can be rendered.`,
    );
  }
  if (frame.payloadLength < asset.canvas.rgb565ByteCount) {
    throw new PixaParseError(
      `PIXA key frame payload is ${frame.payloadLength} bytes, expected ${asset.canvas.rgb565ByteCount}.`,
    );
  }
  const start = asset.payloadOffset + frame.payloadOffset;
  const end = start + asset.canvas.rgb565ByteCount;
  if (end > asset.bytes.byteLength) {
    throw new PixaParseError("PIXA key frame payload exceeds file length.");
  }
  const rgba = new Uint8ClampedArray(
    new ArrayBuffer(asset.canvas.pixelCount * 4),
  );
  for (let pixel = 0; pixel < asset.canvas.pixelCount; pixel += 1) {
    const source = start + pixel * PIXA_RGB565_PIXEL_BYTES;
    const value = asset.bytes[source] | (asset.bytes[source + 1] << 8);
    const target = pixel * 4;
    rgba[target] = (((value >> 11) & 0x1f) * 255) / 31;
    rgba[target + 1] = (((value >> 5) & 0x3f) * 255) / 63;
    rgba[target + 2] = ((value & 0x1f) * 255) / 31;
    rgba[target + 3] = 255;
  }
  return { data: rgba, height: asset.canvas.height, width: asset.canvas.width };
}

export function drawPixaFrame(
  ctx: CanvasRenderingContext2D,
  asset: PixaAsset,
  frameIndex: number,
): void {
  const frame = renderPixaFrameRGBA(asset, frameIndex);
  const image = new ImageData(frame.data, frame.width, frame.height);
  ctx.canvas.width = frame.width;
  ctx.canvas.height = frame.height;
  ctx.putImageData(image, 0, 0);
}

function parseClips(
  view: DataView,
  clipOffset: number,
  clipCount: number,
  frameCount: number,
): PixaClip[] {
  const clips: PixaClip[] = [];
  for (let index = 0; index < clipCount; index += 1) {
    const base = clipOffset + index * PIXA_CLIP_ENTRY_SIZE;
    const firstFrame = view.getUint32(base + 36, true);
    const clipFrameCount = view.getUint32(base + 40, true);
    if (firstFrame > frameCount || clipFrameCount > frameCount - firstFrame) {
      throw new PixaParseError("Invalid PIXA clip frame range.");
    }
    clips.push({
      anchorX: view.getInt16(base + 32, true),
      anchorY: view.getInt16(base + 34, true),
      firstFrame,
      frameCount: clipFrameCount,
      loop: (view.getUint16(base + 48, true) & 1) !== 0,
      name: readNullTerminatedUtf8(view, base, PIXA_CLIP_NAME_SIZE),
      totalDurationMs: view.getUint32(base + 44, true),
    });
  }
  return clips;
}

function parseFrames(
  view: DataView,
  frameOffset: number,
  frameCount: number,
  payloadLength: number,
): PixaFrame[] {
  const frames: PixaFrame[] = [];
  for (let index = 0; index < frameCount; index += 1) {
    const base = frameOffset + index * PIXA_FRAME_ENTRY_SIZE;
    const payloadOffset = view.getUint32(base + 4, true);
    const framePayloadLength = view.getUint32(base + 8, true);
    if (
      payloadOffset > payloadLength ||
      framePayloadLength > payloadLength - payloadOffset
    ) {
      throw new PixaParseError("Invalid PIXA frame payload range.");
    }
    const typeCode = view.getUint8(base + 2);
    frames.push({
      durationMs: view.getUint16(base, true),
      payloadLength: framePayloadLength,
      payloadOffset,
      type: frameType(typeCode),
      typeCode,
    });
  }
  return frames;
}

function frameType(typeCode: number): PixaFrameType {
  if (typeCode === 0) {
    return "key";
  }
  if (typeCode === 1) {
    return "diff";
  }
  return "unknown";
}

function requireRange(
  view: DataView,
  offset: number,
  length: number,
  label: string,
): void {
  if (
    !Number.isSafeInteger(offset) ||
    !Number.isSafeInteger(length) ||
    offset < 0 ||
    length < 0 ||
    offset > view.byteLength ||
    length > view.byteLength - offset
  ) {
    throw new PixaParseError(`Invalid PIXA ${label} range.`);
  }
}

function positiveModulo(value: number, divisor: number): number {
  return ((value % divisor) + divisor) % divisor;
}

function toDataView(input: ArrayBuffer | ArrayBufferView): DataView {
  if (input instanceof ArrayBuffer) {
    return new DataView(input);
  }
  return new DataView(input.buffer, input.byteOffset, input.byteLength);
}

function readAscii(view: DataView, offset: number, length: number): string {
  let out = "";
  for (let i = 0; i < length; i += 1) {
    out += String.fromCharCode(view.getUint8(offset + i));
  }
  return out;
}

function readNullTerminatedUtf8(
  view: DataView,
  offset: number,
  maxLength: number,
): string {
  let length = 0;
  while (length < maxLength && view.getUint8(offset + length) !== 0) {
    length += 1;
  }
  return new TextDecoder().decode(
    new Uint8Array(view.buffer, view.byteOffset + offset, length),
  );
}
