import 'dart:async';

import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/payload_codec.dart';
import 'package:gizclaw/src/rpc_client.dart';
import 'package:gizclaw/src/rpc_frame.dart';
import 'package:gizclaw/src/transport.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('sends request envelope and decodes RPC response', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory, createId: () => 'rpc-1');

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'demo-workspace'),
    );
    await Future<void>.delayed(Duration.zero);

    final channel = factory.channels.single;
    expect(channel.label, 'giznet/v1/service/0');
    expect(channel.sent, hasLength(1));
    expect(decodeFrames(channel.sent.single).last.type, rpcFrameTypeEos);

    final response = rpc.RpcResponse(
      id: 'rpc-1',
      payload: encodeRpcResponsePayload(
        'server.workspace.get',
        payload.WorkspaceGetResponse(
          value: payload.Workspace(name: 'demo-workspace'),
        ),
      ),
    );
    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(response.writeToBuffer()),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    final decoded = await future;
    expect(decoded.value.name, 'demo-workspace');
  });

  test('surfaces protobuf RPC errors', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory, createId: () => 'rpc-err');

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'missing'),
    );
    await Future<void>.delayed(Duration.zero);

    factory.channels.single.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: 'rpc-err',
            error: rpc.RpcError(
              code: rpc.RpcErrorCode.RPC_ERROR_CODE_NOT_FOUND,
              message: 'not found',
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    expect(
      future,
      throwsA(
        isA<RpcError>()
            .having((error) => error.code, 'code', 404)
            .having((error) => error.message, 'message', 'not found'),
      ),
    );
  });

  test('accepts omitted payload for an empty response message', () async {
    final result = decodeRpcResponse(
      'server.friend.invite_token.clear',
      rpc.RpcResponse(id: 'rpc-empty').writeToBuffer(),
      const [],
      'rpc-empty',
    );

    expect(result.response, isA<payload.FriendInviteTokenClearResponse>());
  });

  test('rejects mismatched RPC response ids', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory, createId: () => 'rpc-want');

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'demo-workspace'),
    );
    await Future<void>.delayed(Duration.zero);

    factory.channels.single.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: 'rpc-got',
            payload: encodeRpcResponsePayload(
              'server.workspace.get',
              payload.WorkspaceGetResponse(
                value: payload.Workspace(name: 'demo-workspace'),
              ),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    await expectLater(
      future,
      throwsA(
        isA<FormatException>().having(
          (error) => error.message,
          'message',
          contains('RPC response id mismatch'),
        ),
      ),
    );
  });

  test('rejects empty RPC response ids', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory, createId: () => 'rpc-want');

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'demo-workspace'),
    );
    await Future<void>.delayed(Duration.zero);

    factory.channels.single.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            payload: encodeRpcResponsePayload(
              'server.workspace.get',
              payload.WorkspaceGetResponse(
                value: payload.Workspace(name: 'demo-workspace'),
              ),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );

    await expectLater(
      future,
      throwsA(
        isA<FormatException>().having(
          (error) => error.message,
          'message',
          contains('RPC response id mismatch'),
        ),
      ),
    );
  });

  test('does not open a channel when request encoding fails', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory);

    await expectLater(
      client.call('unknown.method', payload.PingRequest()),
      throwsArgumentError,
    );
    expect(factory.channels, isEmpty);
  });

  test('does not open a channel when binary request encoding fails', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory);

    await expectLater(
      client.callBinary('unknown.method', payload.PingRequest()),
      throwsArgumentError,
    );
    expect(factory.channels, isEmpty);
  });

  test('completes plain RPC responses with continuation envelopes', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory, createId: () => 'rpc-continuation');

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'large-workspace'),
    );
    await Future<void>.delayed(Duration.zero);

    final largeName = 'w' * 70000;
    final response = rpc.RpcResponse(
      id: 'rpc-continuation',
      payload: encodeRpcResponsePayload(
        'server.workspace.get',
        payload.WorkspaceGetResponse(value: payload.Workspace(name: largeName)),
      ),
    );
    final frames = encodeEnvelopeFrames(response.writeToBuffer());
    expect(frames, hasLength(greaterThan(1)));

    factory.channels.single.addMessage(
      concatBytes([...frames, encodeFrame(rpcFrameTypeEos)]),
    );

    final decoded = await future;
    expect(decoded.value.name, largeName);
  });

  test('surfaces binary RPC errors with continuation envelopes', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(
      factory,
      createId: () => 'rpc-binary-continuation-error',
      requestTimeout: const Duration(milliseconds: 100),
    );

    final future = client.callBinary(
      'server.workspace.history.audio.get',
      payload.WorkspaceHistoryAudioGetRequest(
        historyId: 'history-1',
        workspaceName: 'main',
      ),
    );
    await Future<void>.delayed(Duration.zero);

    final response = rpc.RpcResponse(
      id: 'rpc-binary-continuation-error',
      error: rpc.RpcError(
        code: rpc.RpcErrorCode.RPC_ERROR_CODE_INTERNAL_ERROR,
        message: 'x' * 70000,
      ),
    );
    final frames = encodeEnvelopeFrames(response.writeToBuffer());
    expect(frames, hasLength(greaterThan(1)));

    factory.channels.single.addMessage(
      concatBytes([...frames, encodeFrame(rpcFrameTypeEos)]),
    );

    await expectLater(
      future,
      throwsA(
        isA<RpcError>()
            .having((error) => error.code, 'code', -32603)
            .having((error) => error.message.length, 'message length', 70000),
      ),
    );
  });

  test('sends a request once when open state is emitted again', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(factory, createId: () => 'rpc-send-once');

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'demo-workspace'),
    );
    await Future<void>.delayed(Duration.zero);

    final channel = factory.channels.single;
    channel.setState(GizClawDataChannelState.open);
    await Future<void>.delayed(Duration.zero);
    expect(channel.sent, hasLength(1));

    channel.addMessage(
      concatBytes([
        ...encodeEnvelopeFrames(
          rpc.RpcResponse(
            id: 'rpc-send-once',
            payload: encodeRpcResponsePayload(
              'server.workspace.get',
              payload.WorkspaceGetResponse(
                value: payload.Workspace(name: 'demo-workspace'),
              ),
            ),
          ).writeToBuffer(),
        ),
        encodeFrame(rpcFrameTypeEos),
      ]),
    );
    await future;
  });

  test('times out if the channel never returns a response EOS', () async {
    final factory = FakeDataChannelFactory();
    final client = PeerRpcClient(
      factory,
      createId: () => 'rpc-timeout',
      requestTimeout: const Duration(milliseconds: 10),
    );

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'slow'),
    );

    expect(future, throwsA(isA<TimeoutException>()));
  });

  test('times out while waiting for the RPC channel to open', () async {
    final gate = Completer<void>();
    final factory = FakeDataChannelFactory(createGate: gate.future);
    final client = PeerRpcClient(
      factory,
      createId: () => 'rpc-open-timeout',
      requestTimeout: const Duration(milliseconds: 10),
    );

    final future = client.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: 'slow-open'),
    );

    await expectLater(future, throwsA(isA<TimeoutException>()));
    expect(factory.channels, isEmpty);

    gate.complete();
    await Future<void>.delayed(Duration.zero);
    expect(factory.channels.single.state, GizClawDataChannelState.closed);
  });
}
