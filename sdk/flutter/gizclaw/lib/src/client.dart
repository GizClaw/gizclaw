import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';

import 'generated/rpc/payload/enums.pbenum.dart' as enums;
import 'generated/rpc/payload.pb.dart' as payload;
import 'rpc_client.dart';
import 'service_http.dart';
import 'transport.dart';

const int _maxIconDownloadBytes = 2 * 1024 * 1024;

/// Copies caller-controlled fields from a Workspace response into its write
/// payload without carrying output-only lifecycle metadata.
payload.WorkspacePutBody workspacePutBodyFromWorkspace(
  payload.Workspace workspace,
) {
  final body = payload.WorkspacePutBody();
  if (workspace.hasParameters()) {
    body.parameters = workspace.parameters.deepCopy();
  }
  if (workspace.hasToolkit()) {
    body.toolkit = workspace.toolkit.deepCopy();
  }
  return body;
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

  Future<payload.ServerRegisterResponse> register(String token) {
    return rpc.call<payload.ServerRegisterResponse>(
      'server.register',
      payload.ServerRegisterRequest(token: token),
    );
  }

  /// Creates a long-lived, recoverable API key for this connected device.
  Future<payload.APIKeyCreateResponse> createApiKey({
    required String displayName,
    bool manageApiKeys = false,
  }) {
    return rpc.call<payload.APIKeyCreateResponse>(
      'server.api_key.create',
      payload.APIKeyCreateRequest(
        displayName: displayName,
        manageApiKeys: manageApiKeys,
      ),
    );
  }

  /// Lists API keys owned by this connected device, including complete keys.
  Future<payload.APIKeyListResponse> listApiKeys({String? cursor, int? limit}) {
    return rpc.call<payload.APIKeyListResponse>(
      'server.api_key.list',
      payload.APIKeyListRequest(
        cursor: cursor,
        limit: limit == null ? null : Int64(limit),
      ),
    );
  }

  /// Revokes an API key owned by this connected device.
  Future<payload.APIKeyRevokeResponse> revokeApiKey(String name) {
    return rpc.call<payload.APIKeyRevokeResponse>(
      'server.api_key.revoke',
      payload.APIKeyRevokeRequest(name: name),
    );
  }

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

  /// Reads the public display name and emoji of up to 16 Peers by public key.
  /// The response holds one item per distinct key, in request order.
  Future<payload.ProfileGetResponse> getProfiles(List<String> peerPublicKeys) {
    return rpc.call<payload.ProfileGetResponse>(
      'server.profile.get',
      payload.ProfileGetRequest(peerPublicKeys: peerPublicKeys),
    );
  }

  /// Updates this authenticated device's runtime debug mode.
  Future<payload.ServerPutRuntimeResponse> putServerRuntime(String debugMode) {
    return rpc.call<payload.ServerPutRuntimeResponse>(
      'server.runtime.put',
      payload.ServerPutRuntimeRequest(debugMode: debugMode),
    );
  }

  Future<payload.WorkflowListResponse> listWorkflows({
    required String collection,
    String? cursor,
    int? limit,
  }) {
    final request = payload.WorkflowListRequest(collection: collection);
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

  Future<payload.WorkflowGetResponse> getWorkflow(String name) {
    return rpc.call<payload.WorkflowGetResponse>(
      'server.workflow.get',
      payload.WorkflowGetRequest(name: name),
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

  /// Pages the opaque app_config keys of the selected RuntimeProfile.
  Future<payload.AppConfigListResponse> listAppConfig({
    String? cursor,
    int? limit,
  }) {
    final request = payload.AppConfigListRequest();
    if (cursor != null) {
      request.cursor = cursor;
    }
    if (limit != null) {
      request.limit = Int64(limit);
    }
    return rpc.call<payload.AppConfigListResponse>(
      'server.app_config.list',
      request,
    );
  }

  /// Reads one opaque app_config value verbatim.
  Future<payload.AppConfigGetResponse> getAppConfig(String key) {
    return rpc.call<payload.AppConfigGetResponse>(
      'server.app_config.get',
      payload.AppConfigGetRequest(key: key),
    );
  }

  Future<payload.WorkspaceListResponse> listWorkspaces({
    required String collection,
    String? cursor,
    int? limit,
    String? prefix,
  }) {
    final request = payload.WorkspaceListRequest(collection: collection);
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

  Future<payload.FriendInfoGetResponse> getFriendInfo(String name) {
    return rpc.call<payload.FriendInfoGetResponse>(
      'server.friend.info.get',
      payload.FriendInfoGetRequest(name: name),
    );
  }

  /// Rings the device of the Friend named [name]. A repeated ping within the
  /// pair's rate-limit window reports `SOCIAL_PING_RESULT_RATE_LIMITED`.
  Future<payload.FriendPingResponse> pingFriend(String name) {
    return rpc.call<payload.FriendPingResponse>(
      'server.friend.ping',
      payload.FriendPingRequest(name: name),
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

  Future<payload.FriendDeleteResponse> deleteFriend(String name) {
    return rpc.call<payload.FriendDeleteResponse>(
      'server.friend.delete',
      payload.FriendDeleteRequest(name: name),
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

  Future<payload.FriendGroupDeleteResponse> deleteFriendGroup(String name) {
    return rpc.call<payload.FriendGroupDeleteResponse>(
      'server.friend_group.delete',
      payload.FriendGroupDeleteRequest(name: name),
    );
  }

  /// Rallies every other online member of the caller's Friend Group named
  /// [name]. A repeated rally within the group's rate-limit window reports
  /// `SOCIAL_PING_RESULT_RATE_LIMITED`.
  Future<payload.FriendGroupPingResponse> pingFriendGroup(String name) {
    return rpc.call<payload.FriendGroupPingResponse>(
      'server.friend_group.ping',
      payload.FriendGroupPingRequest(name: name),
    );
  }

  Future<payload.FriendGroupInviteTokenGetResponse> getFriendGroupInviteToken(
    String friendGroupName,
  ) {
    return rpc.call<payload.FriendGroupInviteTokenGetResponse>(
      'server.friend_group.invite_token.get',
      payload.FriendGroupInviteTokenGetRequest(
        friendGroupName: friendGroupName,
      ),
    );
  }

  Future<payload.FriendGroupInviteTokenCreateResponse>
  createFriendGroupInviteToken(String friendGroupName) {
    return rpc.call<payload.FriendGroupInviteTokenCreateResponse>(
      'server.friend_group.invite_token.create',
      payload.FriendGroupInviteTokenCreateRequest(
        friendGroupName: friendGroupName,
      ),
    );
  }

  Future<payload.FriendGroupInviteTokenClearResponse>
  clearFriendGroupInviteToken(String friendGroupName) {
    return rpc.call<payload.FriendGroupInviteTokenClearResponse>(
      'server.friend_group.invite_token.clear',
      payload.FriendGroupInviteTokenClearRequest(
        friendGroupName: friendGroupName,
      ),
    );
  }

  Future<payload.FriendGroupJoinResponse> joinFriendGroup({
    required String name,
    required String inviteToken,
  }) {
    final normalizedName = name.trim();
    final normalizedInviteToken = inviteToken.trim();
    if (normalizedName.isEmpty) {
      throw ArgumentError.value(name, 'name', 'must not be empty');
    }
    if (normalizedInviteToken.isEmpty) {
      throw ArgumentError.value(
        inviteToken,
        'inviteToken',
        'must not be empty',
      );
    }
    return rpc.call<payload.FriendGroupJoinResponse>(
      'server.friend_group.join',
      payload.FriendGroupJoinRequest(
        name: normalizedName,
        inviteToken: normalizedInviteToken,
      ),
    );
  }

  Future<payload.FriendGroupMemberListResponse> listFriendGroupMembers(
    String friendGroupName, {
    String? cursor,
    int? limit,
  }) {
    final request = payload.FriendGroupMemberListRequest(
      friendGroupName: friendGroupName,
    );
    if (cursor != null) request.cursor = cursor;
    if (limit != null) request.limit = Int64(limit);
    return rpc.call<payload.FriendGroupMemberListResponse>(
      'server.friend_group.members.list',
      request,
    );
  }

  Future<payload.FriendGroupMemberDeleteResponse> deleteFriendGroupMember(
    String friendGroupName,
    String name,
  ) {
    return rpc.call<payload.FriendGroupMemberDeleteResponse>(
      'server.friend_group.members.delete',
      payload.FriendGroupMemberDeleteRequest(
        friendGroupName: friendGroupName,
        name: name,
      ),
    );
  }

  Future<payload.WorkspaceGetResponse> getWorkspace(String name) {
    return rpc.call<payload.WorkspaceGetResponse>(
      'server.workspace.get',
      payload.WorkspaceGetRequest(name: name),
    );
  }

  Future<payload.WorkspaceCreateResponse> createWorkspace(
    payload.WorkspaceCreateBody workspace,
  ) {
    return rpc.call<payload.WorkspaceCreateResponse>(
      'server.workspace.create',
      payload.WorkspaceCreateRequest(value: workspace),
    );
  }

  Future<payload.WorkspacePutResponse> putWorkspace(
    String name,
    payload.WorkspacePutBody workspace,
  ) {
    return rpc.call<payload.WorkspacePutResponse>(
      'server.workspace.put',
      payload.WorkspacePutRequest(name: name, body: workspace),
    );
  }

  /// Updates supported Workspace parameters, ignoring unsupported fields.
  Future<payload.WorkspaceParametersSetResponse> setWorkspaceParameters(
    String name,
    payload.WorkspaceParametersPatch parameters,
  ) {
    return rpc.call<payload.WorkspaceParametersSetResponse>(
      'server.workspace.parameters.set',
      payload.WorkspaceParametersSetRequest(name: name, parameters: parameters),
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

  Future<payload.ServerReloadRunWorkspaceWithOptionsResponse>
  reloadRunWorkspaceWithOptions({
    String? workspaceName,
    payload.WorkspaceParametersPatch? parameters,
  }) {
    return rpc.call<payload.ServerReloadRunWorkspaceWithOptionsResponse>(
      'server.run.workspace.reload-with-options',
      payload.ServerReloadRunWorkspaceWithOptionsRequest(
        workspaceName: workspaceName,
        parameters: parameters,
      ),
    );
  }

  Future<payload.ServerPlayRunWorkspaceHistoryResponse> playRunWorkspaceHistory(
    String historyName,
  ) {
    return rpc.call<payload.ServerPlayRunWorkspaceHistoryResponse>(
      'server.run.workspace.history.play',
      payload.ServerPlayRunWorkspaceHistoryRequest(
        value: payload.PeerRunHistoryPlayRequest(historyName: historyName),
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
