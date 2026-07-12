import 'dart:convert';
import 'dart:typed_data';

Uint8List makePixa({
  int width = 2,
  int height = 1,
  List<String> clips = const ['idle'],
  int frameType = 0,
  List<int> payload = const [0x00, 0xf8, 0xe0, 0x07],
  void Function(ByteData data)? mutate,
}) {
  const headerSize = 40;
  const clipEntrySize = 56;
  const frameEntrySize = 16;
  const paletteOffset = headerSize;
  final clipOffset = paletteOffset + 2;
  final frameOffset = clipOffset + clips.length * clipEntrySize;
  final payloadOffset = frameOffset + frameEntrySize;
  final totalLength = payloadOffset + payload.length;
  final bytes = Uint8List(totalLength);
  final data = ByteData.sublistView(bytes);

  bytes.setAll(0, ascii.encode('PIXA'));
  data.setUint16(4, 1, Endian.little);
  data.setUint16(6, headerSize, Endian.little);
  data.setUint16(8, width, Endian.little);
  data.setUint16(10, height, Endian.little);
  data.setUint16(12, 1, Endian.little);
  data.setUint16(14, clips.length, Endian.little);
  data.setUint32(16, 1, Endian.little);
  data.setUint32(20, paletteOffset, Endian.little);
  data.setUint32(24, clipOffset, Endian.little);
  data.setUint32(28, frameOffset, Endian.little);
  data.setUint32(32, payloadOffset, Endian.little);
  data.setUint32(36, payload.length, Endian.little);

  for (var i = 0; i < clips.length; i += 1) {
    final base = clipOffset + i * clipEntrySize;
    final nameBytes = utf8.encode(clips[i]).take(32).toList();
    bytes.setAll(base, nameBytes);
    data.setUint32(base + 36, 0, Endian.little);
    data.setUint32(base + 40, 1, Endian.little);
    data.setUint32(base + 44, 120, Endian.little);
    data.setUint16(base + 48, 1, Endian.little);
  }

  data.setUint16(frameOffset, 120, Endian.little);
  data.setUint8(frameOffset + 2, frameType);
  data.setUint32(frameOffset + 4, 0, Endian.little);
  data.setUint32(frameOffset + 8, payload.length, Endian.little);
  bytes.setAll(payloadOffset, payload);

  mutate?.call(data);
  return bytes;
}
