import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';

const gizClawDevelopmentServerEndpoint = 'ap.dev.gizclaw.com:9820';
const gizClawProductionServerEndpoint = 'ap.gizclaw.com:9820';

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

class GizClawConnectionController {
  GizClawConnectionController(GizClawConnectionProfile profile)
    : _profile = profile;

  GizClawConnectionProfile _profile;

  rtc.RTCPeerConnection? _peerConnection;
  rtc.RTCPeerConnection? _pendingPeerConnection;
  GizClawClient? _client;
  FlutterWebRtcDataChannelFactory? _dataChannelFactory;
  String? _clientPublicKey;
  String? _serverId;
  int _profileRevision = 0;

  GizClawClient? get client => _client;
  FlutterWebRtcDataChannelFactory? get dataChannelFactory =>
      _dataChannelFactory;
  rtc.RTCPeerConnection? get peerConnection => _peerConnection;
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
      throw StateError('No GizClaw development connection is configured');
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
    String? preparedClientPublicKey;
    final peerConnection = await connectFlutterGiznetWebRtc(
      addAudioTransceiver: true,
      prepareOffer: (sdp) async {
        final offer = await prepareEncryptedGiznetWebRtcOffer(identity, sdp);
        preparedClientPublicKey = offer.clientPublicKey;
        return offer;
      },
      sendOffer: (offer) =>
          _sendOffer(baseUri.resolve(info.signalingPath), offer),
    );
    var registeredAsPending = false;
    try {
      _ensureCurrentProfile(profileRevision, activeProfile);
      _pendingPeerConnection = peerConnection;
      registeredAsPending = true;
      await _waitForPeerConnection(peerConnection);
      await _prepareAudioPlayback(peerConnection);
      _ensureCurrentProfile(profileRevision, activeProfile);
      _pendingPeerConnection = null;
      _peerConnection = peerConnection;
      _serverId = info.publicKey;
      _clientPublicKey = preparedClientPublicKey;
      _dataChannelFactory = FlutterWebRtcDataChannelFactory(peerConnection);
      return _client = GizClawClient(_dataChannelFactory!);
    } catch (_) {
      if (identical(_pendingPeerConnection, peerConnection)) {
        _pendingPeerConnection = null;
        await peerConnection.close();
      } else if (!registeredAsPending) {
        await peerConnection.close();
      }
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
    await pendingPeerConnection?.close();
    await peerConnection?.close();
  }

  void _ensureCurrentProfile(
    int revision,
    GizClawConnectionProfile activeProfile,
  ) {
    if (revision != _profileRevision || !identical(activeProfile, _profile)) {
      throw StateError('GizClaw connection profile changed during setup');
    }
  }
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

Future<GiznetServerInfo> _fetchServerInfo(Uri baseUri) async {
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
