import 'dart:async';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/payload_codec.dart';
import 'package:gizclaw/src/peer_rpc_server.dart';
import 'package:gizclaw/src/rpc_frame.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('serves server-initiated all.ping requests', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      _rpcRequestBytes(
        id: 'srv-ping',
        method: rpc.RpcMethod.RPC_METHOD_ALL_PING,
        payloadBytes: encodeRpcRequestPayload(
          'all.ping',
          payload.PingRequest(clientSendTime: fixnum.Int64(1)),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-ping');
    expect(response.hasError(), isFalse);
    final decoded =
        decodeRpcResponsePayload('all.ping', response.payload)
            as payload.PingResponse;
    expect(decoded.serverTime.toInt(), greaterThan(0));
  });

  test('serves server-initiated all.speed_test.run requests', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      concatBytes([
        _rpcRequestEnvelopeBytes(
          id: 'srv-speed',
          method: rpc.RpcMethod.RPC_METHOD_ALL_SPEED_TEST_RUN,
          payloadBytes: encodeRpcRequestPayload(
            'all.speed_test.run',
            payload.SpeedTestRequest(
              downContentLength: fixnum.Int64(3),
              upContentLength: fixnum.Int64(2),
            ),
          ),
        ),
        encodeFrame(rpcFrameTypeBinary, [7, 8]),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );
    await Future<void>.delayed(Duration.zero);

    final frames = decodeFrames(concatBytes(channel.sent));
    final response = rpc.RpcResponse.fromBuffer(frames.first.payload);
    expect(response.id, 'srv-speed');
    final decoded =
        decodeRpcResponsePayload('all.speed_test.run', response.payload)
            as payload.SpeedTestResponse;
    expect(decoded.downContentLength.toInt(), 3);
    expect(decoded.upContentLength.toInt(), 2);
    expect(frames[1].type, rpcFrameTypeBinary);
    expect(frames[1].payload, [0, 0, 0]);
    expect(frames.last.type, rpcFrameTypeEos);
  });

  test('rejects server-initiated all.ping without payload', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      _rpcRequestBytes(
        id: 'srv-missing-ping',
        method: rpc.RpcMethod.RPC_METHOD_ALL_PING,
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-missing-ping');
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);
  });

  test('rejects server-initiated all.speed_test.run without payload', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    serveGizClawPeerRpcChannel(channel);

    channel.addMessage(
      _rpcRequestEnvelopeBytes(
        id: 'srv-missing-speed',
        method: rpc.RpcMethod.RPC_METHOD_ALL_SPEED_TEST_RUN,
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-missing-speed');
    expect(response.error.code, rpc.RpcErrorCode.RPC_ERROR_CODE_INVALID_PARAMS);
  });
}

Uint8List _rpcRequestBytes({
  required String id,
  required rpc.RpcMethod method,
  List<int>? payloadBytes,
}) {
  return concatBytes([
    _rpcRequestEnvelopeBytes(
      id: id,
      method: method,
      payloadBytes: payloadBytes,
    ),
    encodeFrame(rpcFrameTypeEos),
  ]);
}

Uint8List _rpcRequestEnvelopeBytes({
  required String id,
  required rpc.RpcMethod method,
  List<int>? payloadBytes,
}) {
  return concatBytes(
    encodeEnvelopeFrames(
      rpc.RpcRequest(
        id: id,
        method: method,
        payload: payloadBytes,
      ).writeToBuffer(),
    ),
  );
}

rpc.RpcResponse _singleEnvelopeResponse(FakeDataChannel channel) {
  final frames = decodeFrames(concatBytes(channel.sent));
  expect(frames, hasLength(2));
  expect(frames.first.type, rpcFrameTypeBinary);
  expect(frames.last.type, rpcFrameTypeEos);
  return rpc.RpcResponse.fromBuffer(frames.first.payload);
}
