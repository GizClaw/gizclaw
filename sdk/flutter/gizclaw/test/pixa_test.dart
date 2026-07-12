import 'dart:typed_data';

import 'package:gizclaw/src/pixa.dart';
import 'package:test/test.dart';

import 'pixa_test_data.dart';

void main() {
  test('parses pixa headers, clips, frames, and typed-list views', () {
    final source = Uint8List.fromList([
      9,
      9,
      ...makePixa(clips: ['idle', 'wave']),
      8,
    ]);
    final asset = parsePixa(
      Uint8List.sublistView(source, 2, source.length - 1),
    );

    expect(asset.version, 1);
    expect(asset.canvas.width, 2);
    expect(asset.canvas.height, 1);
    expect(asset.canvas.pixelCount, 2);
    expect(asset.canvas.rgb565ByteCount, 4);
    expect(asset.colorCount, 1);
    expect(asset.clipCount, 2);
    expect(asset.frameCount, 1);
    expect(asset.clipOffset, 42);
    expect(asset.frameOffset, 154);
    expect(asset.clips, hasLength(2));
    expect(asset.clips[1].name, 'wave');
    expect(asset.clips[0].loop, isTrue);
    expect(asset.frames.single.type, PixaFrameType.key);
    expect(asset.frames.single.typeCode, 0);
    expect(asset.payloadLength, 4);
  });

  test('rejects malformed header and table ranges', () {
    expect(() => parsePixa(Uint8List(8)), throwsA(isA<PixaParseException>()));

    final badMagic = makePixa();
    badMagic[0] = 0x58;
    expect(() => parsePixa(badMagic), throwsA(isA<PixaParseException>()));

    final badVersion = makePixa();
    ByteData.sublistView(badVersion).setUint16(4, 2, Endian.little);
    expect(() => parsePixa(badVersion), throwsA(isA<PixaParseException>()));

    final badRange = makePixa();
    ByteData.sublistView(
      badRange,
    ).setUint32(32, badRange.length + 1, Endian.little);
    expect(() => parsePixa(badRange), throwsA(isA<PixaParseException>()));

    expect(
      () => parsePixa(makePixa(width: 0)),
      throwsA(isA<PixaParseException>()),
    );

    final emptyClip = makePixa();
    ByteData.sublistView(emptyClip).setUint32(82, 0, Endian.little);
    expect(() => parsePixa(emptyClip), throwsA(isA<PixaParseException>()));
  });

  test('validates petdef and badgedef pixa contracts', () {
    expect(validatePixa(makePixa(clips: ['default'])), isA<PixaAsset>());
    expect(
      validatePixa(
        makePixa(clips: ['default']),
        mode: PixaValidationMode.petdef,
      ),
      isA<PixaAsset>(),
    );
    expect(
      () => validatePixa(makePixa(clips: []), mode: PixaValidationMode.petdef),
      throwsA(isA<PixaParseException>()),
    );

    expect(
      validatePixa(
        makePixa(clips: ['icon']),
        mode: PixaValidationMode.badgedef,
      ),
      isA<PixaAsset>(),
    );
    expect(
      () => validatePixa(
        makePixa(clips: ['idle']),
        mode: PixaValidationMode.badgedef,
      ),
      throwsA(isA<PixaParseException>()),
    );
  });

  test('selects clips and frame indexes deterministically', () {
    final asset = parsePixa(makePixa(clips: ['idle', 'wave']));
    expect(findPixaClip(asset, 'wave')?.name, 'wave');
    expect(findPixaClip(asset, 'missing'), isNull);
    expect(selectPixaClip(asset, 'wave')?.name, 'wave');
    expect(selectPixaClip(asset, 'missing')?.name, 'idle');
    expect(selectPixaClip(asset)?.name, 'idle');

    final timed = parsePixa(
      makePixa(
        frameDurations: [50, 350],
        framePayloads: [
          [0x00, 0xf8, 0xe0, 0x07],
          [0x1f, 0x00, 0xff, 0xff],
        ],
      ),
    );
    final clip = timed.clips.single;
    expect(pixaClipFrameIndex(timed, clip, 0), 0);
    expect(pixaClipFrameIndex(timed, clip, 49), 0);
    expect(pixaClipFrameIndex(timed, clip, 50), 1);
    expect(pixaClipFrameIndex(timed, clip, 399), 1);
    expect(pixaClipFrameIndex(timed, clip, 400), 0);
    expect(pixaClipFrameIndex(timed, clip, -1), 1);
  });

  test('renders RGB565 key frames to RGBA', () {
    final asset = parsePixa(makePixa());
    final image = renderPixaFrameRgba(asset, 0);

    expect(image.width, 2);
    expect(image.height, 1);
    expect(image.data, [255, 0, 0, 255, 0, 255, 0, 255]);
  });

  test('parses unsupported frame metadata and rejects it in renderer', () {
    final asset = parsePixa(makePixa(frameType: 1));

    expect(asset.frames.single.type, PixaFrameType.diff);
    expect(asset.frames.single.typeCode, 1);
    expect(
      () => renderPixaFrameRgba(asset, -1),
      throwsA(isA<PixaParseException>()),
    );
    expect(
      () => renderPixaFrameRgba(asset, 0),
      throwsA(isA<PixaParseException>()),
    );

    final unknown = parsePixa(makePixa(frameType: 37));
    expect(unknown.frames.single.type, PixaFrameType.unknown);
    expect(unknown.frames.single.typeCode, 37);
  });
}
