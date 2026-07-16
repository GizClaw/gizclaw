import 'package:flutter/services.dart';

class PcmAudioLevels {
  const PcmAudioLevels({required this.input, required this.output});

  final double input;
  final double output;
}

class PcmAudioLevelSource {
  PcmAudioLevelSource._();

  static const _channel = EventChannel(
    'com.gizclaw.opensource/pcm_audio_levels',
  );

  static final Stream<PcmAudioLevels> levels = _channel
      .receiveBroadcastStream()
      .map(_decodeLevels);

  static PcmAudioLevels _decodeLevels(Object? event) {
    if (event is! Map<Object?, Object?>) {
      throw const FormatException('Invalid PCM audio level event');
    }
    final input = event['input'];
    final output = event['output'];
    if (input is! num || output is! num) {
      throw const FormatException('PCM audio levels must be numeric');
    }
    return PcmAudioLevels(
      input: input.toDouble().clamp(0.0, 1.0),
      output: output.toDouble().clamp(0.0, 1.0),
    );
  }
}
