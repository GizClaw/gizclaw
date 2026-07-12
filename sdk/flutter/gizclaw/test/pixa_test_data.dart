import 'dart:convert';
import 'dart:typed_data';

Uint8List makePixa({
  int width = 2,
  int height = 1,
  List<String> clips = const ['idle'],
  int frameType = 0,
  List<int>? frameTypes,
  List<int> frameDurations = const [120],
  List<int> payload = const [0x00, 0xf8, 0xe0, 0x07],
  List<List<int>>? framePayloads,
  void Function(ByteData data)? mutate,
}) {
  const headerSize = 40;
  const clipEntrySize = 56;
  const frameEntrySize = 16;
  final payloads = framePayloads ?? [payload];
  final types = frameTypes ?? List<int>.filled(payloads.length, frameType);
  if (frameDurations.length != payloads.length ||
      types.length != payloads.length) {
    throw ArgumentError.value(
      payloads.length,
      'framePayloads',
      'frame duration, type, and payload counts must match',
    );
  }

  const paletteOffset = headerSize;
  final clipOffset = paletteOffset + 2;
  final frameOffset = clipOffset + clips.length * clipEntrySize;
  final payloadOffset = frameOffset + payloads.length * frameEntrySize;
  final payloadLength = payloads.fold<int>(
    0,
    (total, bytes) => total + bytes.length,
  );
  final totalLength = payloadOffset + payloadLength;
  final bytes = Uint8List(totalLength);
  final data = ByteData.sublistView(bytes);

  bytes.setAll(0, ascii.encode('PIXA'));
  data.setUint16(4, 1, Endian.little);
  data.setUint16(6, headerSize, Endian.little);
  data.setUint16(8, width, Endian.little);
  data.setUint16(10, height, Endian.little);
  data.setUint16(12, 1, Endian.little);
  data.setUint16(14, clips.length, Endian.little);
  data.setUint32(16, payloads.length, Endian.little);
  data.setUint32(20, paletteOffset, Endian.little);
  data.setUint32(24, clipOffset, Endian.little);
  data.setUint32(28, frameOffset, Endian.little);
  data.setUint32(32, payloadOffset, Endian.little);
  data.setUint32(36, payloadLength, Endian.little);

  for (var i = 0; i < clips.length; i += 1) {
    final base = clipOffset + i * clipEntrySize;
    final nameBytes = utf8.encode(clips[i]).take(32).toList();
    bytes.setAll(base, nameBytes);
    data.setUint32(base + 36, 0, Endian.little);
    data.setUint32(base + 40, payloads.length, Endian.little);
    data.setUint32(
      base + 44,
      frameDurations.fold<int>(0, (total, duration) => total + duration),
      Endian.little,
    );
    data.setUint16(base + 48, 1, Endian.little);
  }

  var framePayloadOffset = 0;
  for (var i = 0; i < payloads.length; i += 1) {
    final base = frameOffset + i * frameEntrySize;
    data.setUint16(base, frameDurations[i], Endian.little);
    data.setUint8(base + 2, types[i]);
    data.setUint32(base + 4, framePayloadOffset, Endian.little);
    data.setUint32(base + 8, payloads[i].length, Endian.little);
    bytes.setAll(payloadOffset + framePayloadOffset, payloads[i]);
    framePayloadOffset += payloads[i].length;
  }

  mutate?.call(data);
  return bytes;
}
