import 'dart:async';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/widgets.dart';

import 'pixa.dart';

Future<ui.Image> pixaFrameRgbaToImage(PixaFrameRgba frame) {
  final completer = Completer<ui.Image>();
  ui.decodeImageFromPixels(
    Uint8List.sublistView(frame.data),
    frame.width,
    frame.height,
    ui.PixelFormat.rgba8888,
    completer.complete,
  );
  return completer.future;
}

class PixaFramePainter extends CustomPainter {
  const PixaFramePainter(
    this.image, {
    this.fit = BoxFit.contain,
    this.alignment = Alignment.center,
    this.filterQuality = FilterQuality.none,
  });

  final ui.Image image;
  final BoxFit fit;
  final AlignmentGeometry alignment;
  final FilterQuality filterQuality;

  @override
  void paint(Canvas canvas, Size size) {
    final sourceSize = Size(image.width.toDouble(), image.height.toDouble());
    final fitted = applyBoxFit(fit, sourceSize, size);
    final resolvedAlignment = alignment.resolve(TextDirection.ltr);
    final sourceRect = resolvedAlignment.inscribe(
      fitted.source,
      Offset.zero & sourceSize,
    );
    final destinationRect = resolvedAlignment.inscribe(
      fitted.destination,
      Offset.zero & size,
    );
    canvas.drawImageRect(
      image,
      sourceRect,
      destinationRect,
      Paint()..filterQuality = filterQuality,
    );
  }

  @override
  bool shouldRepaint(covariant PixaFramePainter oldDelegate) {
    return image != oldDelegate.image ||
        fit != oldDelegate.fit ||
        alignment != oldDelegate.alignment ||
        filterQuality != oldDelegate.filterQuality;
  }
}
