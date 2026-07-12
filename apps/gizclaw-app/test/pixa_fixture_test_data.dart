import 'dart:convert';
import 'dart:typed_data';

Uint8List makePetPixaFixture() {
  const headerSize = 40;
  const clipEntrySize = 56;
  const frameEntrySize = 16;
  const width = 2;
  const height = 1;
  const payload = [0x00, 0xf8, 0x1f, 0x00];
  const paletteOffset = headerSize;
  const clipOffset = paletteOffset + 2;
  const frameOffset = clipOffset + clipEntrySize;
  const payloadOffset = frameOffset + frameEntrySize;
  final bytes = Uint8List(payloadOffset + payload.length);
  final data = ByteData.sublistView(bytes);

  bytes.setAll(0, ascii.encode('PIXA'));
  data.setUint16(4, 1, Endian.little);
  data.setUint16(6, headerSize, Endian.little);
  data.setUint16(8, width, Endian.little);
  data.setUint16(10, height, Endian.little);
  data.setUint16(12, 1, Endian.little);
  data.setUint16(14, 1, Endian.little);
  data.setUint32(16, 1, Endian.little);
  data.setUint32(20, paletteOffset, Endian.little);
  data.setUint32(24, clipOffset, Endian.little);
  data.setUint32(28, frameOffset, Endian.little);
  data.setUint32(32, payloadOffset, Endian.little);
  data.setUint32(36, payload.length, Endian.little);

  bytes.setAll(clipOffset, utf8.encode('idle'));
  data.setUint32(clipOffset + 40, 1, Endian.little);
  data.setUint32(clipOffset + 44, 120, Endian.little);
  data.setUint16(clipOffset + 48, 1, Endian.little);

  data.setUint16(frameOffset, 120, Endian.little);
  data.setUint32(frameOffset + 8, payload.length, Endian.little);
  bytes.setAll(payloadOffset, payload);
  return bytes;
}
