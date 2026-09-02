/*
 * Error classification for the controller-side C SDK.
 *
 * The mapping is the shared controller contract. It matches
 * `classifyGizClawControlError` in `sdk/flutter/gizclaw_control/lib/src/errors.dart`
 * and `classifyGizClawControlError` in `sdk/js/gizclaw-control/index.ts`; the
 * authoritative `DEVICE_*` code strings come from
 * `pkgs/gizclaw/peer_service_serve_peer_http_device_control.go`.
 */
#include "gzc_control_internal.h"

gzc_control_error_kind_t gzc_control_classify(int status_code, gzc_str_t code) {
  if (gzc_control_str_eq_cstr(code, "DEVICE_OFFLINE")) {
    return GZC_CONTROL_ERROR_DEVICE_OFFLINE;
  }
  if (gzc_control_str_eq_cstr(code, "DEVICE_TIMEOUT")) {
    return GZC_CONTROL_ERROR_DEVICE_TIMEOUT;
  }
  if (gzc_control_str_eq_cstr(code, "DEVICE_REJECTED")) {
    return GZC_CONTROL_ERROR_DEVICE_REJECTED;
  }
  if (gzc_control_str_eq_cstr(code, "DEVICE_UNSUPPORTED")) {
    return GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED;
  }
  if (gzc_control_str_eq_cstr(code, "DEVICE_ERROR")) {
    return GZC_CONTROL_ERROR_DEVICE_ERROR;
  }
  switch (status_code) {
  case 400:
    return GZC_CONTROL_ERROR_INVALID_REQUEST;
  case 401:
    return GZC_CONTROL_ERROR_UNAUTHORIZED;
  case 403:
    return GZC_CONTROL_ERROR_FORBIDDEN;
  case 404:
    return GZC_CONTROL_ERROR_NOT_FOUND;
  case 409:
    return GZC_CONTROL_ERROR_CONFLICT;
  default:
    break;
  }
  if (status_code >= 500 && status_code < 600) {
    return GZC_CONTROL_ERROR_SERVER;
  }
  return GZC_CONTROL_ERROR_UNEXPECTED_STATUS;
}

const char *gzc_control_error_kind_string(gzc_control_error_kind_t kind) {
  switch (kind) {
  case GZC_CONTROL_ERROR_NONE:
    return "none";
  case GZC_CONTROL_ERROR_UNAUTHORIZED:
    return "unauthorized";
  case GZC_CONTROL_ERROR_FORBIDDEN:
    return "forbidden";
  case GZC_CONTROL_ERROR_NOT_FOUND:
    return "notFound";
  case GZC_CONTROL_ERROR_DEVICE_OFFLINE:
    return "deviceOffline";
  case GZC_CONTROL_ERROR_DEVICE_TIMEOUT:
    return "deviceTimeout";
  case GZC_CONTROL_ERROR_DEVICE_REJECTED:
    return "deviceRejected";
  case GZC_CONTROL_ERROR_DEVICE_UNSUPPORTED:
    return "deviceUnsupported";
  case GZC_CONTROL_ERROR_DEVICE_ERROR:
    return "deviceError";
  case GZC_CONTROL_ERROR_CONFLICT:
    return "conflict";
  case GZC_CONTROL_ERROR_INVALID_REQUEST:
    return "invalidRequest";
  case GZC_CONTROL_ERROR_SERVER:
    return "server";
  case GZC_CONTROL_ERROR_UNEXPECTED_STATUS:
    return "unexpectedStatus";
  case GZC_CONTROL_ERROR_MALFORMED_RESPONSE:
    return "malformedResponse";
  case GZC_CONTROL_ERROR_NETWORK:
    return "network";
  }
  return "unknown";
}
