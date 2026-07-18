import 'dart:typed_data';

import 'package:fixnum/fixnum.dart' as fixnum;
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/gizclaw.dart';
import 'package:protobuf/protobuf.dart' show GeneratedMessage;
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
          PingRequest(clientSendTime: fixnum.Int64(1)),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = _singleEnvelopeResponse(channel);
    expect(response.id, 'srv-ping');
    expect(response.hasError(), isFalse);
    final decoded =
        decodeRpcResponsePayload('all.ping', response.payload) as PingResponse;
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
            SpeedTestRequest(
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
            as SpeedTestResponse;
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

  final device = DeviceInfo(
    name: 'Test Phone',
    hardware: HardwareInfo(
      hardwareRevision: 'revision-1',
      manufacturer: 'GizClaw',
      model: 'Phone Pro',
    ),
    identifiers: DeviceIdentifiers(
      sn: 'serial-1',
      imeis: [PeerIMEI(name: 'cellular', serial: 'imei-1')],
      labels: [PeerLabel(key: 'platform', value: 'test')],
    ),
  );

  test('serves client device info and identifiers', () async {
    final infoChannel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(infoChannel.close);
    serveGizClawPeerRpcChannel(
      infoChannel,
      handlers: GizClawPeerRpcHandlers(deviceInfo: () => device),
    );

    final infoResponse = await _callInbound(
      infoChannel,
      id: 'info-1',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_INFO_GET,
      methodName: 'client.info.get',
      request: ClientGetInfoRequest(),
    );
    final info =
        decodeRpcResponsePayload('client.info.get', infoResponse.payload)
            as ClientGetInfoResponse;
    expect(info.value.hardwareRevision, 'revision-1');
    expect(info.value.manufacturer, 'GizClaw');
    expect(info.value.model, 'Phone Pro');

    final identifiersChannel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(identifiersChannel.close);
    serveGizClawPeerRpcChannel(
      identifiersChannel,
      handlers: GizClawPeerRpcHandlers(deviceInfo: () => device),
    );
    final identifiersResponse = await _callInbound(
      identifiersChannel,
      id: 'identifiers-1',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_IDENTIFIERS_GET,
      methodName: 'client.identifiers.get',
      request: ClientGetIdentifiersRequest(),
    );
    final identifiers =
        decodeRpcResponsePayload(
              'client.identifiers.get',
              identifiersResponse.payload,
            )
            as ClientGetIdentifiersResponse;
    expect(identifiers.value.sn, 'serial-1');
    expect(identifiers.value.imeis.single.serial, 'imei-1');
    expect(identifiers.value.labels.single.value, 'test');
  });

  test('serves configured client tool invocations', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    ToolInvokeRequest? invoked;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        invokeTool: (request) {
          invoked = request;
          return ToolInvokeResponse(dataJson: '{"ok":true}');
        },
      ),
    );

    final response = await _callInbound(
      channel,
      id: 'tool-1',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
      methodName: 'client.tool.invoke',
      request: ToolInvokeRequest(
        callId: 'call-1',
        toolId: 'tool-1',
        method: 'run',
      ),
    );
    final result =
        decodeRpcResponsePayload('client.tool.invoke', response.payload)
            as ToolInvokeResponse;
    expect(invoked?.callId, 'call-1');
    expect(result.dataJson, '{"ok":true}');
  });

  test('waits for client request EOS before invoking a handler', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    var invocationCount = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        invokeTool: (request) {
          invocationCount++;
          return ToolInvokeResponse(dataJson: '{"ok":true}');
        },
      ),
    );

    channel.addMessage(
      _rpcRequestEnvelopeBytes(
        id: 'tool-wait-eos',
        method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
        payloadBytes: encodeRpcRequestPayload(
          'client.tool.invoke',
          ToolInvokeRequest(callId: 'call-wait-eos'),
        ),
      ),
    );
    await Future<void>.delayed(Duration.zero);

    expect(invocationCount, 0);
    expect(channel.sent, isEmpty);

    channel.addMessage(encodeFrame(rpcFrameTypeEos));
    for (var attempt = 0; channel.sent.length < 2; attempt++) {
      if (attempt == 20) fail('inbound RPC response was not sent');
      await Future<void>.delayed(Duration.zero);
    }

    expect(invocationCount, 1);
    expect(_singleEnvelopeResponse(channel).id, 'tool-wait-eos');
  });

  test('rejects an unexpected client request body', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    var invocationCount = 0;
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(
        deviceInfo: () => device,
        invokeTool: (request) {
          invocationCount++;
          return ToolInvokeResponse();
        },
      ),
    );

    channel.addMessage(
      concatBytes([
        _rpcRequestEnvelopeBytes(
          id: 'tool-body',
          method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
          payloadBytes: encodeRpcRequestPayload(
            'client.tool.invoke',
            ToolInvokeRequest(callId: 'call-body'),
          ),
        ),
        encodeFrame(rpcFrameTypeBinary, [1]),
      ]),
    );
    await Future<void>.delayed(Duration.zero);

    expect(invocationCount, 0);
    expect(channel.sent, isEmpty);
    expect(channel.state, GizClawDataChannelState.closed);
  });

  test('reports an unconfigured client tool handler', () async {
    final channel = FakeDataChannel('giznet/v1/service/0');
    addTearDown(channel.close);
    serveGizClawPeerRpcChannel(
      channel,
      handlers: GizClawPeerRpcHandlers(deviceInfo: () => device),
    );

    final response = await _callInbound(
      channel,
      id: 'tool-missing',
      method: rpc.RpcMethod.RPC_METHOD_CLIENT_TOOL_INVOKE,
      methodName: 'client.tool.invoke',
      request: ToolInvokeRequest(callId: 'call-missing'),
    );
    expect(
      response.error.code,
      rpc.RpcErrorCode.RPC_ERROR_CODE_METHOD_NOT_FOUND,
    );
    expect(response.error.message, contains('handler not configured'));
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

Future<rpc.RpcResponse> _callInbound(
  FakeDataChannel channel, {
  required String id,
  required rpc.RpcMethod method,
  required String methodName,
  required GeneratedMessage request,
}) async {
  final sentBefore = channel.sent.length;
  channel.addMessage(
    concatBytes([
      ...encodeEnvelopeFrames(
        rpc.RpcRequest(
          id: id,
          method: method,
          payload: encodeRpcRequestPayload(methodName, request),
        ).writeToBuffer(),
      ),
      encodeFrame(rpcFrameTypeEos),
    ]),
  );
  for (var attempt = 0; channel.sent.length < sentBefore + 2; attempt++) {
    if (attempt == 20) fail('inbound RPC response was not sent');
    await Future<void>.delayed(Duration.zero);
  }
  final frames = decodeFrames(
    Uint8List.fromList(
      channel.sent.skip(sentBefore).expand((message) => message).toList(),
    ),
  );
  expect(frames.last.type, rpcFrameTypeEos);
  return rpc.RpcResponse.fromBuffer(frames.first.payload);
}
