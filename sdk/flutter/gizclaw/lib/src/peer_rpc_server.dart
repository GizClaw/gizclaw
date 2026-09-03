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

/// Thrown by a device control handler to answer the Server with a specific
/// RPC error code, for example `INVALID_PARAMS` for an unknown sound or
/// `NOT_FOUND` for an unknown saved network.
class GizClawDeviceControlException implements Exception {
  const GizClawDeviceControlException(this.code, [this.message = '']);

  final rpc.RpcErrorCode code;
  final String message;

  @override
  String toString() => 'GizClawDeviceControlException($code, $message)';
}

/// Implements the Server-initiated `client.device.*` and `client.wifi.*`
/// methods. A null handler answers `METHOD_NOT_FOUND`, which the Server maps
/// to `501 DEVICE_UNSUPPORTED`.
class GizClawDeviceControlHandlers {
  const GizClawDeviceControlHandlers({
    this.status,
    this.setVolume,
    this.playSound,
    this.reboot,
    this.wifiStatus,
    this.savedWifi,
    this.forgetWifi,
  });

  final FutureOr<payload.PeerStatus> Function()? status;
  final FutureOr<payload.PeerStatus> Function(int level, bool muted)? setVolume;
  final FutureOr<void> Function(String sound, int? durationMs)? playSound;
  final FutureOr<void> Function(int? delayMs)? reboot;
  final FutureOr<payload.WifiStatus> Function()? wifiStatus;
  final FutureOr<List<payload.WifiSavedNetwork>> Function()? savedWifi;
  final FutureOr<void> Function(String ssid)? forgetWifi;
}

class GizClawPeerRpcHandlers {
  GizClawPeerRpcHandlers({
    required this.deviceInfo,
    Map<String, GizClawToolHandler> tools = const {},
    this.deviceControl,
    this.deviceIdentifiers,
  }) : tools = Map.unmodifiable(tools);

  final GizClawDeviceInfoProvider deviceInfo;
  final Map<String, GizClawToolHandler> tools;
  final GizClawDeviceControlHandlers? deviceControl;

  /// Answers `client.identifiers.get`. When null the identifiers reported by
  /// [deviceInfo] are used, so a device that already reports them there needs
  /// no separate provider.
  final GizClawDeviceIdentifiersProvider? deviceIdentifiers;
}

const _deviceControlMethods = {
  'client.device.status.get',
  'client.device.volume.set',
  'client.device.sound.play',
  'client.device.reboot',
  'client.wifi.status.get',
  'client.wifi.saved.list',
  'client.wifi.saved.forget',
};
const _deviceControlMaxBytes = 32;

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
                rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
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
      case 'client.device.status.get':
      case 'client.device.volume.set':
      case 'client.device.sound.play':
      case 'client.device.reboot':
      case 'client.wifi.status.get':
      case 'client.wifi.saved.list':
      case 'client.wifi.saved.forget':
        return;
      default:
        _ignoreBody = true;
        _unawaited(
          _sendEnvelopeOnly(
            _rpcErrorResponse(
              request.id,
              rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
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
        _ when _deviceControlMethods.contains(methodName) =>
          await _serveDeviceControl(request, methodName),
        _ => throw StateError('unsupported client method: $methodName'),
      };
    } on GizClawDeviceControlException catch (error) {
      response = _rpcErrorResponse(request.id, error.code, error.message);
    } catch (error) {
      response = _rpcErrorResponse(
        request.id,
        rpc.RpcErrorCode.RPC_ERROR_CODE_INTERNAL_ERROR,
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
        rpc.RpcErrorCode.RPC_ERROR_CODE_INTERNAL_ERROR,
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
        rpc.RpcErrorCode.RPC_ERROR_CODE_INTERNAL_ERROR,
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
        rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
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
        rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
        'invalid params',
      );
    }
    final name = params.invokeName.trim();
    if (!RegExp(r'^[A-Za-z_][A-Za-z0-9_-]{0,63}$').hasMatch(name)) {
      return _rpcErrorResponse(
        request.id,
        rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
        'invalid Tool name',
      );
    }
    final handler = handlers?.tools[name];
    if (handler == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
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
        rpc.RpcErrorCode.RPC_ERROR_CODE_INTERNAL_ERROR,
        'Tool handler failed',
      );
    }
  }

  Future<rpc.RpcResponse> _serveDeviceControl(
    rpc.RpcRequest request,
    String methodName,
  ) async {
    final handlers = this.handlers?.deviceControl;
    rpc.RpcResponse unsupported() => _rpcErrorResponse(
      request.id,
      rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
      'unsupported method: $methodName',
    );
    rpc.RpcResponse invalid() => _rpcErrorResponse(
      request.id,
      rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
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
        rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
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
            rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS,
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
    rpc.RpcErrorCode code,
    String message,
  ) {
    return rpc.RpcResponse(
      id: id,
      error: rpc.RpcError(code: code, message: message),
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
