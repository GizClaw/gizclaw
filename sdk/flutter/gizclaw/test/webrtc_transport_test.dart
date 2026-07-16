import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
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
      expect(pc.addTransceiverCalls.single.track, isNull);
      expect(pc.createOfferCalls, 0);
    },
  );

  test('attaches the exact local audio track before creating offer', () async {
    final pc = _FakePeerConnection(stopAfterAudio: false);
    final track = _FakeMediaStreamTrack(id: 'mic-1', kind: 'audio');
    final stream = _FakeMediaStream('stream-1', [track]);

    final connected = await connectFlutterGiznetWebRtc(
      createPacketDataChannel: false,
      localAudioStream: stream,
      peerConnection: pc,
      prepareOffer: (_) async => _preparedOffer(answerSdp: 'answer-sdp'),
      sendOffer: (_) async => [1, 2, 3],
    );

    expect(connected, same(pc));
    expect(pc.addTransceiverCalls, hasLength(1));
    expect(pc.addTransceiverCalls.single.track, same(track));
    expect(pc.addTransceiverCalls.single.kind, isNull);
    expect(
      pc.addTransceiverCalls.single.init?.direction,
      rtc.TransceiverDirection.SendRecv,
    );
    expect(pc.addTransceiverCalls.single.init?.streams, [same(stream)]);
    expect(pc.operations, ['addTransceiver', 'createOffer']);
  });

  test('rejects a local stream when audio transceiver is disabled', () async {
    final track = _FakeMediaStreamTrack(id: 'mic-1', kind: 'audio');
    final stream = _FakeMediaStream('stream-1', [track]);
    var createCalls = 0;

    await expectLater(
      connectFlutterGiznetWebRtc(
        addAudioTransceiver: false,
        createPeerConnection: (_) async {
          createCalls++;
          return _FakePeerConnection();
        },
        localAudioStream: stream,
        prepareOffer: (_) => throw UnimplementedError(),
        sendOffer: (_) => throw UnimplementedError(),
      ),
      throwsArgumentError,
    );

    expect(createCalls, 0);
  });

  test('rejects a local stream without exactly one audio track', () async {
    for (final tracks in <List<rtc.MediaStreamTrack>>[
      const [],
      [
        _FakeMediaStreamTrack(id: 'mic-1', kind: 'audio'),
        _FakeMediaStreamTrack(id: 'mic-2', kind: 'audio'),
      ],
    ]) {
      await expectLater(
        connectFlutterGiznetWebRtc(
          localAudioStream: _FakeMediaStream('stream-1', tracks),
          prepareOffer: (_) => throw UnimplementedError(),
          sendOffer: (_) => throw UnimplementedError(),
        ),
        throwsArgumentError,
      );
    }
  });

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

  test(
    'closes caller-owned packet channel when connection setup fails',
    () async {
      final pc = _FakePeerConnection(stopAfterAudio: false);

      await expectLater(
        connectFlutterGiznetWebRtc(
          addAudioTransceiver: false,
          peerConnection: pc,
          prepareOffer: (_) async => _preparedOffer(),
          sendOffer: (_) async => throw StateError('send failed'),
        ),
        throwsStateError,
      );

      expect(pc.closeCalls, 0);
      expect(pc.disposeCalls, 0);
      expect(pc.createdDataChannels.single.closeCalls, 1);
    },
  );

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

  test('treats an already-open native data channel as open', () async {
    final pc = _FakePeerConnection(
      channelInitialState: rtc.RTCDataChannelState.RTCDataChannelOpen,
    );
    final factory = FlutterWebRtcDataChannelFactory(pc);

    final channel = await factory.createDataChannel('giznet/v1/service/0');

    expect(pc.dataChannelInits.single.id, -1);
    expect(channel.state, GizClawDataChannelState.open);
  });

  test('waits for open state instead of bufferedAmount readiness', () async {
    final pc = _FakePeerConnection(channelsNativeReady: true);
    final factory = FlutterWebRtcDataChannelFactory(pc);
    var completed = false;

    final future = factory.createDataChannel('giznet/v1/service/0').then((
      channel,
    ) {
      completed = true;
      return channel;
    });
    await Future<void>.delayed(Duration.zero);
    expect(pc.createdDataChannels, hasLength(1));
    await Future<void>.delayed(const Duration(milliseconds: 10));
    expect(completed, isFalse);

    pc.createdDataChannels.single.emitState(
      rtc.RTCDataChannelState.RTCDataChannelOpen,
    );
    final channel = await future;
    expect(channel.state, GizClawDataChannelState.open);
  });

  test('recovers when a native open event is missed', () async {
    final pc = _FakePeerConnection(channelsNativeReady: true);
    final factory = FlutterWebRtcDataChannelFactory(pc);
    var completed = false;

    final future = factory.createDataChannel('giznet/v1/service/0').then((
      channel,
    ) {
      completed = true;
      return channel;
    });
    await Future<void>.delayed(const Duration(milliseconds: 10));
    expect(completed, isFalse);

    final channel = await future;
    expect(channel.state, GizClawDataChannelState.open);
    final getBufferedAmountCalls =
        pc.createdDataChannels.single.getBufferedAmountCalls;
    await Future<void>.delayed(const Duration(milliseconds: 30));
    expect(
      pc.createdDataChannels.single.getBufferedAmountCalls,
      getBufferedAmountCalls,
    );
  });

  test('closes native data channel when open wait fails', () async {
    final pc = _FakePeerConnection(
      channelInitialState: rtc.RTCDataChannelState.RTCDataChannelClosed,
    );
    final factory = FlutterWebRtcDataChannelFactory(pc);

    await expectLater(
      factory.createDataChannel('giznet/v1/service/0'),
      throwsStateError,
    );

    expect(pc.createdDataChannels.single.closeCalls, 1);
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

  test(
    'serves inbound RPC channels as open when native state is transient',
    () async {
      final pc = _FakePeerConnection();
      serveFlutterGiznetWebRtcRpc(pc);

      final channel = _FakeRtcDataChannel(label: 'giznet/v1/service/0');
      pc.onDataChannel?.call(channel);
      channel.emitBinaryMessage(
        _rpcRequestBytes(
          id: 'srv-ping',
          method: rpc.RpcMethod.RPC_METHOD_ALL_PING,
          payloadBytes: encodeRpcRequestPayload(
            'all.ping',
            PingRequest(clientSendTime: fixnum.Int64(1)),
          ),
        ),
      );
      await Future<void>.delayed(Duration.zero);

      final frames = decodeFrames(
        Uint8List.fromList(
          channel.sent.expand((message) => message.binary).toList(),
        ),
      );
      expect(frames, hasLength(2));
      final response = rpc.RpcResponse.fromBuffer(frames.first.payload);
      expect(response.id, 'srv-ping');
      expect(response.hasError(), isFalse);
    },
  );

  test('passes client RPC handlers to inbound channels', () async {
    final pc = _FakePeerConnection();
    serveFlutterGiznetWebRtcRpc(
      pc,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => DeviceInfo(name: 'Test Phone'),
      ),
    );

    final channel = _FakeRtcDataChannel(label: 'giznet/v1/service/0');
    pc.onDataChannel?.call(channel);
    channel.emitBinaryMessage(
      _rpcRequestBytes(
        id: 'srv-info',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_INFO_GET,
        payloadBytes: encodeRpcRequestPayload(
          'client.info.get',
          ClientGetInfoRequest(),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final frames = decodeFrames(
      Uint8List.fromList(
        channel.sent.expand((message) => message.binary).toList(),
      ),
    );
    final response = rpc.RpcResponse.fromBuffer(frames.first.payload);
    final info =
        decodeRpcResponsePayload('client.info.get', response.payload)
            as ClientGetInfoResponse;
    expect(info.value.name, 'Test Phone');
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

  test('splits large service writes into bounded native messages', () async {
    final native = _FakeRtcDataChannel(
      initialState: rtc.RTCDataChannelState.RTCDataChannelOpen,
    );
    final channel = FlutterWebRtcDataChannel(
      native,
      initialState: GizClawDataChannelState.open,
    );
    final bytes = Uint8List.fromList(
      List.generate(3001, (index) => index % 251),
    );

    await channel.send(bytes);

    expect(native.sent.map((message) => message.binary.length), [
      1400,
      1400,
      201,
    ]);
    expect(native.sent.expand((message) => message.binary).toList(), bytes);
  });

  test('waits for bufferedAmount before sending native chunks', () async {
    final native = _FakeRtcDataChannel(
      bufferedAmountValue: 1024 * 1024 + 1,
      initialState: rtc.RTCDataChannelState.RTCDataChannelOpen,
    );
    final channel = FlutterWebRtcDataChannel(
      native,
      initialState: GizClawDataChannelState.open,
    );
    final future = channel.send(Uint8List.fromList([1, 2, 3]));

    await Future<void>.delayed(const Duration(milliseconds: 10));
    expect(native.sent, isEmpty);

    native.bufferedAmountValue = 0;
    await future;
    expect(native.sent, hasLength(1));
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
  const _AddTransceiverCall(this.track, this.kind, this.init);

  final rtc.MediaStreamTrack? track;
  final rtc.RTCRtpMediaType? kind;
  final rtc.RTCRtpTransceiverInit? init;
}

class _FakePeerConnection extends rtc.RTCPeerConnection {
  _FakePeerConnection({
    this.channelInitialState,
    this.channelsNativeReady = false,
    this.completeIceAfterHandler = false,
    this.stopAfterAudio = true,
  });

  final rtc.RTCDataChannelState? channelInitialState;
  final bool channelsNativeReady;
  final bool completeIceAfterHandler;
  final bool stopAfterAudio;
  final addTransceiverCalls = <_AddTransceiverCall>[];
  final createdDataChannels = <_FakeRtcDataChannel>[];
  final dataChannelInits = <rtc.RTCDataChannelInit>[];
  final operations = <String>[];
  int closeCalls = 0;
  int createOfferCalls = 0;
  int disposeCalls = 0;
  rtc.RTCSessionDescription? localDescription;
  rtc.RTCSessionDescription? remoteDescription;

  @override
  Future<rtc.RTCRtpTransceiver> addTransceiver({
    rtc.MediaStreamTrack? track,
    rtc.RTCRtpMediaType? kind,
    rtc.RTCRtpTransceiverInit? init,
  }) async {
    operations.add('addTransceiver');
    addTransceiverCalls.add(_AddTransceiverCall(track, kind, init));
    if (stopAfterAudio) {
      throw _StopAfterAudio();
    }
    return _FakeRtpTransceiver();
  }

  @override
  Future<rtc.RTCDataChannel> createDataChannel(
    String label,
    rtc.RTCDataChannelInit dataChannelDict,
  ) async {
    dataChannelInits.add(dataChannelDict);
    final channel = _FakeRtcDataChannel(
      initialState: channelInitialState,
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
    operations.add('createOffer');
    createOfferCalls++;
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

class _FakeRtpTransceiver extends rtc.RTCRtpTransceiver {
  @override
  Future<rtc.TransceiverDirection?> getCurrentDirection() async =>
      rtc.TransceiverDirection.SendRecv;

  @override
  Future<rtc.TransceiverDirection> getDirection() async =>
      rtc.TransceiverDirection.SendRecv;

  @override
  String get mid => '0';

  @override
  rtc.RTCRtpReceiver get receiver => throw UnimplementedError();

  @override
  rtc.RTCRtpSender get sender => throw UnimplementedError();

  @override
  Future<void> setCodecPreferences(
    List<rtc.RTCRtpCodecCapability> codecs,
  ) async {}

  @override
  Future<void> setDirection(rtc.TransceiverDirection direction) async {}

  @override
  Future<void> stop() async {}

  @override
  bool get stoped => false;

  @override
  String get transceiverId => 'transceiver-0';
}

class _FakeMediaStream extends rtc.MediaStream {
  _FakeMediaStream(String id, this.tracks) : super(id, 'test');

  final List<rtc.MediaStreamTrack> tracks;

  @override
  bool get active => true;

  @override
  Future<void> addTrack(
    rtc.MediaStreamTrack track, {
    bool addToNative = true,
  }) async {
    tracks.add(track);
  }

  @override
  List<rtc.MediaStreamTrack> getAudioTracks() =>
      tracks.where((track) => track.kind == 'audio').toList(growable: false);

  @override
  Future<void> getMediaTracks() async {}

  @override
  List<rtc.MediaStreamTrack> getTracks() => List.unmodifiable(tracks);

  @override
  List<rtc.MediaStreamTrack> getVideoTracks() =>
      tracks.where((track) => track.kind == 'video').toList(growable: false);

  @override
  Future<void> removeTrack(
    rtc.MediaStreamTrack track, {
    bool removeFromNative = true,
  }) async {
    tracks.remove(track);
  }
}

class _FakeMediaStreamTrack extends rtc.MediaStreamTrack {
  _FakeMediaStreamTrack({required this.id, required this.kind});

  @override
  final String id;

  @override
  final String kind;

  @override
  String get label => id;

  @override
  bool enabled = false;

  @override
  bool get muted => false;

  @override
  Future<void> dispose() async {}

  @override
  Future<void> stop() async {}
}

class _FakeRtcDataChannel extends rtc.RTCDataChannel {
  _FakeRtcDataChannel({
    this.bufferedAmountValue = 0,
    rtc.RTCDataChannelState? initialState,
    String label = 'test',
    this.nativeReady = false,
  }) : _label = label,
       _state = initialState {
    stateChangeStream = const Stream.empty();
    messageStream = const Stream.empty();
  }

  final String _label;
  final bool nativeReady;
  rtc.RTCDataChannelState? _state;
  int closeCalls = 0;
  int getBufferedAmountCalls = 0;
  int? bufferedAmountValue;
  final sent = <rtc.RTCDataChannelMessage>[];

  void emitState(rtc.RTCDataChannelState state) {
    _state = state;
    onDataChannelState?.call(state);
  }

  void emitMessage(String text) {
    onMessage?.call(rtc.RTCDataChannelMessage(text));
  }

  void emitBinaryMessage(Uint8List bytes) {
    onMessage?.call(rtc.RTCDataChannelMessage.fromBinary(bytes));
  }

  @override
  int? get bufferedAmount => bufferedAmountValue;

  @override
  Future<int> getBufferedAmount() async {
    getBufferedAmountCalls++;
    if (!nativeReady) throw StateError('Data channel is not open');
    return 0;
  }

  @override
  Future<void> close() async {
    closeCalls++;
    _state = rtc.RTCDataChannelState.RTCDataChannelClosed;
  }

  @override
  int? get id => 1;

  @override
  String? get label => _label;

  @override
  Future<void> send(rtc.RTCDataChannelMessage message) async {
    sent.add(message);
  }

  @override
  rtc.RTCDataChannelState? get state => _state;
}

Uint8List _rpcRequestBytes({
  required String id,
  required rpc.RpcMethod method,
  List<int>? payloadBytes,
}) {
  return concatBytes([
    ...encodeEnvelopeFrames(
      rpc.RpcRequest(
        id: id,
        method: method,
        payload: payloadBytes,
      ).writeToBuffer(),
    ),
    encodeFrame(rpcFrameTypeEos),
  ]);
}
