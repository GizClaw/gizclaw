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

  test('fails closed without linked RTP stats and sends no BOS', () async {
    final harness = await _VoiceHarness.create(statsProvider: () async => []);
    addTearDown(harness.close);

    await harness.controller.startInput();

    expect(harness.channel.sent, isEmpty);
    expect(harness.track.enabled, isFalse);
    expect(
      (harness.controller.lastError as MicrophoneInputException).kind,
      MicrophoneInputFailureKind.statsUnavailable,
    );
  });

  test('stalled RTP counters send no boundary and request recovery', () async {
    var recoveries = 0;
    final harness = await _VoiceHarness.create(
      statsProvider: () async => _audioStats(duration: 1, packets: 1),
      onMicrophoneStalled: () async => recoveries += 1,
    );
    addTearDown(harness.close);

    await harness.controller.startInput();
    await Future<void>.delayed(Duration.zero);

    expect(harness.channel.sent, isEmpty);
    expect(harness.track.enabled, isFalse);
    expect(recoveries, 1);
    expect(
      (harness.controller.lastError as MicrophoneInputException).kind,
      MicrophoneInputFailureKind.stalled,
    );
  });

  test('source samples alone do not satisfy outbound RTP readiness', () async {
    var duration = 0.0;
    var recoveries = 0;
    final harness = await _VoiceHarness.create(
      statsProvider: () async =>
          _audioStats(duration: duration += 1, packets: 1),
      onMicrophoneStalled: () async => recoveries += 1,
    );
    addTearDown(harness.close);

    await harness.controller.startInput();
    await Future<void>.delayed(Duration.zero);

    expect(harness.channel.sent, isEmpty);
    expect(recoveries, 1);
    expect(
      (harness.controller.lastError as MicrophoneInputException).kind,
      MicrophoneInputFailureKind.stalled,
    );
  });

  test('counter advance sends BOS and release sends exactly one EOS', () async {
    var statsCalls = 0;
    final harness = await _VoiceHarness.create(
      statsProvider: () async {
        statsCalls += 1;
        if (statsCalls == 1) return _audioStats(duration: 1, packets: 1);
        if (statsCalls == 2) return _audioStats(duration: 2, packets: 2);
        throw StateError('final stats unavailable');
      },
    );
    addTearDown(harness.close);
    bool? trackEnabledAtBos;
    bool? trackEnabledAtEos;
    harness.channel.onSend = (bytes) {
      final payload = decodeFrames(bytes).single.payload;
      final event = jsonDecode(utf8.decode(payload)) as Map<String, dynamic>;
      switch (event['type']) {
        case 'bos':
          trackEnabledAtBos = harness.track.enabled;
        case 'eos':
          trackEnabledAtEos = harness.track.enabled;
      }
    };

    await harness.controller.startInput();
    expect(trackEnabledAtBos, isFalse);
    expect(harness.track.enabled, isTrue);
    final eosGate = Completer<void>();
    harness.channel.sendGate = eosGate.future;
    final finish = harness.controller.finishInput();
    await Future<void>.delayed(Duration.zero);

    expect(harness.track.enabled, isFalse);
    expect(trackEnabledAtEos, isFalse);

    eosGate.complete();
    await finish;
    harness.channel.sendGate = null;
    await Future.wait([
      harness.controller.finishInput(),
      harness.controller.finishInput(),
    ]);

    expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
    await harness.controller.close();
    expect(harness.track.stopCalls, 0);
  });

  test('does not send a new BOS while EOS is still in flight', () async {
    var counter = 0;
    final harness = await _VoiceHarness.create(
      statsProvider: () async =>
          _audioStats(duration: (++counter).toDouble(), packets: counter),
    );
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

  test('sends EOS before requesting stalled microphone recovery', () async {
    var statsCalls = 0;
    List<String>? eventsAtRecovery;
    late final _VoiceHarness harness;
    harness = await _VoiceHarness.create(
      statsProvider: () async {
        statsCalls += 1;
        return _audioStats(
          duration: statsCalls.toDouble(),
          packets: statsCalls < 3 ? statsCalls : 2,
        );
      },
      onMicrophoneStalled: () async {
        eventsAtRecovery = _sentEventTypes(harness.channel);
      },
    );
    addTearDown(harness.close);

    await harness.controller.startInput();
    await harness.controller.finishInput();
    await Future<void>.delayed(Duration.zero);

    expect(eventsAtRecovery, ['bos', 'eos']);
  });

  test(
    'captures microphone recovery failures without an unhandled error',
    () async {
      final harness = await _VoiceHarness.create(
        statsProvider: () async => _audioStats(duration: 1, packets: 1),
        onMicrophoneStalled: () async {
          throw StateError('recovery failed');
        },
      );
      addTearDown(harness.close);

      await harness.controller.startInput();
      await Future<void>.delayed(Duration.zero);

      expect(harness.controller.lastError, isA<StateError>());
      expect(
        harness.controller.lastError.toString(),
        contains('recovery failed'),
      );
    },
  );

  test(
    'early release waits for readiness then sends one BOS and EOS',
    () async {
      var statsCalls = 0;
      final readinessStarted = Completer<void>();
      final readiness = Completer<List<rtc.StatsReport>>();
      final harness = await _VoiceHarness.create(
        statsProvider: () {
          statsCalls += 1;
          if (statsCalls == 1) {
            return Future.value(_audioStats(duration: 1, packets: 1));
          }
          if (statsCalls == 2) {
            readinessStarted.complete();
            return readiness.future;
          }
          return Future.value(_audioStats(duration: 3, packets: 3));
        },
      );
      addTearDown(harness.close);

      final start = harness.controller.startInput();
      await readinessStarted.future;
      await harness.controller.finishInput();
      readiness.complete(_audioStats(duration: 2, packets: 2));
      await start;

      expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
      expect(harness.track.enabled, isFalse);
    },
  );

  test('reuses the negotiated track across repeated PTT intervals', () async {
    var counter = 0;
    final harness = await _VoiceHarness.create(
      statsProvider: () async =>
          _audioStats(duration: (++counter).toDouble(), packets: counter),
    );
    addTearDown(harness.close);

    await harness.controller.startInput();
    await harness.controller.finishInput();
    await harness.controller.startInput();
    await harness.controller.finishInput();

    expect(_sentEventTypes(harness.channel), ['bos', 'eos', 'bos', 'eos']);
    expect(harness.track.stopCalls, 0);
  });

  test(
    'closing after BOS sends one EOS without stopping borrowed track',
    () async {
      var counter = 0;
      final harness = await _VoiceHarness.create(
        statsProvider: () async =>
            _audioStats(duration: (++counter).toDouble(), packets: counter),
      );
      addTearDown(harness.database.close);

      await harness.controller.startInput();
      await harness.controller.close();

      expect(_sentEventTypes(harness.channel), ['bos', 'eos']);
      expect(harness.track.enabled, isFalse);
      expect(harness.track.stopCalls, 0);
    },
  );

  test(
    'releases a borrowed track before a successor takes ownership',
    () async {
      var counter = 0;
      var ownsInputTrack = true;
      final harness = await _VoiceHarness.create(
        statsProvider: () async =>
            _audioStats(duration: (++counter).toDouble(), packets: counter),
        ownsInputTrack: () => ownsInputTrack,
      );
      addTearDown(harness.database.close);

      await harness.controller.startInput();
      harness.controller.releaseInputTrack();
      expect(harness.track.enabled, isFalse);

      ownsInputTrack = false;
      harness.track.enabled = true;
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

List<rtc.StatsReport> _audioStats({
  required double duration,
  required int packets,
}) => [
  rtc.StatsReport('source-1', 'media-source', 1, {
    'kind': 'audio',
    'trackIdentifier': 'mic-1',
    'totalSamplesDuration': duration,
  }),
  rtc.StatsReport('outbound-1', 'outbound-rtp', 1, {
    'kind': 'audio',
    'mediaSourceId': 'source-1',
    'packetsSent': packets,
  }),
];

List<String> _sentEventTypes(_MemoryDataChannel channel) => channel.sent
    .map((bytes) => decodeFrames(bytes).single.payload)
    .map(utf8.decode)
    .map(jsonDecode)
    .map((value) => (value as Map<String, dynamic>)['type'] as String)
    .toList();

class _VoiceHarness {
  _VoiceHarness(this.controller, this.channel, this.database, this.track);

  static Future<_VoiceHarness> create({
    required WebRtcStatsProvider statsProvider,
    Future<void> Function()? onMicrophoneStalled,
    bool Function()? ownsInputTrack,
  }) async {
    final database = AppDatabase.forTesting(NativeDatabase.memory());
    final factory = _MemoryDataChannelFactory();
    final track = _BorrowedTrack();
    final controller = WorkspaceChatController(
      workspaceName: 'translator',
      repository: _EmptyHistoryRepository(database),
      serverId: 'server-a',
      client: GizClawClient(factory),
      dataChannelFactory: factory,
      peerConnection: _StatsPeerConnection(),
      inputTrack: track,
      ownsInputTrack: ownsInputTrack,
      statsProvider: statsProvider,
      readinessPollInterval: const Duration(milliseconds: 1),
      readinessTimeout: const Duration(milliseconds: 5),
      onMicrophoneStalled: onMicrophoneStalled,
    );
    await controller.start(activate: false);
    return _VoiceHarness(controller, factory.channel, database, track);
  }

  final WorkspaceChatController controller;
  final _MemoryDataChannel channel;
  final AppDatabase database;
  final _BorrowedTrack track;

  Future<void> close() async {
    await controller.close();
    await database.close();
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
  bool enabled = false;

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
