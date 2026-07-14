import 'dart:ui' as ui;
import 'dart:typed_data';

import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';

import 'giz_ui/giz_ui.dart';

class PixaSprite extends StatefulWidget {
  const PixaSprite({
    super.key,
    required this.asset,
    this.clipName,
    this.elapsed = Duration.zero,
    this.width,
    this.height,
    this.fit = BoxFit.contain,
    this.transparentEdgeBackground = false,
    this.placeholder,
    this.errorBuilder,
  });

  final PixaAsset asset;
  final String? clipName;
  final Duration elapsed;
  final double? width;
  final double? height;
  final BoxFit fit;
  final bool transparentEdgeBackground;
  final Widget? placeholder;
  final Widget Function(BuildContext context, Object error)? errorBuilder;

  @override
  State<PixaSprite> createState() => _PixaSpriteState();
}

class _PixaSpriteState extends State<PixaSprite> {
  late Future<ui.Image> _image;
  late double _width;
  late double _height;
  ui.Image? _currentImage;
  int _imageRequest = 0;

  @override
  void initState() {
    super.initState();
    _loadFrame();
  }

  @override
  void didUpdateWidget(covariant PixaSprite oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.asset != widget.asset ||
        oldWidget.clipName != widget.clipName ||
        oldWidget.elapsed != widget.elapsed ||
        oldWidget.width != widget.width ||
        oldWidget.height != widget.height ||
        oldWidget.transparentEdgeBackground !=
            widget.transparentEdgeBackground) {
      _loadFrame();
    }
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: _width,
      height: _height,
      child: FutureBuilder<ui.Image>(
        future: _image,
        builder: (context, snapshot) {
          if (snapshot.hasError) {
            return widget.errorBuilder?.call(context, snapshot.error!) ??
                const Center(
                  child: Icon(GizIcons.exclamationmark_triangle, size: 18),
                );
          }
          if (!snapshot.hasData) {
            return widget.placeholder ?? const SizedBox.expand();
          }
          return CustomPaint(
            size: Size(_width, _height),
            painter: PixaFramePainter(snapshot.requireData, fit: widget.fit),
          );
        },
      ),
    );
  }

  @override
  void dispose() {
    _imageRequest += 1;
    _currentImage?.dispose();
    _currentImage = null;
    super.dispose();
  }

  void _loadFrame() {
    _imageRequest += 1;
    final request = _imageRequest;
    try {
      final clip = selectPixaClip(widget.asset, widget.clipName);
      if (clip == null) {
        throw ArgumentError.value(
          widget.asset,
          'asset',
          'PIXA asset has no clips',
        );
      }

      final frameIndex = pixaClipFrameIndex(
        widget.asset,
        clip,
        widget.elapsed.inMilliseconds,
      );
      var rgba = renderPixaFrameRgba(widget.asset, frameIndex);
      if (widget.transparentEdgeBackground) {
        rgba = removePixaEdgeBackground(rgba);
      }
      _width = widget.width ?? rgba.width.toDouble();
      _height = widget.height ?? rgba.height.toDouble();
      _image = pixaFrameRgbaToImage(rgba).then((image) {
        if (!mounted || request != _imageRequest) {
          image.dispose();
          return image;
        }
        _currentImage?.dispose();
        _currentImage = image;
        return image;
      });
    } catch (error, stackTrace) {
      _currentImage?.dispose();
      _currentImage = null;
      _width = widget.width ?? widget.asset.canvas.width.toDouble();
      _height = widget.height ?? widget.asset.canvas.height.toDouble();
      _image = Future<ui.Image>.error(error, stackTrace);
    }
  }
}

PixaFrameRgba removePixaEdgeBackground(
  PixaFrameRgba frame, {
  int tolerance = 12,
}) {
  if (frame.width == 0 || frame.height == 0) return frame;

  final data = Uint8ClampedList.fromList(frame.data);
  final corners = <int>[
    0,
    frame.width - 1,
    (frame.height - 1) * frame.width,
    frame.width * frame.height - 1,
  ];
  var backgroundPixel = corners.first;
  var bestScore = -1;
  for (final candidate in corners) {
    var score = 0;
    for (final corner in corners) {
      if (_matchesPixel(data, candidate, corner, tolerance)) score += 1;
    }
    if (score > bestScore) {
      backgroundPixel = candidate;
      bestScore = score;
    }
  }
  final backgroundOffset = backgroundPixel * 4;
  final backgroundRed = data[backgroundOffset];
  final backgroundGreen = data[backgroundOffset + 1];
  final backgroundBlue = data[backgroundOffset + 2];

  final pixelCount = frame.width * frame.height;
  final visited = Uint8List(pixelCount);
  final queue = <int>[];

  void enqueue(int pixel) {
    if (visited[pixel] != 0 ||
        !_matchesColor(
          data,
          pixel,
          backgroundRed,
          backgroundGreen,
          backgroundBlue,
          tolerance,
        )) {
      return;
    }
    visited[pixel] = 1;
    queue.add(pixel);
  }

  for (var x = 0; x < frame.width; x += 1) {
    enqueue(x);
    enqueue((frame.height - 1) * frame.width + x);
  }
  for (var y = 1; y < frame.height - 1; y += 1) {
    enqueue(y * frame.width);
    enqueue(y * frame.width + frame.width - 1);
  }

  for (var head = 0; head < queue.length; head += 1) {
    final pixel = queue[head];
    final offset = pixel * 4;
    data.fillRange(offset, offset + 4, 0);
    final x = pixel % frame.width;
    final y = pixel ~/ frame.width;
    if (x > 0) enqueue(pixel - 1);
    if (x + 1 < frame.width) enqueue(pixel + 1);
    if (y > 0) enqueue(pixel - frame.width);
    if (y + 1 < frame.height) enqueue(pixel + frame.width);
  }

  return PixaFrameRgba(width: frame.width, height: frame.height, data: data);
}

bool _matchesColor(
  Uint8ClampedList data,
  int pixel,
  int red,
  int green,
  int blue,
  int tolerance,
) {
  final offset = pixel * 4;
  return (data[offset] - red).abs() <= tolerance &&
      (data[offset + 1] - green).abs() <= tolerance &&
      (data[offset + 2] - blue).abs() <= tolerance;
}

bool _matchesPixel(
  Uint8ClampedList data,
  int firstPixel,
  int secondPixel,
  int tolerance,
) {
  final first = firstPixel * 4;
  final second = secondPixel * 4;
  return (data[first] - data[second]).abs() <= tolerance &&
      (data[first + 1] - data[second + 1]).abs() <= tolerance &&
      (data[first + 2] - data[second + 2]).abs() <= tolerance;
}
