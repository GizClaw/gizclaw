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
  requireRange(view, clipOffset, clipCount * PIXA_CLIP_ENTRY_SIZE, "clip table");
  requireRange(view, frameOffset, frameCount * PIXA_FRAME_ENTRY_SIZE, "frame table");
  requireRange(view, payloadOffset, payloadLength, "payload");

  const clips = parseClips(view, clipOffset, clipCount, frameCount);
  const frames = parseFrames(view, frameOffset, frameCount, payloadLength);
  const pixelCount = width * height;

  return {
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

function parseClips(view: DataView, clipOffset: number, clipCount: number, frameCount: number): PixaClip[] {
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

function parseFrames(view: DataView, frameOffset: number, frameCount: number, payloadLength: number): PixaFrame[] {
  const frames: PixaFrame[] = [];
  for (let index = 0; index < frameCount; index += 1) {
    const base = frameOffset + index * PIXA_FRAME_ENTRY_SIZE;
    const payloadOffset = view.getUint32(base + 4, true);
    const framePayloadLength = view.getUint32(base + 8, true);
    if (payloadOffset > payloadLength || framePayloadLength > payloadLength - payloadOffset) {
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

function requireRange(view: DataView, offset: number, length: number, label: string): void {
  if (!Number.isSafeInteger(offset) || !Number.isSafeInteger(length) || offset < 0 || length < 0 || offset > view.byteLength || length > view.byteLength - offset) {
    throw new PixaParseError(`Invalid PIXA ${label} range.`);
  }
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

function readNullTerminatedUtf8(view: DataView, offset: number, maxLength: number): string {
  let length = 0;
  while (length < maxLength && view.getUint8(offset + length) !== 0) {
    length += 1;
  }
  return new TextDecoder().decode(new Uint8Array(view.buffer, view.byteOffset + offset, length));
}
