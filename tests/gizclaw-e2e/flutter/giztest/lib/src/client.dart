/// Scenario clients: an ephemeral WebRTC device peer built on `gizclaw`, and
/// the Public HTTP surface reached through `gizclaw_control`.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:flutter_webrtc/flutter_webrtc.dart' as rtc;
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_control/gizclaw_control.dart' as control;
import 'package:http/http.dart' as http;
import 'package:protobuf/protobuf.dart';

import 'document.dart';
import 'variables.dart';

const _connectTimeout = Duration(seconds: 30);
const _rpcTimeout = Duration(seconds: 30);

/// One Peer RPC failure, with the numeric code a scenario's expect_error
/// compares against.
class ScenarioRpcError implements Exception {
  const ScenarioRpcError(this.code, this.message);

  final int code;
  final String message;

  @override
  String toString() => 'rpc failed (code $code): $message';
}

/// One Public HTTP response as a scenario step sees it.
class HttpStepResult {
  const HttpStepResult(this.status, this.body);

  final int status;
  final Object? body;
}

/// Turns a client access point into an HTTP base URL. A bare host:port
/// resolves to http, so scenarios can name either form.
Uri httpBaseUrl(String accessPoint) => normalizeGiznetAccessPoint(accessPoint);

/// Converts snake_case JSON keys into the lowerCamelCase proto3 JSON names the
/// Dart protobuf runtime expects.
Object? snakeToCamelKeys(Object? value) {
  if (value is Map) {
    return <String, Object?>{
      for (final entry in value.entries)
        _snakeToCamel(entry.key.toString()): snakeToCamelKeys(entry.value),
    };
  }
  if (value is List) {
    return value.map(snakeToCamelKeys).toList();
  }
  return value;
}

/// Converts lowerCamelCase proto3 JSON keys back into the snake_case proto
/// field names the Go runner emits, so scenario pointers match across runners.
Object? camelToSnakeKeys(Object? value) {
  if (value is Map) {
    return <String, Object?>{
      for (final entry in value.entries)
        _camelToSnake(entry.key.toString()): camelToSnakeKeys(entry.value),
    };
  }
  if (value is List) {
    return value.map(camelToSnakeKeys).toList();
  }
  return value;
}

String _snakeToCamel(String name) {
  final parts = name.split('_');
  if (parts.length == 1) {
    return name;
  }
  return parts.first +
      parts
          .skip(1)
          .map(
            (part) =>
                part.isEmpty ? part : part[0].toUpperCase() + part.substring(1),
          )
          .join();
}

String _camelToSnake(String name) => name.replaceAllMapped(
  RegExp('[A-Z]'),
  (match) => '_${match.group(0)!.toLowerCase()}',
);

/// Projects an RPC response to proto3 JSON, unwrapping a response whose only
/// field is a `value` message so scenario pointers start below it. The Go
/// runner and the JavaScript codec apply the same rule.
Object? unwrapValueMessage(GeneratedMessage response) {
  final fields = response.info_.byIndex;
  final json = response.toProto3Json();
  if (fields.length != 1 || fields.single.name != 'value') {
    return json;
  }
  if (json is Map && json.length == 1 && json['value'] is Map) {
    return json['value'];
  }
  return fields.single.isGroupOrMessage ? const <String, Object?>{} : json;
}

/// Reads the `{error_code, error_message}` form a scenario uses to make a
/// device provider fail.
GizClawDeviceControlException? _scriptedFailure(Object? response) {
  if (response is! Map || !response.containsKey('error_code')) {
    return null;
  }
  final code = response['error_code'];
  if (code is! int) {
    throw StateError('error_code must be an integer');
  }
  final message = response['error_message'];
  return GizClawDeviceControlException(
    StatusCode.valueOf(code) ?? StatusCode.STATUS_CODE_INTERNAL,
    message is String ? message : '',
  );
}

Map<String, Object?> _asObject(Object? value) =>
    value is Map ? value.cast<String, Object?>() : const {};

/// Bounds a scripted device delay. Node clamps a larger `setTimeout` delay to
/// a single millisecond, which would turn a scenario written to exercise a
/// timeout into one that passes on an immediate answer. Every runner rejects
/// the same values so a document behaves identically everywhere.
const maxScriptedDelayMs = 2147483647;

class ScenarioClient {
  ScenarioClient._(
    this.name,
    this.endpoint,
    this.fingerprint,
    this._peerConnection,
    this._client,
    this.inbound,
    this._privateKey,
    this._handlers,
  );

  final String name;
  final String endpoint;
  final String fingerprint;

  // The peer connection and RPC client are replaced by [reconnect], which
  // dials a new Peer on the same identity. The private key and the scripted
  // providers are kept so that redial installs the same device behavior.
  rtc.RTCPeerConnection _peerConnection;
  GizClawClient _client;
  final List<int> _privateKey;
  final GizClawPeerRpcHandlers _handlers;

  /// Cumulative inbound call counts keyed by client RPC method name.
  final Map<String, int> inbound;

  final _controlClients = <String, control.GizClawControlClient>{};

  /// Brings up one ephemeral device peer with every `client.*` provider the
  /// document's steps script for it installed before signaling starts.
  static Future<ScenarioClient> connect(
    String name,
    ClientSpec spec,
    List<Step> steps,
    Variables variables,
  ) async {
    final endpoint = variables.resolveString(spec.accessPoint, 'access_point');
    final base = httpBaseUrl(endpoint);
    final random = Random.secure();
    final privateKey = List<int>.generate(32, (_) => random.nextInt(256));

    final httpClient = http.Client();
    try {
      final handlers = _buildHandlers(name, steps, variables);
      var publicKey = '';
      final peerConnection = await _dial(
        httpClient,
        base,
        handlers.handlers,
        privateKey,
        (value) => publicKey = value,
      );
      final client = GizClawClient(
        FlutterWebRtcDataChannelFactory(peerConnection),
        requestTimeout: _rpcTimeout,
      );
      final scenario = ScenarioClient._(
        name,
        endpoint,
        publicKey.length > 12 ? publicKey.substring(0, 12) : publicKey,
        peerConnection,
        client,
        handlers.inbound,
        privateKey,
        handlers.handlers,
      );
      final token = spec.registrationToken;
      if (token != null && token.isNotEmpty) {
        await client.register(
          variables.resolveString(token, 'registration_token'),
        );
      }
      return scenario;
    } finally {
      httpClient.close();
    }
  }

  /// Drops this client's Peer connection and dials a replacement on the same
  /// identity, the way a device that switched network or rebooted reaches the
  /// Server again.
  ///
  /// The scripted providers are reinstalled and keep their inbound counts, so
  /// `expect_calls` still sees the total across both connections.
  Future<void> reconnect({Duration? await_}) async {
    await _peerConnection.close();
    final httpClient = http.Client();
    try {
      final peerConnection = await _dial(
        httpClient,
        httpBaseUrl(endpoint),
        _handlers,
        _privateKey,
        (_) {},
        timeout: await_,
      );
      _peerConnection = peerConnection;
      _client = GizClawClient(
        FlutterWebRtcDataChannelFactory(peerConnection),
        requestTimeout: _rpcTimeout,
      );
    } finally {
      httpClient.close();
    }
  }

  /// Runs GizNet signaling and returns the negotiated peer connection.
  static Future<rtc.RTCPeerConnection> _dial(
    http.Client httpClient,
    Uri base,
    GizClawPeerRpcHandlers handlers,
    List<int> privateKey,
    void Function(String) onPublicKey, {
    Duration? timeout,
  }) async {
    final connectTimeout = timeout ?? _connectTimeout;
    final infoResponse = await httpClient
        .get(base.replace(path: '${base.path}/server-info'))
        .timeout(connectTimeout);
    if (infoResponse.statusCode != 200) {
      throw StateError('server-info returned HTTP ${infoResponse.statusCode}');
    }
    final info = GiznetServerInfo.fromJson(
      jsonDecode(infoResponse.body) as Map<String, Object?>,
    );
    final identity = GiznetSignalingIdentity(
      clientPrivateKey: privateKey,
      serverPublicKey: base58Decode(info.transportPublicKey),
    );
    // An Edge gateway can advertise its own origin, which is where the
    // encrypted offer must go.
    final signalingBase = info.transport?.endpoint.isNotEmpty ?? false
        ? resolveGiznetTransportBaseUrl(base, info.transport!.endpoint)
        : base;
    return connectFlutterGiznetWebRtc(
      peerRpcHandlers: handlers,
      prepareOffer: (offerSdp) async {
        final offer = await prepareEncryptedGiznetWebRtcOffer(
          identity,
          offerSdp,
        );
        onPublicKey(offer.clientPublicKey);
        return offer;
      },
      sendOffer: (offer) async {
        final response = await httpClient
            .post(
              signalingBase.replace(
                path: '${signalingBase.path}${info.transportSignalingPath}',
              ),
              body: offer.body,
              headers: {
                'Content-Type': 'application/octet-stream',
                'X-Giznet-Nonce': offer.nonce,
                'X-Giznet-Public-Key': offer.clientPublicKey,
                'X-Giznet-Timestamp': '${offer.timestamp}',
              },
            )
            .timeout(connectTimeout);
        if (response.statusCode != 200) {
          throw StateError('signaling returned HTTP ${response.statusCode}');
        }
        return response.bodyBytes;
      },
    );
  }

  /// Sends one unary Peer RPC by name and returns the decoded response as
  /// snake_case JSON.
  Future<Object?> callRpc(String method, Object? params) async {
    final descriptor = rpcMethodsByName[method];
    if (descriptor == null) {
      throw StateError('unsupported RPC method $method');
    }
    final request = newPayloadMessage(descriptor.requestType);
    final json = snakeToCamelKeys(params ?? const <String, Object?>{});
    request.mergeFromProto3Json(json, ignoreUnknownFields: false);
    try {
      final response = await _client.rpc.call<GeneratedMessage>(
        method,
        request,
      );
      return camelToSnakeKeys(unwrapValueMessage(response));
    } on RpcStatus catch (error) {
      throw ScenarioRpcError(error.code, error.message);
    }
  }

  /// Sends one Public HTTP request through the control SDK so the request
  /// building, bearer injection and error classification under test are the
  /// ones a controller app would use.
  Future<HttpStepResult> callHttp(
    String method,
    String pathWithQuery,
    Map<String, String> headers,
    Object? body,
  ) async {
    String? token;
    final extra = <String, String>{};
    for (final entry in headers.entries) {
      if (entry.key.toLowerCase() == 'authorization') {
        final match = RegExp(r'^Bearer\s+(.+)$').firstMatch(entry.value);
        token = match?.group(1);
        if (token == null) {
          extra[entry.key] = entry.value;
        }
        continue;
      }
      extra[entry.key] = entry.value;
    }
    final response = await _controlFor(
      token,
    ).send(method: method, path: pathWithQuery, headers: extra, body: body);
    return HttpStepResult(response.statusCode, response.json);
  }

  control.GizClawControlClient _controlFor(String? token) {
    final key = token ?? '';
    return _controlClients.putIfAbsent(
      key,
      () => control.GizClawControlClient(
        // The e2e stack serves plaintext HTTP on localhost.
        allowInsecureTransport: true,
        apiKey: key.isEmpty ? 'unauthenticated' : key,
        baseUrl: httpBaseUrl(endpoint),
      ),
    );
  }

  Future<void> close() async {
    for (final control in _controlClients.values) {
      control.close();
    }
    await _peerConnection.close();
  }
}

class _Handlers {
  const _Handlers(this.handlers, this.inbound);

  final GizClawPeerRpcHandlers handlers;
  final Map<String, int> inbound;
}

/// Turns the document's client_rpc steps for one client into the device-side
/// providers the SDK installs, and counts every inbound call.
_Handlers _buildHandlers(
  String clientName,
  List<Step> steps,
  Variables variables,
) {
  final inbound = <String, int>{};
  void count(String method) => inbound[method] = (inbound[method] ?? 0) + 1;

  Map<String, Object?> deviceInfo = const {};
  Map<String, Object?>? identifiers;
  var control = const GizClawDeviceControlHandlers();

  for (final step in steps) {
    final clientRpc = step.clientRpc;
    if (step.client != clientName || clientRpc == null) {
      continue;
    }
    final method = clientRpc['method'] as String;
    final scripted = variables.resolve(clientRpc['response']);
    final failure = _scriptedFailure(scripted);
    final object = _asObject(scripted);
    inbound[method] = 0;

    switch (method) {
      case 'client.info.get':
        deviceInfo = object;
      case 'client.identifiers.get':
        identifiers = object;
      case 'client.device.audioplayer.get':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            playlistGet: control.audioplayer?.playlistGet,
            playlistSet: control.audioplayer?.playlistSet,
            playlistAppend: control.audioplayer?.playlistAppend,
            play: control.audioplayer?.play,
            stop: control.audioplayer?.stop,
            modeSet: control.audioplayer?.modeSet,
            get: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerGetResponse()
                ..mergeFromProto3Json(snakeToCamelKeys({'value': object}));
            },
          ),
        );
      case 'client.device.audioplayer.playlist.get':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            get: control.audioplayer?.get,
            playlistSet: control.audioplayer?.playlistSet,
            playlistAppend: control.audioplayer?.playlistAppend,
            play: control.audioplayer?.play,
            stop: control.audioplayer?.stop,
            modeSet: control.audioplayer?.modeSet,
            playlistGet: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerPlaylistGetResponse()
                ..mergeFromProto3Json(snakeToCamelKeys(object));
            },
          ),
        );
      case 'client.device.audioplayer.playlist.set':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            get: control.audioplayer?.get,
            playlistGet: control.audioplayer?.playlistGet,
            playlistAppend: control.audioplayer?.playlistAppend,
            play: control.audioplayer?.play,
            stop: control.audioplayer?.stop,
            modeSet: control.audioplayer?.modeSet,
            playlistSet: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerPlaylistSetResponse()
                ..mergeFromProto3Json(snakeToCamelKeys({'value': object}));
            },
          ),
        );
      case 'client.device.audioplayer.playlist.append':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            get: control.audioplayer?.get,
            playlistGet: control.audioplayer?.playlistGet,
            playlistSet: control.audioplayer?.playlistSet,
            play: control.audioplayer?.play,
            stop: control.audioplayer?.stop,
            modeSet: control.audioplayer?.modeSet,
            playlistAppend: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerPlaylistAppendResponse()
                ..mergeFromProto3Json(snakeToCamelKeys({'value': object}));
            },
          ),
        );
      case 'client.device.audioplayer.play':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            get: control.audioplayer?.get,
            playlistGet: control.audioplayer?.playlistGet,
            playlistSet: control.audioplayer?.playlistSet,
            playlistAppend: control.audioplayer?.playlistAppend,
            stop: control.audioplayer?.stop,
            modeSet: control.audioplayer?.modeSet,
            play: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerPlayResponse()
                ..mergeFromProto3Json(snakeToCamelKeys({'value': object}));
            },
          ),
        );
      case 'client.device.audioplayer.stop':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            get: control.audioplayer?.get,
            playlistGet: control.audioplayer?.playlistGet,
            playlistSet: control.audioplayer?.playlistSet,
            playlistAppend: control.audioplayer?.playlistAppend,
            play: control.audioplayer?.play,
            modeSet: control.audioplayer?.modeSet,
            stop: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerStopResponse()
                ..mergeFromProto3Json(snakeToCamelKeys({'value': object}));
            },
          ),
        );
      case 'client.device.audioplayer.mode.set':
        control = _copyControl(
          control,
          audioplayer: GizClawAudioPlayerHandlers(
            get: control.audioplayer?.get,
            playlistGet: control.audioplayer?.playlistGet,
            playlistSet: control.audioplayer?.playlistSet,
            playlistAppend: control.audioplayer?.playlistAppend,
            play: control.audioplayer?.play,
            stop: control.audioplayer?.stop,
            modeSet: (_) {
              count(method);
              if (failure != null) throw failure;
              return ClientDeviceAudioPlayerModeSetResponse()
                ..mergeFromProto3Json(snakeToCamelKeys({'value': object}));
            },
          ),
        );
      case 'client.device.status.get':
        control = _copyControl(
          control,
          status: () {
            count(method);
            if (failure != null) throw failure;
            return _peerStatus(object);
          },
        );
      case 'client.device.volume.set':
        // The scripted status is echoed with the requested level and mute
        // state so an HTTP round trip can assert them.
        control = _copyControl(
          control,
          setVolume: (level, muted) {
            count(method);
            if (failure != null) throw failure;
            return _peerStatus({...object, 'volume': level, 'muted': muted});
          },
        );
      case 'client.device.sound.play':
        control = _copyControl(
          control,
          playSound: (_, _) {
            count(method);
            if (failure != null) throw failure;
          },
        );
      case 'client.device.reboot':
        control = _copyControl(
          control,
          reboot: (_) {
            count(method);
            if (failure != null) throw failure;
          },
        );
      case 'client.wifi.status.get':
        control = _copyControl(
          control,
          wifiStatus: () {
            count(method);
            if (failure != null) throw failure;
            return WifiStatus()..mergeFromProto3Json(
              snakeToCamelKeys(object),
              ignoreUnknownFields: true,
            );
          },
        );
      case 'client.wifi.saved.list':
        control = _copyControl(
          control,
          savedWifi: () {
            count(method);
            if (failure != null) throw failure;
            final networks = object['networks'];
            if (networks is! List) {
              return const <WifiSavedNetwork>[];
            }
            return networks
                .map(
                  (item) => WifiSavedNetwork()
                    ..mergeFromProto3Json(
                      snakeToCamelKeys(item),
                      ignoreUnknownFields: true,
                    ),
                )
                .toList();
          },
        );
      case 'client.wifi.saved.forget':
        control = _copyControl(
          control,
          forgetWifi: (_) {
            count(method);
            if (failure != null) throw failure;
          },
        );
      case 'client.wifi.scan':
        control = _copyControl(
          control,
          scanWifi: (_) {
            count(method);
            if (failure != null) throw failure;
            final delayMs = object['delay_ms'];
            if (delayMs != null) {
              if (delayMs is! int || delayMs < 0) {
                throw StateError('delay_ms must be a non-negative integer');
              }
              if (delayMs > maxScriptedDelayMs) {
                throw StateError(
                  'delay_ms must be at most $maxScriptedDelayMs',
                );
              }
              sleep(Duration(milliseconds: delayMs));
            }
            final networks = object['networks'];
            if (networks is! List) return const <WifiScanResult>[];
            return networks
                .map(
                  (item) => WifiScanResult()
                    ..mergeFromProto3Json(
                      snakeToCamelKeys(item),
                      ignoreUnknownFields: true,
                    ),
                )
                .toList();
          },
        );
      case 'client.wifi.connect':
        control = _copyControl(
          control,
          connectWifi: (_, _) {
            count(method);
            if (failure != null) throw failure;
          },
        );
      default:
        throw StateError('unsupported client RPC $method');
    }
  }

  final info = DeviceInfo()
    ..mergeFromProto3Json(
      snakeToCamelKeys(deviceInfo),
      ignoreUnknownFields: true,
    );
  final scriptedIdentifiers = identifiers;
  if (scriptedIdentifiers != null) {
    info.identifiers = DeviceIdentifiers()
      ..mergeFromProto3Json(
        snakeToCamelKeys(scriptedIdentifiers),
        ignoreUnknownFields: true,
      );
  }
  return _Handlers(
    GizClawPeerRpcHandlers(
      deviceInfo: () {
        count('client.info.get');
        return info;
      },
      // Installed only when a step scripts it, so client.identifiers.get keeps
      // its own call count instead of being attributed to client.info.get.
      deviceIdentifiers: scriptedIdentifiers == null
          ? null
          : () {
              count('client.identifiers.get');
              return DeviceIdentifiers()..mergeFromProto3Json(
                snakeToCamelKeys(scriptedIdentifiers),
                ignoreUnknownFields: true,
              );
            },
      deviceControl: control,
    ),
    inbound,
  );
}

PeerStatus _peerStatus(Map<String, Object?> json) =>
    PeerStatus()
      ..mergeFromProto3Json(snakeToCamelKeys(json), ignoreUnknownFields: true);

GizClawDeviceControlHandlers _copyControl(
  GizClawDeviceControlHandlers base, {
  GizClawAudioPlayerHandlers? audioplayer,
  PeerStatus Function()? status,
  PeerStatus Function(int level, bool muted)? setVolume,
  void Function(String sound, int? durationMs)? playSound,
  void Function(int? delayMs)? reboot,
  WifiStatus Function()? wifiStatus,
  List<WifiSavedNetwork> Function()? savedWifi,
  void Function(String ssid)? forgetWifi,
  List<WifiScanResult> Function(int? timeoutMs)? scanWifi,
  void Function(String ssid, String? passphrase)? connectWifi,
}) {
  return GizClawDeviceControlHandlers(
    audioplayer: audioplayer ?? base.audioplayer,
    connectWifi: connectWifi ?? base.connectWifi,
    forgetWifi: forgetWifi ?? base.forgetWifi,
    playSound: playSound ?? base.playSound,
    reboot: reboot ?? base.reboot,
    savedWifi: savedWifi ?? base.savedWifi,
    scanWifi: scanWifi ?? base.scanWifi,
    setVolume: setVolume ?? base.setVolume,
    status: status ?? base.status,
    wifiStatus: wifiStatus ?? base.wifiStatus,
  );
}
