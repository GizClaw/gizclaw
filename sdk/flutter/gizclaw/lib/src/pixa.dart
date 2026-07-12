import 'dart:convert';
import 'dart:typed_data';

const pixaMagic = 'PIXA';
const pixaVersion = 1;
const pixaHeaderSize = 40;
const pixaClipEntrySize = 56;
const pixaFrameEntrySize = 16;
const pixaClipNameSize = 32;
const pixaRgb565BytesPerPixel = 2;

class PixaParseException implements FormatException {
  PixaParseException(this.message, [this.source, this.offset]);

  @override
  final String message;

  @override
  final dynamic source;

  @override
  final int? offset;

  @override
  String toString() {
    final where = offset == null ? '' : ' at offset $offset';
    return 'PixaParseException: $message$where';
  }
}

class PixaCanvas {
  const PixaCanvas({required this.width, required this.height});

  final int width;
  final int height;

  int get pixelCount => width * height;

  int get rgb565ByteCount => pixelCount * pixaRgb565BytesPerPixel;
}

class PixaClip {
  const PixaClip({
    required this.name,
    required this.anchorX,
    required this.anchorY,
    required this.firstFrame,
    required this.frameCount,
    required this.totalDurationMs,
    required this.loop,
  });

  final String name;
  final int anchorX;
  final int anchorY;
  final int firstFrame;
  final int frameCount;
  final int totalDurationMs;
  final bool loop;
}

enum PixaFrameType { key, diff, unknown }

class PixaFrame {
  const PixaFrame({
    required this.durationMs,
    required this.type,
    required this.typeCode,
    required this.payloadOffset,
    required this.payloadLength,
  });

  final int durationMs;
  final PixaFrameType type;
  final int typeCode;
  final int payloadOffset;
  final int payloadLength;
}

class PixaAsset {
  const PixaAsset({
    required this.version,
    required this.canvas,
    required this.colorCount,
    required this.paletteOffset,
    required this.clipOffset,
    required this.frameOffset,
    required this.payloadOffset,
    required this.payloadLength,
    required this.clips,
    required this.frames,
    required this.bytes,
  });

  final int version;
  final PixaCanvas canvas;
  final int colorCount;
  final int paletteOffset;
  final int clipOffset;
  final int frameOffset;
  final int payloadOffset;
  final int payloadLength;
  final List<PixaClip> clips;
  final List<PixaFrame> frames;
  final Uint8List bytes;

  int get clipCount => clips.length;

  int get frameCount => frames.length;
}

enum PixaValidationMode { petdef, badgedef }

class PixaFrameRgba {
  const PixaFrameRgba({
    required this.width,
    required this.height,
    required this.data,
  });

  final int width;
  final int height;
  final Uint8ClampedList data;
}

PixaFrameType pixaFrameType(int typeCode) {
  return switch (typeCode) {
    0 => PixaFrameType.key,
    1 => PixaFrameType.diff,
    _ => PixaFrameType.unknown,
  };
}

PixaAsset parsePixa(Uint8List input) {
  final bytes = Uint8List.sublistView(input);
  final data = ByteData.sublistView(bytes);
  if (bytes.lengthInBytes < pixaHeaderSize) {
    throw PixaParseException('PIXA file is shorter than the header', bytes);
  }

  final magic = ascii.decode(
    Uint8List.sublistView(bytes, 0, 4),
    allowInvalid: true,
  );
  if (magic != pixaMagic) {
    throw PixaParseException('Invalid PIXA magic', bytes, 0);
  }

  final version = data.getUint16(4, Endian.little);
  if (version != pixaVersion) {
    throw PixaParseException('Unsupported PIXA version $version', bytes, 4);
  }

  final headerSize = data.getUint16(6, Endian.little);
  if (headerSize != pixaHeaderSize) {
    throw PixaParseException(
      'Unsupported PIXA header size $headerSize',
      bytes,
      6,
    );
  }

  final width = data.getUint16(8, Endian.little);
  final height = data.getUint16(10, Endian.little);
  final colorCount = data.getUint16(12, Endian.little);
  final clipCount = data.getUint16(14, Endian.little);
  final frameCount = data.getUint32(16, Endian.little);
  final paletteOffset = data.getUint32(20, Endian.little);
  final clipOffset = data.getUint32(24, Endian.little);
  final frameOffset = data.getUint32(28, Endian.little);
  final payloadOffset = data.getUint32(32, Endian.little);
  final payloadLength = data.getUint32(36, Endian.little);

  if (width == 0 || height == 0) {
    throw PixaParseException('Invalid PIXA canvas size', bytes, 8);
  }

  _requireRange(
    bytes,
    paletteOffset,
    colorCount * pixaRgb565BytesPerPixel,
    'palette',
  );
  _requireRange(bytes, clipOffset, clipCount * pixaClipEntrySize, 'clip table');
  _requireRange(
    bytes,
    frameOffset,
    frameCount * pixaFrameEntrySize,
    'frame table',
  );
  _requireRange(bytes, payloadOffset, payloadLength, 'payload');

  final clips = <PixaClip>[];
  for (var i = 0; i < clipCount; i += 1) {
    final base = clipOffset + i * pixaClipEntrySize;
    final name = _readNullTerminatedUtf8(bytes, base, pixaClipNameSize);
    final firstFrame = data.getUint32(base + 36, Endian.little);
    final clipFrameCount = data.getUint32(base + 40, Endian.little);
    if (clipFrameCount == 0) {
      throw PixaParseException(
        'PIXA clip "$name" contains no frames',
        bytes,
        base,
      );
    }
    if (firstFrame > frameCount || clipFrameCount > frameCount - firstFrame) {
      throw PixaParseException(
        'PIXA clip "$name" references frames outside the frame table',
        bytes,
        base,
      );
    }

    clips.add(
      PixaClip(
        name: name,
        anchorX: data.getInt16(base + 32, Endian.little),
        anchorY: data.getInt16(base + 34, Endian.little),
        firstFrame: firstFrame,
        frameCount: clipFrameCount,
        totalDurationMs: data.getUint32(base + 44, Endian.little),
        loop: (data.getUint16(base + 48, Endian.little) & 1) == 1,
      ),
    );
  }

  final frames = <PixaFrame>[];
  for (var i = 0; i < frameCount; i += 1) {
    final base = frameOffset + i * pixaFrameEntrySize;
    final framePayloadOffset = data.getUint32(base + 4, Endian.little);
    final framePayloadLength = data.getUint32(base + 8, Endian.little);
    if (framePayloadOffset > payloadLength ||
        framePayloadLength > payloadLength - framePayloadOffset) {
      throw PixaParseException(
        'PIXA frame $i references bytes outside the payload',
        bytes,
        base,
      );
    }

    final typeCode = data.getUint8(base + 2);
    frames.add(
      PixaFrame(
        durationMs: data.getUint16(base, Endian.little),
        type: pixaFrameType(typeCode),
        typeCode: typeCode,
        payloadOffset: framePayloadOffset,
        payloadLength: framePayloadLength,
      ),
    );
  }

  return PixaAsset(
    version: version,
    canvas: PixaCanvas(width: width, height: height),
    colorCount: colorCount,
    paletteOffset: paletteOffset,
    clipOffset: clipOffset,
    frameOffset: frameOffset,
    payloadOffset: payloadOffset,
    payloadLength: payloadLength,
    clips: List.unmodifiable(clips),
    frames: List.unmodifiable(frames),
    bytes: bytes,
  );
}

PixaAsset validatePixa(Uint8List bytes, {PixaValidationMode? mode}) {
  final asset = parsePixa(bytes);
  switch (mode) {
    case PixaValidationMode.petdef:
      _validatePetDefPixa(asset);
    case PixaValidationMode.badgedef:
      _validateBadgeDefPixa(asset);
    case null:
      break;
  }
  return asset;
}

PixaClip? findPixaClip(PixaAsset asset, String name) {
  for (final clip in asset.clips) {
    if (clip.name == name) {
      return clip;
    }
  }
  return null;
}

PixaClip? selectPixaClip(PixaAsset asset, [String? preferredName]) {
  final preferred = preferredName?.trim();
  if (preferred != null && preferred.isNotEmpty) {
    final selected = findPixaClip(asset, preferred);
    if (selected != null) {
      return selected;
    }
  }

  final idle = findPixaClip(asset, 'idle');
  if (idle != null) {
    return idle;
  }
  return asset.clips.isEmpty ? null : asset.clips.first;
}

int pixaClipFrameIndex(PixaAsset asset, PixaClip clip, int elapsedMs) {
  if (clip.frameCount <= 0) {
    return clip.firstFrame;
  }

  if (clip.totalDurationMs <= 0) {
    return clip.firstFrame;
  }

  if (clip.firstFrame >= asset.frames.length ||
      clip.frameCount > asset.frames.length - clip.firstFrame) {
    throw PixaParseException(
      'PIXA clip "${clip.name}" references frames outside the frame table',
      asset.bytes,
    );
  }

  final bounded = clip.loop
      ? _positiveModulo(elapsedMs, clip.totalDurationMs)
      : elapsedMs.clamp(0, clip.totalDurationMs - 1);
  var elapsedInClip = bounded;
  for (var offset = 0; offset < clip.frameCount; offset += 1) {
    final frameIndex = clip.firstFrame + offset;
    final duration = asset.frames[frameIndex].durationMs;
    if (duration <= 0 || elapsedInClip < duration) {
      return frameIndex;
    }
    elapsedInClip -= duration;
  }
  return clip.firstFrame + clip.frameCount - 1;
}

PixaFrameRgba renderPixaFrameRgba(PixaAsset asset, int frameIndex) {
  if (frameIndex < 0 || frameIndex >= asset.frames.length) {
    throw PixaParseException('PIXA frame $frameIndex does not exist');
  }
  final frame = asset.frames[frameIndex];
  if (frame.type != PixaFrameType.key) {
    throw PixaParseException(
      'PIXA frame $frameIndex is ${frame.type.name}; only key frames can be rendered',
    );
  }

  final pixelCount = asset.canvas.pixelCount;
  final expectedLength = asset.canvas.rgb565ByteCount;
  if (frame.payloadLength < expectedLength) {
    throw PixaParseException(
      'PIXA key frame is shorter than the RGB565 canvas payload',
      asset.bytes,
      asset.payloadOffset + frame.payloadOffset,
    );
  }

  final start = asset.payloadOffset + frame.payloadOffset;
  final end = start + expectedLength;
  _requireRange(asset.bytes, start, expectedLength, 'frame payload');

  final payload = ByteData.sublistView(asset.bytes, start, end);
  final output = Uint8ClampedList(pixelCount * 4);
  for (var pixel = 0; pixel < pixelCount; pixel += 1) {
    final value = payload.getUint16(pixel * 2, Endian.little);
    final outputOffset = pixel * 4;
    output[outputOffset] = (((value >> 11) & 0x1f) * 255 / 31).round();
    output[outputOffset + 1] = (((value >> 5) & 0x3f) * 255 / 63).round();
    output[outputOffset + 2] = ((value & 0x1f) * 255 / 31).round();
    output[outputOffset + 3] = 255;
  }

  return PixaFrameRgba(
    width: asset.canvas.width,
    height: asset.canvas.height,
    data: output,
  );
}

void _validatePetDefPixa(PixaAsset asset) {
  if (asset.clips.isEmpty) {
    throw PixaParseException(
      'PetDef PIXA must contain at least one clip',
      asset.bytes,
    );
  }
  if (asset.frames.isEmpty) {
    throw PixaParseException(
      'PetDef PIXA must contain at least one frame',
      asset.bytes,
    );
  }
}

void _validateBadgeDefPixa(PixaAsset asset) {
  final iconClips = asset.clips.where((clip) => clip.name == 'icon').toList();
  if (iconClips.isEmpty) {
    throw PixaParseException(
      'BadgeDef PIXA must contain an icon clip',
      asset.bytes,
    );
  }

  final icon = iconClips.first;
  if (icon.frameCount != 1) {
    throw PixaParseException(
      'BadgeDef icon clip must contain exactly one frame',
      asset.bytes,
    );
  }

  final frame = asset.frames[icon.firstFrame];
  if (frame.type != PixaFrameType.key) {
    throw PixaParseException(
      'BadgeDef icon clip must reference a key frame',
      asset.bytes,
    );
  }
}

String _readNullTerminatedUtf8(Uint8List bytes, int offset, int length) {
  var end = offset;
  final limit = offset + length;
  while (end < limit && bytes[end] != 0) {
    end += 1;
  }
  return utf8.decode(
    Uint8List.sublistView(bytes, offset, end),
    allowMalformed: true,
  );
}

void _requireRange(Uint8List bytes, int offset, int length, String label) {
  if (offset < 0 || length < 0 || offset > bytes.lengthInBytes - length) {
    throw PixaParseException(
      'PIXA $label range is outside the file',
      bytes,
      offset,
    );
  }
}

int _positiveModulo(int value, int divisor) {
  final remainder = value % divisor;
  return remainder < 0 ? remainder + divisor : remainder;
}
