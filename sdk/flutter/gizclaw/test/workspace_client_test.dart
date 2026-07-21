import 'dart:async';

import 'package:gizclaw/src/client.dart';
import 'package:gizclaw/src/generated/rpc/rpc.pb.dart' as rpc;
import 'package:gizclaw/src/generated/rpc/payload.pb.dart' as payload;
import 'package:gizclaw/src/generated/rpc/payload.pbenum.dart' as enums;
import 'package:gizclaw/src/payload_codec.dart';
import 'package:gizclaw/src/rpc_frame.dart';
import 'package:protobuf/protobuf.dart';
import 'package:test/test.dart';

import 'fake_transport.dart';

void main() {
  test('lists visible models with pagination', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.listModels(cursor: 'model-cursor', limit: 25);
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload('server.model.list', request.payload)
            as payload.ModelListRequest;
    expect(body.cursor, 'model-cursor');
    expect(body.limit.toInt(), 25);
    _respond(
      factory.channels.single,
      request.id,
      'server.model.list',
      payload.ModelListResponse(
        items: [
          payload.Model(
            alias: 'chat-model',
            kind: enums.ModelKind.MODEL_KIND_LLM,
          ),
        ],
      ),
    );
    expect((await future).items.single.alias, 'chat-model');
  });

  test('creates a typed workspace document', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final workspace = payload.WorkspaceCreateBody(
      name: 'mobile-ast-device',
      workflowAlias: 'volc-ast-translate',
      collection: 'translates',
    );

    final future = client.createWorkspace(workspace);
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload('server.workspace.create', request.payload)
            as payload.WorkspaceCreateRequest;
    expect(body.value.name, 'mobile-ast-device');
    expect(body.value.workflowAlias, 'volc-ast-translate');
    expect(body.value.collection, 'translates');

    final responseWorkspace = payload.Workspace(
      name: workspace.name,
      system: true,
      workflowAlias: workspace.workflowAlias,
    );
    _respond(
      factory.channels.single,
      request.id,
      'server.workspace.create',
      payload.WorkspaceCreateResponse(value: responseWorkspace),
    );

    final created = (await future).value;
    expect(created.name, 'mobile-ast-device');
    expect(created.system, isTrue);
  });

  test('updates a typed workspace document', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);
    final workspace = payload.WorkspacePutBody(
      parameters: payload.WorkspaceParameters(),
    );

    final future = client.putWorkspace('mobile-ast-device', workspace);
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload('server.workspace.put', request.payload)
            as payload.WorkspacePutRequest;
    expect(body.name, 'mobile-ast-device');
    expect(body.body.hasParameters(), isTrue);
    _respond(
      factory.channels.single,
      request.id,
      'server.workspace.put',
      payload.WorkspacePutResponse(
        value: payload.Workspace(
          name: 'mobile-ast-device',
          workflowAlias: 'volc-ast-translate',
        ),
      ),
    );

    expect((await future).value.name, 'mobile-ast-device');
  });

  test('selects and reloads a run workspace', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final selected = client.setRunWorkspace('voice-room');
    final setRequest = await _request(factory, 0);
    final setPayload =
        decodeRpcRequestPayload('server.run.workspace.set', setRequest.payload)
            as payload.ServerSetRunWorkspaceRequest;
    expect(setPayload.value.workspaceName, 'voice-room');
    _respond(
      factory.channels[0],
      setRequest.id,
      'server.run.workspace.set',
      payload.ServerSetRunWorkspaceResponse(
        value: payload.PeerRunWorkspaceState(workspaceName: 'voice-room'),
      ),
    );
    expect((await selected).value.workspaceName, 'voice-room');

    final reloaded = client.reloadRunWorkspace();
    final reloadRequest = await _request(factory, 1);
    expect(
      decodeRpcRequestPayload(
        'server.run.workspace.reload',
        reloadRequest.payload,
      ),
      isA<payload.ServerReloadRunWorkspaceRequest>(),
    );
    _respond(
      factory.channels[1],
      reloadRequest.id,
      'server.run.workspace.reload',
      payload.ServerReloadRunWorkspaceResponse(
        value: payload.PeerRunWorkspaceState(activeWorkspaceName: 'voice-room'),
      ),
    );
    expect((await reloaded).value.activeWorkspaceName, 'voice-room');
  });

  test('reads the active run workspace', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.getRunWorkspace();
    final request = await _request(factory, 0);
    expect(
      decodeRpcRequestPayload('server.run.workspace.get', request.payload),
      isA<payload.ServerGetRunWorkspaceRequest>(),
    );
    _respond(
      factory.channels.single,
      request.id,
      'server.run.workspace.get',
      payload.ServerGetRunWorkspaceResponse(
        value: payload.PeerRunWorkspaceState(
          activeWorkspaceName: 'voice-room',
          workspaceName: 'voice-room',
        ),
      ),
    );

    expect((await future).value.activeWorkspaceName, 'voice-room');
  });

  test('requests ascending workspace history pages', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.listWorkspaceHistory(
      workspaceName: 'voice-room',
      cursor: 'cursor-1',
      limit: 25,
    );
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload(
              'server.workspace.history.list',
              request.payload,
            )
            as payload.WorkspaceHistoryListRequest;
    expect(body.workspaceName, 'voice-room');
    expect(body.cursor, 'cursor-1');
    expect(body.limit.toInt(), 25);
    expect(body.order.value, 1);
    _respond(
      factory.channels.single,
      request.id,
      'server.workspace.history.list',
      payload.WorkspaceHistoryListResponse(
        value: payload.PeerRunHistoryListResponse(),
      ),
    );
    expect((await future).value.items, isEmpty);
  });

  test('requests workspace history replay', () async {
    final factory = FakeDataChannelFactory();
    final client = GizClawClient(factory);

    final future = client.playRunWorkspaceHistory('history-voice-1');
    final request = await _request(factory, 0);
    final body =
        decodeRpcRequestPayload(
              'server.run.workspace.history.play',
              request.payload,
            )
            as payload.ServerPlayRunWorkspaceHistoryRequest;
    expect(body.value.historyId, 'history-voice-1');
    _respond(
      factory.channels.single,
      request.id,
      'server.run.workspace.history.play',
      payload.ServerPlayRunWorkspaceHistoryResponse(
        value: payload.PeerRunHistoryPlayResponse(
          accepted: true,
          historyId: 'history-voice-1',
          state: 'playing',
        ),
      ),
    );
    expect((await future).value.accepted, isTrue);
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
