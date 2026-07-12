import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:gizclaw/src/pixa.dart';
import 'package:gizclaw/src/pixa_flutter.dart';

import 'pixa_test_data.dart';

void main() {
  test('converts rendered pixa RGBA frames into Flutter images', () async {
    final asset = parsePixa(makePixa());
    final rgba = renderPixaFrameRgba(asset, 0);

    final image = await pixaFrameRgbaToImage(rgba);
    addTearDown(image.dispose);

    expect(image.width, 2);
    expect(image.height, 1);
  });

  test('repaints pixa frames when painter options change', () async {
    final asset = parsePixa(makePixa());
    final rgba = renderPixaFrameRgba(asset, 0);
    final image = await pixaFrameRgbaToImage(rgba);
    addTearDown(image.dispose);

    const fit = BoxFit.contain;
    final painter = PixaFramePainter(image, fit: fit);

    expect(painter.shouldRepaint(PixaFramePainter(image, fit: fit)), isFalse);
    expect(
      painter.shouldRepaint(PixaFramePainter(image, fit: BoxFit.cover)),
      isTrue,
    );
  });
}
