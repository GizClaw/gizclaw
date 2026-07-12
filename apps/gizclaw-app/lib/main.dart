import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:gizclaw/gizclaw.dart';

import 'pixa_sprite.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'GizClaw Pixa',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xff0f8b8d),
          brightness: Brightness.light,
        ),
        scaffoldBackgroundColor: const Color(0xfff7f9fb),
        useMaterial3: true,
      ),
      home: const PixaSmokePage(),
    );
  }
}

class PixaSmokePage extends StatelessWidget {
  const PixaSmokePage({super.key});

  static final PixaAsset _asset = validatePixa(
    _makePixaSmokeAsset(),
    mode: PixaValidationMode.petdef,
  );

  static const Duration _previewTime = Duration(milliseconds: 180);

  @override
  Widget build(BuildContext context) {
    final textTheme = Theme.of(context).textTheme;
    return Scaffold(
      appBar: AppBar(
        title: const Text('GizClaw Pixa'),
        backgroundColor: const Color(0xfff7f9fb),
      ),
      body: SafeArea(
        minimum: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text('Miso Preview', style: textTheme.headlineSmall),
            const SizedBox(height: 24),
            Expanded(
              child: DecoratedBox(
                decoration: BoxDecoration(
                  border: Border.all(color: const Color(0xffb8c2cc)),
                  color: Colors.white,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Center(
                  child: PixaSprite(
                    asset: _asset,
                    clipName: 'idle',
                    elapsed: _previewTime,
                    width: 192,
                    height: 96,
                  ),
                ),
              ),
            ),
            const SizedBox(height: 20),
            Text(
              'clip idle  |  ${_asset.canvas.width}x${_asset.canvas.height}  |  ${_asset.frames.length} frames',
              style: textTheme.bodyMedium?.copyWith(
                color: const Color(0xff0f766e),
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}

Uint8List _makePixaSmokeAsset() {
  const headerSize = 40;
  const clipEntrySize = 56;
  const frameEntrySize = 16;
  const width = 2;
  const height = 1;
  const framePayloads = [
    [0x00, 0xf8, 0x1f, 0x00],
    [0x1f, 0x00, 0xe0, 0x07],
  ];
  const frameDurations = [140, 260];
  const paletteOffset = headerSize;
  const clipOffset = paletteOffset + 2;
  const frameOffset = clipOffset + clipEntrySize;
  final payloadOffset = frameOffset + frameEntrySize * framePayloads.length;
  final payloadSize = framePayloads.fold<int>(
    0,
    (total, payload) => total + payload.length,
  );
  final bytes = Uint8List(payloadOffset + payloadSize);
  final data = ByteData.sublistView(bytes);

  bytes.setAll(0, ascii.encode('PIXA'));
  data.setUint16(4, 1, Endian.little);
  data.setUint16(6, headerSize, Endian.little);
  data.setUint16(8, width, Endian.little);
  data.setUint16(10, height, Endian.little);
  data.setUint16(12, 1, Endian.little);
  data.setUint16(14, 1, Endian.little);
  data.setUint32(16, framePayloads.length, Endian.little);
  data.setUint32(20, paletteOffset, Endian.little);
  data.setUint32(24, clipOffset, Endian.little);
  data.setUint32(28, frameOffset, Endian.little);
  data.setUint32(32, payloadOffset, Endian.little);
  data.setUint32(36, payloadSize, Endian.little);

  bytes.setAll(clipOffset, utf8.encode('idle'));
  data.setUint32(clipOffset + 40, 1, Endian.little);
  data.setUint32(clipOffset + 44, 400, Endian.little);
  data.setUint16(clipOffset + 48, framePayloads.length, Endian.little);

  var payloadCursor = payloadOffset;
  for (var index = 0; index < framePayloads.length; index += 1) {
    final entryOffset = frameOffset + frameEntrySize * index;
    final payload = framePayloads[index];
    data.setUint16(entryOffset, frameDurations[index], Endian.little);
    data.setUint32(
      entryOffset + 4,
      payloadCursor - payloadOffset,
      Endian.little,
    );
    data.setUint32(entryOffset + 8, payload.length, Endian.little);
    bytes.setAll(payloadCursor, payload);
    payloadCursor += payload.length;
  }

  return bytes;
}
