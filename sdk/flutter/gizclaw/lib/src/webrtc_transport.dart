import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;

import 'peer_rpc_server.dart';
import 'signaling.dart';
import 'transport.dart';

const _dataChannelMessageChunkSize = 1400;
const _dataChannelBufferHighWaterMark = 1024 * 1024;
const _dataChannelBufferLowWaterMark = 256 * 1024;
const _dataChannelNativeReadyGracePeriod = Duration(milliseconds: 250);
const _dataChannelStatePollDelay = Duration(milliseconds: 250);

final _servedPeerConnections = Expando<_ServedPeerConnection>();

class _ServedPeerConnection {
  _ServedPeerConnection(this.handler, this.handlers);

  final void Function(rtc.RTCDataChannel) handler;
  GizClawPeerRpcHandlers? handlers;
}

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

void serveFlutterGiznetWebRtcRpc(
  rtc.RTCPeerConnection peerConnection, {
  GizClawPeerRpcHandlers? handlers,
}) {
  final installed = _servedPeerConnections[peerConnection];
  if (installed != null && peerConnection.onDataChannel == installed.handler) {
    installed.handlers = handlers;
    return;
  }
  final previous = peerConnection.onDataChannel;
  late final _ServedPeerConnection state;
  void handler(rtc.RTCDataChannel channel) {
    previous?.call(channel);
    if (channel.label == giznetServiceDataChannelLabel(servicePeerRpc)) {
      serveGizClawPeerRpcChannel(
        FlutterWebRtcDataChannel(
          channel,
          initialState: GizClawDataChannelState.open,
        ),
        handlers: state.handlers,
      );
    }
  }

  state = _ServedPeerConnection(handler, handlers);
  _servedPeerConnections[peerConnection] = state;
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
  rtc.MediaStream? localAudioStream,
  required Future<PreparedGiznetWebRtcOffer> Function(String offerSdp)
  prepareOffer,
  GizClawPeerRpcHandlers? peerRpcHandlers,
  rtc.RTCPeerConnection? peerConnection,
  required SendGiznetWebRtcOffer sendOffer,
}) async {
  rtc.MediaStreamTrack? localAudioTrack;
  if (localAudioStream != null) {
    if (!addAudioTransceiver) {
      throw ArgumentError.value(
        addAudioTransceiver,
        'addAudioTransceiver',
        'must be true when localAudioStream is supplied',
      );
    }
    final audioTracks = localAudioStream.getAudioTracks();
    if (audioTracks.length != 1) {
      throw ArgumentError.value(
        localAudioStream,
        'localAudioStream',
        'must contain exactly one audio track',
      );
    }
    localAudioTrack = audioTracks.single;
  }
  final ownsPeerConnection = peerConnection == null;
  final pc = peerConnection ?? await createPeerConnection(configuration);
  final stopPeerConnectionLogging = !kReleaseMode
      ? _logPeerConnectionStates(pc)
      : null;
  rtc.RTCDataChannel? packetDataChannel;
  try {
    serveFlutterGiznetWebRtcRpc(pc, handlers: peerRpcHandlers);
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
      final init = rtc.RTCRtpTransceiverInit(
        direction: rtc.TransceiverDirection.SendRecv,
        streams: localAudioStream == null ? null : [localAudioStream],
      );
      if (localAudioTrack == null) {
        await pc.addTransceiver(
          kind: rtc.RTCRtpMediaType.RTCRtpMediaTypeAudio,
          init: init,
        );
      } else {
        await pc.addTransceiver(track: localAudioTrack, init: init);
      }
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
  } finally {
    stopPeerConnectionLogging?.call();
  }
}

class FlutterWebRtcDataChannel implements GizClawDataChannel {
  FlutterWebRtcDataChannel(
    this._channel, {
    GizClawDataChannelState? initialState,
  }) : _state = initialState ?? _convertState(_channel.state) {
    _channel.bufferedAmountLowThreshold = _dataChannelBufferLowWaterMark;
    _channel.onBufferedAmountLow = (currentAmount) {
      if (currentAmount <= _dataChannelBufferLowWaterMark) {
        _lowWaterWaiter?.complete();
        _lowWaterWaiter = null;
      }
    };
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
      if (state == rtc.RTCDataChannelState.RTCDataChannelClosing ||
          state == rtc.RTCDataChannelState.RTCDataChannelClosed) {
        _failLowWaterWaiter();
      }
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
  Future<void> _sendTail = Future<void>.value();
  Completer<void>? _lowWaterWaiter;
  bool _writeBackpressured = false;

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
  Future<void> close() async {
    _state = GizClawDataChannelState.closing;
    _failLowWaterWaiter();
    await _channel.close();
  }

  @override
  Future<void> send(Uint8List bytes) {
    final previous = _sendTail;
    final released = Completer<void>();
    _sendTail = released.future;
    return _sendAfter(previous, released, bytes);
  }

  Future<void> _sendAfter(
    Future<void> previous,
    Completer<void> released,
    Uint8List bytes,
  ) async {
    try {
      await previous;
      await _sendBytes(bytes);
    } finally {
      released.complete();
    }
  }

  Future<void> _sendBytes(Uint8List bytes) async {
    var sent = false;
    try {
      for (
        var offset = 0;
        offset < bytes.length;
        offset += _dataChannelMessageChunkSize
      ) {
        await _waitForWriteBudget();
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
        sent = true;
      }
    } catch (_) {
      if (sent) {
        _state = GizClawDataChannelState.closing;
        _failLowWaterWaiter();
        try {
          await _channel.close();
        } catch (_) {
          // Preserve the original partial-write failure.
        }
      }
      rethrow;
    }
  }

  Future<void> _waitForWriteBudget() async {
    while (true) {
      if (_state == GizClawDataChannelState.closed ||
          _state == GizClawDataChannelState.closing) {
        throw StateError('WebRTC data channel closed while sending');
      }
      final amount = _channel.bufferedAmount ?? 0;
      if (_writeBackpressured) {
        if (amount <= _dataChannelBufferLowWaterMark) {
          _writeBackpressured = false;
          return;
        }
      } else if (amount < _dataChannelBufferHighWaterMark) {
        return;
      } else {
        _writeBackpressured = true;
      }

      final waiter = Completer<void>();
      _lowWaterWaiter = waiter;
      if ((_channel.bufferedAmount ?? 0) <= _dataChannelBufferLowWaterMark) {
        _lowWaterWaiter = null;
        _writeBackpressured = false;
        return;
      }
      await waiter.future;
    }
  }

  void _failLowWaterWaiter() {
    final waiter = _lowWaterWaiter;
    _lowWaterWaiter = null;
    if (waiter != null && !waiter.isCompleted) {
      waiter.completeError(
        StateError('WebRTC data channel closed while sending'),
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

VoidCallback _logPeerConnectionStates(rtc.RTCPeerConnection pc) {
  final previousConnectionState = pc.onConnectionState;
  late final void Function(rtc.RTCPeerConnectionState) connectionHandler;
  connectionHandler = (state) {
    previousConnectionState?.call(state);
    debugPrint('GizClaw WebRTC peer state: $state');
  };
  pc.onConnectionState = connectionHandler;
  final previousIceConnectionState = pc.onIceConnectionState;
  late final void Function(rtc.RTCIceConnectionState) iceConnectionHandler;
  iceConnectionHandler = (state) {
    previousIceConnectionState?.call(state);
    debugPrint('GizClaw WebRTC ICE state: $state');
  };
  pc.onIceConnectionState = iceConnectionHandler;
  final previousIceGatheringState = pc.onIceGatheringState;
  late final void Function(rtc.RTCIceGatheringState) iceGatheringHandler;
  iceGatheringHandler = (state) {
    previousIceGatheringState?.call(state);
    debugPrint('GizClaw WebRTC ICE gathering state: $state');
  };
  pc.onIceGatheringState = iceGatheringHandler;
  final previousSignalingState = pc.onSignalingState;
  late final void Function(rtc.RTCSignalingState) signalingHandler;
  signalingHandler = (state) {
    previousSignalingState?.call(state);
    debugPrint('GizClaw WebRTC signaling state: $state');
  };
  pc.onSignalingState = signalingHandler;
  return () {
    if (pc.onConnectionState == connectionHandler) {
      pc.onConnectionState = previousConnectionState;
    }
    if (pc.onIceConnectionState == iceConnectionHandler) {
      pc.onIceConnectionState = previousIceConnectionState;
    }
    if (pc.onIceGatheringState == iceGatheringHandler) {
      pc.onIceGatheringState = previousIceGatheringState;
    }
    if (pc.onSignalingState == signalingHandler) {
      pc.onSignalingState = previousSignalingState;
    }
  };
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

  Future<void> startNativeReadinessPolling() async {
    await probeNativeReadiness();
    if (completer.isCompleted) return;
    pollTimer = Timer.periodic(
      _dataChannelStatePollDelay,
      (_) => unawaited(probeNativeReadiness()),
    );
  }

  handler = (state) {
    previous?.call(state);
    completeIfReady(state);
  };
  channel.onDataChannelState = handler;
  completeIfReady(channel.state);
  if (!completer.isCompleted) {
    nativeReadyTimer = Timer(_dataChannelNativeReadyGracePeriod, () {
      unawaited(startNativeReadinessPolling());
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
