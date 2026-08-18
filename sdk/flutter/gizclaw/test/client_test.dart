import 'dart:async';

import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('registers the current server connection', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.register('registration-secret');
    await Future<void>.delayed(Duration.zero);

    final channel = factory.channels.single;
    final requestFrames = decodeFrames(channel.sent.single);
    final request = rpc.RpcRequest.fromBuffer(requestFrames.first.payload);
    expect(request.method, rpc.RpcMethod.RPC_METHOD_SERVER_REGISTER);
    final params =
        decodeRpcRequestPayload('server.register', request.payload)
            as ServerRegisterRequest;
    expect(params.token, 'registration-secret');

    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.register',
              ServerRegisterResponse(runtimeProfileName: 'profile-a'),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    final response = await future;
    expect(response.runtimeProfileName, 'profile-a');
  });

  test('creates an API key for the current device', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.createApiKey(
      displayName: 'phone',
      manageApiKeys: true,
    );
    await Future<void>.delayed(Duration.zero);

    final channel = factory.channels.single;
    final request = rpc.RpcRequest.fromBuffer(
      decodeFrames(channel.sent.single).first.payload,
    );
    expect(request.method, rpc.RpcMethod.RPC_METHOD_SERVER_API_KEY_CREATE);
    final params =
        decodeRpcRequestPayload('server.api_key.create', request.payload)
            as APIKeyCreateRequest;
    expect(params.displayName, 'phone');
    expect(params.manageApiKeys, isTrue);

    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.api_key.create',
              APIKeyCreateResponse(apiKey: 'gizclaw_sk_v1_secret'),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    expect((await future).apiKey, 'gizclaw_sk_v1_secret');
  });

  test('lists and revokes API keys for the current device', () async {
    final listFactory = FakeDataChannelFactory();
    final listClient = GizClawClient(listFactory);
    final listFuture = listClient.listApiKeys(cursor: 'key_cursor', limit: 25);
    await Future<void>.delayed(Duration.zero);

    final listChannel = listFactory.channels.single;
    final listRequest = rpc.RpcRequest.fromBuffer(
      decodeFrames(listChannel.sent.single).first.payload,
    );
    expect(listRequest.method, rpc.RpcMethod.RPC_METHOD_SERVER_API_KEY_LIST);
    final listParams =
        decodeRpcRequestPayload('server.api_key.list', listRequest.payload)
            as APIKeyListRequest;
    expect(listParams.cursor, 'key_cursor');
    expect(listParams.limit.toInt(), 25);
    listChannel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: listRequest.id,
            payload: encodeRpcResponsePayload(
              'server.api_key.list',
              APIKeyListResponse(items: [APIKey(name: 'key_a')]),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );
    expect((await listFuture).items.single.name, 'key_a');

    final revokeFactory = FakeDataChannelFactory();
    final revokeClient = GizClawClient(revokeFactory);
    final revokeFuture = revokeClient.revokeApiKey('key_a');
    await Future<void>.delayed(Duration.zero);

    final revokeChannel = revokeFactory.channels.single;
    final revokeRequest = rpc.RpcRequest.fromBuffer(
      decodeFrames(revokeChannel.sent.single).first.payload,
    );
    expect(
      revokeRequest.method,
      rpc.RpcMethod.RPC_METHOD_SERVER_API_KEY_REVOKE,
    );
    final revokeParams =
        decodeRpcRequestPayload('server.api_key.revoke', revokeRequest.payload)
            as APIKeyRevokeRequest;
    expect(revokeParams.name, 'key_a');
    revokeChannel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: revokeRequest.id,
            payload: encodeRpcResponsePayload(
              'server.api_key.revoke',
              APIKeyRevokeResponse(),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );
    await revokeFuture;
  });

  test('uploads the local device info to the server', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final profile = DeviceProfile(name: 'Test Phone', emoji: '📱');

    final future = client.putServerInfo(profile);
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

    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: request.id,
            payload: encodeRpcResponsePayload(
              'server.info.put',
              ServerPutInfoResponse(
                value: DeviceInfo(name: profile.name, emoji: profile.emoji),
              ),
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
