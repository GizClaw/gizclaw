import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';

enum MicrophoneAvailability { ready, recovering, unavailable }

enum MicrophoneFailureKind { permissionDenied, captureUnavailable }

@immutable
class MicrophoneStatus {
  const MicrophoneStatus(this.availability, {this.failureKind});

  const MicrophoneStatus.ready()
    : availability = MicrophoneAvailability.ready,
      failureKind = null;

  const MicrophoneStatus.recovering()
    : availability = MicrophoneAvailability.recovering,
      failureKind = null;

  const MicrophoneStatus.unavailable({this.failureKind})
    : availability = MicrophoneAvailability.unavailable;

  final MicrophoneAvailability availability;
  final MicrophoneFailureKind? failureKind;

  @override
  bool operator ==(Object other) =>
      other is MicrophoneStatus &&
      other.availability == availability &&
      other.failureKind == failureKind;

  @override
  int get hashCode => Object.hash(availability, failureKind);
}

typedef AcquireMicrophoneStream = Future<rtc.MediaStream> Function();
typedef FetchGizClawServerInfo = Future<GiznetServerInfo> Function(Uri baseUri);
typedef ConnectGizClawWebRtc =
    Future<rtc.RTCPeerConnection> Function({
      required rtc.MediaStream? localAudioStream,
      required GizClawPeerRpcHandlers peerRpcHandlers,
      required Future<PreparedGiznetWebRtcOffer> Function(String offerSdp)
      prepareOffer,
      required SendGiznetWebRtcOffer sendOffer,
    });
typedef PublishGizClawClientInfo =
    Future<void> Function(GizClawClient client, DeviceInfo deviceInfo);
typedef RegisterGizClawServer =
    Future<void> Function(GizClawClient client, String token);
typedef SetMicrophoneSending = Future<void> Function(bool active);
typedef ConfigureMicrophoneSending =
    Future<SetMicrophoneSending> Function(
      rtc.RTCPeerConnection peerConnection,
      rtc.MediaStreamTrack microphoneTrack,
    );
typedef PrepareAudioOutput = Future<void> Function();

class GizClawConnectionProfile {
  const GizClawConnectionProfile({
    required this.endpoint,
    required this.clientPrivateKey,
    this.clientPublicKey,
    this.registrationToken = '',
  });

  factory GizClawConnectionProfile.fromEnvironment() {
    return const GizClawConnectionProfile(
      endpoint: String.fromEnvironment('GIZCLAW_ENDPOINT'),
      clientPrivateKey: String.fromEnvironment('GIZCLAW_PRIVATE_KEY'),
      registrationToken: String.fromEnvironment('GIZCLAW_REGISTRATION_TOKEN'),
    );
  }

  final String endpoint;
  final String clientPrivateKey;
  final String? clientPublicKey;
  final String registrationToken;

  bool get isConfigured => endpoint.isNotEmpty && clientPrivateKey.isNotEmpty;

  GizClawConnectionProfile copyWith({
    String? endpoint,
    String? registrationToken,
  }) {
    return GizClawConnectionProfile(
      endpoint: endpoint ?? this.endpoint,
      clientPrivateKey: clientPrivateKey,
      clientPublicKey: clientPublicKey,
      registrationToken: registrationToken ?? this.registrationToken,
    );
  }
}

class GizClawConnectionController extends ChangeNotifier {
  GizClawConnectionController(
    GizClawConnectionProfile profile, {
    AcquireMicrophoneStream? acquireMicrophoneStream,
    ConnectGizClawWebRtc? connectWebRtc,
    DeviceInfo? deviceInfo,
    FetchGizClawServerInfo? fetchServerInfo,
    PublishGizClawClientInfo? publishClientInfo,
    RegisterGizClawServer? registerServer,
    ConfigureMicrophoneSending? configureMicrophoneSending,
    PrepareAudioOutput? prepareAudioOutput,
  }) : _acquireMicrophoneStream =
           acquireMicrophoneStream ?? _defaultAcquireMicrophoneStream,
       _configureMicrophoneSending =
           configureMicrophoneSending ?? _defaultConfigureMicrophoneSending,
       _connectWebRtc = connectWebRtc ?? _defaultConnectGizClawWebRtc,
       _deviceInfo = deviceInfo ?? DeviceInfo(name: 'GizClaw App'),
       _fetchServerInfo = fetchServerInfo ?? _defaultFetchServerInfo,
       _prepareAudioOutput = prepareAudioOutput ?? _defaultPrepareAudioOutput,
       _profile = profile,
       _publishClientInfo = publishClientInfo ?? _defaultPublishClientInfo,
       _registerServer = registerServer ?? _defaultRegisterServer;

  GizClawConnectionProfile _profile;
  final AcquireMicrophoneStream _acquireMicrophoneStream;
  final ConfigureMicrophoneSending _configureMicrophoneSending;
  final ConnectGizClawWebRtc _connectWebRtc;
  final DeviceInfo _deviceInfo;
  final FetchGizClawServerInfo _fetchServerInfo;
  final PrepareAudioOutput _prepareAudioOutput;
  final PublishGizClawClientInfo _publishClientInfo;
  final RegisterGizClawServer _registerServer;

  rtc.RTCPeerConnection? _peerConnection;
  rtc.RTCPeerConnection? _pendingPeerConnection;
  rtc.MediaStream? _microphoneStream;
  rtc.MediaStream? _pendingMicrophoneStream;
  rtc.MediaStreamTrack? _microphoneTrack;
  SetMicrophoneSending? _setMicrophoneSending;
  GizClawClient? _client;
  FlutterWebRtcDataChannelFactory? _dataChannelFactory;
  String? _clientPublicKey;
  String? _serverId;
  int _connectionRevision = 0;
  MicrophoneStatus _microphoneStatus = const MicrophoneStatus.unavailable();
  Object? _lastMicrophoneError;
  Future<void>? _closeFuture;
  bool _disposed = false;

  GizClawClient? get client => _client;
  GizClawDataChannelFactory? get dataChannelFactory => _dataChannelFactory;
  rtc.RTCPeerConnection? get peerConnection => _peerConnection;
  rtc.MediaStreamTrack? get microphoneTrack => _microphoneTrack;
  MicrophoneStatus get microphoneStatus => _microphoneStatus;
  Object? get lastMicrophoneError => _lastMicrophoneError;
  String? get clientPublicKey => _clientPublicKey ?? _profile.clientPublicKey;
  String? get serverId => _serverId;
  GizClawConnectionProfile get profile => _profile;
  bool get isConnected =>
      _peerConnection?.connectionState ==
      rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected;

  Future<GizClawClient> connect() async {
    final closing = _closeFuture;
    if (closing != null) await closing;
    if (_disposed) throw StateError('GizClaw connection is disposed');
    if (_client != null && isConnected) return _client!;
    if (_client != null || _peerConnection != null) {
      await close();
    }
    final activeProfile = profile;
    final connectionRevision = _connectionRevision;
    if (!activeProfile.isConfigured) {
      throw StateError('No GizClaw server connection is configured');
    }

    final baseUri = _baseUri(activeProfile.endpoint);
    final info = await _fetchServerInfo(baseUri);
    _ensureCurrentProfile(connectionRevision, activeProfile);
    final identity = GiznetSignalingIdentity(
      clientPrivateKey: base58Decode(activeProfile.clientPrivateKey),
      clientPublicKey: activeProfile.clientPublicKey == null
          ? null
          : base58Decode(activeProfile.clientPublicKey!),
      serverPublicKey: base58Decode(info.publicKey),
    );
    await _configureAppleAudioSession();
    _setMicrophoneStatus(const MicrophoneStatus.recovering());
    rtc.MediaStream? microphoneStream;
    rtc.MediaStreamTrack? microphoneTrack;
    try {
      microphoneStream = await _acquireMicrophoneStream();
      final audioTracks = microphoneStream.getAudioTracks();
      if (audioTracks.length != 1) {
        throw StateError(
          'Microphone capture returned ${audioTracks.length} audio tracks',
        );
      }
      microphoneTrack = audioTracks.single;
      microphoneTrack.enabled = false;
      _pendingMicrophoneStream = microphoneStream;
    } catch (error) {
      if (microphoneStream != null) {
        await _disposeMediaStream(microphoneStream);
      }
      microphoneStream = null;
      microphoneTrack = null;
      _lastMicrophoneError = error;
      _setMicrophoneStatus(
        MicrophoneStatus.unavailable(
          failureKind: _microphoneFailureKind(error),
        ),
      );
    }
    rtc.RTCPeerConnection? peerConnection;
    try {
      _ensureCurrentProfile(connectionRevision, activeProfile);
      String? preparedClientPublicKey;
      peerConnection = await _connectWebRtc(
        localAudioStream: microphoneStream,
        peerRpcHandlers: GizClawPeerRpcHandlers(deviceInfo: () => _deviceInfo),
        prepareOffer: (sdp) async {
          final offer = await prepareEncryptedGiznetWebRtcOffer(identity, sdp);
          preparedClientPublicKey = offer.clientPublicKey;
          return offer;
        },
        sendOffer: (offer) =>
            _sendOffer(baseUri.resolve(info.signalingPath), offer),
      );
      _ensureCurrentProfile(connectionRevision, activeProfile);
      _pendingPeerConnection = peerConnection;
      if (microphoneTrack != null) {
        try {
          final setMicrophoneSending = await _configureMicrophoneSending(
            peerConnection,
            microphoneTrack,
          );
          await setMicrophoneSending(false);
          _setMicrophoneSending = setMicrophoneSending;
        } catch (error) {
          microphoneTrack.enabled = false;
          microphoneTrack = null;
          _lastMicrophoneError = error;
          _setMicrophoneStatus(
            const MicrophoneStatus.unavailable(
              failureKind: MicrophoneFailureKind.captureUnavailable,
            ),
          );
        }
      }
      await _waitForPeerConnection(peerConnection);
      await _prepareAudioPlayback(peerConnection, _prepareAudioOutput);
      _ensureCurrentProfile(connectionRevision, activeProfile);
      final dataChannelFactory = FlutterWebRtcDataChannelFactory(
        peerConnection,
      );
      final client = GizClawClient(dataChannelFactory);
      final registrationToken = activeProfile.registrationToken.trim();
      if (registrationToken.isNotEmpty) {
        await _registerServer(client, registrationToken);
      }
      await _publishClientInfo(client, _deviceInfo);
      _ensureCurrentProfile(connectionRevision, activeProfile);
      _pendingPeerConnection = null;
      _pendingMicrophoneStream = null;
      _peerConnection = peerConnection;
      _observePeerConnectionState(peerConnection, () {
        if (identical(_peerConnection, peerConnection)) notifyListeners();
      });
      _microphoneStream = microphoneStream;
      _microphoneTrack = microphoneTrack;
      if (microphoneTrack != null) {
        _lastMicrophoneError = null;
        microphoneTrack.onEnded = () {
          if (!identical(_microphoneTrack, microphoneTrack)) return;
          _lastMicrophoneError = StateError('Microphone track ended');
          _setMicrophoneStatus(
            const MicrophoneStatus.unavailable(
              failureKind: MicrophoneFailureKind.captureUnavailable,
            ),
          );
        };
        _setMicrophoneStatus(const MicrophoneStatus.ready());
      }
      _serverId = info.publicKey;
      _clientPublicKey = preparedClientPublicKey;
      _dataChannelFactory = dataChannelFactory;
      return _client = client;
    } catch (error, stackTrace) {
      _setMicrophoneSending = null;
      final peerConnections = <rtc.RTCPeerConnection>[];
      if (peerConnection != null &&
          identical(_pendingPeerConnection, peerConnection)) {
        _pendingPeerConnection = null;
        peerConnections.add(peerConnection);
      } else if (peerConnection != null) {
        peerConnections.add(peerConnection);
      }
      final streams = <rtc.MediaStream>[];
      if (identical(_pendingMicrophoneStream, microphoneStream)) {
        _pendingMicrophoneStream = null;
        if (microphoneStream != null) {
          streams.add(microphoneStream);
        }
      }
      _setMicrophoneStatus(const MicrophoneStatus.unavailable());
      try {
        await _disposeWebRtcResources(
          streams: streams,
          peerConnections: peerConnections,
        );
      } catch (cleanupError) {
        if (!kReleaseMode) {
          debugPrint('GizClaw WebRTC cleanup failed: $cleanupError');
        }
      }
      Error.throwWithStackTrace(error, stackTrace);
    }
  }

  Future<GizClawClient> reconnect() async {
    await close();
    return connect();
  }

  Future<void> updateProfile(GizClawConnectionProfile profile) async {
    if (profile.endpoint == _profile.endpoint &&
        profile.clientPrivateKey == _profile.clientPrivateKey &&
        profile.registrationToken == _profile.registrationToken) {
      return;
    }
    _profile = profile;
    await close();
  }

  Future<void> close() {
    final active = _closeFuture;
    if (active != null) return active;
    _connectionRevision += 1;
    late final Future<void> closing;
    closing = _close().whenComplete(() {
      if (identical(_closeFuture, closing)) _closeFuture = null;
    });
    return _closeFuture = closing;
  }

  Future<void> _close() async {
    _client = null;
    _dataChannelFactory = null;
    _clientPublicKey = null;
    _serverId = null;
    final pendingPeerConnection = _pendingPeerConnection;
    _pendingPeerConnection = null;
    final peerConnection = _peerConnection;
    _peerConnection = null;
    final pendingMicrophoneStream = _pendingMicrophoneStream;
    _pendingMicrophoneStream = null;
    final microphoneStream = _microphoneStream;
    _microphoneStream = null;
    final microphoneTrack = _microphoneTrack;
    _microphoneTrack = null;
    final setMicrophoneSending = _setMicrophoneSending;
    _setMicrophoneSending = null;
    if (setMicrophoneSending != null) {
      try {
        await setMicrophoneSending(false);
      } catch (_) {
        // Closing the peer connection below is the final send-side teardown.
      }
    }
    microphoneTrack?.enabled = false;
    microphoneTrack?.onEnded = null;
    _setMicrophoneStatus(const MicrophoneStatus.unavailable());
    final streams = <rtc.MediaStream>[?pendingMicrophoneStream];
    if (microphoneStream != null &&
        !identical(microphoneStream, pendingMicrophoneStream)) {
      streams.add(microphoneStream);
    }
    final peerConnections = <rtc.RTCPeerConnection>[?pendingPeerConnection];
    if (peerConnection != null &&
        !identical(peerConnection, pendingPeerConnection)) {
      peerConnections.add(peerConnection);
    }
    await _disposeWebRtcResources(
      streams: streams,
      peerConnections: peerConnections,
    );
  }

  Future<void> setMicrophoneSending(bool active) async {
    final setMicrophoneSending = _setMicrophoneSending;
    final microphoneTrack = _microphoneTrack;
    if (setMicrophoneSending == null || microphoneTrack == null) {
      throw StateError('GizClaw microphone sender is unavailable');
    }
    try {
      if (!active) microphoneTrack.enabled = false;
      await setMicrophoneSending(active);
      if (active) microphoneTrack.enabled = true;
    } catch (error) {
      microphoneTrack.enabled = false;
      _lastMicrophoneError = error;
      _setMicrophoneStatus(
        const MicrophoneStatus.unavailable(
          failureKind: MicrophoneFailureKind.captureUnavailable,
        ),
      );
      rethrow;
    }
    try {
      await _prepareAudioOutput();
    } catch (error) {
      if (!kReleaseMode) {
        debugPrint(
          'GizClaw audio output route restore after microphone '
          'active=$active failed: $error',
        );
      }
    }
  }

  void _setMicrophoneStatus(MicrophoneStatus status) {
    if (_microphoneStatus == status) return;
    _microphoneStatus = status;
    notifyListeners();
  }

  void _ensureCurrentProfile(
    int revision,
    GizClawConnectionProfile activeProfile,
  ) {
    if (revision != _connectionRevision ||
        !identical(activeProfile, _profile)) {
      throw StateError('GizClaw connection profile changed during setup');
    }
  }

  @override
  void notifyListeners() {
    if (_disposed) return;
    super.notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    unawaited(close());
    super.dispose();
  }
}

Future<void> _defaultRegisterServer(GizClawClient client, String token) async {
  await client.register(token);
}

Future<rtc.MediaStream> _defaultAcquireMicrophoneStream() =>
    rtc.navigator.mediaDevices.getUserMedia({
      'audio': {
        'channelCount': 1,
        'echoCancellation': true,
        'noiseSuppression': true,
      },
      'video': false,
    });

Future<SetMicrophoneSending> _defaultConfigureMicrophoneSending(
  rtc.RTCPeerConnection peerConnection,
  rtc.MediaStreamTrack microphoneTrack,
) async {
  final senders = await peerConnection.getSenders();
  rtc.RTCRtpSender? microphoneSender;
  for (final sender in senders) {
    if (sender.track?.id == microphoneTrack.id) {
      microphoneSender = sender;
      break;
    }
  }
  if (microphoneSender == null) {
    for (final sender in senders) {
      if (sender.track?.kind == 'audio') {
        microphoneSender = sender;
        break;
      }
    }
  }
  if (microphoneSender == null) {
    throw StateError('WebRTC microphone sender is unavailable');
  }

  final initialEncodings = microphoneSender.parameters.encodings;
  if (initialEncodings == null || initialEncodings.isEmpty) {
    throw StateError('WebRTC microphone sender has no encoding');
  }
  var sending = initialEncodings.every((encoding) => encoding.active);
  return (active) async {
    if (sending == active) return;
    if (!kReleaseMode) {
      debugPrint('GizClaw microphone sender: setting active=$active');
    }
    final parameters = microphoneSender!.parameters;
    final encodings = parameters.encodings;
    if (encodings == null || encodings.isEmpty) {
      throw StateError('WebRTC microphone sender has no encoding');
    }
    final previous = [for (final encoding in encodings) encoding.active];
    for (final encoding in encodings) {
      encoding.active = active;
    }
    try {
      if (!await microphoneSender.setParameters(parameters)) {
        throw StateError('WebRTC microphone sender rejected parameters');
      }
      sending = active;
      if (!kReleaseMode) {
        debugPrint('GizClaw microphone sender: active=$active');
      }
    } catch (_) {
      for (var index = 0; index < encodings.length; index += 1) {
        encodings[index].active = previous[index];
      }
      rethrow;
    }
  };
}

Future<rtc.RTCPeerConnection> _defaultConnectGizClawWebRtc({
  required rtc.MediaStream? localAudioStream,
  required GizClawPeerRpcHandlers peerRpcHandlers,
  required Future<PreparedGiznetWebRtcOffer> Function(String offerSdp)
  prepareOffer,
  required SendGiznetWebRtcOffer sendOffer,
}) => connectFlutterGiznetWebRtc(
  addAudioTransceiver: true,
  localAudioStream: localAudioStream,
  peerRpcHandlers: peerRpcHandlers,
  prepareOffer: prepareOffer,
  sendOffer: sendOffer,
);

Future<void> _defaultPublishClientInfo(
  GizClawClient client,
  DeviceInfo deviceInfo,
) async {
  final current = await client.getServerInfo();
  if (current.value.hasName() || current.value.hasEmoji()) return;
  await client.putServerInfo(
    DeviceProfile(name: deviceInfo.hasName() ? deviceInfo.name : null),
  );
}

MicrophoneFailureKind _microphoneFailureKind(Object error) =>
    error.toString().contains('NotAllowedError')
    ? MicrophoneFailureKind.permissionDenied
    : MicrophoneFailureKind.captureUnavailable;

Future<void> _disposeMediaStream(rtc.MediaStream stream) async {
  Object? stopError;
  StackTrace? stopStackTrace;
  try {
    await _stopMediaStreamTracks(stream);
  } catch (error, stackTrace) {
    stopError = error;
    stopStackTrace = stackTrace;
  }
  try {
    await stream.dispose();
  } catch (_) {
    if (stopError == null) rethrow;
  }
  if (stopError case final error?) {
    Error.throwWithStackTrace(error, stopStackTrace!);
  }
}

Future<void> _stopMediaStreamTracks(rtc.MediaStream stream) =>
    Future.wait([for (final track in stream.getTracks()) track.stop()]);

Future<void> _prepareAudioPlayback(
  rtc.RTCPeerConnection peerConnection,
  PrepareAudioOutput prepareAudioOutput,
) async {
  for (final receiver in await peerConnection.getReceivers()) {
    final track = receiver.track;
    if (track?.kind == 'audio') track!.enabled = true;
  }
  await prepareAudioOutput();
}

Future<void> _defaultPrepareAudioOutput() async {
  if (Platform.isAndroid || Platform.isIOS) {
    await rtc.Helper.setSpeakerphoneOnButPreferBluetooth();
  }
}

Future<void> _disposePeerConnection(
  rtc.RTCPeerConnection peerConnection,
) async {
  Object? closeError;
  StackTrace? closeStackTrace;
  try {
    await peerConnection.close();
  } catch (error, stackTrace) {
    closeError = error;
    closeStackTrace = stackTrace;
  }
  try {
    await peerConnection.dispose();
  } catch (_) {
    if (closeError == null) rethrow;
  }
  if (closeError case final error?) {
    Error.throwWithStackTrace(error, closeStackTrace!);
  }
}

Future<void> _disposeWebRtcResources({
  required Iterable<rtc.MediaStream> streams,
  required Iterable<rtc.RTCPeerConnection> peerConnections,
}) async {
  Object? firstError;
  StackTrace? firstStackTrace;

  Future<void> attempt(Future<void> Function() action) async {
    try {
      await action();
    } catch (error, stackTrace) {
      firstError ??= error;
      firstStackTrace ??= stackTrace;
    }
  }

  for (final stream in streams) {
    await attempt(() => _disposeMediaStream(stream));
  }
  for (final peerConnection in peerConnections) {
    await attempt(() => _disposePeerConnection(peerConnection));
  }
  if (firstError case final error?) {
    Error.throwWithStackTrace(error, firstStackTrace!);
  }
}

Future<void>? _appleAudioSessionConfiguration;

Future<void> _configureAppleAudioSession() async {
  if (!Platform.isIOS) return;
  final existing = _appleAudioSessionConfiguration;
  if (existing != null) return existing;
  final configuration = _applyAppleAudioSessionConfiguration();
  _appleAudioSessionConfiguration = configuration;
  try {
    await configuration;
  } catch (_) {
    if (identical(_appleAudioSessionConfiguration, configuration)) {
      _appleAudioSessionConfiguration = null;
    }
    rethrow;
  }
}

Future<void> _applyAppleAudioSessionConfiguration() =>
    rtc.Helper.setAppleAudioConfiguration(
      rtc.AppleAudioConfiguration(
        appleAudioCategory: rtc.AppleAudioCategory.playAndRecord,
        appleAudioCategoryOptions: {
          rtc.AppleAudioCategoryOption.allowBluetooth,
          rtc.AppleAudioCategoryOption.defaultToSpeaker,
          rtc.AppleAudioCategoryOption.mixWithOthers,
        },
        appleAudioMode: rtc.AppleAudioMode.voiceChat,
      ),
    );

Future<void> _waitForPeerConnection(rtc.RTCPeerConnection peerConnection) {
  if (peerConnection.connectionState ==
      rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected) {
    return Future.value();
  }
  final completer = Completer<void>();
  final previous = peerConnection.onConnectionState;
  peerConnection.onConnectionState = (state) {
    previous?.call(state);
    if (state == rtc.RTCPeerConnectionState.RTCPeerConnectionStateConnected &&
        !completer.isCompleted) {
      completer.complete();
    } else if ((state ==
                rtc.RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
            state == rtc.RTCPeerConnectionState.RTCPeerConnectionStateClosed) &&
        !completer.isCompleted) {
      completer.completeError(StateError('WebRTC connection failed'));
    }
  };
  return completer.future.timeout(const Duration(seconds: 30));
}

void _observePeerConnectionState(
  rtc.RTCPeerConnection peerConnection,
  VoidCallback onStateChanged,
) {
  final previous = peerConnection.onConnectionState;
  peerConnection.onConnectionState = (state) {
    previous?.call(state);
    if (!kReleaseMode) {
      debugPrint('GizClaw WebRTC connection state: $state');
    }
    onStateChanged();
  };
}

Uri _baseUri(String endpoint) {
  final normalized = normalizeGizClawEndpoint(endpoint);
  final value = normalized.contains('://') ? normalized : 'http://$normalized';
  final uri = Uri.parse(value);
  if (!uri.hasAuthority) {
    throw FormatException('Invalid GizClaw endpoint');
  }
  return uri.path.endsWith('/') ? uri : uri.replace(path: '${uri.path}/');
}

String normalizeGizClawEndpoint(String endpoint) {
  final trimmed = endpoint.trim();
  if (trimmed.isEmpty) return '';
  final hasScheme = trimmed.contains('://');
  final uri = Uri.tryParse(hasScheme ? trimmed : 'http://$trimmed');
  final explicitPort = _explicitEndpointPort(trimmed, hasScheme: hasScheme);
  if (uri == null ||
      !uri.hasAuthority ||
      uri.host.isEmpty ||
      explicitPort == null ||
      explicitPort < 1 ||
      explicitPort > 65535 ||
      uri.userInfo.isNotEmpty ||
      uri.hasQuery ||
      uri.hasFragment ||
      (uri.path.isNotEmpty && uri.path != '/') ||
      (uri.scheme != 'http' && uri.scheme != 'https')) {
    throw const FormatException(
      'Use a domain or IP address with a port, for example gizclaw.local:9820',
    );
  }
  final host = uri.host.contains(':') ? '[${uri.host}]' : uri.host;
  if (!hasScheme) {
    return '$host:$explicitPort';
  }
  return '${uri.scheme}://$host:$explicitPort';
}

int? _explicitEndpointPort(String value, {required bool hasScheme}) {
  final authorityStart = hasScheme ? value.indexOf('://') + 3 : 0;
  var authorityEnd = value.length;
  for (final separator in ['/', '?', '#']) {
    final index = value.indexOf(separator, authorityStart);
    if (index >= 0 && index < authorityEnd) authorityEnd = index;
  }
  final authority = value.substring(authorityStart, authorityEnd);
  final separator = authority.lastIndexOf(':');
  if (separator <= 0 || separator == authority.length - 1) return null;
  return int.tryParse(authority.substring(separator + 1));
}

Future<GiznetServerInfo> _defaultFetchServerInfo(Uri baseUri) async {
  final client = HttpClient();
  client.connectionTimeout = _httpRequestTimeout;
  try {
    return await (() async {
      final request = await client.getUrl(baseUri.resolve('/server-info'));
      final response = await request.close();
      final body = await utf8.decoder.bind(response).join();
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw HttpException('server-info failed with ${response.statusCode}');
      }
      return GiznetServerInfo.fromJson(
        jsonDecode(body) as Map<String, Object?>,
      );
    })().timeout(_httpRequestTimeout);
  } finally {
    client.close(force: true);
  }
}

Future<List<int>> _sendOffer(Uri uri, PreparedGiznetWebRtcOffer offer) async {
  final client = HttpClient();
  client.connectionTimeout = _httpRequestTimeout;
  try {
    return await (() async {
      final request = await client.postUrl(uri);
      request.headers.contentType = ContentType.binary;
      request.headers.set('X-Giznet-Nonce', offer.nonce);
      request.headers.set('X-Giznet-Public-Key', offer.clientPublicKey);
      request.headers.set('X-Giznet-Timestamp', offer.timestamp.toString());
      request.add(offer.body);
      final response = await request.close();
      final bytes = await response.fold<List<int>>(<int>[], (all, chunk) {
        all.addAll(chunk);
        return all;
      });
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw HttpException(
          'WebRTC signaling failed with ${response.statusCode}',
        );
      }
      return bytes;
    })().timeout(_httpRequestTimeout);
  } finally {
    client.close(force: true);
  }
}

const _httpRequestTimeout = Duration(seconds: 15);
