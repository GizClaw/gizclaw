import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_app/data/database/app_database.dart';
import 'package:gizclaw_app/data/repositories/workspace_chat_repository.dart';
import 'package:gizclaw_app/data/workspace_chat_controller.dart';

void main() {
  test('derives an inbound PCM level from cumulative WebRTC energy', () {
    expect(
      audioLevelFromEnergyDelta(
        previousEnergy: 1,
        previousDuration: 10,
        energy: 1.04,
        duration: 11,
      ),
      closeTo(0.2, 0.0001),
    );
  });

  test('ignores reset and stalled WebRTC energy counters', () {
    expect(
      audioLevelFromEnergyDelta(
        previousEnergy: 2,
        previousDuration: 10,
        energy: 1,
        duration: 11,
      ),
      0,
    );
    expect(
      audioLevelFromEnergyDelta(
        previousEnergy: 1,
        previousDuration: 10,
        energy: 1.1,
        duration: 10,
      ),
      0,
    );
  });

  test('normalizes standard and legacy WebRTC audio levels', () {
    expect(normalizedAudioLevel(0.25), 0.25);
    expect(normalizedAudioLevel('0.5'), 0.5);
    expect(normalizedAudioLevel(16384), closeTo(0.5, 0.001));
    expect(normalizedAudioLevel(null), 0);
  });

  test('sends BOS before enabling RTP and disables RTP before EOS', () async {
    final harness = await _VoiceHarness.create();
    addTearDown(harness.close);
    bool? sendingAtBos;
    bool? sendingAtEos;
    harness.channel.onSend = (bytes) {
      final type = _eventType(bytes);
      if (type == 'bos') sendingAtBos = harness.sending;
      if (type == 'eos') sendingAtEos = harness.sending;
    };

    await harness.controller.startInput();
    await harness.controller.finishInput();

    expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
    expect(sendingAtBos, isFalse);
    expect(sendingAtEos, isFalse);
    expect(harness.sendingStates, [true, false]);
    expect(harness.track.enabled, isTrue);
  });

  test('does not require WebRTC stats before sending BOS', () async {
    final harness = await _VoiceHarness.create();
    addTearDown(harness.close);

    await harness.controller.startInput();

    expect(_sentEventTypes(harness.channel), ['bos']);
    expect(harness.controller.recording, isTrue);
  });

  test('does not send a new BOS while EOS is still in flight', () async {
    final harness = await _VoiceHarness.create();
    addTearDown(harness.close);

    await harness.controller.startInput();
    final eosGate = Completer<void>();
    harness.channel.sendGate = eosGate.future;
    final finish = harness.controller.finishInput();
    await Future<void>.delayed(Duration.zero);

    await harness.controller.startInput();
    expect(_sentEventTypes(harness.channel), ['bos', 'eos']);

    eosGate.complete();
    await finish;
    harness.channel.sendGate = null;
    await harness.controller.startInput();
    expect(_sentEventTypes(harness.channel), ['bos', 'eos', 'bos']);
  });

  test('sender enable failure closes the BOS interval with EOS', () async {
    final harness = await _VoiceHarness.create(
      setInputSending: (active) async {
        if (active) throw StateError('sender failed');
      },
    );
    addTearDown(harness.close);

    await harness.controller.startInput();

    expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
    expect(harness.controller.recording, isFalse);
    expect(harness.controller.lastError, isA<StateError>());
    expect(harness.controller.lastError.toString(), contains('sender failed'));
  });

  test('early release waits for sender enable then sends one EOS', () async {
    final enableStarted = Completer<void>();
    final enableGate = Completer<void>();
    final harness = await _VoiceHarness.create(
      setInputSending: (active) async {
        if (active) {
          enableStarted.complete();
          await enableGate.future;
        }
      },
    );
    addTearDown(harness.close);

    final start = harness.controller.startInput();
    await enableStarted.future;
    await harness.controller.finishInput();
    enableGate.complete();
    await start;

    expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
  });

  test('reuses the negotiated track across repeated PTT intervals', () async {
    final harness = await _VoiceHarness.create();
    addTearDown(harness.close);

    await harness.controller.startInput();
    await harness.controller.finishInput();
    await harness.controller.startInput();
    await harness.controller.finishInput();

    expect(_sentEventTypes(harness.channel), ['bos', 'eos', 'bos', 'eos']);
    expect(harness.sendingStates, [true, false, true, false]);
    expect(harness.track.stopCalls, 0);
    expect(harness.track.enabled, isTrue);
  });

  test(
    'closing after BOS sends one EOS without stopping borrowed track',
    () async {
      final harness = await _VoiceHarness.create();
      addTearDown(harness.database.close);

      await harness.controller.startInput();
      await harness.controller.close();

      expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
      expect(harness.sending, isFalse);
      expect(harness.track.enabled, isTrue);
      expect(harness.track.stopCalls, 0);
    },
  );

  test(
    'releases a borrowed track before a successor takes ownership',
    () async {
      var ownsInputTrack = true;
      final harness = await _VoiceHarness.create(
        ownsInputTrack: () => ownsInputTrack,
      );
      addTearDown(harness.database.close);

      await harness.controller.startInput();
      await harness.controller.releaseInputTrack();
      expect(harness.sending, isFalse);

      ownsInputTrack = false;
      await harness.controller.close();

      expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
      expect(harness.track.enabled, isTrue);
      expect(harness.track.stopCalls, 0);
    },
  );

  test('appends a final text.done chunk to streamed deltas', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: WorkspaceChatRepository(database),
      serverId: null,
    );
    addTearDown(controller.close);

    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.delta',
        streamId: 'answer-1',
        label: 'assistant',
        text: 'Hello ',
      ),
    );
    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.delta',
        streamId: 'answer-1',
        label: 'assistant',
        text: 'world',
      ),
    );
    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.done',
        streamId: 'answer-1',
        label: 'assistant',
        text: '!',
      ),
    );

    expect(controller.messages.single.text, 'Hello world!');
    expect(controller.messages.single.state, WorkspaceMessageState.complete);
  });

  test('accepts text.done containing the complete streamed text', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: WorkspaceChatRepository(database),
      serverId: null,
    );
    addTearDown(controller.close);

    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.delta',
        streamId: 'answer-2',
        label: 'assistant',
        text: 'Complete ',
      ),
    );
    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.done',
        streamId: 'answer-2',
        label: 'assistant',
        text: 'Complete response',
      ),
    );

    expect(controller.messages.single.text, 'Complete response');
  });

  test('keeps a history-only viewer in error when refresh fails', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: _FailingHistoryRepository(database),
      serverId: 'server-a',
      client: GizClawClient(_NeverDataChannelFactory()),
    );
    addTearDown(controller.close);

    await controller.start(conversation: false);

    expect(controller.state, WorkspaceChatState.error);
    expect(controller.lastError, isA<StateError>());
  });

  test('keeps repeated live text until a new history row arrives', () async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(database.close);
    final repository = _ControlledHistoryRepository(database)
      ..history = const [
        CachedWorkspaceMessage(
          id: 'history-old',
          incoming: true,
          text: 'OK',
          createdAt: null,
          replayAvailable: false,
        ),
      ];
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: repository,
      serverId: 'server-a',
      client: GizClawClient(_NeverDataChannelFactory()),
    );
    addTearDown(controller.close);
    addTearDown(repository.close);
    await controller.start(conversation: false);

    controller.handleEventForTesting(
      const PeerStreamEvent(
        type: 'text.done',
        streamId: 'answer-new',
        label: 'assistant',
        text: 'OK',
      ),
    );

    expect(controller.messages, hasLength(2));
    repository.emit([
      ...repository.history,
      const CachedWorkspaceMessage(
        id: 'history-new',
        incoming: true,
        text: 'OK',
        createdAt: null,
        replayAvailable: false,
      ),
    ]);
    await Future<void>.delayed(Duration.zero);

    expect(controller.messages, hasLength(2));
    expect(controller.messages.map((message) => message.id), [
      'history-old',
      'history-new',
    ]);
  });
}

class _FailingHistoryRepository extends WorkspaceChatRepository {
  _FailingHistoryRepository(super.database);

  @override
  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async {
    throw StateError('history unavailable');
  }
}

class _ControlledHistoryRepository extends WorkspaceChatRepository {
  _ControlledHistoryRepository(super.database);

  final _controller = StreamController<List<CachedWorkspaceMessage>>();
  List<CachedWorkspaceMessage> history = const [];

  @override
  Stream<List<CachedWorkspaceMessage>> watchHistory(
    String serverId,
    String workspaceName,
  ) => _controller.stream;

  @override
  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async => history;

  void emit(List<CachedWorkspaceMessage> value) {
    history = value;
    _controller.add(value);
  }

  Future<void> close() => _controller.close();
}

class _NeverDataChannelFactory implements GizClawDataChannelFactory {
  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) {
    throw UnsupportedError('No data channel is used by this test');
  }
}

List<String> _sentEventTypes(_MemoryDataChannel channel) => channel.sent
    .map((bytes) => decodeFrames(bytes).single.payload)
    .map(utf8.decode)
    .map(jsonDecode)
    .map((value) => (value as Map<String, dynamic>)['type'] as String)
    .toList();

String _eventType(Uint8List bytes) =>
    (jsonDecode(utf8.decode(decodeFrames(bytes).single.payload))
            as Map<String, dynamic>)['type']
        as String;

class _VoiceHarness {
  _VoiceHarness(
    this.controller,
    this.channel,
    this.database,
    this.track,
    this._senderState,
  );

  static Future<_VoiceHarness> create({
    SetInputSending? setInputSending,
    bool Function()? ownsInputTrack,
  }) async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final factory = _MemoryDataChannelFactory();
    final track = _BorrowedTrack();
    final senderState = _SenderState();
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: _EmptyHistoryRepository(database),
      serverId: 'server-a',
      client: GizClawClient(factory),
      dataChannelFactory: factory,
      peerConnection: _StatsPeerConnection(),
      inputTrack: track,
      setInputSending: (active) async {
        await setInputSending?.call(active);
        senderState.set(active);
      },
      ownsInputTrack: ownsInputTrack,
    );
    await controller.start(activate: false);
    return _VoiceHarness(
      controller,
      factory.channel,
      database,
      track,
      senderState,
    );
  }

  final WorkspaceChatController controller;
  final _MemoryDataChannel channel;
  final AppDatabase database;
  final _BorrowedTrack track;
  final _SenderState _senderState;

  bool get sending => _senderState.active;
  List<bool> get sendingStates => _senderState.changes;

  Future<void> close() async {
    await controller.close();
    await database.close();
  }
}

class _SenderState {
  bool active = false;
  final changes = <bool>[];

  void set(bool value) {
    if (active == value) return;
    active = value;
    changes.add(value);
  }
}

class _EmptyHistoryRepository extends WorkspaceChatRepository {
  _EmptyHistoryRepository(super.database);

  @override
  Future<List<CachedWorkspaceMessage>> refresh({
    required GizClawClient client,
    required String serverId,
    required String workspaceName,
  }) async => const [];
}

class _StatsPeerConnection extends Fake implements rtc.RTCPeerConnection {}

class _BorrowedTrack extends Fake implements rtc.MediaStreamTrack {
  @override
  String get id => 'mic-1';

  @override
  String get kind => 'audio';

  @override
  bool enabled = true;

  int stopCalls = 0;

  @override
  Future<void> stop() async => stopCalls += 1;
}

class _MemoryDataChannelFactory implements GizClawDataChannelFactory {
  final channel = _MemoryDataChannel();

  @override
  Future<GizClawDataChannel> createDataChannel(
    String label, {
    GizClawDataChannelOptions options = const GizClawDataChannelOptions(),
  }) async => channel;
}

class _MemoryDataChannel implements GizClawDataChannel {
  final sent = <Uint8List>[];
  final _messages = StreamController<Uint8List>.broadcast();
  Future<void>? sendGate;
  void Function(Uint8List)? onSend;

  @override
  int? get bufferedAmount => 0;

  @override
  String get label => giznetWebRtcEventDataChannelLabel;

  @override
  Stream<Uint8List> get messages => _messages.stream;

  @override
  GizClawDataChannelState get state => GizClawDataChannelState.open;

  @override
  Stream<GizClawDataChannelState> get states => const Stream.empty();

  @override
  Future<void> close() => _messages.close();

  @override
  Future<void> send(Uint8List bytes) async {
    onSend?.call(bytes);
    sent.add(bytes);
    final gate = sendGate;
    if (gate != null) await gate;
  }
}
