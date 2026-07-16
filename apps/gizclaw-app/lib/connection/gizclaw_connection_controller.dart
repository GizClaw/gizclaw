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

class GizClawConnectionProfile {
  const GizClawConnectionProfile({
    required this.endpoint,
    required this.clientPrivateKey,
    this.clientPublicKey,
  });

  factory GizClawConnectionProfile.fromEnvironment() {
    return const GizClawConnectionProfile(
      endpoint: String.fromEnvironment('GIZCLAW_ENDPOINT'),
      clientPrivateKey: String.fromEnvironment('GIZCLAW_PRIVATE_KEY'),
    );
  }

  final String endpoint;
  final String clientPrivateKey;
  final String? clientPublicKey;

  bool get isConfigured => endpoint.isNotEmpty && clientPrivateKey.isNotEmpty;

  GizClawConnectionProfile copyWith({String? endpoint}) {
    return GizClawConnectionProfile(
      endpoint: endpoint ?? this.endpoint,
      clientPrivateKey: clientPrivateKey,
      clientPublicKey: clientPublicKey,
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
  }) : _acquireMicrophoneStream =
           acquireMicrophoneStream ?? _defaultAcquireMicrophoneStream,
       _connectWebRtc = connectWebRtc ?? _defaultConnectGizClawWebRtc,
       _deviceInfo = deviceInfo ?? DeviceInfo(name: 'GizClaw App'),
       _fetchServerInfo = fetchServerInfo ?? _defaultFetchServerInfo,
       _profile = profile,
       _publishClientInfo = publishClientInfo ?? _defaultPublishClientInfo;

  GizClawConnectionProfile _profile;
  final AcquireMicrophoneStream _acquireMicrophoneStream;
  final ConnectGizClawWebRtc _connectWebRtc;
  final DeviceInfo _deviceInfo;
  final FetchGizClawServerInfo _fetchServerInfo;
  final PublishGizClawClientInfo _publishClientInfo;

  rtc.RTCPeerConnection? _peerConnection;
  rtc.RTCPeerConnection? _pendingPeerConnection;
  rtc.MediaStream? _microphoneStream;
  rtc.MediaStream? _pendingMicrophoneStream;
  rtc.MediaStreamTrack? _microphoneTrack;
  GizClawClient? _client;
  FlutterWebRtcDataChannelFactory? _dataChannelFactory;
  String? _clientPublicKey;
  String? _serverId;
  int _profileRevision = 0;
  MicrophoneStatus _microphoneStatus = const MicrophoneStatus.unavailable();
  Object? _lastMicrophoneError;
  bool _disposed = false;

  GizClawClient? get client => _client;
  FlutterWebRtcDataChannelFactory? get dataChannelFactory =>
      _dataChannelFactory;
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
    if (_client != null && isConnected) return _client!;
    final activeProfile = profile;
    final profileRevision = _profileRevision;
    if (!activeProfile.isConfigured) {
      throw StateError('No GizClaw server connection is configured');
    }

    if (_client != null || _peerConnection != null) {
      await close();
    }

    final baseUri = _baseUri(activeProfile.endpoint);
    final info = await _fetchServerInfo(baseUri);
    _ensureCurrentProfile(profileRevision, activeProfile);
    final identity = GiznetSignalingIdentity(
      clientPrivateKey: base58Decode(activeProfile.clientPrivateKey),
      clientPublicKey: activeProfile.clientPublicKey == null
          ? null
          : base58Decode(activeProfile.clientPublicKey!),
      serverPublicKey: base58Decode(info.publicKey),
    );
    if (Platform.isIOS) {
      await rtc.Helper.setAppleAudioIOMode(
        rtc.AppleAudioIOMode.localAndRemote,
        preferSpeakerOutput: true,
      );
    }
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
      if (microphoneStream != null) await _stopMediaStream(microphoneStream);
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
      _ensureCurrentProfile(profileRevision, activeProfile);
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
      _ensureCurrentProfile(profileRevision, activeProfile);
      _pendingPeerConnection = peerConnection;
      await _waitForPeerConnection(peerConnection);
      await _prepareAudioPlayback(peerConnection);
      _ensureCurrentProfile(profileRevision, activeProfile);
      final dataChannelFactory = FlutterWebRtcDataChannelFactory(
        peerConnection,
      );
      final client = GizClawClient(dataChannelFactory);
      await _publishClientInfo(client, _deviceInfo);
      _ensureCurrentProfile(profileRevision, activeProfile);
      _pendingPeerConnection = null;
      _pendingMicrophoneStream = null;
      _peerConnection = peerConnection;
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
    } catch (_) {
      if (peerConnection != null &&
          identical(_pendingPeerConnection, peerConnection)) {
        _pendingPeerConnection = null;
        await peerConnection.close();
      } else if (peerConnection != null) {
        await peerConnection.close();
      }
      if (identical(_pendingMicrophoneStream, microphoneStream)) {
        _pendingMicrophoneStream = null;
        if (microphoneStream != null) await _stopMediaStream(microphoneStream);
      }
      _setMicrophoneStatus(const MicrophoneStatus.unavailable());
      rethrow;
    }
  }

  Future<GizClawClient> reconnect() async {
    await close();
    return connect();
  }

  Future<void> updateProfile(GizClawConnectionProfile profile) async {
    if (profile.endpoint == _profile.endpoint &&
        profile.clientPrivateKey == _profile.clientPrivateKey) {
      return;
    }
    _profileRevision += 1;
    _profile = profile;
    await close();
  }

  Future<void> close() async {
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
    microphoneTrack?.enabled = false;
    microphoneTrack?.onEnded = null;
    _setMicrophoneStatus(const MicrophoneStatus.unavailable());
    await pendingPeerConnection?.close();
    await peerConnection?.close();
    if (pendingMicrophoneStream != null) {
      await _stopMediaStream(pendingMicrophoneStream);
    }
    if (microphoneStream != null &&
        !identical(microphoneStream, pendingMicrophoneStream)) {
      await _stopMediaStream(microphoneStream);
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
    if (revision != _profileRevision || !identical(activeProfile, _profile)) {
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

Future<rtc.MediaStream> _defaultAcquireMicrophoneStream() =>
    rtc.navigator.mediaDevices.getUserMedia({
      'audio': {
        'channelCount': 1,
        'echoCancellation': true,
        'noiseSuppression': true,
      },
      'video': false,
    });

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
  await client.putServerInfo(deviceInfo);
}

MicrophoneFailureKind _microphoneFailureKind(Object error) =>
    error.toString().contains('NotAllowedError')
    ? MicrophoneFailureKind.permissionDenied
    : MicrophoneFailureKind.captureUnavailable;

Future<void> _stopMediaStream(rtc.MediaStream stream) async {
  await Future.wait([for (final track in stream.getTracks()) track.stop()]);
}

Future<void> _prepareAudioPlayback(rtc.RTCPeerConnection peerConnection) async {
  for (final receiver in await peerConnection.getReceivers()) {
    final track = receiver.track;
    if (track?.kind == 'audio') track!.enabled = true;
  }
  if (Platform.isIOS) await rtc.Helper.ensureAudioSession();
  if (Platform.isIOS || Platform.isAndroid) {
    await rtc.Helper.setSpeakerphoneOnButPreferBluetooth();
  }
}

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
