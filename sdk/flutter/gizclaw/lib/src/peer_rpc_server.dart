import 'dart:async';
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
typedef GizClawToolInvoker =
    FutureOr<payload.ToolInvokeResponse> Function(
      payload.ToolInvokeRequest request,
    );

class GizClawPeerRpcHandlers {
  const GizClawPeerRpcHandlers({required this.deviceInfo, this.invokeTool});

  final GizClawDeviceInfoProvider deviceInfo;
  final GizClawToolInvoker? invokeTool;
}

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
        _ => throw StateError('unsupported client method: $methodName'),
      };
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
    final provider = handlers?.deviceInfo;
    if (provider == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.RpcErrorCode.RPC_ERROR_CODE_INTERNAL_ERROR,
        'peer client not configured',
      );
    }
    final device = await provider();
    final identifiers = payload.DeviceIdentifiers();
    if (device.hasIdentifiers()) {
      if (device.identifiers.hasSn()) identifiers.sn = device.identifiers.sn;
      identifiers.imeis.addAll(device.identifiers.imeis);
      identifiers.labels.addAll(device.identifiers.labels);
    }
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
    final invokeTool = handlers?.invokeTool;
    if (invokeTool == null) {
      return _rpcErrorResponse(
        request.id,
        rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
        'client.tool.invoke handler not configured',
      );
    }
    final result = await invokeTool(params);
    return _rpcPayloadResponse(request.id, 'client.tool.invoke', result);
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
        methodName == 'client.tool.invoke';
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
