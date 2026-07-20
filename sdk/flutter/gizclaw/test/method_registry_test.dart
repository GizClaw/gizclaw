import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

void main() {
  test('contains canonical workflow and workspace RPC method IDs', () {
    expect(rpcMethodByName('server.workflow.list').id, 33);
    expect(rpcMethodByName('server.workspace.list').id, 25);
    expect(rpcMethodByName('server.workspace.get').id, 26);
    expect(rpcMethodByName('server.run.say').id, 21);
    expect(rpcMethodByName('all.ping').id, 1);
    expect(rpcMethodByName('server.route.resolve').id, 87);
  });

  test('encodes and decodes typed payloads by method metadata', () {
    final request = WorkspaceGetRequest(name: 'demo-workspace');
    final encoded = encodeRpcRequestPayload('server.workspace.get', request);
    final decoded =
        decodeRpcRequestPayload('server.workspace.get', encoded)
            as WorkspaceGetRequest;

    expect(decoded.name, 'demo-workspace');
  });

  test('rejects mismatched payload type', () {
    expect(
      () => encodeRpcRequestPayload(
        'server.workspace.get',
        WorkflowListRequest(),
      ),
      throwsArgumentError,
    );
  });

  test('exports generated enum payload types from public barrel', () {
    expect(ASTTranslateMode.ASTTRANSLATE_MODE_S2S.value, 2);
  });
}
