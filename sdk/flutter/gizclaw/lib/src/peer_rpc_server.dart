import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:protobuf/protobuf.dart' show GeneratedMessage;

import 'generated/rpc/rpc.pb.dart' as rpc;
import 'generated/rpc/payload.pb.dart' as payload;
import 'method_registry.dart';
import 'payload_codec.dart';
import 'rpc_frame.dart';
import 'transport.dart';

const _rpcSpeedTestFrameSize = 32 * 1024;
const _rpcSpeedTestMaxContentLength = 1 << 30;

typedef GizClawDeviceInfoProvider = FutureOr<payload.DeviceInfo> Function();
typedef GizClawDeviceIdentifiersProvider =
    FutureOr<payload.DeviceIdentifiers> Function();
typedef GizClawToolHandler =
    FutureOr<Object?> Function(Map<String, Object?> arguments);

/// Receives `client.social.ping`: a Friend pinged this device, or a Friend
/// Group member rallied the group when `friendGroupName` is set. It should
/// alert the user and return promptly; the Server counts an error or a late
/// acknowledgement as not delivered.
typedef GizClawSocialPingHandler =
    FutureOr<void> Function(payload.ClientSocialPingRequest request);

/// Thrown by a device control handler to answer the Server with a specific
/// RPC error code, for example `INVALID_PARAMS` for an unknown sound or
/// `NOT_FOUND` for an unknown saved network.
class GizClawDeviceControlException implements Exception {
  const GizClawDeviceControlException(this.code, [this.message = '']);

  final rpc.StatusCode code;
  final String message;

  @override
  String toString() => 'GizClawDeviceControlException($code, $message)';
}

/// Device single-player providers. Playlist mutations must be atomic.
class GizClawAudioPlayerHandlers {
  const GizClawAudioPlayerHandlers({
    this.get,
    this.playlistGet,
    this.playlistSet,
    this.playlistAppend,
    this.play,
    this.stop,
    this.modeSet,
  });
  final FutureOr<payload.ClientDeviceAudioPlayerGetResponse> Function(
    payload.ClientDeviceAudioPlayerGetRequest request,
  )?
  get;
  final FutureOr<payload.ClientDeviceAudioPlayerPlaylistGetResponse> Function(
    payload.ClientDeviceAudioPlayerPlaylistGetRequest request,
  )?
  playlistGet;
  final FutureOr<payload.ClientDeviceAudioPlayerPlaylistSetResponse> Function(
    payload.ClientDeviceAudioPlayerPlaylistSetRequest request,
  )?
  playlistSet;
  final FutureOr<payload.ClientDeviceAudioPlayerPlaylistAppendResponse>
  Function(payload.ClientDeviceAudioPlayerPlaylistAppendRequest request)?
  playlistAppend;
  final FutureOr<payload.ClientDeviceAudioPlayerPlayResponse> Function(
    payload.ClientDeviceAudioPlayerPlayRequest request,
  )?
  play;
  final FutureOr<payload.ClientDeviceAudioPlayerStopResponse> Function(
    payload.ClientDeviceAudioPlayerStopRequest request,
  )?
  stop;
  final FutureOr<payload.ClientDeviceAudioPlayerModeSetResponse> Function(
    payload.ClientDeviceAudioPlayerModeSetRequest request,
  )?
  modeSet;
}

/// Implements the Server-initiated `client.device.*` and `client.wifi.*`
/// methods. A null handler answers `METHOD_NOT_FOUND`, which the Server maps
/// to `501 DEVICE_UNSUPPORTED`.
class GizClawDeviceControlHandlers {
  const GizClawDeviceControlHandlers({
    this.audioplayer,
    this.status,
    this.setVolume,
    this.playSound,
    this.find,
    this.reboot,
    this.wifiStatus,
    this.savedWifi,
    this.forgetWifi,
    this.scanWifi,
    this.connectWifi,
    this.updateFirmware,
    this.getSettings,
    this.setSettings,
    this.factoryReset,
  });

  final GizClawAudioPlayerHandlers? audioplayer;
  final FutureOr<payload.PeerStatus> Function()? status;
  final FutureOr<payload.PeerStatus> Function(int level, bool muted)? setVolume;
  final FutureOr<void> Function(String sound, int? durationMs)? playSound;

  /// Rings the built-in find-me sound for `client.device.find`. `durationMs`
  /// is null when the caller leaves the ring time to the device.
  final FutureOr<void> Function(int? durationMs)? find;
  final FutureOr<void> Function(int? delayMs)? reboot;
  final FutureOr<payload.WifiStatus> Function()? wifiStatus;
  final FutureOr<List<payload.WifiSavedNetwork>> Function()? savedWifi;
  final FutureOr<void> Function(String ssid)? forgetWifi;
  final FutureOr<List<payload.WifiScanResult>> Function(int? timeoutMs)?
  scanWifi;
  final FutureOr<void> Function(String ssid, String? passphrase)? connectWifi;

  /// Runs one OTA for `client.firmware.update`. `channel` is null when the
  /// caller leaves the choice to the device; `sha256` is the package digest the
  /// caller resolved, and the handler throws
  /// [GizClawDeviceControlException] with `STATUS_CODE_INVALID_ARGUMENT` when it
  /// does not match the package the device resolves.
  final FutureOr<void> Function(
    payload.FirmwareChannelName? channel,
    String? sha256,
  )?
  updateFirmware;

  /// Reports every option this device supports for
  /// `client.device.settings.get`. An option the device has no hardware for
  /// stays unset rather than carrying a placeholder, which is how a caller
  /// tells "unsupported" from "off".
  final FutureOr<payload.DeviceSettings> Function()? getSettings;

  /// Applies only the members present in [patch] for
  /// `client.device.settings.set` and returns the device's full settings
  /// afterwards, so the caller sees what was accepted. An unsupported member is
  /// ignored rather than rejected. An out-of-range member is rejected before
  /// this handler runs.
  final FutureOr<payload.DeviceSettings> Function(payload.DeviceSettings patch)?
  setSettings;

  /// Erases device-local state for `client.device.factory_reset`.
  /// `keepNetwork` retains saved Wi-Fi and cellular configuration.
  ///
  /// Like [reboot], the handler must complete promptly and only then perform
  /// the reset: the acknowledgement is sent from its return, so a handler that
  /// tears down networking or blocks first leaves the caller without the
  /// response the method promises. Schedule the reset and return.
  final FutureOr<void> Function(bool keepNetwork)? factoryReset;
}

class GizClawPeerRpcHandlers {
  GizClawPeerRpcHandlers({
    required this.deviceInfo,
    Map<String, GizClawToolHandler> tools = const {},
    this.deviceControl,
    this.deviceIdentifiers,
    this.socialPing,
  }) : tools = Map.unmodifiable(tools);

  final GizClawDeviceInfoProvider deviceInfo;
  final Map<String, GizClawToolHandler> tools;
  final GizClawDeviceControlHandlers? deviceControl;

  /// Answers `client.identifiers.get`. When null the identifiers reported by
  /// [deviceInfo] are used, so a device that already reports them there needs
  /// no separate provider.
  final GizClawDeviceIdentifiersProvider? deviceIdentifiers;

  /// Answers `client.social.ping`. When null the device answers
  /// `METHOD_NOT_FOUND`, which the Server counts as not delivered. Throw
  /// [GizClawDeviceControlException] to answer a specific RPC error code.
  final GizClawSocialPingHandler? socialPing;
}

const _deviceControlMethods = {
  'client.device.audioplayer.get',
  'client.device.audioplayer.playlist.get',
  'client.device.audioplayer.playlist.set',
  'client.device.audioplayer.playlist.append',
  'client.device.audioplayer.play',
  'client.device.audioplayer.stop',
  'client.device.audioplayer.mode.set',

  'client.device.status.get',
  'client.device.volume.set',
  'client.device.sound.play',
  'client.device.find',
  'client.device.reboot',
  'client.wifi.status.get',
  'client.wifi.saved.list',
  'client.wifi.saved.forget',
  'client.wifi.scan',
  'client.wifi.connect',
  'client.firmware.update',
  'client.device.settings.get',
  'client.device.settings.set',
  'client.device.factory_reset',
};
const _deviceControlMaxBytes = 32;

// Mirrors the DeviceSettings.locale bound in api/proto/rpc/nanopb.options.
const _deviceSettingsLocaleMaxBytes = 35;

// BCP 47 well-formedness at the subtag level: a 2-8 letter primary subtag and
// hyphen-separated 1-8 character alphanumeric subtags, e.g. "zh-Hant-TW".
final _deviceSettingsLocalePattern = RegExp(
  r'^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$',
);

void serveGizClawPeerRpcChannel(
  GizClawDataChannel channel, {
  GizClawPeerRpcHandlers? handlers,
}) {
  _InboundPeerRpcChannel(channel, handlers).start();
}

class _InboundPeerRpcChannel {
  _InboundPeerRpcChannel(this.channel, this.handlers);

  final GizClawDataChannel channel;
  final GizClawPeerRpcHandlers? handlers;
  final _envelopeChunks = <Uint8List>[];
  var _buffer = Uint8List(0);
  var _closed = false;
  var _envelopeLength = 0;
  var _ignoreBody = false;
  var _uploaded = 0;
  rpc.RpcRequest? _request;
  late StreamSubscription<Uint8List> _messages;
  late StreamSubscription<GizClawDataChannelState> _states;

  void start() {
    _messages = channel.messages.listen(
      _handleMessage,
      onError: (_) => _close(),
      onDone: _close,
    );
    _states = channel.states.listen((state) {
      if (state == GizClawDataChannelState.closed) {
        _close();
      }
    }, onError: (_) => _close());
  }

  void _handleMessage(Uint8List chunk) {
    if (_closed) {
      return;
    }
    try {
      _buffer = concatBytes([_buffer, chunk]);
      for (;;) {
        final result = tryReadFrame(_buffer);
        if (result == null) {
          return;
        }
        _buffer = result.rest;
        _handleFrame(result.frame);
      }
    } catch (_) {
      _close();
    }
  }

  void _handleFrame(RpcFrame frame) {
    final request = _request;
    if (request == null) {
      if (frame.type == rpcFrameTypeText) {
        _envelopeLength += frame.payload.length;
        if (_envelopeLength > rpcMaxEnvelopeSize) {
          throw const FormatException('RPC protobuf envelope too large');
        }
        _envelopeChunks.add(Uint8List.fromList(frame.payload));
        return;
      }
      if (frame.type == rpcFrameTypeBinary) {
        if (_envelopeChunks.isNotEmpty) {
          throw const FormatException('RPC request has duplicate envelope');
        }
        _startRequest(rpc.RpcRequest.fromBuffer(frame.payload));
        return;
      }
      if (frame.type == rpcFrameTypeEos && _envelopeChunks.isNotEmpty) {
        final continuedRequest = rpc.RpcRequest.fromBuffer(
          concatBytes(_envelopeChunks),
        );
        _startRequest(continuedRequest);
        final methodName = _methodName(continuedRequest);
        if (methodName == 'all.ping') {
          _finishPing(continuedRequest);
        } else if (_isClientMethod(methodName)) {
          _finishClientRequest(continuedRequest);
        }
        return;
      }
      throw FormatException('expected RPC request envelope, got ${frame.type}');
    }

    if (_ignoreBody) {
      return;
    }
    final methodName = _methodName(request);
    if (methodName == 'all.ping') {
      if (frame.type != rpcFrameTypeEos) {
        throw FormatException('expected ping EOS frame, got ${frame.type}');
      }
      _finishPing(request);
      return;
    }
    if (methodName == 'all.speed_test.run') {
      if (frame.type == rpcFrameTypeBinary) {
        _uploaded += frame.payload.length;
        return;
      }
      if (frame.type != rpcFrameTypeEos) {
        throw FormatException(
          'expected speed-test body/EOS, got ${frame.type}',
        );
      }
      final params =
          decodeRpcRequestPayload(methodName, request.payload)
              as payload.SpeedTestRequest;
      if (_uploaded != params.upContentLength.toInt()) {
        throw StateError(
          'speed test upload length mismatch: got $_uploaded, '
          'want ${params.upContentLength}',
        );
      }
      _ignoreBody = true;
      return;
    }
    if (_isClientMethod(methodName)) {
      if (frame.type != rpcFrameTypeEos) {
        throw FormatException(
          'expected client RPC EOS frame, got ${frame.type}',
        );
      }
      _finishClientRequest(request);
      return;
    }
    _ignoreBody = true;
  }

  void _startRequest(rpc.RpcRequest request) {
    if (request.id.isEmpty || !request.hasMethod()) {
      throw const FormatException('invalid RPC request envelope');
    }
    _request = request;
    final methodName = _methodName(request);
    switch (methodName) {
      case 'all.ping':
        return;
      case 'all.speed_test.run':
        final params = _validSpeedTestParams(request);
        if (params == null) {
          _ignoreBody = true;
          _unawaited(
            _sendEnvelopeOnly(
              _rpcErrorResponse(
                request.id,
                rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
                'invalid params',
              ),
            ).catchError((_) => _close()),
          );
          return;
        }
        _unawaited(
          _sendSpeedTestResponse(
            request.id,
            params,
          ).catchError((_) => _close()),
        );
        return;
      case 'client.info.get':
      case 'client.identifiers.get':
      case 'client.tool.invoke':
      case 'client.device.audioplayer.get':
      case 'client.device.audioplayer.playlist.get':
      case 'client.device.audioplayer.playlist.set':
      case 'client.device.audioplayer.playlist.append':
      case 'client.device.audioplayer.play':
      case 'client.device.audioplayer.stop':
      case 'client.device.audioplayer.mode.set':
      case 'client.device.status.get':
      case 'client.device.volume.set':
      case 'client.device.sound.play':
      case 'client.device.find':
      case 'client.device.reboot':
      case 'client.wifi.status.get':
      case 'client.wifi.saved.list':
      case 'client.wifi.saved.forget':
      case 'client.wifi.scan':
      case 'client.wifi.connect':
      case 'client.firmware.update':
      case 'client.device.settings.get':
      case 'client.device.settings.set':
      case 'client.device.factory_reset':
      case 'client.rpc.methods.get':
      case 'client.social.ping':
        return;
      default:
        _ignoreBody = true;
        _unawaited(
          _sendEnvelopeOnly(
            _rpcErrorResponse(
              request.id,
              rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
              'unsupported method: $methodName',
            ),
          ).catchError((_) => _close()),
        );
    }
  }

  Future<void> _serveClientRequest(rpc.RpcRequest request) async {
    final methodName = _methodName(request);
    late rpc.RpcResponse response;
    try {
      response = switch (methodName) {
        'client.info.get' => await _getClientInfo(request),
        'client.identifiers.get' => await _getClientIdentifiers(request),
        'client.tool.invoke' => await _invokeClientTool(request),
        'client.social.ping' => await _serveSocialPing(request),
        'client.rpc.methods.get' => _serveRpcMethods(request),
        _ when _deviceControlMethods.contains(methodName) =>
          await _serveDeviceControl(request, methodName),
        _ => throw StateError('unsupported client method: $methodName'),
      };
    } on GizClawDeviceControlException catch (error) {
      response = _rpcErrorResponse(request.id, error.code, error.message);
    } catch (error) {
      response = _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        error.toString(),
      );
    }
    await _sendEnvelopeOnly(response);
  }

  void _finishClientRequest(rpc.RpcRequest request) {
    _ignoreBody = true;
    _unawaited(_serveClientRequest(request).catchError((_) => _close()));
  }

  Future<rpc.RpcResponse> _getClientInfo(rpc.RpcRequest request) async {
    final invalid = _validateClientRequest(request, 'client.info.get');
    if (invalid != null) return invalid;
    final provider = handlers?.deviceInfo;
    if (provider == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        'peer client not configured',
      );
    }
    final device = await provider();
    final info = payload.HardwareInfo();
    if (device.hasHardware()) {
      final hardware = device.hardware;
      if (hardware.hasHardwareRevision()) {
        info.hardwareRevision = hardware.hardwareRevision;
      }
      if (hardware.hasManufacturer()) {
        info.manufacturer = hardware.manufacturer;
      }
      if (hardware.hasModel()) info.model = hardware.model;
    }
    return _rpcPayloadResponse(
      request.id,
      'client.info.get',
      payload.ClientGetInfoResponse(value: info),
    );
  }

  Future<rpc.RpcResponse> _getClientIdentifiers(rpc.RpcRequest request) async {
    final invalid = _validateClientRequest(request, 'client.identifiers.get');
    if (invalid != null) return invalid;
    final identifiersProvider = handlers?.deviceIdentifiers;
    final provider = handlers?.deviceInfo;
    if (identifiersProvider == null && provider == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        'peer client not configured',
      );
    }
    final source = identifiersProvider != null
        ? await identifiersProvider()
        : (await provider!()).identifiers;
    final identifiers = payload.DeviceIdentifiers();
    if (source.hasSn()) identifiers.sn = source.sn;
    identifiers.imeis.addAll(source.imeis);
    identifiers.labels.addAll(source.labels);
    return _rpcPayloadResponse(
      request.id,
      'client.identifiers.get',
      payload.ClientGetIdentifiersResponse(value: identifiers),
    );
  }

  Future<rpc.RpcResponse> _invokeClientTool(rpc.RpcRequest request) async {
    if (!request.hasPayload()) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
    late payload.ToolInvokeRequest params;
    try {
      params =
          decodeRpcRequestPayload('client.tool.invoke', request.payload)
              as payload.ToolInvokeRequest;
    } catch (_) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
    final name = params.invokeName.trim();
    if (!RegExp(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$').hasMatch(name)) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid Tool name',
      );
    }
    final handler = handlers?.tools[name];
    if (handler == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
        'Tool unavailable',
      );
    }
    try {
      final rawArguments = params.hasArgs()
          ? params.args.toProto3Json()
          : <String, Object?>{};
      if (rawArguments is! Map) {
        throw const FormatException('Tool arguments must be an object');
      }
      final arguments = rawArguments.map(
        (key, value) => MapEntry(key.toString(), value),
      );
      final encoded = jsonEncode(await handler(arguments));
      if (utf8.encode(encoded).length > 64 * 1024) {
        throw const FormatException('Tool result is too large');
      }
      return _rpcPayloadResponse(
        request.id,
        'client.tool.invoke',
        payload.ToolInvokeResponse(dataJson: encoded),
      );
    } catch (_) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INTERNAL,
        'Tool handler failed',
      );
    }
  }

  /// Answers `client.rpc.methods.get` from the handlers actually installed, so
  /// the list cannot drift from what this device accepts. `client.info.get`
  /// and `client.identifiers.get` are always answered, the latter falling back
  /// to [GizClawPeerRpcHandlers.deviceInfo].
  rpc.RpcResponse _serveRpcMethods(rpc.RpcRequest request) {
    final control = handlers?.deviceControl;
    final player = control?.audioplayer;
    final installed = <String, Object?>{
      'client.social.ping': handlers?.socialPing,
      'client.device.status.get': control?.status,
      'client.device.volume.set': control?.setVolume,
      'client.device.sound.play': control?.playSound,
      'client.device.find': control?.find,
      'client.device.reboot': control?.reboot,
      'client.device.settings.get': control?.getSettings,
      'client.device.settings.set': control?.setSettings,
      'client.device.factory_reset': control?.factoryReset,
      'client.firmware.update': control?.updateFirmware,
      'client.wifi.status.get': control?.wifiStatus,
      'client.wifi.saved.list': control?.savedWifi,
      'client.wifi.saved.forget': control?.forgetWifi,
      'client.wifi.scan': control?.scanWifi,
      'client.wifi.connect': control?.connectWifi,
      'client.device.audioplayer.get': player?.get,
      'client.device.audioplayer.playlist.get': player?.playlistGet,
      'client.device.audioplayer.playlist.set': player?.playlistSet,
      'client.device.audioplayer.playlist.append': player?.playlistAppend,
      'client.device.audioplayer.play': player?.play,
      'client.device.audioplayer.stop': player?.stop,
      'client.device.audioplayer.mode.set': player?.modeSet,
    };
    return _rpcPayloadResponse(
      request.id,
      'client.rpc.methods.get',
      payload.ClientRpcMethodsGetResponse(
        methods: [
          'client.info.get',
          'client.identifiers.get',
          for (final entry in installed.entries)
            if (entry.value != null) entry.key,
          'client.rpc.methods.get',
        ],
      ),
    );
  }

  Future<rpc.RpcResponse> _serveSocialPing(rpc.RpcRequest request) async {
    const methodName = 'client.social.ping';
    late payload.ClientSocialPingRequest params;
    try {
      params =
          decodeRpcRequestPayload(
                methodName,
                request.hasPayload() ? request.payload : const [],
              )
              as payload.ClientSocialPingRequest;
    } catch (_) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
    if (params.fromPeerPublicKey.isEmpty) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
    final handler = handlers?.socialPing;
    if (handler == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
        'unsupported method: $methodName',
      );
    }
    await handler(params);
    return _rpcPayloadResponse(
      request.id,
      methodName,
      payload.ClientSocialPingResponse(),
    );
  }

  Future<rpc.RpcResponse> _serveDeviceControl(
    rpc.RpcRequest request,
    String methodName,
  ) async {
    final handlers = this.handlers?.deviceControl;
    rpc.RpcResponse unsupported() => _rpcErrorResponse(
      request.id,
      rpc.StatusCode.STATUS_CODE_UNIMPLEMENTED,
      'unsupported method: $methodName',
    );
    rpc.RpcResponse invalid() => _rpcErrorResponse(
      request.id,
      rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
      'invalid params',
    );
    GeneratedMessage? params;
    try {
      params = decodeRpcRequestPayload(
        methodName,
        request.hasPayload() ? request.payload : const [],
      );
    } catch (_) {
      return invalid();
    }
    switch (methodName) {
      case 'client.device.audioplayer.get':
        final handler = handlers?.audioplayer?.get;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerGetRequest;
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.audioplayer.playlist.get':
        final handler = handlers?.audioplayer?.playlistGet;
        if (handler == null) return unsupported();
        final player =
            params as payload.ClientDeviceAudioPlayerPlaylistGetRequest;
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.audioplayer.playlist.set':
        final handler = handlers?.audioplayer?.playlistSet;
        if (handler == null) return unsupported();
        final player =
            params as payload.ClientDeviceAudioPlayerPlaylistSetRequest;
        if (!_validAudioPlayerItems(player.items, false)) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.audioplayer.playlist.append':
        final handler = handlers?.audioplayer?.playlistAppend;
        if (handler == null) return unsupported();
        final player =
            params as payload.ClientDeviceAudioPlayerPlaylistAppendRequest;
        if (!_validAudioPlayerItems(player.items, true)) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.audioplayer.play':
        final handler = handlers?.audioplayer?.play;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerPlayRequest;
        if (!player.hasIndex() || player.index >= 32) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.audioplayer.stop':
        final handler = handlers?.audioplayer?.stop;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerStopRequest;
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.audioplayer.mode.set':
        final handler = handlers?.audioplayer?.modeSet;
        if (handler == null) return unsupported();
        final player = params as payload.ClientDeviceAudioPlayerModeSetRequest;
        if (!const {'off', 'one', 'all'}.contains(player.repeat)) {
          return invalid();
        }
        return _rpcPayloadResponse(
          request.id,
          methodName,
          await handler(player),
        );
      case 'client.device.status.get':
        final handler = handlers?.status;
        if (handler == null) return unsupported();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceStatusGetResponse(value: await handler()),
        );
      case 'client.device.volume.set':
        final handler = handlers?.setVolume;
        if (handler == null) return unsupported();
        final volume = params as payload.ClientDeviceVolumeSetRequest;
        final level = volume.level.toInt();
        if (level < 0 || level > 100) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceVolumeSetResponse(
            value: await handler(level, volume.muted),
          ),
        );
      case 'client.device.sound.play':
        final handler = handlers?.playSound;
        if (handler == null) return unsupported();
        final sound = params as payload.ClientDeviceSoundPlayRequest;
        if (sound.sound.isEmpty ||
            utf8.encode(sound.sound).length > _deviceControlMaxBytes) {
          return invalid();
        }
        await handler(
          sound.sound,
          sound.hasDurationMs() ? sound.durationMs.toInt() : null,
        );
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceSoundPlayResponse(),
        );
      case 'client.device.find':
        final handler = handlers?.find;
        if (handler == null) return unsupported();
        final find = params as payload.ClientDeviceFindRequest;
        final durationMs = find.hasDurationMs()
            ? find.durationMs.toInt()
            : null;
        if (durationMs != null && durationMs < 0) return invalid();
        await handler(durationMs);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceFindResponse(),
        );
      case 'client.device.reboot':
        final handler = handlers?.reboot;
        if (handler == null) return unsupported();
        final reboot = params as payload.ClientDeviceRebootRequest;
        await handler(reboot.hasDelayMs() ? reboot.delayMs.toInt() : null);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceRebootResponse(),
        );
      case 'client.device.settings.get':
        final handler = handlers?.getSettings;
        if (handler == null) return unsupported();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceSettingsGetResponse(value: await handler()),
        );
      case 'client.device.settings.set':
        final handler = handlers?.setSettings;
        if (handler == null) return unsupported();
        final patch = (params as payload.ClientDeviceSettingsSetRequest).value;
        // Reject the whole patch before applying any of it, so a bad member
        // cannot leave the device half-configured.
        if (!_validDeviceSettingsPatch(patch)) return invalid();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceSettingsSetResponse(value: await handler(patch)),
        );
      case 'client.device.factory_reset':
        final handler = handlers?.factoryReset;
        if (handler == null) return unsupported();
        final reset = params as payload.ClientDeviceFactoryResetRequest;
        await handler(reset.hasKeepNetwork() && reset.keepNetwork);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientDeviceFactoryResetResponse(),
        );
      case 'client.firmware.update':
        final handler = handlers?.updateFirmware;
        if (handler == null) return unsupported();
        final update = params as payload.ClientFirmwareUpdateRequest;
        await handler(
          update.hasChannel() ? update.channel : null,
          update.hasSha256() ? update.sha256 : null,
        );
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientFirmwareUpdateResponse(),
        );
      case 'client.wifi.status.get':
        final handler = handlers?.wifiStatus;
        if (handler == null) return unsupported();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiStatusGetResponse(value: await handler()),
        );
      case 'client.wifi.saved.list':
        final handler = handlers?.savedWifi;
        if (handler == null) return unsupported();
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiSavedListResponse(networks: await handler()),
        );
      case 'client.wifi.saved.forget':
        final handler = handlers?.forgetWifi;
        if (handler == null) return unsupported();
        final forget = params as payload.ClientWifiSavedForgetRequest;
        if (forget.ssid.isEmpty ||
            utf8.encode(forget.ssid).length > _deviceControlMaxBytes) {
          return invalid();
        }
        await handler(forget.ssid);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiSavedForgetResponse(),
        );
      case 'client.wifi.scan':
        final handler = handlers?.scanWifi;
        if (handler == null) return unsupported();
        final scan = params as payload.ClientWifiScanRequest;
        final timeoutMs = scan.hasTimeoutMs() ? scan.timeoutMs.toInt() : null;
        if (timeoutMs != null && (timeoutMs < 1000 || timeoutMs > 15000)) {
          return invalid();
        }
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiScanResponse(networks: await handler(timeoutMs)),
        );
      case 'client.wifi.connect':
        final handler = handlers?.connectWifi;
        if (handler == null) return unsupported();
        final connect = params as payload.ClientWifiConnectRequest;
        final ssidBytes = utf8.encode(connect.ssid).length;
        final passphrase = connect.hasPassphrase() ? connect.passphrase : null;
        final passphraseBytes = passphrase == null
            ? null
            : utf8.encode(passphrase).length;
        if (ssidBytes == 0 ||
            ssidBytes > _deviceControlMaxBytes ||
            (passphraseBytes != null &&
                (passphraseBytes < 8 || passphraseBytes > 63))) {
          return invalid();
        }
        await handler(connect.ssid, passphrase);
        return _rpcPayloadResponse(
          request.id,
          methodName,
          payload.ClientWifiConnectResponse(),
        );
      default:
        return unsupported();
    }
  }

  rpc.RpcResponse? _validateClientRequest(
    rpc.RpcRequest request,
    String methodName,
  ) {
    try {
      decodeRpcRequestPayload(
        methodName,
        request.hasPayload() ? request.payload : const [],
      );
      return null;
    } catch (_) {
      return _rpcErrorResponse(
        request.id,
        rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
        'invalid params',
      );
    }
  }

  rpc.RpcResponse _rpcPayloadResponse(
    String id,
    String methodName,
    GeneratedMessage response,
  ) {
    return rpc.RpcResponse(
      id: id,
      payload: encodeRpcResponsePayload(methodName, response),
    );
  }

  void _finishPing(rpc.RpcRequest request) {
    if (!request.hasPayload()) {
      _ignoreBody = true;
      _unawaited(
        _sendEnvelopeOnly(
          _rpcErrorResponse(
            request.id,
            rpc.StatusCode.STATUS_CODE_INVALID_ARGUMENT,
            'missing params',
          ),
        ).catchError((_) => _close()),
      );
      return;
    }
    decodeRpcRequestPayload('all.ping', request.payload);
    _ignoreBody = true;
    _unawaited(
      _sendEnvelopeOnly(
        rpc.RpcResponse(
          id: request.id,
          payload: encodeRpcResponsePayload(
            'all.ping',
            payload.PingResponse(
              serverTime: fixnum.Int64(DateTime.now().millisecondsSinceEpoch),
            ),
          ),
        ),
      ).catchError((_) => _close()),
    );
  }

  payload.SpeedTestRequest? _validSpeedTestParams(rpc.RpcRequest request) {
    if (!request.hasPayload()) {
      return null;
    }
    final params =
        decodeRpcRequestPayload('all.speed_test.run', request.payload)
            as payload.SpeedTestRequest;
    final down = params.downContentLength.toInt();
    final up = params.upContentLength.toInt();
    if (down < 0 ||
        up < 0 ||
        down > _rpcSpeedTestMaxContentLength ||
        up > _rpcSpeedTestMaxContentLength) {
      return null;
    }
    return params;
  }

  Future<void> _sendSpeedTestResponse(
    String id,
    payload.SpeedTestRequest params,
  ) async {
    final responseEnvelope = rpc.RpcResponse(
      id: id,
      payload: encodeRpcResponsePayload(
        'all.speed_test.run',
        payload.SpeedTestResponse(
          downContentLength: params.downContentLength,
          upContentLength: params.upContentLength,
        ),
      ),
    ).writeToBuffer();
    final frames = encodeEnvelopeFrames(responseEnvelope);
    await _sendFrames(frames);
    if (responseEnvelope.length > rpcMaxFramePayloadSize) {
      await _sendFrame(encodeFrame(rpcFrameTypeEos));
    }
    final chunk = Uint8List(_rpcSpeedTestFrameSize);
    final downLength = params.downContentLength.toInt();
    for (var offset = 0; offset < downLength; offset += chunk.length) {
      final remaining = downLength - offset;
      final size = remaining < chunk.length ? remaining : chunk.length;
      await _sendFrame(encodeFrame(rpcFrameTypeBinary, chunk.sublist(0, size)));
    }
    await _sendFrame(encodeFrame(rpcFrameTypeEos));
  }

  Future<void> _sendEnvelopeOnly(rpc.RpcResponse response) async {
    await _sendFrames(encodeEnvelopeFrames(response.writeToBuffer()));
    await _sendFrame(encodeFrame(rpcFrameTypeEos));
  }

  Future<void> _sendFrames(List<Uint8List> frames) async {
    for (final frame in frames) {
      await _sendFrame(frame);
    }
  }

  Future<void> _sendFrame(Uint8List frame) async {
    if (channel.state != GizClawDataChannelState.open) {
      throw StateError('RPC data channel is ${channel.state}, want open');
    }
    await channel.send(frame);
  }

  rpc.RpcResponse _rpcErrorResponse(
    String id,
    rpc.StatusCode code,
    String message,
  ) {
    return rpc.RpcResponse(
      id: id,
      status: rpc.RpcStatus(code: code, message: message),
    );
  }

  String _methodName(rpc.RpcRequest request) {
    return rpcMethodNamesById[request.method.value] ??
        'unknown:${request.method.value}';
  }

  bool _isClientMethod(String methodName) {
    return methodName == 'client.info.get' ||
        methodName == 'client.identifiers.get' ||
        methodName == 'client.tool.invoke' ||
        methodName == 'client.social.ping' ||
        methodName == 'client.rpc.methods.get' ||
        _deviceControlMethods.contains(methodName);
  }

  void _close() {
    if (_closed) {
      return;
    }
    _closed = true;
    _unawaited(_messages.cancel());
    _unawaited(_states.cancel());
    _unawaited(channel.close());
  }
}

void _unawaited(Future<void> future) {}

bool _validAudioPlayerItems(List<payload.AudioPlayerItem> items, bool append) {
  if (items.length > 32 || (append && items.isEmpty)) return false;
  return items.every((item) {
    final uri = Uri.tryParse(item.url);
    if (utf8.encode(item.url).length > 1024 ||
        uri == null ||
        uri.scheme != 'https' ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasFragment) {
      return false;
    }
    return utf8.encode(item.title).length <= 128 &&
        utf8.encode(item.sourceRef).length <= 128;
  });
}

/// Mirrors the DeviceSettings ranges in api/proto/rpc/payload/system.proto. An
/// unset member leaves that option unchanged; an explicitly unspecified enum is
/// rejected.
bool _validDeviceSettingsPatch(payload.DeviceSettings patch) {
  bool percent(bool present, fixnum.Int64 value) =>
      !present || (value >= 0 && value <= 100);
  if (!percent(patch.hasScreenBrightness(), patch.screenBrightness) ||
      !percent(patch.hasLedBrightness(), patch.ledBrightness)) {
    return false;
  }
  if (patch.hasScreenOffTimeoutMs() && patch.screenOffTimeoutMs < 0) {
    return false;
  }
  if (patch.hasLocale() &&
      (utf8.encode(patch.locale).length > _deviceSettingsLocaleMaxBytes ||
          !_deviceSettingsLocalePattern.hasMatch(patch.locale))) {
    return false;
  }
  if (patch.hasDefaultInteractionMode() &&
      patch.defaultInteractionMode ==
          payload.DeviceInteractionMode.DEVICE_INTERACTION_MODE_UNSPECIFIED) {
    return false;
  }
  if (patch.hasKeyFeedback() &&
      patch.keyFeedback ==
          payload.DeviceKeyFeedback.DEVICE_KEY_FEEDBACK_UNSPECIFIED) {
    return false;
  }
  return true;
}
