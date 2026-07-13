import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;

import 'peer_rpc_server.dart';
import 'signaling.dart';
import 'transport.dart';

const _dataChannelMessageChunkSize = 1400;
const _dataChannelBufferHighWaterMark = 1024 * 1024;
const _dataChannelSendRetryDelay = Duration(milliseconds: 5);
const _dataChannelNativeReadyGracePeriod = Duration(milliseconds: 250);
const _dataChannelStatePollDelay = Duration(milliseconds: 10);

final _servedPeerConnections = Expando<void Function(rtc.RTCDataChannel)>();

class FlutterWebRtcDataChannelFactory implements GizClawDataChannelFactory {
  FlutterWebRtcDataChannelFactory(this.peerConnection);

  final rtc.RTCPeerConnection peerConnection;

  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) async {
    final init = rtc.RTCDataChannelInit()
      ..id = -1
      ..ordered = options.ordered
      ..binaryType = 'binary';
    final maxRetransmits = options.maxRetransmits;
    if (maxRetransmits != null) {
      init.maxRetransmits = maxRetransmits;
    }
    final channel = await peerConnection.createDataChannel(label, init);
    try {
      await _waitForDataChannelOpen(channel);
    } catch (_) {
      await channel.close();
      rethrow;
    }
    return FlutterWebRtcDataChannel(
      channel,
      initialState: GizClawDataChannelState.open,
    );
  }
}

void serveFlutterGiznetWebRtcRpc(rtc.RTCPeerConnection peerConnection) {
  final installed = _servedPeerConnections[peerConnection];
  if (installed != null && peerConnection.onDataChannel == installed) {
    return;
  }
  final previous = peerConnection.onDataChannel;
  void handler(rtc.RTCDataChannel channel) {
    previous?.call(channel);
    if (channel.label == giznetServiceDataChannelLabel(servicePeerRpc)) {
      serveGizClawPeerRpcChannel(
        FlutterWebRtcDataChannel(
          channel,
          initialState: GizClawDataChannelState.open,
        ),
      );
    }
  }

  _servedPeerConnections[peerConnection] = handler;
  peerConnection.onDataChannel = handler;
}

typedef SendGiznetWebRtcOffer =
    Future<List<int>> Function(PreparedGiznetWebRtcOffer offer);

Future<rtc.RTCPeerConnection> connectFlutterGiznetWebRtc({
  bool addAudioTransceiver = true,
  Future<rtc.RTCPeerConnection> Function(Map<String, dynamic> configuration)
      createPeerConnection =
      rtc.createPeerConnection,
  bool createPacketDataChannel = true,
  Map<String, dynamic> configuration = const {},
  required Future<PreparedGiznetWebRtcOffer> Function(String offerSdp)
  prepareOffer,
  rtc.RTCPeerConnection? peerConnection,
  required SendGiznetWebRtcOffer sendOffer,
}) async {
  final ownsPeerConnection = peerConnection == null;
  final pc = peerConnection ?? await createPeerConnection(configuration);
  rtc.RTCDataChannel? packetDataChannel;
  try {
    serveFlutterGiznetWebRtcRpc(pc);
    if (createPacketDataChannel) {
      final init = rtc.RTCDataChannelInit()
        ..id = -1
        ..ordered = false
        ..maxRetransmits = 0
        ..binaryType = 'binary';
      packetDataChannel = await pc.createDataChannel(
        giznetWebRtcPacketDataChannelLabel,
        init,
      );
    }
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
    await pc.setRemoteDescription(
      rtc.RTCSessionDescription(answerSdp, 'answer'),
    );
    if (packetDataChannel != null) {
      await _waitForDataChannelOpen(packetDataChannel);
    }
    return pc;
  } catch (_) {
    if (packetDataChannel != null) {
      await packetDataChannel.close();
    }
    if (ownsPeerConnection) {
      await _disposePeerConnection(pc);
    }
    rethrow;
  }
}

class FlutterWebRtcDataChannel implements GizClawDataChannel {
  FlutterWebRtcDataChannel(
    this._channel, {
    GizClawDataChannelState? initialState,
  }) : _state = initialState ?? _convertState(_channel.state) {
    _channel.onMessage = (message) {
      if (message.isBinary) {
        _messages.add(Uint8List.fromList(message.binary));
      } else {
        _messages.add(Uint8List.fromList(utf8.encode(message.text)));
      }
    };
    _channel.onDataChannelState = (state) {
      _state = _convertState(state);
      _states.add(_state);
      if (state == rtc.RTCDataChannelState.RTCDataChannelClosed) {
        _unawaited(_messages.close());
        _unawaited(_states.close());
      }
    };
  }

  final rtc.RTCDataChannel _channel;
  GizClawDataChannelState _state;
  final _messages = StreamController<Uint8List>.broadcast();
  final _states = StreamController<GizClawDataChannelState>.broadcast();

  @override
  int? get bufferedAmount => _channel.bufferedAmount;

  @override
  String get label => _channel.label ?? '';

  @override
  Stream<Uint8List> get messages => _messages.stream;

  @override
  GizClawDataChannelState get state => _state;

  @override
  Stream<GizClawDataChannelState> get states => _states.stream;

  @override
  Future<void> close() => _channel.close();

  @override
  Future<void> send(Uint8List bytes) async {
    for (
      var offset = 0;
      offset < bytes.length;
      offset += _dataChannelMessageChunkSize
    ) {
      while ((_channel.bufferedAmount ?? 0) > _dataChannelBufferHighWaterMark) {
        if (_state == GizClawDataChannelState.closed ||
            _state == GizClawDataChannelState.closing) {
          throw StateError('WebRTC data channel closed while sending');
        }
        await Future<void>.delayed(_dataChannelSendRetryDelay);
      }
      if (_state != GizClawDataChannelState.open) {
        throw StateError('WebRTC data channel is $_state, want open');
      }
      final end = offset + _dataChannelMessageChunkSize > bytes.length
          ? bytes.length
          : offset + _dataChannelMessageChunkSize;
      await _channel.send(
        rtc.RTCDataChannelMessage.fromBinary(
          Uint8List.sublistView(bytes, offset, end),
        ),
      );
    }
  }
}

GizClawDataChannelState _convertState(rtc.RTCDataChannelState? state) {
  switch (state) {
    case rtc.RTCDataChannelState.RTCDataChannelConnecting:
    case null:
      return GizClawDataChannelState.connecting;
    case rtc.RTCDataChannelState.RTCDataChannelOpen:
      return GizClawDataChannelState.open;
    case rtc.RTCDataChannelState.RTCDataChannelClosing:
      return GizClawDataChannelState.closing;
    case rtc.RTCDataChannelState.RTCDataChannelClosed:
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
  late void Function(rtc.RTCIceGatheringState) handler;
  void restoreHandler() {
    if (pc.onIceGatheringState == handler) {
      pc.onIceGatheringState = previous;
    }
  }

  void completeIfReady(rtc.RTCIceGatheringState? state) {
    if (state == rtc.RTCIceGatheringState.RTCIceGatheringStateComplete &&
        !completer.isCompleted) {
      restoreHandler();
      completer.complete();
    }
  }

  handler = (state) {
    previous?.call(state);
    completeIfReady(state);
  };
  pc.onIceGatheringState = handler;
  completeIfReady(pc.iceGatheringState);
  return completer.future.timeout(
    const Duration(seconds: 30),
    onTimeout: () {
      restoreHandler();
      throw TimeoutException('WebRTC ICE gathering timed out');
    },
  );
}

Future<void> _waitForDataChannelOpen(rtc.RTCDataChannel channel) {
  final state = channel.state;
  if (state == rtc.RTCDataChannelState.RTCDataChannelOpen) {
    return Future.value();
  }
  if (state == rtc.RTCDataChannelState.RTCDataChannelClosed) {
    throw StateError('WebRTC data channel closed');
  }
  final completer = Completer<void>();
  final previous = channel.onDataChannelState;
  Timer? nativeReadyTimer;
  Timer? pollTimer;
  Timer? timeoutTimer;
  var probingNativeReadiness = false;
  late void Function(rtc.RTCDataChannelState) handler;
  void restoreHandler() {
    nativeReadyTimer?.cancel();
    pollTimer?.cancel();
    timeoutTimer?.cancel();
    if (channel.onDataChannelState == handler) {
      channel.onDataChannelState = previous;
    }
  }

  void completeIfReady(rtc.RTCDataChannelState? state) {
    if (completer.isCompleted) {
      return;
    }
    if (state == rtc.RTCDataChannelState.RTCDataChannelOpen) {
      restoreHandler();
      completer.complete();
    } else if (state == rtc.RTCDataChannelState.RTCDataChannelClosed) {
      restoreHandler();
      completer.completeError(StateError('WebRTC data channel closed'));
    }
  }

  Future<void> probeNativeReadiness() async {
    if (completer.isCompleted || probingNativeReadiness) return;
    completeIfReady(channel.state);
    if (completer.isCompleted || channel.state != null) return;
    probingNativeReadiness = true;
    try {
      await channel.getBufferedAmount();
      if (!completer.isCompleted && channel.state == null) {
        restoreHandler();
        completer.complete();
      }
    } catch (_) {
      // The native channel is not ready yet; the next poll will retry.
    } finally {
      probingNativeReadiness = false;
    }
  }

  handler = (state) {
    previous?.call(state);
    completeIfReady(state);
  };
  channel.onDataChannelState = handler;
  completeIfReady(channel.state);
  if (!completer.isCompleted) {
    nativeReadyTimer = Timer(_dataChannelNativeReadyGracePeriod, () {
      unawaited(probeNativeReadiness());
      pollTimer = Timer.periodic(
        _dataChannelStatePollDelay,
        (_) => unawaited(probeNativeReadiness()),
      );
    });
    timeoutTimer = Timer(const Duration(seconds: 30), () {
      if (completer.isCompleted) return;
      restoreHandler();
      completer.completeError(
        TimeoutException('WebRTC data channel open timed out'),
      );
    });
  }
  return completer.future;
}

Future<void> _disposePeerConnection(rtc.RTCPeerConnection pc) async {
  try {
    await pc.close();
  } catch (_) {}
  try {
    await pc.dispose();
  } catch (_) {}
}
