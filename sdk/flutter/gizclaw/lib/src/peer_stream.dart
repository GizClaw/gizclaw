import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'rpc_frame.dart';
import 'transport.dart';

class PeerStreamEvent {
  const PeerStreamEvent({
    required this.type,
    this.error,
    this.kind,
    this.label,
    this.lastUpdatedAt,
    this.streamId,
    this.text,
  });

  final String? error;
  final String? kind;
  final String? label;
  final DateTime? lastUpdatedAt;
  final String? streamId;
  final String? text;
  final String type;

  bool get isHistoryReplay => streamId?.startsWith('history-replay-') ?? false;

  factory PeerStreamEvent.fromJson(Map<String, Object?> json) {
    final type = json['type'];
    if (type is! String || type.isEmpty) {
      throw const FormatException('peer stream event type is required');
    }
    return PeerStreamEvent(
      type: type,
      error: json['error'] as String?,
      kind: json['kind'] as String?,
      label: json['label'] as String?,
      lastUpdatedAt: DateTime.tryParse(
        json['last_updated_at'] as String? ?? '',
      ),
      streamId: json['stream_id'] as String?,
      text: json['text'] as String?,
    );
  }
}

class WorkspaceEventSession {
  WorkspaceEventSession._(this._channel) {
    _subscription = _channel.messages.listen(
      _handleMessage,
      onError: _events.addError,
      onDone: _events.close,
    );
  }

  static Future<WorkspaceEventSession> open(
    GizClawDataChannelFactory factory,
  ) async {
    final channel = await factory.createDataChannel(
      giznetWebRtcEventDataChannelLabel,
      options: const GizClawDataChannelOptions(ordered: true),
    );
    return WorkspaceEventSession._(channel);
  }

  final GizClawDataChannel _channel;
  final _events = StreamController<PeerStreamEvent>.broadcast();
  late final StreamSubscription<Uint8List> _subscription;
  var _receiveBuffer = Uint8List(0);
  var _closed = false;

  Stream<PeerStreamEvent> get events => _events.stream;

  Future<void> beginAudio(String streamId) {
    if (_closed) {
      throw StateError('workspace event session is closed');
    }
    return _send({
      'v': 1,
      'type': 'bos',
      'kind': 'audio',
      'mime_type': 'audio/opus',
      'stream_id': streamId,
    });
  }

  Future<void> endAudio(String streamId, {String? error}) {
    if (_closed) {
      throw StateError('workspace event session is closed');
    }
    return _send({
      'v': 1,
      'type': 'eos',
      'kind': 'audio',
      'mime_type': 'audio/opus',
      'stream_id': streamId,
      if (error != null && error.isNotEmpty) 'error': error,
    });
  }

  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    await _subscription.cancel();
    await _channel.close();
    await _events.close();
  }

  Future<void> _send(Map<String, Object?> event) {
    return _channel.send(
      encodeFrame(rpcFrameTypeText, utf8.encode(jsonEncode(event))),
    );
  }

  void _handleMessage(Uint8List bytes) {
    try {
      _receiveBuffer = concatBytes([_receiveBuffer, bytes]);
      while (_receiveBuffer.isNotEmpty) {
        final result = tryReadFrame(_receiveBuffer);
        if (result == null) return;
        _receiveBuffer = result.rest;
        final frame = result.frame;
        if (frame.type == rpcFrameTypeEos) {
          unawaited(_events.close());
          return;
        }
        if (frame.type != rpcFrameTypeText && frame.type != rpcFrameTypeJson) {
          throw FormatException(
            'peer stream event frame type ${frame.type} is unsupported',
          );
        }
        final value = jsonDecode(utf8.decode(frame.payload));
        if (value is! Map<String, Object?>) {
          throw const FormatException('peer stream event must be an object');
        }
        _events.add(PeerStreamEvent.fromJson(value));
      }
    } catch (error, stackTrace) {
      _receiveBuffer = Uint8List(0);
      _events.addError(error, stackTrace);
    }
  }
}
