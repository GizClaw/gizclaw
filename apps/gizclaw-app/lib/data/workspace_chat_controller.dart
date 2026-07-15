import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';

import 'repositories/workspace_chat_repository.dart';

enum WorkspaceChatState { loading, connecting, connected, offline, error }

enum WorkspaceMessageState { complete, streaming, failed }

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
    this.onAccessDenied,
    this.onTransportClosed,
  });

  final GizClawClient? client;
  final GizClawDataChannelFactory? dataChannelFactory;
  final Future<bool> Function()? onAccessDenied;
  final rtc.RTCPeerConnection? peerConnection;
  final Future<void> Function()? onTransportClosed;
  final WorkspaceChatRepository repository;
  final String? serverId;
  final String workspaceName;

  StreamSubscription<List<CachedWorkspaceMessage>>? _historySubscription;
  StreamSubscription<PeerStreamEvent>? _eventSubscription;
  WorkspaceEventSession? _session;
  rtc.MediaStream? _inputStream;
  rtc.MediaStreamTrack? _inputTrack;
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
  bool _inputSenderConfigured = false;
  bool _transportRecoveryRequested = false;
  final Map<String, _AudioEnergySample> _sentAudioEnergy = {};
  final Map<String, _AudioEnergySample> _receivedAudioEnergy = {};
  String? replayingHistoryId;
  bool _disposed = false;
  double inputLevel = 0;
  double outputLevel = 0;

  List<WorkspaceChatMessage> get messages => [..._cached, ..._transient];

  bool get canRecord =>
      state == WorkspaceChatState.connected &&
      _session != null &&
      peerConnection != null;

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

  Future<void> startInput() async {
    final session = _session;
    if (session == null || !canRecord || recording || startingInput) return;
    startingInput = true;
    lastError = null;
    notifyListeners();
    try {
      final track = await _ensureInputTrack();
      track.enabled = true;
      await _bindInputTrack(track);
      final streamId =
          'audio-${DateTime.now().microsecondsSinceEpoch.toRadixString(36)}';
      _activeStreamId = streamId;
      await Future<void>.delayed(const Duration(milliseconds: 260));
      await session.beginAudio(streamId);
      recording = true;
      if (_finishPending) {
        _finishPending = false;
        await finishInput();
      }
    } catch (error) {
      _inputTrack?.enabled = false;
      _activeStreamId = null;
      _finishPending = false;
      _handleError(error, changeState: false);
    } finally {
      startingInput = false;
      notifyListeners();
    }
  }

  Future<void> finishInput({String? error}) async {
    if (startingInput && !recording) {
      _finishPending = true;
      return;
    }
    final session = _session;
    final streamId = _activeStreamId;
    if (session == null || streamId == null || !recording) return;
    try {
      await session.endAudio(streamId, error: error);
    } catch (sendError) {
      _handleError(sendError, changeState: false);
    } finally {
      _inputTrack?.enabled = false;
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
    _levelTimer?.cancel();
    _levelTimer = Timer.periodic(
      const Duration(milliseconds: 90),
      (_) => unawaited(_sampleAudioLevels()),
    );
  }

  Future<void> _sampleAudioLevels() async {
    final pc = peerConnection;
    if (_disposed || pc == null) return;
    try {
      final reports = await pc.getStats();
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
    }
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

  Future<rtc.MediaStreamTrack> _ensureInputTrack() async {
    final existing = _inputTrack;
    if (existing != null) return existing;
    if (peerConnection == null) {
      throw StateError('WebRTC connection is unavailable');
    }
    final media = await rtc.navigator.mediaDevices.getUserMedia({
      'audio': {
        'channelCount': 1,
        'echoCancellation': true,
        'noiseSuppression': true,
      },
      'video': false,
    });
    final tracks = media.getAudioTracks();
    if (tracks.isEmpty) {
      for (final track in media.getTracks()) {
        track.stop();
      }
      throw StateError('Microphone capture returned no audio track');
    }
    final track = tracks.first;
    _inputStream = media;
    _inputTrack = track;
    return track;
  }

  Future<void> _bindInputTrack(rtc.MediaStreamTrack track) async {
    final pc = peerConnection;
    final stream = _inputStream;
    if (pc == null || stream == null) {
      throw StateError('WebRTC microphone stream is unavailable');
    }
    rtc.RTCRtpTransceiver? audioTransceiver;
    for (final transceiver in await pc.getTransceivers()) {
      if (transceiver.sender.track?.kind == 'audio' ||
          transceiver.receiver.track?.kind == 'audio') {
        audioTransceiver = transceiver;
        break;
      }
    }
    if (audioTransceiver == null) {
      throw StateError('WebRTC audio transceiver is unavailable');
    }
    await audioTransceiver.sender.replaceTrack(track);
    if (!_inputSenderConfigured) {
      await audioTransceiver.sender.setStreams([stream]);
      _inputSenderConfigured = true;
    }
    await rtc.Helper.ensureAudioSession();
    await rtc.Helper.setSpeakerphoneOnButPreferBluetooth();
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
      _replaceCachedHistory(history);
      lastError = null;
      notifyListeners();
    } catch (error) {
      if (error is RpcError &&
          _isAccessDenied(error) &&
          onAccessDenied != null) {
        try {
          if (await onAccessDenied!()) {
            _handleError(
              StateError(
                'This workspace was deleted or you no longer have access to it.',
              ),
              changeState: _session == null && _cached.isEmpty,
            );
            return;
          }
        } catch (_) {
          // Preserve the original history error when reconciliation fails.
        }
      }
      _handleError(error, changeState: _session == null && _cached.isEmpty);
    }
  }

  bool _isAccessDenied(RpcError error) =>
      error.code == 400 && error.message.trim() == 'acl: denied';

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

  void _resetRecording() {
    _inputTrack?.enabled = false;
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

  @override
  void dispose() {
    _disposed = true;
    _historyRefreshTimer?.cancel();
    _levelTimer?.cancel();
    _resetRecording();
    _inputTrack?.stop();
    for (final track
        in _inputStream?.getTracks() ?? const <rtc.MediaStreamTrack>[]) {
      track.stop();
    }
    unawaited(_historySubscription?.cancel());
    unawaited(_eventSubscription?.cancel());
    unawaited(_session?.close());
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
  final factor = target > current ? 0.5 : 0.16;
  return current + (target - current) * factor;
}

double _settleLevel(double current, double target) {
  final smoothed = _smoothLevel(current, target);
  return (smoothed - target).abs() < 0.005 ? target : smoothed;
}
