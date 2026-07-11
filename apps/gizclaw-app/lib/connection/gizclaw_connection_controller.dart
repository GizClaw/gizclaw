import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';

class GizClawConnectionProfile {
  const GizClawConnectionProfile({
    required this.endpoint,
    required this.clientPrivateKey,
  });

  factory GizClawConnectionProfile.fromEnvironment() {
    return const GizClawConnectionProfile(
      endpoint: String.fromEnvironment('GIZCLAW_ENDPOINT'),
      clientPrivateKey: String.fromEnvironment('GIZCLAW_PRIVATE_KEY'),
    );
  }

  final String endpoint;
  final String clientPrivateKey;

  bool get isConfigured => endpoint.isNotEmpty && clientPrivateKey.isNotEmpty;
}

class GizClawConnectionController {
  GizClawConnectionController(this.profile);

  final GizClawConnectionProfile profile;

  rtc.RTCPeerConnection? _peerConnection;
  GizClawClient? _client;
  FlutterWebRtcDataChannelFactory? _dataChannelFactory;
  String? _serverId;

  GizClawClient? get client => _client;
  FlutterWebRtcDataChannelFactory? get dataChannelFactory =>
      _dataChannelFactory;
  rtc.RTCPeerConnection? get peerConnection => _peerConnection;
  String? get serverId => _serverId;

  Future<GizClawClient> connect() async {
    if (_client != null) return _client!;
    if (!profile.isConfigured) {
      throw StateError('No GizClaw development connection is configured');
    }

    final baseUri = _baseUri(profile.endpoint);
    final info = await _fetchServerInfo(baseUri);
    _serverId = info.publicKey;
    final identity = GiznetSignalingIdentity(
      clientPrivateKey: base58Decode(profile.clientPrivateKey),
      serverPublicKey: base58Decode(info.publicKey),
    );
    final peerConnection = await connectFlutterGiznetWebRtc(
      addAudioTransceiver: true,
      prepareOffer: (sdp) => prepareEncryptedGiznetWebRtcOffer(identity, sdp),
      sendOffer: (offer) =>
          _sendOffer(baseUri.resolve(info.signalingPath), offer),
    );
    await _waitForPeerConnection(peerConnection);
    _peerConnection = peerConnection;
    _dataChannelFactory = FlutterWebRtcDataChannelFactory(peerConnection);
    return _client = GizClawClient(_dataChannelFactory!);
  }

  Future<void> close() async {
    _client = null;
    _dataChannelFactory = null;
    _serverId = null;
    final peerConnection = _peerConnection;
    _peerConnection = null;
    await peerConnection?.close();
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
  final value = endpoint.contains('://') ? endpoint : 'http://$endpoint';
  final uri = Uri.parse(value);
  if (!uri.hasAuthority) {
    throw FormatException('Invalid GizClaw endpoint');
  }
  return uri.path.endsWith('/') ? uri : uri.replace(path: '${uri.path}/');
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
