import 'dart:async';
import 'dart:typed_data';

import 'package:protobuf/protobuf.dart';

import 'generated/rpc/rpc.pb.dart' as rpc;
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
    int? maxBodyBytes,
    Duration? timeout,
  }) {
    return _call(
      methodName,
      request,
      expectBody: true,
      id: id,
      maxBodyBytes: maxBodyBytes,
      timeout: timeout,
    );
  }

  Future<T> callUpload<T extends GeneratedMessage>(
    String methodName,
    GeneratedMessage request,
    Uint8List body, {
    String? id,
    Duration? timeout,
  }) async {
    final result = await _call(
      methodName,
      request,
      expectBody: false,
      requestBody: body,
      id: id,
      timeout: timeout,
    );
    return result.response as T;
  }

  Future<RpcCallResult> _call(
    String methodName,
    GeneratedMessage request, {
    required bool expectBody,
    Uint8List? requestBody,
    String? id,
    int? maxBodyBytes,
    Duration? timeout,
  }) {
    late final String requestId;
    late final Uint8List encodedRequest;
    late final _ResponseReader responseReader;
    try {
      requestId = id ?? _createId();
      encodedRequest = encodeRpcRequest(
        methodName,
        request,
        id: requestId,
        body: requestBody,
      );
      responseReader = _ResponseReader(
        methodName,
        expectBody: expectBody,
        maxBodyBytes: maxBodyBytes,
        requestId: requestId,
      );
    } catch (error, stackTrace) {
      return Future<RpcCallResult>.error(error, stackTrace);
    }
    final completer = Completer<RpcCallResult>();
    final requestTimeout = timeout ?? _requestTimeout;
    GizClawDataChannel? channel;
    var requestSent = false;
    Timer? timer;
    StreamSubscription<Uint8List>? messages;
    StreamSubscription<GizClawDataChannelState>? states;

    Future<void> cleanup() async {
      timer?.cancel();
      final messageSubscription = messages;
      if (messageSubscription != null) {
        await messageSubscription.cancel();
      }
      final stateSubscription = states;
      if (stateSubscription != null) {
        await stateSubscription.cancel();
      }
      final activeChannel = channel;
      if (activeChannel != null) {
        await activeChannel.close();
      }
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

    timer = Timer(requestTimeout, () {
      fail(TimeoutException('RPC request timed out', requestTimeout));
    });

    Future<void> openChannel() async {
      try {
        channel = await _factory.createDataChannel(
          _channelLabel,
          options: const GizClawDataChannelOptions(ordered: true),
        );
      } catch (error, stackTrace) {
        fail(error, stackTrace);
        return;
      }

      final activeChannel = channel;
      if (activeChannel == null) {
        fail(StateError('RPC data channel was not created'));
        return;
      }
      if (completer.isCompleted) {
        _unawaited(activeChannel.close());
        return;
      }

      Future<void> sendRequest() async {
        if (requestSent || completer.isCompleted) {
          return;
        }
        requestSent = true;
        try {
          await activeChannel.send(encodedRequest);
        } catch (error, stackTrace) {
          fail(error, stackTrace);
        }
      }

      messages = activeChannel.messages.listen(
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
      states = activeChannel.states.listen((state) {
        if (state == GizClawDataChannelState.open) {
          _unawaited(sendRequest());
        } else if (state == GizClawDataChannelState.closed &&
            !completer.isCompleted) {
          fail(StateError('RPC data channel closed before response'));
        }
      }, onError: fail);

      if (activeChannel.state == GizClawDataChannelState.open) {
        await sendRequest();
      } else if (activeChannel.state == GizClawDataChannelState.closed) {
        fail(StateError('RPC data channel is closed'));
      }
    }

    _unawaited(openChannel());
    return completer.future;
  }
}

Uint8List encodeRpcRequest(
  String methodName,
  GeneratedMessage request, {
  required String id,
  Uint8List? body,
}) {
  final descriptor = rpcMethodByName(methodName);
  final method = rpc.RpcMethod.valueOf(descriptor.id);
  if (method == null) {
    throw ArgumentError.value(
      descriptor.id,
      'id',
      'unknown protobuf method id',
    );
  }
  final payload = encodeRpcRequestPayload(methodName, request);
  final envelope = rpc.RpcRequest(id: id, method: method, payload: payload);
  return concatBytes([
    ...encodeEnvelopeFrames(envelope.writeToBuffer()),
    if (body != null)
      for (
        var offset = 0;
        offset < body.length;
        offset += rpcMaxFramePayloadSize
      )
        encodeFrame(
          rpcFrameTypeBinary,
          body.sublist(
            offset,
            offset + rpcMaxFramePayloadSize > body.length
                ? body.length
                : offset + rpcMaxFramePayloadSize,
          ),
        ),
    encodeFrame(rpcFrameTypeEos),
  ]);
}

RpcCallResult decodeRpcResponse(
  String methodName,
  List<int> envelopeBytes,
  List<int> body,
  String requestId,
) {
  final envelope = rpc.RpcResponse.fromBuffer(envelopeBytes);
  if (envelope.id != requestId) {
    throw FormatException(
      'RPC response id mismatch: got ${envelope.id}, want $requestId',
    );
  }
  switch (envelope.whichBody()) {
    case rpc.RpcResponse_Body.payload:
      return RpcCallResult(
        body: Uint8List.fromList(body),
        response: decodeRpcResponsePayload(methodName, envelope.payload),
      );
    case rpc.RpcResponse_Body.error:
      throw RpcError(
        envelope.error.code.value,
        envelope.error.message,
        requestId: envelope.id,
      );
    case rpc.RpcResponse_Body.notSet:
      throw FormatException(
        'RPC response body is not set for $methodName '
        '(request ${envelope.id})',
      );
  }
}

String _defaultRpcId() {
  final now = DateTime.now().microsecondsSinceEpoch.toRadixString(36);
  return 'dart-$now';
}

class _ResponseReader {
  _ResponseReader(
    this.methodName, {
    required this.expectBody,
    this.maxBodyBytes,
    required this.requestId,
  }) {
    if (maxBodyBytes != null && maxBodyBytes! < 0) {
      throw ArgumentError.value(
        maxBodyBytes,
        'maxBodyBytes',
        'must be non-negative',
      );
    }
  }

  final bool expectBody;
  final int? maxBodyBytes;
  final String methodName;
  final String requestId;
  final _body = BytesBuilder(copy: false);
  int _bodyLength = 0;
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
          return decodeRpcResponse(
            methodName,
            _responseEnvelope!,
            const [],
            requestId,
          );
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
      _bodyLength += frame.payload.length;
      final maxBytes = maxBodyBytes;
      if (maxBytes != null && _bodyLength > maxBytes) {
        throw FormatException('RPC response body exceeds $maxBytes bytes');
      }
      _body.add(frame.payload);
      return null;
    }
    if (frame.type == rpcFrameTypeEos) {
      final envelope = _responseEnvelope;
      if (envelope == null) {
        throw const FormatException('RPC response missing envelope');
      }
      return decodeRpcResponse(
        methodName,
        envelope,
        _body.takeBytes(),
        requestId,
      );
    }
    throw FormatException('expected RPC response body/EOS, got ${frame.type}');
  }
}

bool _responseEnvelopeHasError(List<int> envelopeBytes) {
  return rpc.RpcResponse.fromBuffer(envelopeBytes).hasError();
}

void _unawaited(Future<void> future) {}
