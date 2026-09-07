import 'dart:async';

import 'package:gizclaw/src/client.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/payload_codec.dart';
import 'package:gizclaw/src/rpc_frame.dart';
import 'package:protobuf/protobuf.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('lists app config keys with pagination', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.listAppConfig(cursor: 'app-cursor', limit: 2);
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload('server.app_config.list', request.payload)
            as payload.AppConfigListRequest;
    expect(body.cursor, 'app-cursor');
    expect(body.limit.toInt(), 2);
    _respond(
      factory.channels.single,
      request.id,
      'server.app_config.list',
      payload.AppConfigListResponse(
        keys: ['app.entrypoints', 'ui.theme'],
        hasNext: true,
        nextCursor: 'next-cursor',
        runtimeProfileName: 'default',
        runtimeProfileRevision: 'revision',
      ),
    );
    final response = await future;
    expect(response.keys, ['app.entrypoints', 'ui.theme']);
    expect(response.hasNext, isTrue);
    expect(response.nextCursor, 'next-cursor');
    expect(response.runtimeProfileRevision, 'revision');
  });

  test('reads one opaque app config value verbatim', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    const value = '{\n  "theme": "深色"\n}\n';

    final future = client.getAppConfig('ui.theme');
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload('server.app_config.get', request.payload)
            as payload.AppConfigGetRequest;
    expect(body.key, 'ui.theme');
    _respond(
      factory.channels.single,
      request.id,
      'server.app_config.get',
      payload.AppConfigGetResponse(
        value: value,
        runtimeProfileName: 'default',
        runtimeProfileRevision: 'revision',
      ),
    );
    expect((await future).value, value);
  });
}

Future<rpc.RpcRequest> _request(
  FakeDataChannelFactory factory,
  int index,
) async {
  while (factory.channels.length <= index ||
      factory.channels[index].sent.isEmpty) {
    await Future<void>.delayed(Duration.zero);
  }
  final frames = decodeFrames(factory.channels[index].sent.single);
  return rpc.RpcRequest.fromBuffer(frames.first.payload);
}

void _respond(
  FakeDataChannel channel,
  String id,
  String method,
  GeneratedMessage response,
) {
  channel.addMessage(
    concatBytes([
      ...encodeEnvelopeFrames(
        rpc.RpcResponse(
          id: id,
          payload: encodeRpcResponsePayload(method, response),
        ).writeToBuffer(),
      ),
      encodeFrame(rpcFrameTypeEos),
    ]),
  );
}
