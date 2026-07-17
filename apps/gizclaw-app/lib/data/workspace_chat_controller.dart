import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';

import '../audio/pcm_audio_level_source.dart';
import 'repositories/workspace_chat_repository.dart';

enum WorkspaceChatState { loading, connecting, connected, offline, error }

enum WorkspaceMessageState { complete, streaming, failed }

typedef SetInputSending = Future<void> Function(bool active);
typedef WorkspaceAccessErrorCallback =
    Future<void> Function(
      String workspaceName,
      Object error,
      GizClawClient sourceClient,
      String sourceServerId,
    );

class WorkspaceChatMessage {
  const WorkspaceChatMessage({
    required this.id,
    required this.incoming,
    required this.text,
    required this.state,
    this.replayAvailable = false,
    this.createdAt,
  });

  final DateTime? createdAt;
  final String id;
  final bool incoming;
  final bool replayAvailable;
  final WorkspaceMessageState state;
  final String text;

  WorkspaceChatMessage copyWith({String? text, WorkspaceMessageState? state}) {
    return WorkspaceChatMessage(
      id: id,
      incoming: incoming,
      replayAvailable: replayAvailable,
      text: text ?? this.text,
      state: state ?? this.state,
      createdAt: createdAt,
    );
  }
}

class WorkspaceChatController extends ChangeNotifier {
  WorkspaceChatController({
    required this.workspaceName,
    required this.repository,
    required this.serverId,
    this.client,
    this.dataChannelFactory,
    this.peerConnection,
    this.inputTrack,
    this.setInputSending,
    this.ownsInputTrack,
    this.onTransportClosed,
    this.onWorkspaceAccessError,
    this.pcmAudioLevels,
  });

  final GizClawClient? client;
  final GizClawDataChannelFactory? dataChannelFactory;
  final rtc.RTCPeerConnection? peerConnection;
  final rtc.MediaStreamTrack? inputTrack;
  final SetInputSending? setInputSending;
  final bool Function()? ownsInputTrack;
  final Future<void> Function()? onTransportClosed;
  final WorkspaceAccessErrorCallback? onWorkspaceAccessError;
  final Stream<PcmAudioLevels>? pcmAudioLevels;
  final WorkspaceChatRepository repository;
  final String? serverId;
  final String workspaceName;

  StreamSubscription<List<CachedWorkspaceMessage>>? _historySubscription;
  StreamSubscription<PeerStreamEvent>? _eventSubscription;
  StreamSubscription<PcmAudioLevels>? _pcmLevelSubscription;
  WorkspaceEventSession? _session;
  String? _activeStreamId;
  Timer? _historyRefreshTimer;
  Timer? _levelTimer;
  List<WorkspaceChatMessage> _cached = const [];
  final List<WorkspaceChatMessage> _transient = [];
  final Map<String, Set<String>> _historyIdsAtStreamStart = {};
  WorkspaceChatState state = WorkspaceChatState.loading;
  Object? lastError;
  bool recording = false;
  bool startingInput = false;
  bool playingOutput = false;
  bool _finishPending = false;
  bool _transportRecoveryRequested = false;
  bool _samplingAudioLevels = false;
  bool _hasPcmAudioLevels = false;
  final Map<String, _AudioEnergySample> _sentAudioEnergy = {};
  final Map<String, _AudioEnergySample> _receivedAudioEnergy = {};
  String? replayingHistoryId;
  bool _disposed = false;
  bool _inputTrackReleased = false;
  Future<void>? _closeFuture;
  Future<void>? _finishInputInFlight;
  Future<void>? _startInputInFlight;
  double inputLevel = 0;
  double outputLevel = 0;

  List<WorkspaceChatMessage> get messages => [..._cached, ..._transient];

  bool get canRecord =>
      state == WorkspaceChatState.connected &&
      _session != null &&
      peerConnection != null &&
      inputTrack != null &&
      setInputSending != null &&
      _ownsInputTrack;

  bool get _ownsInputTrack =>
      !_inputTrackReleased && (ownsInputTrack?.call() ?? true);

  Future<void> releaseInputTrack() async {
    if (_inputTrackReleased) return;
    final startInput = _startInputInFlight;
    if (startInput != null) {
      _finishPending = true;
      await startInput;
    }
    if (recording) await finishInput(error: 'interrupted');
    try {
      await _deactivateInputSending();
    } catch (error) {
      lastError = error;
    } finally {
      _inputTrackReleased = true;
    }
  }

  Future<void> start({bool activate = true, bool conversation = true}) async {
    final stableServerId = serverId;
    if (stableServerId != null) {
      _historySubscription = repository
          .watchHistory(stableServerId, workspaceName)
          .listen((history) {
            _replaceCachedHistory(history);
            notifyListeners();
          });
    }
    final activeClient = client;
    final factory = dataChannelFactory;
    if (!conversation) {
      if (stableServerId == null || activeClient == null) {
        state = WorkspaceChatState.offline;
      } else {
        state = WorkspaceChatState.loading;
        notifyListeners();
        await _refreshHistory();
        if (state != WorkspaceChatState.error) {
          state = WorkspaceChatState.connected;
        }
      }
      notifyListeners();
      return;
    }
    if (stableServerId == null ||
        activeClient == null ||
        factory == null ||
        peerConnection == null) {
      state = WorkspaceChatState.offline;
      notifyListeners();
      return;
    }
    state = WorkspaceChatState.connecting;
    notifyListeners();
    try {
      if (activate) {
        try {
          await activeClient.setRunWorkspace(workspaceName);
        } catch (error) {
          throw StateError('select workspace: $error');
        }
        try {
          await activeClient.reloadRunWorkspace();
        } catch (error) {
          throw StateError('start workspace: $error');
        }
      }
      final session = await WorkspaceEventSession.open(factory);
      _session = session;
      _eventSubscription = session.events.listen(
        _handleEvent,
        onError: (Object error) => _handleError(error),
        onDone: () {
          if (_disposed) return;
          assert(() {
            debugPrint('Workspace event channel closed for $workspaceName');
            return true;
          }());
          _resetRecording();
          if (state == WorkspaceChatState.connected) {
            state = WorkspaceChatState.offline;
            notifyListeners();
          }
          _requestTransportRecovery();
        },
      );
      state = WorkspaceChatState.connected;
      _startLevelMonitor();
      notifyListeners();
      await _refreshHistory();
    } catch (error) {
      _handleError(error);
    }
  }

  Future<void> startInput() {
    final session = _session;
    if (_disposed ||
        session == null ||
        !canRecord ||
        recording ||
        _finishInputInFlight != null) {
      return Future.value();
    }
    final active = _startInputInFlight;
    if (active != null) return active;
    late final Future<void> start;
    start = _startInput(session).whenComplete(() {
      if (identical(_startInputInFlight, start)) _startInputInFlight = null;
    });
    return _startInputInFlight = start;
  }

  Future<void> _startInput(WorkspaceEventSession session) async {
    startingInput = true;
    lastError = null;
    notifyListeners();
    try {
      final setSending = setInputSending;
      if (inputTrack == null || setSending == null) {
        throw StateError('Microphone sender is unavailable');
      }
      final streamId =
          'audio-${DateTime.now().microsecondsSinceEpoch.toRadixString(36)}';
      _activeStreamId = streamId;
      await session.beginAudio(streamId);
      if (!_ownsInputTrack) {
        await session.endAudio(streamId, error: 'interrupted');
        _activeStreamId = null;
        return;
      }
      try {
        await setSending(true);
      } catch (error) {
        try {
          await session.endAudio(
            streamId,
            error: 'microphone_sender_enable_failed',
          );
        } catch (_) {
          // Preserve the sender failure as the actionable error.
        }
        _activeStreamId = null;
        rethrow;
      }
      recording = true;
      if (!_ownsInputTrack) {
        await finishInput(error: 'interrupted');
        return;
      }
      if (_finishPending) {
        _finishPending = false;
        await finishInput();
      }
    } catch (error) {
      try {
        await _deactivateInputSending();
      } catch (_) {
        // Preserve the original sender or signaling error.
      }
      _activeStreamId = null;
      _finishPending = false;
      _handleError(error, changeState: false);
    } finally {
      startingInput = false;
      notifyListeners();
    }
  }

  Future<void> finishInput({String? error}) {
    if (startingInput && !recording) {
      _finishPending = true;
      return Future.value();
    }
    final active = _finishInputInFlight;
    if (active != null) return active;
    final session = _session;
    final streamId = _activeStreamId;
    if (session == null || streamId == null || !recording) {
      return Future.value();
    }
    late final Future<void> finish;
    finish = _finishInput(session, streamId, error).whenComplete(() {
      if (identical(_finishInputInFlight, finish)) {
        _finishInputInFlight = null;
      }
    });
    return _finishInputInFlight = finish;
  }

  Future<void> _finishInput(
    WorkspaceEventSession session,
    String streamId,
    String? error,
  ) async {
    var eosError = error;
    try {
      try {
        await _deactivateInputSending();
      } catch (senderError) {
        lastError = senderError;
        eosError ??= 'microphone_sender_disable_failed';
      }
      await session.endAudio(streamId, error: eosError);
    } catch (sendError) {
      _handleError(sendError, changeState: false);
    } finally {
      _activeStreamId = null;
      recording = false;
      notifyListeners();
      _historyRefreshTimer?.cancel();
      _historyRefreshTimer = Timer(
        const Duration(milliseconds: 900),
        _refreshHistory,
      );
    }
  }

  void _startLevelMonitor() {
    final levels = pcmAudioLevels;
    if (levels != null && _pcmLevelSubscription == null) {
      _pcmLevelSubscription = levels.listen(
        _handlePcmAudioLevels,
        onError: (_) => _hasPcmAudioLevels = false,
        onDone: () {
          _hasPcmAudioLevels = false;
          _pcmLevelSubscription = null;
        },
      );
    }
    _levelTimer?.cancel();
    _levelTimer = Timer.periodic(
      const Duration(milliseconds: 100),
      (_) => unawaited(_sampleAudioLevels()),
    );
  }

  Future<void> _sampleAudioLevels() async {
    final pc = peerConnection;
    if (_disposed || pc == null || _samplingAudioLevels || _hasPcmAudioLevels) {
      return;
    }
    if (!recording && !playingOutput) {
      final settledInput = _settleLevel(inputLevel, 0);
      final settledOutput = _settleLevel(outputLevel, 0);
      if (settledInput != inputLevel || settledOutput != outputLevel) {
        inputLevel = settledInput;
        outputLevel = settledOutput;
        notifyListeners();
      }
      return;
    }
    _samplingAudioLevels = true;
    try {
      final reports = await pc.getStats();
      if (_hasPcmAudioLevels) return;
      var input = 0.0;
      var output = 0.0;
      for (final report in reports) {
        final mediaKind = report.values['kind'] ?? report.values['mediaType'];
        if (mediaKind != 'audio') continue;
        final level = normalizedAudioLevel(report.values['audioLevel']);
        if (report.type == 'media-source' || report.type == 'outbound-rtp') {
          final energyLevel = _energyLevelForReport(
            report,
            samples: _sentAudioEnergy,
          );
          input = math.max(input, math.max(level, energyLevel));
        }
        if (report.type == 'inbound-rtp') {
          final energyLevel = _energyLevelForReport(
            report,
            samples: _receivedAudioEnergy,
          );
          output = math.max(output, math.max(level, energyLevel));
        }
      }
      final nextInput = recording ? input : 0.0;
      final smoothedInput = _settleLevel(inputLevel, nextInput);
      final smoothedOutput = _settleLevel(outputLevel, output);
      if ((smoothedInput - inputLevel).abs() < 0.0001 &&
          (smoothedOutput - outputLevel).abs() < 0.0001) {
        return;
      }
      inputLevel = smoothedInput;
      outputLevel = smoothedOutput;
      notifyListeners();
    } catch (_) {
      // Stats are advisory and must not interrupt the conversation.
    } finally {
      _samplingAudioLevels = false;
    }
  }

  void _handlePcmAudioLevels(PcmAudioLevels levels) {
    if (_disposed) return;
    _hasPcmAudioLevels = true;
    final nextInput = recording ? levels.input : 0.0;
    final nextOutput = levels.output;
    if ((nextInput - inputLevel).abs() < 0.0001 &&
        (nextOutput - outputLevel).abs() < 0.0001) {
      return;
    }
    inputLevel = nextInput;
    outputLevel = nextOutput;
    notifyListeners();
  }

  Future<void> replayHistory(String historyId) async {
    final activeClient = client;
    if (activeClient == null || replayingHistoryId != null) return;
    replayingHistoryId = historyId;
    lastError = null;
    notifyListeners();
    try {
      final response = await activeClient.playRunWorkspaceHistory(historyId);
      if (!response.value.accepted) {
        final message = response.value.message.trim();
        throw StateError(message.isEmpty ? 'Replay was not accepted' : message);
      }
    } catch (error) {
      _handleError(error, changeState: false);
    } finally {
      replayingHistoryId = null;
      notifyListeners();
    }
  }

  void _handleEvent(PeerStreamEvent event) {
    if (event.error?.isNotEmpty == true && event.error != 'interrupted') {
      _handleError(StateError(event.error!), changeState: false);
    }
    final assistantAudio =
        event.label?.toLowerCase() == 'assistant' &&
        (event.kind?.toLowerCase() == 'audio' || event.type == 'eos');
    if (assistantAudio && event.type == 'bos') {
      playingOutput = true;
      notifyListeners();
    } else if (assistantAudio && event.type == 'eos') {
      playingOutput = false;
      notifyListeners();
    }
    if (event.type == 'workspace.history.updated') {
      _historyRefreshTimer?.cancel();
      _historyRefreshTimer = Timer(
        const Duration(milliseconds: 500),
        _refreshHistory,
      );
      return;
    }
    if (event.isHistoryReplay) return;
    if (event.type != 'text.delta' && event.type != 'text.done') return;
    final text = event.text ?? '';
    final label = event.label ?? '';
    final transcript = label.toLowerCase().contains('transcript');
    final id = 'stream-${event.streamId ?? 'assistant'}-$label';
    var index = _transient.indexWhere((message) => message.id == id);
    final done = event.type == 'text.done';
    final accumulatedText = index < 0 ? '' : _transient[index].text;
    final completedText = done && text.startsWith(accumulatedText)
        ? text
        : accumulatedText + text;
    if (index < 0) {
      _historyIdsAtStreamStart[id] = _cached
          .map((message) => message.id)
          .toSet();
      _transient.add(
        WorkspaceChatMessage(
          id: id,
          incoming: !transcript,
          text: text,
          state: done
              ? WorkspaceMessageState.complete
              : WorkspaceMessageState.streaming,
          createdAt: DateTime.now(),
        ),
      );
      index = _transient.length - 1;
    } else {
      final current = _transient[index];
      _transient[index] = current.copyWith(
        text: done ? completedText : current.text + text,
        state: done
            ? WorkspaceMessageState.complete
            : WorkspaceMessageState.streaming,
      );
    }
    if (event.error?.isNotEmpty == true) {
      _transient[index] = _transient[index].copyWith(
        state: WorkspaceMessageState.failed,
      );
    }
    if (done) _removeTransientsNowInHistory();
    notifyListeners();
  }

  @visibleForTesting
  void handleEventForTesting(PeerStreamEvent event) => _handleEvent(event);

  Future<void> _refreshHistory() async {
    final activeClient = client;
    final stableServerId = serverId;
    if (activeClient == null || stableServerId == null) return;
    try {
      final history = await repository.refresh(
        client: activeClient,
        serverId: stableServerId,
        workspaceName: workspaceName,
      );
      if (_disposed) return;
      _replaceCachedHistory(history);
      lastError = null;
      notifyListeners();
    } catch (error) {
      if (_disposed) return;
      await _reconcileWorkspaceAccessError(
        error,
        sourceClient: activeClient,
        sourceServerId: stableServerId,
      );
      if (_disposed) return;
      _handleError(error, changeState: _session == null && _cached.isEmpty);
    }
  }

  Future<void> _reconcileWorkspaceAccessError(
    Object error, {
    required GizClawClient sourceClient,
    required String sourceServerId,
  }) async {
    final reconcile = onWorkspaceAccessError;
    if (reconcile == null ||
        error is! RpcError ||
        (error.code != 403 && error.code != 404)) {
      return;
    }
    try {
      await reconcile(workspaceName, error, sourceClient, sourceServerId);
    } catch (reconciliationError) {
      assert(() {
        debugPrint(
          'Workspace history reconciliation failed for $workspaceName: '
          '$reconciliationError',
        );
        return true;
      }());
    }
  }

  void _replaceCachedHistory(List<CachedWorkspaceMessage> history) {
    _cached = history
        .map(
          (entry) => WorkspaceChatMessage(
            id: entry.id,
            incoming: entry.incoming,
            text: entry.text,
            state: WorkspaceMessageState.complete,
            replayAvailable: entry.replayAvailable,
            createdAt: entry.createdAt,
          ),
        )
        .toList(growable: false);
    _removeTransientsNowInHistory();
  }

  void _removeTransientsNowInHistory() {
    final resolved = <String>{};
    for (final message in _transient) {
      if (message.state != WorkspaceMessageState.complete) continue;
      final historyAtStart = _historyIdsAtStreamStart[message.id] ?? const {};
      if (_cached.any(
        (cached) =>
            !historyAtStart.contains(cached.id) &&
            cached.incoming == message.incoming &&
            cached.text == message.text,
      )) {
        resolved.add(message.id);
      }
    }
    if (resolved.isEmpty) return;
    _transient.removeWhere((message) => resolved.contains(message.id));
    for (final id in resolved) {
      _historyIdsAtStreamStart.remove(id);
    }
  }

  void _handleError(Object error, {bool changeState = true}) {
    lastError = error;
    assert(() {
      debugPrint('Workspace chat failed for $workspaceName: $error');
      return true;
    }());
    if (changeState) {
      _resetRecording();
      state = WorkspaceChatState.error;
    }
    notifyListeners();
  }

  void _requestTransportRecovery() {
    final recover = onTransportClosed;
    if (_disposed || recover == null || _transportRecoveryRequested) return;
    _transportRecoveryRequested = true;
    unawaited(
      recover().catchError((Object error) {
        if (!_disposed) _handleError(error);
      }),
    );
  }

  Future<void> _deactivateInputSending() async {
    if (!_ownsInputTrack) return;
    await setInputSending?.call(false);
  }

  void _resetRecording() {
    unawaited(_deactivateInputSending().catchError((_) {}));
    _activeStreamId = null;
    recording = false;
    startingInput = false;
    playingOutput = false;
    inputLevel = 0;
    outputLevel = 0;
    _sentAudioEnergy.clear();
    _receivedAudioEnergy.clear();
    _finishPending = false;
  }

  @override
  void notifyListeners() {
    if (_disposed) return;
    super.notifyListeners();
  }

  Future<void> close() => _closeFuture ??= _close();

  Future<void> _close() async {
    _disposed = true;
    await releaseInputTrack();
    _historyRefreshTimer?.cancel();
    _historyRefreshTimer = null;
    _levelTimer?.cancel();
    _levelTimer = null;
    final pcmLevelSubscription = _pcmLevelSubscription;
    _pcmLevelSubscription = null;
    _resetRecording();

    final historySubscription = _historySubscription;
    _historySubscription = null;
    final eventSubscription = _eventSubscription;
    _eventSubscription = null;
    final session = _session;
    _session = null;
    await Future.wait([
      if (historySubscription != null) historySubscription.cancel(),
      if (eventSubscription != null) eventSubscription.cancel(),
      if (pcmLevelSubscription != null) pcmLevelSubscription.cancel(),
      if (session != null) session.close(),
    ]);
  }

  @override
  void dispose() {
    unawaited(close());
    super.dispose();
  }
}

@visibleForTesting
double audioLevelFromEnergyDelta({
  required double previousEnergy,
  required double previousDuration,
  required double energy,
  required double duration,
}) {
  final energyDelta = energy - previousEnergy;
  final durationDelta = duration - previousDuration;
  if (energyDelta < 0 || durationDelta <= 0) return 0;
  return math.sqrt(energyDelta / durationDelta).clamp(0.0, 1.0);
}

@visibleForTesting
double normalizedAudioLevel(Object? value) {
  final level = _statDouble(value);
  if (level <= 0) return 0;
  if (level <= 1) return level;
  return (level / 32767).clamp(0.0, 1.0);
}

double _energyLevelForReport(
  rtc.StatsReport report, {
  required Map<String, _AudioEnergySample> samples,
}) {
  final energy = _statDouble(report.values['totalAudioEnergy']);
  final duration = _statDouble(report.values['totalSamplesDuration']);
  if (energy <= 0 || duration <= 0) return 0;
  final previous = samples[report.id];
  samples[report.id] = _AudioEnergySample(energy: energy, duration: duration);
  if (previous == null) return 0;
  return audioLevelFromEnergyDelta(
    previousEnergy: previous.energy,
    previousDuration: previous.duration,
    energy: energy,
    duration: duration,
  );
}

double _statDouble(Object? value) {
  if (value is num) return value.toDouble();
  if (value is String) return double.tryParse(value) ?? 0;
  return 0;
}

class _AudioEnergySample {
  const _AudioEnergySample({required this.energy, required this.duration});

  final double energy;
  final double duration;
}

double _smoothLevel(double current, double target) {
  final factor = target > current ? 0.86 : 0.72;
  return current + (target - current) * factor;
}

double _settleLevel(double current, double target) {
  final smoothed = _smoothLevel(current, target);
  return (smoothed - target).abs() < 0.005 ? target : smoothed;
}
