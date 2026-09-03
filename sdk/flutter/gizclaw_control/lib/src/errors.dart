import 'models.dart';

/// Stable classification of a failed `/gizclaw/v1/*` call.
///
/// Device control codes (`DEVICE_*`) are matched on the `error.code` string
/// the Server emits; the remaining kinds are matched on the HTTP status.
enum GizClawControlErrorKind {
  /// `401`: missing, invalid, or revoked API key.
  unauthorized,

  /// `403`: the API key does not authorize this operation.
  forbidden,

  /// `404`: the key, contact, or saved Wi-Fi network does not exist.
  notFound,

  /// `409 DEVICE_OFFLINE`: the device has no active connection or is
  /// rebooting and has not reconnected.
  deviceOffline,

  /// `504 DEVICE_TIMEOUT`: the device did not answer within the Server's
  /// control timeout.
  deviceTimeout,

  /// `400 DEVICE_REJECTED`: the device rejected the command parameters.
  deviceRejected,

  /// `501 DEVICE_UNSUPPORTED`: the device firmware does not implement the
  /// control method.
  deviceUnsupported,

  /// `502 DEVICE_ERROR`: the device answered with an unexpected RPC error.
  deviceError,

  /// `409` with any other code, such as a pending owner deletion or a
  /// duplicate contact.
  conflict,

  /// `400` with any other code: the request itself was rejected.
  invalidRequest,

  /// Any other `5xx`.
  server,

  /// Any other non-2xx status.
  unexpectedStatus,

  /// A 2xx response whose body could not be decoded as the contract type.
  malformedResponse,

  /// The request never produced an HTTP response: DNS, socket, TLS, or
  /// timeout failure.
  network,
}

/// Failure of one `/gizclaw/v1/*` call.
class GizClawControlException implements Exception {
  const GizClawControlException({
    required this.kind,
    required this.message,
    this.statusCode,
    this.code,
    this.requestId,
    this.details = const {},
    this.cause,
  });

  /// Builds the exception for a non-2xx response.
  ///
  /// [error] is the decoded `ErrorResponse` when the body carried one.
  /// [requestId] is the `X-Request-ID` response header when present.
  factory GizClawControlException.fromResponse({
    required int statusCode,
    ErrorPayload? error,
    String? requestId,
  }) {
    final code = error?.code;
    return GizClawControlException(
      kind: classifyGizClawControlError(statusCode, code),
      statusCode: statusCode,
      code: code,
      message: error?.message ?? 'HTTP $statusCode',
      requestId: requestId,
      details: error?.details ?? const {},
    );
  }

  final GizClawControlErrorKind kind;

  /// HTTP status, or `null` for [GizClawControlErrorKind.network].
  final int? statusCode;

  /// `error.code` from the response body when it carried an `ErrorResponse`.
  final String? code;
  final String message;

  /// `X-Request-ID` response header when the Server set one.
  final String? requestId;

  /// `error.details` from the response body.
  final Map<String, Object?> details;

  /// Underlying transport or decode error.
  final Object? cause;

  @override
  String toString() {
    final buffer = StringBuffer('GizClawControlException(${kind.name}');
    if (statusCode != null) {
      buffer.write(', status $statusCode');
    }
    if (code != null) {
      buffer.write(', code $code');
    }
    if (requestId != null) {
      buffer.write(', request $requestId');
    }
    buffer.write('): $message');
    return buffer.toString();
  }
}

const _deviceCodes = {
  'DEVICE_OFFLINE': GizClawControlErrorKind.deviceOffline,
  'DEVICE_TIMEOUT': GizClawControlErrorKind.deviceTimeout,
  'DEVICE_REJECTED': GizClawControlErrorKind.deviceRejected,
  'DEVICE_UNSUPPORTED': GizClawControlErrorKind.deviceUnsupported,
  'DEVICE_ERROR': GizClawControlErrorKind.deviceError,
};

/// Maps a non-2xx status and optional `error.code` to a
/// [GizClawControlErrorKind].
GizClawControlErrorKind classifyGizClawControlError(
  int statusCode,
  String? code,
) {
  final deviceKind = _deviceCodes[code];
  if (deviceKind != null) {
    return deviceKind;
  }
  return switch (statusCode) {
    400 => GizClawControlErrorKind.invalidRequest,
    401 => GizClawControlErrorKind.unauthorized,
    403 => GizClawControlErrorKind.forbidden,
    404 => GizClawControlErrorKind.notFound,
    409 => GizClawControlErrorKind.conflict,
    >= 500 && < 600 => GizClawControlErrorKind.server,
    _ => GizClawControlErrorKind.unexpectedStatus,
  };
}
