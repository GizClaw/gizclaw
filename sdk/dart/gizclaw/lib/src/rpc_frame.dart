import 'dart:typed_data';

const rpcFrameTypeEos = 0;
const rpcFrameTypeJson = 1;
const rpcFrameTypeBinary = 2;
const rpcFrameTypeText = 3;
const rpcMaxFramePayloadSize = 0xffff;
const rpcMaxEnvelopeSize = rpcMaxFramePayloadSize * 16;

class RpcFrame {
  const RpcFrame(this.type, this.payload);

  final Uint8List payload;
  final int type;
}

class FrameReadResult {
  const FrameReadResult(this.frame, this.rest);

  final RpcFrame frame;
  final Uint8List rest;
}

Uint8List encodeFrame(int type, [List<int> payload = const []]) {
  if (type < 0 || type > 0xffff) {
    throw ArgumentError.value(type, 'type', 'invalid RPC frame type');
  }
  if (payload.length > rpcMaxFramePayloadSize) {
    throw ArgumentError.value(payload.length, 'payload', 'RPC frame too large');
  }
  if (type == rpcFrameTypeEos && payload.isNotEmpty) {
    throw ArgumentError.value(payload.length, 'payload', 'EOS must be empty');
  }
  final out = Uint8List(4 + payload.length);
  final view = ByteData.view(out.buffer);
  view.setUint16(0, payload.length, Endian.little);
  view.setUint16(2, type, Endian.little);
  out.setAll(4, payload);
  return out;
}

List<RpcFrame> decodeFrames(List<int> bytes) {
  var buffer = Uint8List.fromList(bytes);
  final frames = <RpcFrame>[];
  while (buffer.isNotEmpty) {
    final result = tryReadFrame(buffer);
    if (result == null) {
      throw const FormatException('incomplete RPC frame');
    }
    frames.add(result.frame);
    buffer = result.rest;
  }
  return frames;
}

FrameReadResult? tryReadFrame(Uint8List buffer) {
  if (buffer.length < 4) {
    return null;
  }
  final view = ByteData.sublistView(buffer, 0, 4);
  final length = view.getUint16(0, Endian.little);
  final type = view.getUint16(2, Endian.little);
  if (buffer.length < 4 + length) {
    return null;
  }
  if (type == rpcFrameTypeEos && length != 0) {
    throw const FormatException('RPC EOS frame must be empty');
  }
  return FrameReadResult(
    RpcFrame(type, Uint8List.sublistView(buffer, 4, 4 + length)),
    Uint8List.sublistView(buffer, 4 + length),
  );
}

List<Uint8List> encodeEnvelopeFrames(List<int> envelope) {
  if (envelope.length <= rpcMaxFramePayloadSize) {
    return [encodeFrame(rpcFrameTypeBinary, envelope)];
  }
  final frames = <Uint8List>[];
  for (
    var offset = 0;
    offset < envelope.length;
    offset += rpcMaxFramePayloadSize
  ) {
    final end = (offset + rpcMaxFramePayloadSize).clamp(0, envelope.length);
    frames.add(encodeFrame(rpcFrameTypeText, envelope.sublist(offset, end)));
  }
  return frames;
}

Uint8List concatBytes(Iterable<List<int>> chunks) {
  final length = chunks.fold<int>(0, (sum, chunk) => sum + chunk.length);
  final out = Uint8List(length);
  var offset = 0;
  for (final chunk in chunks) {
    out.setAll(offset, chunk);
    offset += chunk.length;
  }
  return out;
}
