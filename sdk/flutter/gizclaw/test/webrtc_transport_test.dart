import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

void main() {
  test(
    'prepares packet, audio, and inbound RPC before creating offer',
    () async {
      final pc = _FakePeerConnection();

      await expectLater(
        connectFlutterGiznetWebRtc(
          peerConnection: pc,
          prepareOffer: (_) => throw UnimplementedError(),
          sendOffer: (_) => throw UnimplementedError(),
        ),
        throwsA(isA<_StopAfterAudio>()),
      );

      expect(pc.onDataChannel, isNotNull);
      expect(pc.createdDataChannels, hasLength(1));
      expect(
        pc.createdDataChannels.single.label,
        giznetWebRtcPacketDataChannelLabel,
      );
      expect(pc.dataChannelInits.single.ordered, isFalse);
      expect(pc.dataChannelInits.single.maxRetransmits, 0);
      expect(pc.dataChannelInits.single.id, -1);
      expect(pc.addTransceiverCalls, hasLength(1));
      expect(
        pc.addTransceiverCalls.single.kind,
        rtc.RTCRtpMediaType.RTCRtpMediaTypeAudio,
      );
      expect(
        pc.addTransceiverCalls.single.init?.direction,
        rtc.TransceiverDirection.SendRecv,
      );
    },
  );

  test('disposes owned peer connection when signaling fails', () async {
    final pc = _FakePeerConnection(stopAfterAudio: false);
    final error = StateError('send failed');

    await expectLater(
      connectFlutterGiznetWebRtc(
        addAudioTransceiver: false,
        createPacketDataChannel: false,
        createPeerConnection: (_) async => pc,
        prepareOffer: (_) async => _preparedOffer(),
        sendOffer: (_) async => throw error,
      ),
      throwsA(same(error)),
    );

    expect(pc.closeCalls, 1);
    expect(pc.disposeCalls, 1);
  });

  test('does not dispose caller-provided peer connection on failure', () async {
    final pc = _FakePeerConnection(stopAfterAudio: false);

    await expectLater(
      connectFlutterGiznetWebRtc(
        addAudioTransceiver: false,
        createPacketDataChannel: false,
        peerConnection: pc,
        prepareOffer: (_) async => _preparedOffer(),
        sendOffer: (_) async => throw StateError('send failed'),
      ),
      throwsStateError,
    );

    expect(pc.closeCalls, 0);
    expect(pc.disposeCalls, 0);
  });

  test('rechecks ICE state after installing the gathering handler', () async {
    final pc = _FakePeerConnection(
      completeIceAfterHandler: true,
      stopAfterAudio: false,
    );
    var previousCalls = 0;
    void previousHandler(rtc.RTCIceGatheringState state) {
      previousCalls++;
    }

    pc.onIceGatheringState = previousHandler;
    final connected = await connectFlutterGiznetWebRtc(
      addAudioTransceiver: false,
      createPacketDataChannel: false,
      peerConnection: pc,
      prepareOffer: (_) async => _preparedOffer(answerSdp: 'answer-sdp'),
      sendOffer: (_) async => [1, 2, 3],
    );

    expect(connected, same(pc));
    expect(pc.onIceGatheringState, same(previousHandler));
    expect(previousCalls, 0);
    expect(pc.remoteDescription?.sdp, 'answer-sdp');
  });

  test('waits for native readiness when the open event was missed', () async {
    final pc = _FakePeerConnection(channelsNativeReady: true);
    final factory = FlutterWebRtcDataChannelFactory(pc);

    final channel = await factory.createDataChannel('giznet/v1/service/0');

    expect(pc.dataChannelInits.single.id, -1);
    expect(channel.state, GizClawDataChannelState.open);
  });

  test('reinstalls inbound RPC handler after app handler replacement', () {
    final pc = _FakePeerConnection();
    serveFlutterGiznetWebRtcRpc(pc);

    var appHandlerCalled = false;
    pc.onDataChannel = (channel) {
      appHandlerCalled = true;
    };
    serveFlutterGiznetWebRtcRpc(pc);

    final channel = _FakeRtcDataChannel(label: 'giznet/v1/service/0');
    pc.onDataChannel?.call(channel);

    expect(appHandlerCalled, isTrue);
    expect(channel.onMessage, isNotNull);
  });

  test('treats a newly created native data channel as connecting', () async {
    final native = _FakeRtcDataChannel();
    final channel = FlutterWebRtcDataChannel(native);

    expect(channel.state, GizClawDataChannelState.connecting);

    final states = <GizClawDataChannelState>[];
    final subscription = channel.states.listen(states.add);
    native.emitState(rtc.RTCDataChannelState.RTCDataChannelOpen);
    await Future<void>.delayed(Duration.zero);
    expect(states, [GizClawDataChannelState.open]);

    native.emitState(rtc.RTCDataChannelState.RTCDataChannelClosed);
    await subscription.asFuture<void>();
    expect(states.last, GizClawDataChannelState.closed);
  });

  test('encodes native text messages as UTF-8', () async {
    final native = _FakeRtcDataChannel();
    final channel = FlutterWebRtcDataChannel(native);

    final message = channel.messages.first;
    native.emitMessage('你好, GizClaw');

    expect(utf8.decode(await message), '你好, GizClaw');
  });
}

class _StopAfterAudio implements Exception {}

PreparedGiznetWebRtcOffer _preparedOffer({String answerSdp = 'answer'}) {
  return PreparedGiznetWebRtcOffer(
    body: Uint8List(0),
    clientPublicKey: 'client',
    nonce: 'nonce',
    openAnswer: (_) async => answerSdp,
    timestamp: 1,
  );
}

class _AddTransceiverCall {
  const _AddTransceiverCall(this.kind, this.init);

  final rtc.RTCRtpMediaType? kind;
  final rtc.RTCRtpTransceiverInit? init;
}

class _FakePeerConnection extends rtc.RTCPeerConnection {
  _FakePeerConnection({
    this.channelsNativeReady = false,
    this.completeIceAfterHandler = false,
    this.stopAfterAudio = true,
  });

  final bool channelsNativeReady;
  final bool completeIceAfterHandler;
  final bool stopAfterAudio;
  final addTransceiverCalls = <_AddTransceiverCall>[];
  final createdDataChannels = <_FakeRtcDataChannel>[];
  final dataChannelInits = <rtc.RTCDataChannelInit>[];
  int closeCalls = 0;
  int disposeCalls = 0;
  rtc.RTCSessionDescription? localDescription;
  rtc.RTCSessionDescription? remoteDescription;

  @override
  Future<rtc.RTCRtpTransceiver> addTransceiver({
    rtc.MediaStreamTrack? track,
    rtc.RTCRtpMediaType? kind,
    rtc.RTCRtpTransceiverInit? init,
  }) async {
    addTransceiverCalls.add(_AddTransceiverCall(kind, init));
    if (stopAfterAudio) {
      throw _StopAfterAudio();
    }
    throw UnimplementedError();
  }

  @override
  Future<rtc.RTCDataChannel> createDataChannel(
    String label,
    rtc.RTCDataChannelInit dataChannelDict,
  ) async {
    dataChannelInits.add(dataChannelDict);
    final channel = _FakeRtcDataChannel(
      label: label,
      nativeReady: channelsNativeReady,
    );
    createdDataChannels.add(channel);
    return channel;
  }

  @override
  rtc.RTCIceGatheringState? get iceGatheringState {
    if (completeIceAfterHandler && onIceGatheringState == null) {
      return rtc.RTCIceGatheringState.RTCIceGatheringStateGathering;
    }
    return rtc.RTCIceGatheringState.RTCIceGatheringStateComplete;
  }

  @override
  Future<rtc.RTCSessionDescription> createOffer([
    Map<String, dynamic> constraints = const {},
  ]) async {
    return rtc.RTCSessionDescription('offer-sdp', 'offer');
  }

  @override
  Future<rtc.RTCSessionDescription> createAnswer([
    Map<String, dynamic> constraints = const {},
  ]) {
    throw UnimplementedError();
  }

  @override
  Future<void> addCandidate(rtc.RTCIceCandidate candidate) {
    throw UnimplementedError();
  }

  @override
  Future<rtc.RTCRtpSender> addTrack(
    rtc.MediaStreamTrack track, [
    rtc.MediaStream? stream,
  ]) {
    throw UnimplementedError();
  }

  @override
  Future<void> addStream(rtc.MediaStream stream) {
    throw UnimplementedError();
  }

  @override
  Future<void> close() async {
    closeCalls++;
  }

  @override
  rtc.RTCDTMFSender createDtmfSender(rtc.MediaStreamTrack track) {
    throw UnimplementedError();
  }

  @override
  Future<void> dispose() async {
    disposeCalls++;
  }

  @override
  rtc.RTCPeerConnectionState? get connectionState => null;

  @override
  Map<String, dynamic> get getConfiguration => const {};

  @override
  Future<rtc.RTCIceConnectionState?> getIceConnectionState() {
    throw UnimplementedError();
  }

  @override
  rtc.RTCIceConnectionState? get iceConnectionState => null;

  @override
  Future<rtc.RTCSessionDescription?> getLocalDescription() async {
    return localDescription;
  }

  @override
  List<rtc.MediaStream?> getLocalStreams() {
    throw UnimplementedError();
  }

  @override
  Future<List<rtc.RTCRtpReceiver>> getReceivers() {
    throw UnimplementedError();
  }

  @override
  Future<rtc.RTCSessionDescription?> getRemoteDescription() async {
    return remoteDescription;
  }

  @override
  List<rtc.MediaStream?> getRemoteStreams() {
    throw UnimplementedError();
  }

  @override
  Future<List<rtc.RTCRtpSender>> getSenders() {
    throw UnimplementedError();
  }

  @override
  Future<rtc.RTCSignalingState?> getSignalingState() {
    throw UnimplementedError();
  }

  @override
  Future<List<rtc.StatsReport>> getStats([rtc.MediaStreamTrack? track]) {
    throw UnimplementedError();
  }

  @override
  Future<List<rtc.RTCRtpTransceiver>> getTransceivers() {
    throw UnimplementedError();
  }

  @override
  Future<void> removeStream(rtc.MediaStream stream) {
    throw UnimplementedError();
  }

  @override
  Future<bool> removeTrack(rtc.RTCRtpSender sender) {
    throw UnimplementedError();
  }

  @override
  Future<void> restartIce() {
    throw UnimplementedError();
  }

  @override
  Future<void> setConfiguration(Map<String, dynamic> configuration) {
    throw UnimplementedError();
  }

  @override
  Future<void> setLocalDescription(
    rtc.RTCSessionDescription description,
  ) async {
    localDescription = description;
  }

  @override
  Future<void> setRemoteDescription(
    rtc.RTCSessionDescription description,
  ) async {
    remoteDescription = description;
  }

  @override
  rtc.RTCSignalingState? get signalingState => null;
}

class _FakeRtcDataChannel extends rtc.RTCDataChannel {
  _FakeRtcDataChannel({String label = 'test', this.nativeReady = false})
    : _label = label {
    stateChangeStream = const Stream.empty();
    messageStream = const Stream.empty();
  }

  final String _label;
  final bool nativeReady;
  rtc.RTCDataChannelState? _state;

  void emitState(rtc.RTCDataChannelState state) {
    _state = state;
    onDataChannelState?.call(state);
  }

  void emitMessage(String text) {
    onMessage?.call(rtc.RTCDataChannelMessage(text));
  }

  @override
  int? get bufferedAmount => 0;

  @override
  Future<int> getBufferedAmount() async {
    if (!nativeReady) throw StateError('Data channel is not open');
    return 0;
  }

  @override
  Future<void> close() async {}

  @override
  int? get id => 1;

  @override
  String? get label => _label;

  @override
  Future<void> send(rtc.RTCDataChannelMessage message) async {}

  @override
  rtc.RTCDataChannelState? get state => _state;
}
