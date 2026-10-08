import 'dart:convert';

import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('firmware metadata reads one key and preserves multiple URLs', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final future = client.getFirmwareMetadata('modem');
    while (factory.channels.isEmpty || factory.channels.single.sent.isEmpty) {
      await Future<void>.delayed(Duration.zero);
    }
    final channel = factory.channels.single;
    final frames = decodeFrames(channel.sent.single);
    final request = rpc.RpcRequest.fromBuffer(frames.first.payload);
    final body =
        decodeRpcRequestPayload('server.firmware.metadata.get', request.payload)
            as payload.FirmwareMetadataGetRequest;
    expect(request.method.value, 138);
    expect(body.key, 'modem');
    final urls = [
      'https://firmware.example/ap.bin',
      'https://firmware.example/cp.bin',
    ];
    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.firmware.metadata.get',
              payload.FirmwareMetadataGetResponse(
                key: 'modem',
                value: jsonEncode({'version': 'vendor-2026.10', 'urls': urls}),
              ),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );
    final result = await future;
    expect(result.key, 'modem');
    expect(jsonDecode(result.value), {
      'version': 'vendor-2026.10',
      'urls': urls,
    });
  });
}
