import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;

import 'signaling.dart';
import 'transport.dart';

class FlutterWebRtcDataChannelFactory implements GizClawDataChannelFactory {
  FlutterWebRtcDataChannelFactory(this.peerConnection);

  final rtc.RTCPeerConnection peerConnection;

  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) async {
    final init = rtc.RTCDataChannelInit()
      ..ordered = options.ordered
      ..binaryType = 'binary';
    final maxRetransmits = options.maxRetransmits;
    if (maxRetransmits != null) {
      init.maxRetransmits = maxRetransmits;
    }
    return FlutterWebRtcDataChannel(
      await peerConnection.createDataChannel(label, init),
    );
  }
}

typedef SendGiznetWebRtcOffer =
    Future<List<int>> Function(PreparedGiznetWebRtcOffer offer);

Future<rtc.RTCPeerConnection> connectFlutterGiznetWebRtc({
  bool addAudioTransceiver = false,
  Map<String, dynamic> configuration = const {},
  required Future<PreparedGiznetWebRtcOffer> Function(String offerSdp)
  prepareOffer,
  rtc.RTCPeerConnection? peerConnection,
  required SendGiznetWebRtcOffer sendOffer,
}) async {
  final pc = peerConnection ?? await rtc.createPeerConnection(configuration);
  if (addAudioTransceiver) {
    await pc.addTransceiver(
      kind: rtc.RTCRtpMediaType.RTCRtpMediaTypeAudio,
      init: rtc.RTCRtpTransceiverInit(
        direction: rtc.TransceiverDirection.SendRecv,
      ),
    );
  }
  final offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await _waitForIceGatheringComplete(pc);
  final local = await pc.getLocalDescription();
  final sdp = local?.sdp;
  if (sdp == null || sdp.isEmpty) {
    throw StateError('WebRTC offer was not created');
  }
  final prepared = await prepareOffer(sdp);
  final encryptedAnswer = await sendOffer(prepared);
  final answerSdp = await prepared.openAnswer(encryptedAnswer);
  await pc.setRemoteDescription(rtc.RTCSessionDescription(answerSdp, 'answer'));
  return pc;
}

class FlutterWebRtcDataChannel implements GizClawDataChannel {
  FlutterWebRtcDataChannel(this._channel) {
    _channel.onMessage = (message) {
      if (message.isBinary) {
        _messages.add(Uint8List.fromList(message.binary));
      } else {
        _messages.add(Uint8List.fromList(message.text.codeUnits));
      }
    };
    _channel.onDataChannelState = (state) {
      _states.add(_convertState(state));
      if (state == rtc.RTCDataChannelState.RTCDataChannelClosed) {
        _unawaited(_messages.close());
        _unawaited(_states.close());
      }
    };
  }

  final rtc.RTCDataChannel _channel;
  final _messages = StreamController<Uint8List>.broadcast();
  final _states = StreamController<GizClawDataChannelState>.broadcast();

  @override
  int? get bufferedAmount => _channel.bufferedAmount;

  @override
  String get label => _channel.label ?? '';

  @override
  Stream<Uint8List> get messages => _messages.stream;

  @override
  GizClawDataChannelState get state => _convertState(_channel.state);

  @override
  Stream<GizClawDataChannelState> get states => _states.stream;

  @override
  Future<void> close() => _channel.close();

  @override
  Future<void> send(Uint8List bytes) {
    return _channel.send(rtc.RTCDataChannelMessage.fromBinary(bytes));
  }
}

GizClawDataChannelState _convertState(rtc.RTCDataChannelState? state) {
  switch (state) {
    case rtc.RTCDataChannelState.RTCDataChannelConnecting:
      return GizClawDataChannelState.connecting;
    case rtc.RTCDataChannelState.RTCDataChannelOpen:
      return GizClawDataChannelState.open;
    case rtc.RTCDataChannelState.RTCDataChannelClosing:
      return GizClawDataChannelState.closing;
    case rtc.RTCDataChannelState.RTCDataChannelClosed:
    case null:
      return GizClawDataChannelState.closed;
  }
}

void _unawaited(Future<void> future) {}

Future<void> _waitForIceGatheringComplete(rtc.RTCPeerConnection pc) {
  if (pc.iceGatheringState ==
      rtc.RTCIceGatheringState.RTCIceGatheringStateComplete) {
    return Future.value();
  }
  final completer = Completer<void>();
  final previous = pc.onIceGatheringState;
  pc.onIceGatheringState = (state) {
    previous?.call(state);
    if (state == rtc.RTCIceGatheringState.RTCIceGatheringStateComplete &&
        !completer.isCompleted) {
      completer.complete();
    }
  };
  return completer.future.timeout(const Duration(seconds: 30));
}
