import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';

import 'generated/rpc/payload/enums.pbenum.dart' as enums;
import 'generated/rpc/payload.pb.dart' as payload;
import 'pixa.dart';
import 'rpc_client.dart';
import 'service_http.dart';
import 'transport.dart';

const int _maxIconDownloadBytes = 2 * 1024 * 1024;

/// Copies caller-controlled fields from a Workspace response into its write
/// payload without carrying output-only lifecycle metadata.
payload.WorkspaceUpsert workspaceUpsertFromWorkspace(
  payload.Workspace workspace,
) {
  final upsert = payload.WorkspaceUpsert(
    name: workspace.name,
    workflowName: workspace.workflowName,
  );
  if (workspace.hasParameters()) {
    upsert.parameters = workspace.parameters.deepCopy();
  }
  if (workspace.hasToolkit()) {
    upsert.toolkit = workspace.toolkit.deepCopy();
  }
  return upsert;
}

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

class IconDownloadResult<T> {
  const IconDownloadResult({required this.metadata, required this.bytes});

  final T metadata;
  final Uint8List bytes;
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

  Future<payload.ServerGetInfoResponse> getServerInfo() {
    return rpc.call<payload.ServerGetInfoResponse>(
      'server.info.get',
      payload.ServerGetInfoRequest(),
    );
  }

  Future<payload.ServerPutInfoResponse> putServerInfo(
    payload.DeviceProfile value,
  ) {
    return rpc.call<payload.ServerPutInfoResponse>(
      'server.info.put',
      payload.ServerPutInfoRequest(value: value),
    );
  }

  Future<payload.WorkflowListResponse> listWorkflows({
    String? cursor,
    int? limit,
    payload.WorkflowLocale? lang,
  }) {
    final request = payload.WorkflowListRequest();
    if (cursor != null) {
      request.cursor = cursor;
    }
    if (limit != null) {
      request.limit = Int64(limit);
    }
    if (lang != null) {
      request.lang = lang;
    }
    return rpc.call<payload.WorkflowListResponse>(
      'server.workflow.list',
      request,
    );
  }

  Future<payload.WorkflowGetResponse> getWorkflow(
    String name, {
    required payload.WorkflowLocale lang,
  }) {
    return rpc.call<payload.WorkflowGetResponse>(
      'server.workflow.get',
      payload.WorkflowGetRequest(name: name, lang: lang),
    );
  }

  Future<payload.ModelListResponse> listModels({String? cursor, int? limit}) {
    final request = payload.ModelListRequest();
    if (cursor != null) {
      request.cursor = cursor;
    }
    if (limit != null) {
      request.limit = Int64(limit);
    }
    return rpc.call<payload.ModelListResponse>('server.model.list', request);
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

  Future<payload.FriendListResponse> listFriends({String? cursor, int? limit}) {
    final request = payload.FriendListRequest();
    if (cursor != null) request.cursor = cursor;
    if (limit != null) request.limit = Int64(limit);
    return rpc.call<payload.FriendListResponse>('server.friend.list', request);
  }

  Future<payload.FriendInfoGetResponse> getFriendInfo(String id) {
    return rpc.call<payload.FriendInfoGetResponse>(
      'server.friend.info.get',
      payload.FriendInfoGetRequest(id: id),
    );
  }

  Future<payload.FriendInviteTokenGetResponse> getFriendInviteToken() {
    return rpc.call<payload.FriendInviteTokenGetResponse>(
      'server.friend.invite_token.get',
      payload.FriendInviteTokenGetRequest(),
    );
  }

  Future<payload.FriendInviteTokenCreateResponse> createFriendInviteToken() {
    return rpc.call<payload.FriendInviteTokenCreateResponse>(
      'server.friend.invite_token.create',
      payload.FriendInviteTokenCreateRequest(),
    );
  }

  Future<payload.FriendInviteTokenClearResponse> clearFriendInviteToken() {
    return rpc.call<payload.FriendInviteTokenClearResponse>(
      'server.friend.invite_token.clear',
      payload.FriendInviteTokenClearRequest(),
    );
  }

  Future<payload.FriendAddResponse> addFriend(String inviteToken) {
    return rpc.call<payload.FriendAddResponse>(
      'server.friend.add',
      payload.FriendAddRequest(inviteToken: inviteToken),
    );
  }

  Future<payload.FriendDeleteResponse> deleteFriend(String id) {
    return rpc.call<payload.FriendDeleteResponse>(
      'server.friend.delete',
      payload.FriendDeleteRequest(id: id),
    );
  }

  Future<payload.FriendGroupListResponse> listFriendGroups({
    String? cursor,
    int? limit,
  }) {
    final request = payload.FriendGroupListRequest();
    if (cursor != null) request.cursor = cursor;
    if (limit != null) request.limit = Int64(limit);
    return rpc.call<payload.FriendGroupListResponse>(
      'server.friend_group.list',
      request,
    );
  }

  Future<payload.FriendGroupCreateResponse> createFriendGroup({
    required String name,
    String description = '',
  }) {
    return rpc.call<payload.FriendGroupCreateResponse>(
      'server.friend_group.create',
      payload.FriendGroupCreateRequest(name: name, description: description),
    );
  }

  Future<payload.WorkspaceGetResponse> getWorkspace(String name) {
    return rpc.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: name),
    );
  }

  Future<payload.WorkspaceCreateResponse> createWorkspace(
    payload.WorkspaceUpsert workspace,
  ) {
    return rpc.call<payload.WorkspaceCreateResponse>(
      'server.workspace.create',
      payload.WorkspaceCreateRequest(value: workspace),
    );
  }

  Future<payload.WorkspacePutResponse> putWorkspace(
    String name,
    payload.WorkspaceUpsert workspace,
  ) {
    return rpc.call<payload.WorkspacePutResponse>(
      'server.workspace.put',
      payload.WorkspacePutRequest(name: name, body: workspace),
    );
  }

  Future<payload.ServerGetRunWorkspaceResponse> getRunWorkspace() {
    return rpc.call<payload.ServerGetRunWorkspaceResponse>(
      'server.run.workspace.get',
      payload.ServerGetRunWorkspaceRequest(),
    );
  }

  Future<payload.ServerSetRunWorkspaceResponse> setRunWorkspace(String name) {
    return rpc.call<payload.ServerSetRunWorkspaceResponse>(
      'server.run.workspace.set',
      payload.ServerSetRunWorkspaceRequest(
        value: payload.AgentSelection(workspaceName: name),
      ),
    );
  }

  Future<payload.ServerReloadRunWorkspaceResponse> reloadRunWorkspace() {
    return rpc.call<payload.ServerReloadRunWorkspaceResponse>(
      'server.run.workspace.reload',
      payload.ServerReloadRunWorkspaceRequest(),
    );
  }

  Future<payload.ServerPlayRunWorkspaceHistoryResponse> playRunWorkspaceHistory(
    String historyId,
  ) {
    return rpc.call<payload.ServerPlayRunWorkspaceHistoryResponse>(
      'server.run.workspace.history.play',
      payload.ServerPlayRunWorkspaceHistoryRequest(
        value: payload.PeerRunHistoryPlayRequest(historyId: historyId),
      ),
    );
  }

  Future<payload.WorkspaceHistoryListResponse> listWorkspaceHistory({
    required String workspaceName,
    String? cursor,
    int? limit,
  }) {
    final request = payload.WorkspaceHistoryListRequest(
      workspaceName: workspaceName,
      order: enums
          .WorkspaceHistoryListRequestOrder
          .WORKSPACE_HISTORY_LIST_REQUEST_ORDER_ASC,
    );
    if (cursor != null) request.cursor = cursor;
    if (limit != null) request.limit = Int64(limit);
    return rpc.call<payload.WorkspaceHistoryListResponse>(
      'server.workspace.history.list',
      request,
    );
  }

  Future<payload.ServerPetListResponse> listPets({String? cursor, int? limit}) {
    final value = payload.GameplayListRequest();
    if (cursor != null) value.cursor = cursor;
    if (limit != null) value.limit = Int64(limit);
    return rpc.call<payload.ServerPetListResponse>(
      'server.pet.list',
      payload.ServerPetListRequest(value: value),
    );
  }

  Future<payload.ServerPetGetResponse> getPet(String id) {
    return rpc.call<payload.ServerPetGetResponse>(
      'server.pet.get',
      payload.ServerPetGetRequest(value: payload.PetGetRequest(id: id)),
    );
  }

  Future<payload.ServerPetAdoptResponse> adoptPet({
    String? displayName,
    String? rulesetName,
  }) {
    final value = payload.PetAdoptRequest();
    if (displayName != null && displayName.isNotEmpty) {
      value.displayName = displayName;
    }
    if (rulesetName != null && rulesetName.isNotEmpty) {
      value.rulesetName = rulesetName;
    }
    return rpc.call<payload.ServerPetAdoptResponse>(
      'server.pet.adopt',
      payload.ServerPetAdoptRequest(value: value),
    );
  }

  Future<payload.ServerPetDriveResponse> drivePet(
    String petId, {
    required String action,
  }) {
    return rpc.call<payload.ServerPetDriveResponse>(
      'server.pet.drive',
      payload.ServerPetDriveRequest(
        value: payload.PetDriveRequest(petId: petId, action: action),
      ),
    );
  }

  Future<payload.ServerPetActionsGetResponse> getPetActions(String petId) {
    return rpc.call<payload.ServerPetActionsGetResponse>(
      'server.pet.actions.get',
      payload.ServerPetActionsGetRequest(
        value: payload.PetGetRequest(id: petId),
      ),
    );
  }

  Future<PixaDownloadResult<payload.ServerPetPixaDownloadResponse>>
  downloadPetPixa(String petId) async {
    final response = await rpc.callBinary(
      'server.pet.pixa.download',
      payload.ServerPetPixaDownloadRequest(
        value: payload.PetPixaDownloadRequest(petId: petId),
      ),
    );
    final metadata = response.response as payload.ServerPetPixaDownloadResponse;
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

  Future<IconDownloadResult<payload.WorkflowIconDownloadResponse>>
  downloadWorkflowIcon(String name, enums.IconFormat format) async {
    final response = await rpc.callBinary(
      'server.workflow.icon.download',
      payload.WorkflowIconDownloadRequest(name: name, format: format),
      maxBodyBytes: _maxIconDownloadBytes,
    );
    return IconDownloadResult(
      metadata: response.response as payload.WorkflowIconDownloadResponse,
      bytes: Uint8List.fromList(response.body),
    );
  }

  Future<IconDownloadResult<payload.WorkspaceIconDownloadResponse>>
  downloadWorkspaceIcon(String name, enums.IconFormat format) async {
    final response = await rpc.callBinary(
      'server.workspace.icon.download',
      payload.WorkspaceIconDownloadRequest(name: name, format: format),
      maxBodyBytes: _maxIconDownloadBytes,
    );
    return IconDownloadResult(
      metadata: response.response as payload.WorkspaceIconDownloadResponse,
      bytes: Uint8List.fromList(response.body),
    );
  }
}
