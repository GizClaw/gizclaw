import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';

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
  GizClawClient? _client;
  FlutterWebRtcDataChannelFactory? _dataChannelFactory;
  String? _clientPublicKey;
  String? _serverId;

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
    if (!profile.isConfigured) {
      throw StateError('No GizClaw development connection is configured');
    }

    if (_client != null || _peerConnection != null) {
      await close();
    }

    final baseUri = _baseUri(profile.endpoint);
    final info = await _fetchServerInfo(baseUri);
    _serverId = info.publicKey;
    final identity = GiznetSignalingIdentity(
      clientPrivateKey: base58Decode(profile.clientPrivateKey),
      clientPublicKey: profile.clientPublicKey == null
          ? null
          : base58Decode(profile.clientPublicKey!),
      serverPublicKey: base58Decode(info.publicKey),
    );
    if (Platform.isIOS) {
      await rtc.Helper.setAppleAudioIOMode(
        rtc.AppleAudioIOMode.localAndRemote,
        preferSpeakerOutput: true,
      );
    }
    final peerConnection = await connectFlutterGiznetWebRtc(
      addAudioTransceiver: true,
      prepareOffer: (sdp) async {
        final offer = await prepareEncryptedGiznetWebRtcOffer(identity, sdp);
        _clientPublicKey = offer.clientPublicKey;
        return offer;
      },
      sendOffer: (offer) =>
          _sendOffer(baseUri.resolve(info.signalingPath), offer),
    );
    await _waitForPeerConnection(peerConnection);
    await _prepareAudioPlayback(peerConnection);
    _peerConnection = peerConnection;
    _dataChannelFactory = FlutterWebRtcDataChannelFactory(peerConnection);
    return _client = GizClawClient(_dataChannelFactory!);
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
    await close();
    _profile = profile;
  }

  Future<void> close() async {
    _client = null;
    _dataChannelFactory = null;
    _clientPublicKey = null;
    _serverId = null;
    final peerConnection = _peerConnection;
    _peerConnection = null;
    await peerConnection?.close();
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
  if (!hasScheme) return '$host:$explicitPort';
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
  try {
    final request = await client.getUrl(baseUri.resolve('/server-info'));
    final response = await request.close();
    final body = await utf8.decoder.bind(response).join();
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw HttpException('server-info failed with ${response.statusCode}');
    }
    return GiznetServerInfo.fromJson(jsonDecode(body) as Map<String, Object?>);
  } finally {
    client.close(force: true);
  }
}

Future<List<int>> _sendOffer(Uri uri, PreparedGiznetWebRtcOffer offer) async {
  final client = HttpClient();
  try {
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
  } finally {
    client.close(force: true);
  }
}
