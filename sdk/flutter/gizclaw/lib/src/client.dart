import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';

import 'generated/rpc/payload.pb.dart' as payload;
import 'pixa.dart';
import 'rpc_client.dart';
import 'service_http.dart';
import 'transport.dart';

class PixaDownloadResult<T> {
  const PixaDownloadResult({
    required this.metadata,
    required this.bytes,
    required this.asset,
  });

  final T metadata;
  final Uint8List bytes;
  final PixaAsset asset;
}

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

  Future<PixaDownloadResult<payload.PetDefPixaDownloadResponse>>
  downloadPetDefPixa(String id) async {
    final response = await rpc.callBinary(
      'server.pet_def.pixa.download',
      payload.PetDefPixaDownloadRequest(id: id),
    );
    final metadata = response.response as payload.PetDefPixaDownloadResponse;
    final bytes = Uint8List.fromList(response.body);
    return PixaDownloadResult(
      metadata: metadata,
      bytes: bytes,
      asset: validatePixa(bytes, mode: PixaValidationMode.petdef),
    );
  }

  Future<PixaDownloadResult<payload.BadgeDefPixaDownloadResponse>>
  downloadBadgeDefPixa(String id) async {
    final response = await rpc.callBinary(
      'server.badge_def.pixa.download',
      payload.BadgeDefPixaDownloadRequest(id: id),
    );
    final metadata = response.response as payload.BadgeDefPixaDownloadResponse;
    final bytes = Uint8List.fromList(response.body);
    return PixaDownloadResult(
      metadata: metadata,
      bytes: bytes,
      asset: validatePixa(bytes, mode: PixaValidationMode.badgedef),
    );
  }
}
