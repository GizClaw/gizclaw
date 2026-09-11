import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'errors.dart';
import 'json.dart';
import 'models.dart';

/// Controller client for the `/gizclaw/v1/*` API-key HTTP surface.
///
/// Every method sends `Authorization: Bearer <apiKey>` to
/// `<baseUrl>/gizclaw/v1/...`, decodes the contract response type, and throws
/// [GizClawControlException] for every failure: non-2xx status, undecodable
/// body, or transport error. Methods never return `null` to signal failure.
///
/// The client is safe to share across isolates' event loops but not across
/// isolates. Call [close] when the client is no longer needed; it closes the
/// underlying `http.Client` only when the client created it.
class GizClawControlClient {
  /// Creates a client for the GizClaw server at [baseUrl].
  ///
  /// [baseUrl] is the server origin, optionally with a path prefix, such as
  /// `https://ap.gizclaw.com`. [apiKey] is the complete `gizclaw_sk_v1_...`
  /// credential. [httpClient] lets callers inject a transport; the SDK creates
  /// and owns one otherwise. [timeout] bounds every request from send to the
  /// end of the response body.
  ///
  /// Every request carries the API key, so [baseUrl] must be `https`. Set
  /// [allowInsecureTransport] to reach a plaintext `http` server, which sends
  /// the credential in the clear and is only appropriate for a local test
  /// deployment.
  GizClawControlClient({
    required Uri baseUrl,
    required String apiKey,
    http.Client? httpClient,
    Duration timeout = const Duration(seconds: 30),
    bool allowInsecureTransport = false,
  }) : _baseUrl = baseUrl,
       _apiKey = apiKey,
       _http = httpClient ?? http.Client(),
       _ownsHttp = httpClient == null,
       _timeout = timeout {
    if (apiKey.isEmpty) {
      throw ArgumentError.value(apiKey, 'apiKey', 'must not be empty');
    }
    if (baseUrl.host.isEmpty ||
        (baseUrl.scheme != 'https' && baseUrl.scheme != 'http')) {
      throw ArgumentError.value(
        baseUrl,
        'baseUrl',
        'must be an absolute http(s) URL',
      );
    }
    if (baseUrl.scheme != 'https' && !allowInsecureTransport) {
      throw ArgumentError.value(
        baseUrl,
        'baseUrl',
        'must use https; set allowInsecureTransport to send the API key '
            'over plaintext',
      );
    }
  }

  static const _prefix = '/gizclaw/v1';

  final Uri _baseUrl;
  final String _apiKey;
  final http.Client _http;
  final bool _ownsHttp;
  final Duration _timeout;
  bool _closed = false;

  /// Server origin this client targets.
  Uri get baseUrl => _baseUrl;

  /// Releases the owned transport. Requests after [close] fail with
  /// [GizClawControlErrorKind.network].
  void close() {
    if (_closed) {
      return;
    }
    _closed = true;
    if (_ownsHttp) {
      _http.close();
    }
  }

  /// `GET /gizclaw/v1/device/audioplayer`.
  Future<AudioPlayerResponse> getAudioPlayer() => _json(
    'GET',
    '/device/audioplayer',
    AudioPlayerResponse.fromJson,
    operation: 'getAudioPlayer',
  );

  /// `GET /gizclaw/v1/device/audioplayer/playlist`.
  Future<AudioPlayerPlaylist> getAudioPlayerPlaylist() => _json(
    'GET',
    '/device/audioplayer/playlist',
    AudioPlayerPlaylist.fromJson,
    operation: 'getAudioPlayerPlaylist',
  );

  /// `PUT /gizclaw/v1/device/audioplayer/playlist`.
  Future<AudioPlayerResponse> setAudioPlayerPlaylist(
    List<AudioPlayerItem> items,
  ) => _json(
    'PUT',
    '/device/audioplayer/playlist',
    AudioPlayerResponse.fromJson,
    operation: 'setAudioPlayerPlaylist',
    body: {'items': items.map((item) => item.toJson()).toList()},
  );

  /// `POST /gizclaw/v1/device/audioplayer/playlist/append`.
  Future<AudioPlayerResponse> appendAudioPlayerPlaylist(
    List<AudioPlayerItem> items,
  ) => _json(
    'POST',
    '/device/audioplayer/playlist/append',
    AudioPlayerResponse.fromJson,
    operation: 'appendAudioPlayerPlaylist',
    body: {'items': items.map((item) => item.toJson()).toList()},
  );

  /// `POST /gizclaw/v1/device/audioplayer/actions/play`.
  Future<AudioPlayerResponse> playAudioPlayer(int index) => _json(
    'POST',
    '/device/audioplayer/actions/play',
    AudioPlayerResponse.fromJson,
    operation: 'playAudioPlayer',
    body: {'index': index},
  );

  /// `POST /gizclaw/v1/device/audioplayer/actions/stop`.
  Future<AudioPlayerResponse> stopAudioPlayer() => _json(
    'POST',
    '/device/audioplayer/actions/stop',
    AudioPlayerResponse.fromJson,
    operation: 'stopAudioPlayer',
  );

  /// `PUT /gizclaw/v1/device/audioplayer/mode`.
  Future<AudioPlayerResponse> setAudioPlayerMode(String repeat) => _json(
    'PUT',
    '/device/audioplayer/mode',
    AudioPlayerResponse.fromJson,
    operation: 'setAudioPlayerMode',
    body: {'repeat': repeat},
  );

  // API keys.

  /// `POST /gizclaw/v1/api-keys`.
  Future<ApiKeyCreateResult> createApiKey(ApiKeyCreateRequest request) {
    return _json(
      'POST',
      '/api-keys',
      ApiKeyCreateResult.fromJson,
      body: request.toJson(),
      operation: 'createApiKey',
    );
  }

  /// `GET /gizclaw/v1/api-keys`.
  Future<ApiKeyList> listApiKeys({String? cursor, int? limit}) {
    return _json(
      'GET',
      '/api-keys',
      ApiKeyList.fromJson,
      query: {'cursor': cursor, 'limit': limit?.toString()},
      operation: 'listApiKeys',
    );
  }

  /// `GET /gizclaw/v1/api-keys/self`.
  Future<ApiKey> getSelfApiKey() {
    return _json(
      'GET',
      '/api-keys/self',
      ApiKey.fromJson,
      operation: 'getSelfApiKey',
    );
  }

  /// `DELETE /gizclaw/v1/api-keys/self`.
  Future<void> revokeSelfApiKey() {
    return _noContent(
      'DELETE',
      '/api-keys/self',
      operation: 'revokeSelfApiKey',
    );
  }

  /// `GET /gizclaw/v1/api-keys/{apiKeyName}`.
  Future<ApiKey> getApiKey(String apiKeyName) {
    return _json(
      'GET',
      '/api-keys/${_segment(apiKeyName, 'apiKeyName')}',
      ApiKey.fromJson,
      operation: 'getApiKey',
    );
  }

  /// `DELETE /gizclaw/v1/api-keys/{apiKeyName}`.
  Future<void> revokeApiKey(String apiKeyName) {
    return _noContent(
      'DELETE',
      '/api-keys/${_segment(apiKeyName, 'apiKeyName')}',
      operation: 'revokeApiKey',
    );
  }

  // Device reads.

  /// `GET /gizclaw/v1/device`.
  Future<DeviceInfo> getDevice() {
    return _json('GET', '/device', DeviceInfo.fromJson, operation: 'getDevice');
  }

  /// `GET /gizclaw/v1/device/runtime`.
  Future<DeviceRuntime> getDeviceRuntime() {
    return _json(
      'GET',
      '/device/runtime',
      DeviceRuntime.fromJson,
      operation: 'getDeviceRuntime',
    );
  }

  /// `GET /gizclaw/v1/device/status`.
  ///
  /// Returns the stored `PeerStatus` snapshot without contacting the device.
  Future<PeerStatus> getDeviceStatus() {
    return _json(
      'GET',
      '/device/status',
      PeerStatus.fromJson,
      operation: 'getDeviceStatus',
    );
  }

  /// `GET /gizclaw/v1/device/telemetry/{field}/latest`.
  Future<PeerTelemetryLatestResponse> getDeviceTelemetryLatest({
    required String field,
  }) {
    if (field.isEmpty) {
      throw ArgumentError.value(field, 'field', 'must not be empty');
    }
    return _json(
      'GET',
      '/device/telemetry/${Uri.encodeComponent(field)}/latest',
      PeerTelemetryLatestResponse.fromJson,
      operation: 'getDeviceTelemetryLatest',
    );
  }

  /// `GET /gizclaw/v1/device/telemetry`.
  Future<PeerTelemetryRangeResponse> queryDeviceTelemetry({
    required String field,
    required int startTimeMs,
    required int endTimeMs,
    int? stepMs,
    int? limit,
    PeerTelemetryOrder? order,
  }) {
    return _json(
      'GET',
      '/device/telemetry',
      PeerTelemetryRangeResponse.fromJson,
      query: {
        'field': field,
        'start_time_ms': '$startTimeMs',
        'end_time_ms': '$endTimeMs',
        'step_ms': stepMs?.toString(),
        'limit': limit?.toString(),
        'order': order?.wireValue,
      },
      operation: 'queryDeviceTelemetry',
    );
  }

  /// `GET /gizclaw/v1/device/telemetry/aggregate`.
  Future<PeerTelemetryAggregateResponse> aggregateDeviceTelemetry({
    required String field,
    required int startTimeMs,
    required int endTimeMs,
    required int bucketMs,
    required PeerTelemetryAggregate aggregate,
  }) {
    return _json(
      'GET',
      '/device/telemetry/aggregate',
      PeerTelemetryAggregateResponse.fromJson,
      query: {
        'field': field,
        'start_time_ms': '$startTimeMs',
        'end_time_ms': '$endTimeMs',
        'bucket_ms': '$bucketMs',
        'aggregate': aggregate.wireValue,
      },
      operation: 'aggregateDeviceTelemetry',
    );
  }

  /// `GET /gizclaw/v1/device/firmware`.
  ///
  /// Returns every firmware channel configured for the bound device. The read
  /// never contacts the device, so it works while the device is offline.
  /// Throws [GizClawControlErrorKind.notFound] when the device has no Firmware
  /// configuration bound.
  Future<DeviceFirmware> getDeviceFirmware() {
    return _json(
      'GET',
      '/device/firmware',
      DeviceFirmware.fromJson,
      operation: 'getDeviceFirmware',
    );
  }

  /// `GET /gizclaw/v1/device/runtime-profile`.
  ///
  /// Returns the RuntimeProfile bound to the device with its workflow
  /// collections and workflow names, sorted by name. The read never contacts
  /// the device, so it works while the device is offline.
  Future<DeviceRuntimeProfile> getDeviceRuntimeProfile() {
    return _json(
      'GET',
      '/device/runtime-profile',
      DeviceRuntimeProfile.fromJson,
      operation: 'getDeviceRuntimeProfile',
    );
  }

  /// `GET /gizclaw/v1/device/workspaces`.
  ///
  /// Returns the Workspaces the device owns, including system Workspaces but
  /// not those whose deletion is pending. [collection] and [workflowName]
  /// filter exactly by the names [getDeviceRuntimeProfile] lists; a
  /// [workflowName] filter never matches a Workspace whose Workflow no longer
  /// resolves. Reading never contacts the device.
  Future<List<DeviceWorkspace>> listDeviceWorkspaces({
    String? collection,
    String? workflowName,
  }) {
    if (collection != null && collection.isEmpty) {
      throw ArgumentError.value(collection, 'collection', 'must not be empty');
    }
    if (workflowName != null && workflowName.isEmpty) {
      throw ArgumentError.value(
        workflowName,
        'workflowName',
        'must not be empty',
      );
    }
    return _json(
      'GET',
      '/device/workspaces',
      (json) => asJsonList(
        json,
        'DeviceWorkspace list',
      ).map(DeviceWorkspace.fromJson).toList(growable: false),
      query: {'collection': collection, 'workflow_name': workflowName},
      operation: 'listDeviceWorkspaces',
    );
  }

  /// `DELETE /gizclaw/v1/device/workspaces/{workspaceId}`.
  ///
  /// Starts the asynchronous deletion of an owned Workspace and returns once
  /// the Server accepted it. The Workspace leaves [listDeviceWorkspaces] at
  /// once; its history and state are removed in the background. A system
  /// Workspace fails with [GizClawControlErrorKind.conflict], and a foreign,
  /// absent, or already deleted Workspace with
  /// [GizClawControlErrorKind.notFound].
  Future<void> deleteDeviceWorkspace(String workspaceId) {
    return _noContent(
      'DELETE',
      '/device/workspaces/${_segment(workspaceId, 'workspaceId')}',
      operation: 'deleteDeviceWorkspace',
    );
  }

  /// `GET /gizclaw/v1/device/workspaces/{workspaceId}/history`.
  ///
  /// Reads persisted chat of an owned Workspace, newest first unless [order]
  /// is [WorkspaceHistoryOrder.asc]. Pass the previous page's
  /// [WorkspaceHistoryPage.nextCursor], or any entry name, as [cursor].
  /// [limit] is 1..200 (default 50). [query] matches literal text.
  /// [startTimeMs] (inclusive) and [endTimeMs] (exclusive) bound the entry
  /// creation time in Unix milliseconds.
  Future<WorkspaceHistoryPage> listDeviceWorkspaceHistory(
    String workspaceId, {
    String? cursor,
    int? limit,
    String? query,
    WorkspaceHistoryOrder? order,
    int? startTimeMs,
    int? endTimeMs,
  }) {
    return _json(
      'GET',
      '/device/workspaces/${_segment(workspaceId, 'workspaceId')}/history',
      WorkspaceHistoryPage.fromJson,
      query: {
        'cursor': cursor,
        'limit': limit?.toString(),
        'query': query,
        'order': order?.wireValue,
        'start_time_ms': startTimeMs?.toString(),
        'end_time_ms': endTimeMs?.toString(),
      },
      operation: 'listDeviceWorkspaceHistory',
    );
  }

  /// `GET /gizclaw/v1/device/workspaces/{workspaceId}/history/{historyId}/audio.ogg`.
  ///
  /// Downloads the stored Ogg audio of one history entry. An entry without
  /// stored audio fails with [GizClawControlErrorKind.notFound].
  Future<Uint8List> downloadDeviceHistoryAudio(
    String workspaceId,
    String historyId,
  ) async {
    final response = await _send(
      'GET',
      _historyAudioRoute(workspaceId, historyId),
      accept: 'audio/ogg',
      operation: 'downloadDeviceHistoryAudio',
    );
    return response.bodyBytes;
  }

  /// URL of [downloadDeviceHistoryAudio] for players that stream it
  /// themselves. The URL is not a credential: every request to it must carry
  /// [authorizationHeaders].
  Uri deviceHistoryAudioUri(String workspaceId, String historyId) {
    return _uri(_historyAudioRoute(workspaceId, historyId), null);
  }

  /// Headers that authorize a request to a URL such as
  /// [deviceHistoryAudioUri]. They carry the API key.
  Map<String, String> get authorizationHeaders => {
    'Authorization': 'Bearer $_apiKey',
  };

  static String _historyAudioRoute(String workspaceId, String historyId) {
    return '/device/workspaces/${_segment(workspaceId, 'workspaceId')}'
        '/history/${_segment(historyId, 'historyId')}/audio.ogg';
  }

  // Device control.

  /// `PUT /gizclaw/v1/device/volume`.
  ///
  /// Returns the `PeerStatus` the device reported after applying the volume.
  Future<DeviceControlStatus> setDeviceVolume({
    required int level,
    required bool muted,
  }) {
    return _json(
      'PUT',
      '/device/volume',
      DeviceControlStatus.fromJson,
      body: DeviceVolumeSetRequest(level: level, muted: muted).toJson(),
      operation: 'setDeviceVolume',
    );
  }

  /// `POST /gizclaw/v1/device/actions/play-sound`.
  Future<void> playDeviceSound({required String sound, int? durationMs}) {
    return _noContent(
      'POST',
      '/device/actions/play-sound',
      body: DevicePlaySoundRequest(
        sound: sound,
        durationMs: durationMs,
      ).toJson(),
      operation: 'playDeviceSound',
    );
  }

  /// `POST /gizclaw/v1/device/actions/find`.
  ///
  /// Rings the device's built-in find-me sound with a rising volume ramp.
  /// [durationMs] is the requested ring time; the device picks its own default
  /// when it is null. A device without a find provider answers
  /// [GizClawControlErrorKind.deviceUnsupported].
  Future<void> findDevice({int? durationMs}) {
    return _noContent(
      'POST',
      '/device/actions/find',
      body: DeviceFindRequest(durationMs: durationMs).toJson(),
      operation: 'findDevice',
    );
  }

  /// `POST /gizclaw/v1/device/actions/reboot`.
  ///
  /// The device acknowledges before rebooting; later control calls fail with
  /// [GizClawControlErrorKind.deviceOffline] until it reconnects.
  Future<void> rebootDevice({int? delayMs}) {
    return _noContent(
      'POST',
      '/device/actions/reboot',
      body: DeviceRebootRequest(delayMs: delayMs).toJson(),
      operation: 'rebootDevice',
    );
  }

  /// `POST /gizclaw/v1/device/actions/firmware-update`.
  ///
  /// Notifies the device to run one OTA. [channel] names a channel from
  /// [getDeviceFirmware] and defaults to the channel the device already uses;
  /// [sha256] declares the package the caller saw, and the device rejects the
  /// call with [GizClawControlErrorKind.deviceRejected] when it resolves a
  /// different package.
  ///
  /// The device acknowledges before it downloads and writes the package, so
  /// later control calls fail with [GizClawControlErrorKind.deviceOffline]
  /// until it reconnects. Firmware that predates this method answers
  /// [GizClawControlErrorKind.deviceUnsupported]; treat that as "no OTA entry
  /// point" rather than as a failed update.
  Future<void> updateDeviceFirmware({
    FirmwareChannelName? channel,
    String? sha256,
  }) {
    return _noContent(
      'POST',
      '/device/actions/firmware-update',
      body: DeviceFirmwareUpdateRequest(
        channel: channel,
        sha256: sha256,
      ).toJson(),
      operation: 'updateDeviceFirmware',
    );
  }

  /// `GET /gizclaw/v1/device/wifi`.
  Future<DeviceWifiStatus> getDeviceWifi() {
    return _json(
      'GET',
      '/device/wifi',
      DeviceWifiStatus.fromJson,
      operation: 'getDeviceWifi',
    );
  }

  /// `POST /gizclaw/v1/device/wifi/scan`.
  Future<DeviceWifiScanResponse> scanDeviceWifi([
    DeviceWifiScanRequest request = const DeviceWifiScanRequest(),
  ]) {
    return _json(
      'POST',
      '/device/wifi/scan',
      DeviceWifiScanResponse.fromJson,
      body: request.toJson(),
      operation: 'scanDeviceWifi',
    );
  }

  /// `PUT /gizclaw/v1/device/wifi`.
  ///
  /// A successful return means the device accepted the credentials and began
  /// switching networks. Poll [getDeviceWifi] after the device reconnects to
  /// observe whether it joined [request.ssid].
  Future<void> connectDeviceWifi(DeviceWifiConnectRequest request) {
    return _noContent(
      'PUT',
      '/device/wifi',
      body: request.toJson(),
      operation: 'connectDeviceWifi',
    );
  }

  /// `GET /gizclaw/v1/device/wifi/saved`.
  Future<DeviceWifiSavedList> listDeviceSavedWifi() {
    return _json(
      'GET',
      '/device/wifi/saved',
      DeviceWifiSavedList.fromJson,
      operation: 'listDeviceSavedWifi',
    );
  }

  /// `DELETE /gizclaw/v1/device/wifi/saved/{ssid}`.
  Future<void> forgetDeviceSavedWifi(String ssid) {
    return _noContent(
      'DELETE',
      '/device/wifi/saved/${_segment(ssid, 'ssid')}',
      operation: 'forgetDeviceSavedWifi',
    );
  }

  // Contacts.

  /// `GET /gizclaw/v1/contacts`.
  Future<ContactList> listContacts({String? cursor, int? limit}) {
    return _json(
      'GET',
      '/contacts',
      ContactList.fromJson,
      query: {'cursor': cursor, 'limit': limit?.toString()},
      operation: 'listContacts',
    );
  }

  /// `POST /gizclaw/v1/contacts`.
  Future<Contact> createContact(ContactCreateRequest request) {
    return _json(
      'POST',
      '/contacts',
      Contact.fromJson,
      body: request.toJson(),
      operation: 'createContact',
    );
  }

  /// `GET /gizclaw/v1/contacts/{contactName}`.
  Future<Contact> getContact(String contactName) {
    return _json(
      'GET',
      '/contacts/${_segment(contactName, 'contactName')}',
      Contact.fromJson,
      operation: 'getContact',
    );
  }

  /// `PUT /gizclaw/v1/contacts/{contactName}`.
  Future<Contact> putContact(String contactName, ContactPutRequest request) {
    return _json(
      'PUT',
      '/contacts/${_segment(contactName, 'contactName')}',
      Contact.fromJson,
      body: request.toJson(),
      operation: 'putContact',
    );
  }

  /// `DELETE /gizclaw/v1/contacts/{contactName}`.
  Future<void> deleteContact(String contactName) {
    return _noContent(
      'DELETE',
      '/contacts/${_segment(contactName, 'contactName')}',
      operation: 'deleteContact',
    );
  }

  // Friends. None of these calls contacts the device.

  /// `GET /gizclaw/v1/friends/invite-token`.
  ///
  /// Throws [GizClawControlErrorKind.notFound] with code
  /// `INVITE_TOKEN_NOT_FOUND` when the device has no active invite token.
  Future<InviteToken> getFriendInviteToken() {
    return _json(
      'GET',
      '/friends/invite-token',
      InviteToken.fromJson,
      operation: 'getFriendInviteToken',
    );
  }

  /// `POST /gizclaw/v1/friends/invite-token`.
  ///
  /// Without [ttl] an active token is returned unchanged and a new one lives
  /// five minutes. With [ttl] (1 minute to 7 days, whole seconds) a new token
  /// lives that long and an active token keeps its value while its expiry is
  /// extended to now + [ttl]; it is never shortened.
  Future<InviteToken> createFriendInviteToken({Duration? ttl}) {
    return _json(
      'POST',
      '/friends/invite-token',
      InviteToken.fromJson,
      body: _inviteTokenBody(ttl),
      operation: 'createFriendInviteToken',
    );
  }

  /// `DELETE /gizclaw/v1/friends/invite-token`.
  Future<void> clearFriendInviteToken() {
    return _noContent(
      'DELETE',
      '/friends/invite-token',
      operation: 'clearFriendInviteToken',
    );
  }

  /// `POST /gizclaw/v1/friends`: befriend the Peer owning [inviteToken].
  ///
  /// Throws [GizClawControlErrorKind.conflict] with code
  /// `FRIEND_ALREADY_EXISTS` when the devices are already Friends.
  Future<Friend> addFriend(String inviteToken) {
    return _json(
      'POST',
      '/friends',
      Friend.fromJson,
      body: {'invite_token': inviteToken},
      operation: 'addFriend',
    );
  }

  /// `GET /gizclaw/v1/friends`.
  Future<FriendList> listFriends({String? cursor, int? limit}) {
    return _json(
      'GET',
      '/friends',
      FriendList.fromJson,
      query: {'cursor': cursor, 'limit': limit?.toString()},
      operation: 'listFriends',
    );
  }

  /// `GET /gizclaw/v1/friends/{friendName}`.
  Future<Friend> getFriend(String friendName) {
    return _json(
      'GET',
      '/friends/${_segment(friendName, 'friendName')}',
      Friend.fromJson,
      operation: 'getFriend',
    );
  }

  /// `DELETE /gizclaw/v1/friends/{friendName}`: end the relationship and
  /// retire the shared Workspace.
  Future<void> deleteFriend(String friendName) {
    return _noContent(
      'DELETE',
      '/friends/${_segment(friendName, 'friendName')}',
      operation: 'deleteFriend',
    );
  }

  // Friend Groups. Groups are named by the bound device's own Group name.

  /// `GET /gizclaw/v1/friend-groups`.
  Future<FriendGroupList> listFriendGroups({String? cursor, int? limit}) {
    return _json(
      'GET',
      '/friend-groups',
      FriendGroupList.fromJson,
      query: {'cursor': cursor, 'limit': limit?.toString()},
      operation: 'listFriendGroups',
    );
  }

  /// `POST /gizclaw/v1/friend-groups`: the device becomes the owner.
  Future<FriendGroup> createFriendGroup({
    required String name,
    String? displayName,
    String? description,
  }) {
    return _json(
      'POST',
      '/friend-groups',
      FriendGroup.fromJson,
      body: withoutNulls({
        'name': name,
        'display_name': displayName,
        'description': description,
      }),
      operation: 'createFriendGroup',
    );
  }

  /// `POST /gizclaw/v1/friend-groups/@join`; [name] becomes the device's own
  /// name for the Group.
  Future<FriendGroupJoinResult> joinFriendGroup({
    required String inviteToken,
    required String name,
  }) {
    return _json(
      'POST',
      '/friend-groups/@join',
      FriendGroupJoinResult.fromJson,
      body: {'invite_token': inviteToken, 'name': name},
      operation: 'joinFriendGroup',
    );
  }

  /// `GET /gizclaw/v1/friend-groups/{friendGroupName}`.
  Future<FriendGroup> getFriendGroup(String friendGroupName) {
    return _json(
      'GET',
      _groupRoute(friendGroupName),
      FriendGroup.fromJson,
      operation: 'getFriendGroup',
    );
  }

  /// `PUT /gizclaw/v1/friend-groups/{friendGroupName}`; owner only.
  Future<FriendGroup> putFriendGroup(
    String friendGroupName, {
    String? displayName,
    String? description,
  }) {
    return _json(
      'PUT',
      _groupRoute(friendGroupName),
      FriendGroup.fromJson,
      body: withoutNulls({
        'display_name': displayName,
        'description': description,
      }),
      operation: 'putFriendGroup',
    );
  }

  /// `DELETE /gizclaw/v1/friend-groups/{friendGroupName}`: dissolve the
  /// Group; owner only.
  Future<void> deleteFriendGroup(String friendGroupName) {
    return _noContent(
      'DELETE',
      _groupRoute(friendGroupName),
      operation: 'deleteFriendGroup',
    );
  }

  /// `POST /gizclaw/v1/friend-groups/{friendGroupName}/@leave`.
  ///
  /// Members and admins leave; the owner receives
  /// [GizClawControlErrorKind.conflict] with code
  /// `FRIEND_GROUP_OWNER_CANNOT_LEAVE` and dissolves the Group instead.
  Future<void> leaveFriendGroup(String friendGroupName) {
    return _noContent(
      'POST',
      '${_groupRoute(friendGroupName)}/@leave',
      operation: 'leaveFriendGroup',
    );
  }

  /// `GET /gizclaw/v1/friend-groups/{friendGroupName}/invite-token`; owner
  /// only.
  Future<InviteToken> getFriendGroupInviteToken(String friendGroupName) {
    return _json(
      'GET',
      '${_groupRoute(friendGroupName)}/invite-token',
      InviteToken.fromJson,
      operation: 'getFriendGroupInviteToken',
    );
  }

  /// `POST /gizclaw/v1/friend-groups/{friendGroupName}/invite-token`; owner
  /// only. [ttl] follows [createFriendInviteToken].
  Future<InviteToken> createFriendGroupInviteToken(
    String friendGroupName, {
    Duration? ttl,
  }) {
    return _json(
      'POST',
      '${_groupRoute(friendGroupName)}/invite-token',
      InviteToken.fromJson,
      body: _inviteTokenBody(ttl),
      operation: 'createFriendGroupInviteToken',
    );
  }

  /// `DELETE /gizclaw/v1/friend-groups/{friendGroupName}/invite-token`; owner
  /// only.
  Future<void> clearFriendGroupInviteToken(String friendGroupName) {
    return _noContent(
      'DELETE',
      '${_groupRoute(friendGroupName)}/invite-token',
      operation: 'clearFriendGroupInviteToken',
    );
  }

  /// `GET /gizclaw/v1/friend-groups/{friendGroupName}/members`.
  Future<FriendGroupMemberList> listFriendGroupMembers(
    String friendGroupName, {
    String? cursor,
    int? limit,
  }) {
    return _json(
      'GET',
      '${_groupRoute(friendGroupName)}/members',
      FriendGroupMemberList.fromJson,
      query: {'cursor': cursor, 'limit': limit?.toString()},
      operation: 'listFriendGroupMembers',
    );
  }

  /// `POST /gizclaw/v1/friend-groups/{friendGroupName}/members`.
  ///
  /// [memberName] is the added Peer's own name for the Group. [role] must be
  /// [FriendGroupRole.admin] or [FriendGroupRole.member].
  Future<FriendGroupMember> addFriendGroupMember(
    String friendGroupName, {
    required String peerPublicKey,
    required String memberName,
    FriendGroupRole role = FriendGroupRole.member,
  }) {
    return _json(
      'POST',
      '${_groupRoute(friendGroupName)}/members',
      FriendGroupMember.fromJson,
      body: {
        'peer_public_key': peerPublicKey,
        'member_name': memberName,
        'role': _mutableRole(role),
      },
      operation: 'addFriendGroupMember',
    );
  }

  /// `PUT /gizclaw/v1/friend-groups/{friendGroupName}/members/{memberName}`;
  /// owner only. [role] must be [FriendGroupRole.admin] or
  /// [FriendGroupRole.member].
  Future<FriendGroupMember> putFriendGroupMember(
    String friendGroupName,
    String memberName,
    FriendGroupRole role,
  ) {
    return _json(
      'PUT',
      _memberRoute(friendGroupName, memberName),
      FriendGroupMember.fromJson,
      body: {'role': _mutableRole(role)},
      operation: 'putFriendGroupMember',
    );
  }

  /// `DELETE /gizclaw/v1/friend-groups/{friendGroupName}/members/{memberName}`.
  Future<void> deleteFriendGroupMember(
    String friendGroupName,
    String memberName,
  ) {
    return _noContent(
      'DELETE',
      _memberRoute(friendGroupName, memberName),
      operation: 'deleteFriendGroupMember',
    );
  }

  static String _groupRoute(String friendGroupName) =>
      '/friend-groups/${_segment(friendGroupName, 'friendGroupName')}';

  static String _memberRoute(String friendGroupName, String memberName) =>
      '${_groupRoute(friendGroupName)}/members/'
      '${_segment(memberName, 'memberName')}';

  static String _mutableRole(FriendGroupRole role) {
    if (role == FriendGroupRole.owner) {
      throw ArgumentError.value(role, 'role', 'must be admin or member');
    }
    return role.wireValue;
  }

  static JsonObject _inviteTokenBody(Duration? ttl) {
    if (ttl == null) {
      return const {};
    }
    if (ttl.inMicroseconds % Duration.microsecondsPerSecond != 0) {
      throw ArgumentError.value(ttl, 'ttl', 'must be whole seconds');
    }
    return {'ttl_seconds': ttl.inSeconds};
  }

  /// Sends one request to `<baseUrl><path>` with the bearer header, for a
  /// route this package does not model yet.
  ///
  /// [path] is absolute and may carry a query string. Unlike the typed
  /// methods, a non-2xx status is returned rather than thrown; classify it
  /// with [classifyGizClawControlError]. Transport failures still throw
  /// [GizClawControlException] with [GizClawControlErrorKind.network].
  Future<GizClawControlResponse> send({
    required String method,
    required String path,
    Map<String, String> headers = const {},
    Object? body,
  }) async {
    if (!path.startsWith('/')) {
      throw ArgumentError.value(path, 'path', 'must be an absolute path');
    }
    if (_closed) {
      throw const GizClawControlException(
        kind: GizClawControlErrorKind.network,
        message: 'send: client is closed',
      );
    }
    final basePath = _baseUrl.path.replaceFirst(RegExp(r'/+$'), '');
    final separator = path.indexOf('?');
    final request = http.Request(
      method,
      _baseUrl.replace(
        path: '$basePath${separator < 0 ? path : path.substring(0, separator)}',
        query: separator < 0 ? null : path.substring(separator + 1),
      ),
    );
    request.headers['Authorization'] = 'Bearer $_apiKey';
    request.headers['Accept'] = 'application/json';
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    request.headers.addAll(headers);
    final http.Response response;
    try {
      response = await _http
          .send(request)
          .then(http.Response.fromStream)
          .timeout(_timeout);
    } on TimeoutException catch (error) {
      throw GizClawControlException(
        kind: GizClawControlErrorKind.network,
        message: 'send: no response within $_timeout',
        cause: error,
      );
    } on http.ClientException catch (error) {
      throw GizClawControlException(
        kind: GizClawControlErrorKind.network,
        message: 'send: ${error.message}',
        cause: error,
      );
    }
    Object? decoded;
    final text = utf8.decode(response.bodyBytes);
    if (text.trim().isNotEmpty) {
      try {
        decoded = jsonDecode(text);
      } on FormatException {
        decoded = text;
      }
    }
    return GizClawControlResponse(
      statusCode: response.statusCode,
      json: decoded,
      requestId: _requestId(response),
    );
  }

  // Transport.

  static String _segment(String value, String name) {
    if (value.isEmpty) {
      throw ArgumentError.value(value, name, 'must not be empty');
    }
    return Uri.encodeComponent(value);
  }

  Uri _uri(String route, Map<String, String?>? query) {
    final basePath = _baseUrl.path.replaceFirst(RegExp(r'/+$'), '');
    final parameters = <String, String>{
      if (query != null)
        for (final entry in query.entries)
          if (entry.value != null) entry.key: entry.value!,
    };
    return Uri(
      scheme: _baseUrl.scheme,
      userInfo: _baseUrl.userInfo,
      host: _baseUrl.host,
      port: _baseUrl.hasPort ? _baseUrl.port : null,
      path: '$basePath$_prefix$route',
      queryParameters: parameters.isEmpty ? null : parameters,
    );
  }

  Future<http.Response> _send(
    String method,
    String route, {
    Map<String, String?>? query,
    JsonObject? body,
    String accept = 'application/json',
    required String operation,
  }) async {
    if (_closed) {
      throw GizClawControlException(
        kind: GizClawControlErrorKind.network,
        message: '$operation: client is closed',
      );
    }
    final request = http.Request(method, _uri(route, query));
    request.headers['Authorization'] = 'Bearer $_apiKey';
    request.headers['Accept'] = accept;
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    final http.Response response;
    try {
      response = await _http
          .send(request)
          .then(http.Response.fromStream)
          .timeout(_timeout);
    } on TimeoutException catch (error) {
      throw GizClawControlException(
        kind: GizClawControlErrorKind.network,
        message: '$operation: no response within $_timeout',
        cause: error,
      );
    } on http.ClientException catch (error) {
      throw GizClawControlException(
        kind: GizClawControlErrorKind.network,
        message: '$operation: ${error.message}',
        cause: error,
      );
    }
    if (response.statusCode >= 200 && response.statusCode < 300) {
      return response;
    }
    throw _failure(response);
  }

  Future<T> _json<T>(
    String method,
    String route,
    T Function(Object? json) decode, {
    Map<String, String?>? query,
    JsonObject? body,
    required String operation,
  }) async {
    final response = await _send(
      method,
      route,
      query: query,
      body: body,
      operation: operation,
    );
    try {
      return decode(jsonDecode(utf8.decode(response.bodyBytes)));
    } on FormatException catch (error) {
      throw GizClawControlException(
        kind: GizClawControlErrorKind.malformedResponse,
        statusCode: response.statusCode,
        message: '$operation: ${error.message}',
        requestId: _requestId(response),
        cause: error,
      );
    }
  }

  Future<void> _noContent(
    String method,
    String route, {
    JsonObject? body,
    required String operation,
  }) async {
    await _send(method, route, body: body, operation: operation);
  }

  GizClawControlException _failure(http.Response response) {
    ErrorPayload? error;
    try {
      final decoded = jsonDecode(utf8.decode(response.bodyBytes));
      error = ErrorResponse.fromJson(decoded).error;
    } on FormatException {
      error = null;
    }
    return GizClawControlException.fromResponse(
      statusCode: response.statusCode,
      error: error,
      requestId: _requestId(response),
    );
  }

  static String? _requestId(http.Response response) {
    final value = response.headers['x-request-id'];
    return value == null || value.isEmpty ? null : value;
  }
}

/// One raw response from [GizClawControlClient.send].
class GizClawControlResponse {
  const GizClawControlResponse({
    required this.statusCode,
    required this.json,
    this.requestId,
  });

  final int statusCode;

  /// Decoded JSON body, the raw text when the body is not JSON, or `null`
  /// when the response carried no body.
  final Object? json;

  /// `X-Request-ID` response header when the Server set one.
  final String? requestId;

  bool get isSuccess => statusCode >= 200 && statusCode < 300;
}
