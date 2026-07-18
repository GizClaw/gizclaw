import 'dart:async';

import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('uploads the local device info to the server', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final device = DeviceInfo(name: 'Test Phone', emoji: '📱');

    final future = client.putServerInfo(device);
    await Future<void>.delayed(Duration.zero);

    final channel = factory.channels.single;
    final requestFrames = decodeFrames(channel.sent.single);
    final request = rpc.RpcRequest.fromBuffer(requestFrames.first.payload);
    expect(request.method, rpc.RpcMethod.RPC_METHOD_SERVER_INFO_PUT);
    final params =
        decodeRpcRequestPayload('server.info.put', request.payload)
            as ServerPutInfoRequest;
    expect(params.value.name, 'Test Phone');
    expect(params.value.emoji, '📱');
    expect(params.value.hasHardware(), isFalse);

    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.info.put',
              ServerPutInfoResponse(value: device),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    final response = await future;
    expect(response.value.name, 'Test Phone');
  });
}
