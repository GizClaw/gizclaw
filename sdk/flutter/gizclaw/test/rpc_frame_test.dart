import 'dart:typed_data';

import 'package:gizclaw/src/rpc_frame.dart';
import 'package:test/test.dart';

void main() {
  test('encodes and decodes little-endian frame headers', () {
    final frame = encodeFrame(rpcFrameTypeBinary, [1, 2, 3]);

    expect(frame.sublist(0, 4), [3, 0, 2, 0]);
    final decoded = decodeFrames(frame);
    expect(decoded, hasLength(1));
    expect(decoded.single.type, rpcFrameTypeBinary);
    expect(decoded.single.payload, [1, 2, 3]);
  });

  test('rejects non-empty EOS frames', () {
    expect(() => encodeFrame(rpcFrameTypeEos, [1]), throwsArgumentError);
    expect(
      () => decodeFrames(Uint8List.fromList([1, 0, 0, 0, 1])),
      throwsFormatException,
    );
  });

  test('returns null for incomplete frame and throws for full decode', () {
    final incomplete = Uint8List.fromList([3, 0, 2, 0, 1]);
    expect(tryReadFrame(incomplete), isNull);
    expect(() => decodeFrames(incomplete), throwsFormatException);
  });

  test('splits oversized envelopes into text continuation frames', () {
    final envelope = Uint8List(rpcMaxFramePayloadSize + 1);
    final frames = encodeEnvelopeFrames(envelope);

    expect(frames, hasLength(2));
    expect(decodeFrames(frames.first).single.type, rpcFrameTypeText);
    expect(decodeFrames(frames.last).single.type, rpcFrameTypeText);
  });
}
