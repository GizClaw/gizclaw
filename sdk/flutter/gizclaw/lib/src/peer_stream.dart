import 'dart:async';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';

import 'generated/events/peer_event.pb.dart' as events;
import 'rpc_frame.dart';
import 'transport.dart';

final _bosEventType = events.PeerEventType.PEER_EVENT_TYPE_BOS;
final _eosEventType = events.PeerEventType.PEER_EVENT_TYPE_EOS;
final _audioStreamKind = events.StreamKind.STREAM_KIND_AUDIO;

class PeerStreamEvent {
  PeerStreamEvent({
    required Object type,
    this.errorCode,
    this.errorMessage,
    this.errorRetryable = false,
    Object? kind,
    this.label,
    this.lastUpdatedAt,
    this.streamId,
    this.text,
    this.workspaceHistoryUpdated,
    this.friendRelationshipUpdated,
    this.friendGroupUpdated,
  }) : eventType = _eventType(type),
       streamKind = _streamKind(kind),
       message = _buildMessage(
         type: _eventType(type),
         errorCode: errorCode,
         errorMessage: errorMessage,
         errorRetryable: errorRetryable,
         kind: _streamKind(kind),
         label: label,
         lastUpdatedAt: lastUpdatedAt,
         streamId: streamId,
         text: text,
         workspaceHistoryUpdated: workspaceHistoryUpdated,
         friendRelationshipUpdated: friendRelationshipUpdated,
         friendGroupUpdated: friendGroupUpdated,
       );

  PeerStreamEvent._(this.message)
    : eventType = message.type,
      errorCode = message.hasEos() && message.eos.hasError()
          ? message.eos.error.code
          : null,
      errorMessage = message.hasEos() && message.eos.hasError()
          ? message.eos.error.message
          : null,
      errorRetryable = message.hasEos() && message.eos.hasError()
          ? message.eos.error.retryable
          : false,
      streamKind = message.hasBos()
          ? message.bos.kind
          : message.hasEos()
          ? message.eos.kind
          : events.StreamKind.STREAM_KIND_UNSPECIFIED,
      label = message.hasBos()
          ? message.bos.label
          : message.hasEos()
          ? message.eos.label
          : message.hasTextDelta()
          ? message.textDelta.label
          : message.hasTextDone()
          ? message.textDone.label
          : null,
      lastUpdatedAt = message.hasWorkspaceHistoryUpdated()
          ? DateTime.fromMillisecondsSinceEpoch(
              message.workspaceHistoryUpdated.lastUpdatedAtUnixMs.toInt(),
              isUtc: true,
            )
          : null,
      streamId = message.hasBos()
          ? message.bos.streamId
          : message.hasEos()
          ? message.eos.streamId
          : message.hasTextDelta()
          ? message.textDelta.streamId
          : message.hasTextDone()
          ? message.textDone.streamId
          : null,
      text = message.hasTextDelta()
          ? message.textDelta.text
          : message.hasTextDone()
          ? message.textDone.text
          : null,
      workspaceHistoryUpdated = message.hasWorkspaceHistoryUpdated()
          ? message.workspaceHistoryUpdated
          : null,
      friendRelationshipUpdated = message.hasFriendRelationshipUpdated()
          ? message.friendRelationshipUpdated
          : null,
      friendGroupUpdated = message.hasFriendGroupUpdated()
          ? message.friendGroupUpdated
          : null;

  final String? errorCode;
  final String? errorMessage;
  final bool errorRetryable;
  final events.FriendGroupUpdated? friendGroupUpdated;
  final events.FriendRelationshipUpdated? friendRelationshipUpdated;
  final events.PeerEventType eventType;
  final String? label;
  final DateTime? lastUpdatedAt;
  final events.PeerEvent message;
  final events.StreamKind streamKind;
  final String? streamId;
  final String? text;
  final events.WorkspaceHistoryUpdated? workspaceHistoryUpdated;

  String? get error => errorMessage;
  String? get kind => switch (streamKind) {
    events.StreamKind.STREAM_KIND_TEXT => 'text',
    events.StreamKind.STREAM_KIND_AUDIO => 'audio',
    events.StreamKind.STREAM_KIND_VIDEO => 'video',
    events.StreamKind.STREAM_KIND_MIXED => 'mixed',
    _ => null,
  };

  String get type => switch (eventType) {
    events.PeerEventType.PEER_EVENT_TYPE_BOS => 'bos',
    events.PeerEventType.PEER_EVENT_TYPE_EOS => 'eos',
    events.PeerEventType.PEER_EVENT_TYPE_TEXT_DELTA => 'text.delta',
    events.PeerEventType.PEER_EVENT_TYPE_TEXT_DONE => 'text.done',
    events.PeerEventType.PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED =>
      'workspace.history.updated',
    events.PeerEventType.PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED =>
      'friend.relationship.updated',
    events.PeerEventType.PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED =>
      'friend_group.updated',
    _ => 'unknown',
  };

  bool get isHistoryReplay => streamId?.startsWith('history-replay-') ?? false;

  static PeerStreamEvent decode(Uint8List bytes) {
    final message = events.PeerEvent.fromBuffer(bytes);
    _validateMessage(message, allowUnknown: true);
    return PeerStreamEvent._(message);
  }
}

events.PeerEventType _eventType(Object value) {
  if (value is events.PeerEventType) return value;
  return switch (value.toString()) {
    'bos' => events.PeerEventType.PEER_EVENT_TYPE_BOS,
    'eos' => events.PeerEventType.PEER_EVENT_TYPE_EOS,
    'text.delta' => events.PeerEventType.PEER_EVENT_TYPE_TEXT_DELTA,
    'text.done' => events.PeerEventType.PEER_EVENT_TYPE_TEXT_DONE,
    'workspace.history.updated' =>
      events.PeerEventType.PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
    'friend.relationship.updated' =>
      events.PeerEventType.PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED,
    'friend_group.updated' =>
      events.PeerEventType.PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED,
    _ => throw FormatException('unsupported peer event type $value'),
  };
}

events.StreamKind _streamKind(Object? value) {
  if (value is events.StreamKind) return value;
  return switch (value?.toString()) {
    'text' => events.StreamKind.STREAM_KIND_TEXT,
    'audio' => events.StreamKind.STREAM_KIND_AUDIO,
    'video' => events.StreamKind.STREAM_KIND_VIDEO,
    'mixed' => events.StreamKind.STREAM_KIND_MIXED,
    _ => events.StreamKind.STREAM_KIND_UNSPECIFIED,
  };
}

class WorkspaceEventSession {
  WorkspaceEventSession._(this._channel) {
    _subscription = _channel.messages.listen(
      _handleMessage,
      onError: _handleChannelError,
      onDone: _handleChannelDone,
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
    return _send(
      PeerStreamEvent(
        type: _bosEventType,
        kind: _audioStreamKind,
        label: 'user',
        streamId: streamId,
      ),
    );
  }

  Future<void> endAudio(String streamId, {String? error}) {
    if (_closed) {
      throw StateError('workspace event session is closed');
    }
    return _send(
      PeerStreamEvent(
        type: _eosEventType,
        kind: _audioStreamKind,
        label: 'user',
        streamId: streamId,
        errorCode: error?.isNotEmpty == true ? 'CLIENT_AUDIO_ERROR' : null,
        errorMessage: error,
      ),
    );
  }

  Future<void> close() async {
    if (_closed) return;
    _closed = true;
    await _subscription.cancel();
    await _channel.close();
    await _events.close();
  }

  Future<void> _send(PeerStreamEvent event) {
    return _channel.send(
      encodeFrame(rpcFrameTypeBinary, event.message.writeToBuffer()),
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
          unawaited(close());
          return;
        }
        if (frame.type != rpcFrameTypeBinary) {
          throw FormatException(
            'peer stream event frame type ${frame.type} is unsupported',
          );
        }
        _events.add(PeerStreamEvent.decode(frame.payload));
      }
    } catch (error, stackTrace) {
      _receiveBuffer = Uint8List(0);
      _fail(error, stackTrace);
    }
  }

  void _handleChannelError(Object error, StackTrace stackTrace) {
    _fail(error, stackTrace);
  }

  void _handleChannelDone() {
    unawaited(close());
  }

  void _fail(Object error, StackTrace stackTrace) {
    if (_closed) return;
    _events.addError(error, stackTrace);
    unawaited(close());
  }
}

events.PeerEvent _buildMessage({
  required events.PeerEventType type,
  required String? errorCode,
  required String? errorMessage,
  required bool errorRetryable,
  required events.StreamKind kind,
  required String? label,
  required DateTime? lastUpdatedAt,
  required String? streamId,
  required String? text,
  required events.WorkspaceHistoryUpdated? workspaceHistoryUpdated,
  required events.FriendRelationshipUpdated? friendRelationshipUpdated,
  required events.FriendGroupUpdated? friendGroupUpdated,
}) {
  final message = events.PeerEvent(version: 1, type: type);
  switch (type) {
    case events.PeerEventType.PEER_EVENT_TYPE_BOS:
      message.bos = events.StreamBegin(
        streamId: streamId ?? '',
        timestampUnixMs: Int64(lastUpdatedAt?.millisecondsSinceEpoch ?? 0),
        kind: kind,
        label: label ?? '',
        mimeType: kind == events.StreamKind.STREAM_KIND_AUDIO
            ? 'audio/opus'
            : '',
      );
    case events.PeerEventType.PEER_EVENT_TYPE_EOS:
      message.eos = events.StreamEnd(
        streamId: streamId ?? '',
        timestampUnixMs: Int64(lastUpdatedAt?.millisecondsSinceEpoch ?? 0),
        kind: kind,
        label: label ?? '',
        mimeType: kind == events.StreamKind.STREAM_KIND_AUDIO
            ? 'audio/opus'
            : '',
        error: errorCode?.isNotEmpty == true || errorMessage?.isNotEmpty == true
            ? events.EventError(
                code: errorCode ?? 'STREAM_ERROR',
                message: errorMessage ?? '',
                retryable: errorRetryable,
              )
            : null,
      );
    case events.PeerEventType.PEER_EVENT_TYPE_TEXT_DELTA:
      message.textDelta = events.TextDelta(
        streamId: streamId ?? '',
        label: label ?? '',
        text: text ?? '',
      );
    case events.PeerEventType.PEER_EVENT_TYPE_TEXT_DONE:
      message.textDone = events.TextDone(
        streamId: streamId ?? '',
        label: label ?? '',
        text: text ?? '',
      );
    case events.PeerEventType.PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED:
      message.workspaceHistoryUpdated =
          workspaceHistoryUpdated ??
          events.WorkspaceHistoryUpdated(
            lastUpdatedAtUnixMs: Int64(
              lastUpdatedAt?.millisecondsSinceEpoch ?? 0,
            ),
          );
    case events.PeerEventType.PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED:
      message.friendRelationshipUpdated =
          friendRelationshipUpdated ?? events.FriendRelationshipUpdated();
    case events.PeerEventType.PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED:
      message.friendGroupUpdated =
          friendGroupUpdated ?? events.FriendGroupUpdated();
    default:
      throw FormatException('unsupported peer event type ${type.value}');
  }
  _validateMessage(message);
  return message;
}

void _validateMessage(events.PeerEvent message, {bool allowUnknown = false}) {
  if (message.version != 1) {
    throw const FormatException('peer event version must be 1');
  }
  final matches = switch (message.type) {
    events.PeerEventType.PEER_EVENT_TYPE_BOS =>
      message.whichPayload() == events.PeerEvent_Payload.bos,
    events.PeerEventType.PEER_EVENT_TYPE_EOS =>
      message.whichPayload() == events.PeerEvent_Payload.eos,
    events.PeerEventType.PEER_EVENT_TYPE_TEXT_DELTA =>
      message.whichPayload() == events.PeerEvent_Payload.textDelta,
    events.PeerEventType.PEER_EVENT_TYPE_TEXT_DONE =>
      message.whichPayload() == events.PeerEvent_Payload.textDone,
    events.PeerEventType.PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED =>
      message.whichPayload() ==
          events.PeerEvent_Payload.workspaceHistoryUpdated,
    events.PeerEventType.PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED =>
      message.whichPayload() ==
          events.PeerEvent_Payload.friendRelationshipUpdated,
    events.PeerEventType.PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED =>
      message.whichPayload() == events.PeerEvent_Payload.friendGroupUpdated,
    _ =>
      allowUnknown && message.whichPayload() == events.PeerEvent_Payload.notSet,
  };
  if (!matches) {
    throw const FormatException('peer event type and payload do not match');
  }
  switch (message.whichPayload()) {
    case events.PeerEvent_Payload.bos:
    case events.PeerEvent_Payload.eos:
    case events.PeerEvent_Payload.textDelta:
    case events.PeerEvent_Payload.textDone:
      if (PeerStreamEvent._(message).streamId?.trim().isEmpty != false) {
        throw const FormatException('stream event requires streamId');
      }
      break;
    case events.PeerEvent_Payload.workspaceHistoryUpdated:
      if (message.workspaceHistoryUpdated.workspaceName.trim().isEmpty) {
        throw const FormatException(
          'workspace history event requires workspaceName',
        );
      }
      break;
    case events.PeerEvent_Payload.friendRelationshipUpdated:
      final payload = message.friendRelationshipUpdated;
      if (payload.peerPublicKey.trim().isEmpty ||
          payload.workspaceName.trim().isEmpty) {
        throw const FormatException(
          'friend relationship event requires peerPublicKey and workspaceName',
        );
      }
      break;
    case events.PeerEvent_Payload.friendGroupUpdated:
      final payload = message.friendGroupUpdated;
      if (payload.friendGroupId.trim().isEmpty ||
          payload.workspaceName.trim().isEmpty) {
        throw const FormatException(
          'friend group event requires friendGroupId and workspaceName',
        );
      }
      break;
    default:
      break;
  }
}
