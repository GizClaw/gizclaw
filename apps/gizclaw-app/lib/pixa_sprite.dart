import 'dart:ui' as ui;

import 'package:flutter/cupertino.dart';
import 'package:gizclaw/gizclaw.dart';

class PixaSprite extends StatefulWidget {
  const PixaSprite({
    super.key,
    required this.asset,
    this.clipName,
    this.elapsed = Duration.zero,
    this.width,
    this.height,
    this.fit = BoxFit.contain,
    this.placeholder,
    this.errorBuilder,
  });

  final PixaAsset asset;
  final String? clipName;
  final Duration elapsed;
  final double? width;
  final double? height;
  final BoxFit fit;
  final Widget? placeholder;
  final Widget Function(BuildContext context, Object error)? errorBuilder;

  @override
  State<PixaSprite> createState() => _PixaSpriteState();
}

class _PixaSpriteState extends State<PixaSprite> {
  late Future<ui.Image> _image;
  late double _width;
  late double _height;

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
        oldWidget.height != widget.height) {
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
                  child: Icon(
                    CupertinoIcons.exclamationmark_triangle,
                    size: 18,
                  ),
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

  void _loadFrame() {
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
        clip,
        widget.elapsed.inMilliseconds,
      );
      final rgba = renderPixaFrameRgba(widget.asset, frameIndex);
      _width = widget.width ?? rgba.width.toDouble();
      _height = widget.height ?? rgba.height.toDouble();
      _image = pixaFrameRgbaToImage(rgba);
    } catch (error, stackTrace) {
      _width = widget.width ?? widget.asset.canvas.width.toDouble();
      _height = widget.height ?? widget.asset.canvas.height.toDouble();
      _image = Future<ui.Image>.error(error, stackTrace);
    }
  }
}
