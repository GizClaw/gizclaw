import 'dart:async';
import 'dart:convert';

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

  /// `GET /gizclaw/v1/device/telemetry/latest`.
  ///
  /// [fields] are [PeerTelemetryField] names; omitted means every supported
  /// field.
  Future<PeerTelemetryLatestResponse> getDeviceTelemetryLatest({
    List<String>? fields,
  }) {
    return _json(
      'GET',
      '/device/telemetry/latest',
      PeerTelemetryLatestResponse.fromJson,
      query: {
        'fields': fields == null || fields.isEmpty ? null : fields.join(','),
      },
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

  /// `GET /gizclaw/v1/device/wifi`.
  Future<DeviceWifiStatus> getDeviceWifi() {
    return _json(
      'GET',
      '/device/wifi',
      DeviceWifiStatus.fromJson,
      operation: 'getDeviceWifi',
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
    request.headers['Accept'] = 'application/json';
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
