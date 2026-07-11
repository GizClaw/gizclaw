import 'dart:async';
import 'dart:typed_data';

import 'package:protobuf/protobuf.dart';

import 'generated/rpc/common.pb.dart' as common;
import 'generated/rpc/peer.pb.dart' as peer;
import 'method_registry.dart';
import 'payload_codec.dart';
import 'rpc_frame.dart';
import 'transport.dart';

const rpcVersion = 1;

class RpcCallResult {
  const RpcCallResult({required this.body, required this.response});

  final Uint8List body;
  final GeneratedMessage response;
}

class RpcError implements Exception {
  RpcError(this.code, this.message, {this.requestId});

  final int code;
  final String message;
  final String? requestId;

  @override
  String toString() => 'RpcError($code, $message)';
}

class PeerRpcClient {
  PeerRpcClient(
    this._factory, {
    String? channelLabel,
    String Function()? createId,
    Duration requestTimeout = const Duration(seconds: 30),
    int service = servicePeerRpc,
  }) : _channelLabel = channelLabel ?? giznetServiceDataChannelLabel(service),
       _createId = createId ?? _defaultRpcId,
       _requestTimeout = requestTimeout;

  final String _channelLabel;
  final String Function() _createId;
  final GizClawDataChannelFactory _factory;
  final Duration _requestTimeout;

  Future<T> call<T extends GeneratedMessage>(
    String methodName,
    GeneratedMessage request, {
    String? id,
    Duration? timeout,
  }) async {
    final result = await _call(
      methodName,
      request,
      expectBody: false,
      id: id,
      timeout: timeout,
    );
    return result.response as T;
  }

  Future<RpcCallResult> callBinary(
    String methodName,
    GeneratedMessage request, {
    String? id,
    Duration? timeout,
  }) {
    return _call(
      methodName,
      request,
      expectBody: true,
      id: id,
      timeout: timeout,
    );
  }

  Future<RpcCallResult> _call(
    String methodName,
    GeneratedMessage request, {
    required bool expectBody,
    String? id,
    Duration? timeout,
  }) async {
    final requestId = id ?? _createId();
    final channel = await _factory.createDataChannel(
      _channelLabel,
      options: const GizClawDataChannelOptions(ordered: true),
    );
    final encodedRequest = encodeRpcRequest(methodName, request, id: requestId);
    final responseReader = _ResponseReader(methodName, expectBody: expectBody);
    final completer = Completer<RpcCallResult>();
    var requestSent = false;
    Timer? timer;
    late StreamSubscription<Uint8List> messages;
    late StreamSubscription<GizClawDataChannelState> states;

    Future<void> cleanup() async {
      timer?.cancel();
      await messages.cancel();
      await states.cancel();
      await channel.close();
    }

    void fail(Object error, [StackTrace? stackTrace]) {
      if (completer.isCompleted) {
        return;
      }
      completer.completeError(error, stackTrace);
      _unawaited(cleanup());
    }

    void complete(RpcCallResult result) {
      if (completer.isCompleted) {
        return;
      }
      completer.complete(result);
      unawaited(cleanup());
    }

    Future<void> sendRequest() async {
      if (requestSent || completer.isCompleted) {
        return;
      }
      requestSent = true;
      try {
        await channel.send(encodedRequest);
      } catch (error, stackTrace) {
        fail(error, stackTrace);
      }
    }

    messages = channel.messages.listen(
      (chunk) {
        try {
          final result = responseReader.add(chunk);
          if (result != null) {
            complete(result);
          }
        } catch (error, stackTrace) {
          fail(error, stackTrace);
        }
      },
      onError: fail,
      onDone: () {
        if (!completer.isCompleted) {
          fail(StateError('RPC data channel closed before EOS'));
        }
      },
    );
    states = channel.states.listen((state) {
      if (state == GizClawDataChannelState.open) {
        _unawaited(sendRequest());
      } else if (state == GizClawDataChannelState.closed &&
          !completer.isCompleted) {
        fail(StateError('RPC data channel closed before response'));
      }
    }, onError: fail);

    timer = Timer(timeout ?? _requestTimeout, () {
      fail(
        TimeoutException('RPC request timed out', timeout ?? _requestTimeout),
      );
    });

    if (channel.state == GizClawDataChannelState.open) {
      await sendRequest();
    } else if (channel.state == GizClawDataChannelState.closed) {
      fail(StateError('RPC data channel is closed'));
    }

    return completer.future;
  }
}

Uint8List encodeRpcRequest(
  String methodName,
  GeneratedMessage request, {
  required String id,
}) {
  final descriptor = rpcMethodByName(methodName);
  final method = peer.RpcMethod.valueOf(descriptor.id);
  if (method == null) {
    throw ArgumentError.value(
      descriptor.id,
      'id',
      'unknown protobuf method id',
    );
  }
  final payload = encodeRpcRequestPayload(methodName, request);
  final envelope = peer.RpcRequest(id: id, method: method, payload: payload);
  return concatBytes([
    ...encodeEnvelopeFrames(envelope.writeToBuffer()),
    encodeFrame(rpcFrameTypeEos),
  ]);
}

RpcCallResult decodeRpcResponse(
  String methodName,
  List<int> envelopeBytes,
  List<int> body,
) {
  final envelope = common.RpcResponse.fromBuffer(envelopeBytes);
  if (envelope.hasError()) {
    throw RpcError(
      envelope.error.code.value,
      envelope.error.message,
      requestId: envelope.id,
    );
  }
  if (!envelope.hasPayload()) {
    throw const FormatException('RPC response missing payload or error');
  }
  return RpcCallResult(
    body: Uint8List.fromList(body),
    response: decodeRpcResponsePayload(methodName, envelope.payload),
  );
}

String _defaultRpcId() {
  final now = DateTime.now().microsecondsSinceEpoch.toRadixString(36);
  return 'dart-$now';
}

class _ResponseReader {
  _ResponseReader(this.methodName, {required this.expectBody});

  final bool expectBody;
  final String methodName;
  final _body = BytesBuilder(copy: false);
  final _envelopeChunks = <Uint8List>[];
  Uint8List _buffer = Uint8List(0);
  bool _envelopeRead = false;
  int _envelopeLength = 0;
  Uint8List? _responseEnvelope;

  RpcCallResult? add(Uint8List chunk) {
    _buffer = concatBytes([_buffer, chunk]);
    for (;;) {
      final result = tryReadFrame(_buffer);
      if (result == null) {
        return null;
      }
      _buffer = result.rest;
      final done = _handleFrame(result.frame);
      if (done != null) {
        return done;
      }
    }
  }

  RpcCallResult? _handleFrame(RpcFrame frame) {
    if (!_envelopeRead) {
      if (frame.type == rpcFrameTypeText) {
        _envelopeLength += frame.payload.length;
        if (_envelopeLength > rpcMaxEnvelopeSize) {
          throw const FormatException('RPC protobuf envelope too large');
        }
        _envelopeChunks.add(Uint8List.fromList(frame.payload));
        return null;
      }
      if (frame.type == rpcFrameTypeBinary) {
        if (_envelopeChunks.isNotEmpty) {
          throw const FormatException('RPC response has duplicate envelope');
        }
        _responseEnvelope = Uint8List.fromList(frame.payload);
        _envelopeRead = true;
        return null;
      }
      if (frame.type == rpcFrameTypeEos && _envelopeChunks.isNotEmpty) {
        _responseEnvelope = concatBytes(_envelopeChunks);
        _envelopeRead = true;
        if (!expectBody || _responseEnvelopeHasError(_responseEnvelope!)) {
          return decodeRpcResponse(methodName, _responseEnvelope!, const []);
        }
        return null;
      }
      throw FormatException(
        'expected RPC response envelope, got ${frame.type}',
      );
    }

    if (frame.type == rpcFrameTypeBinary) {
      if (!expectBody) {
        throw const FormatException('RPC response contains unexpected body');
      }
      _body.add(frame.payload);
      return null;
    }
    if (frame.type == rpcFrameTypeEos) {
      final envelope = _responseEnvelope;
      if (envelope == null) {
        throw const FormatException('RPC response missing envelope');
      }
      return decodeRpcResponse(methodName, envelope, _body.takeBytes());
    }
    throw FormatException('expected RPC response body/EOS, got ${frame.type}');
  }
}

bool _responseEnvelopeHasError(List<int> envelopeBytes) {
  return common.RpcResponse.fromBuffer(envelopeBytes).hasError();
}

void _unawaited(Future<void> future) {}
