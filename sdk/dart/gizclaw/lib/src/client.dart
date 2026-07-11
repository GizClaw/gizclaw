import 'package:fixnum/fixnum.dart';

import 'generated/rpc/payload.pb.dart' as payload;
import 'rpc_client.dart';
import 'service_http.dart';
import 'transport.dart';

class GizClawClient {
  GizClawClient(
    GizClawDataChannelFactory transport, {
    Duration requestTimeout = const Duration(seconds: 30),
  }) : rpc = PeerRpcClient(transport, requestTimeout: requestTimeout),
       peerHttp = ServiceHttpClient(
         transport,
         requestTimeout: requestTimeout,
         service: servicePeerHttp,
       ),
       peerOpenAi = ServiceHttpClient(
         transport,
         requestTimeout: requestTimeout,
         service: servicePeerOpenAi,
       );

  final ServiceHttpClient peerHttp;
  final ServiceHttpClient peerOpenAi;
  final PeerRpcClient rpc;

  Future<payload.WorkflowListResponse> listWorkflows({
    String? cursor,
    int? limit,
  }) {
    final request = payload.WorkflowListRequest();
    if (cursor != null) {
      request.cursor = cursor;
    }
    if (limit != null) {
      request.limit = Int64(limit);
    }
    return rpc.call<payload.WorkflowListResponse>(
      'server.workflow.list',
      request,
    );
  }

  Future<payload.WorkspaceListResponse> listWorkspaces({
    String? cursor,
    int? limit,
    String? prefix,
  }) {
    final request = payload.WorkspaceListRequest();
    if (cursor != null) {
      request.cursor = cursor;
    }
    if (limit != null) {
      request.limit = Int64(limit);
    }
    if (prefix != null) {
      request.prefix = prefix;
    }
    return rpc.call<payload.WorkspaceListResponse>(
      'server.workspace.list',
      request,
    );
  }

  Future<payload.WorkspaceGetResponse> getWorkspace(String name) {
    return rpc.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: name),
    );
  }
}
