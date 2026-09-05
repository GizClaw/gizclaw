/**
 * Controller-side clients for GizClaw HTTP APIs and monitoring.
 *
 * The package talks to `/gizclaw/v1/*` over HTTPS with
 * `Authorization: Bearer gizclaw_sk_v1_...` and reuses the generated Peer HTTP
 * client owned by `@gizclaw/gizclaw/peerhttp`. The device side of GizClaw
 * (WebRTC, RPC, telemetry) stays in `@gizclaw/gizclaw`. Monitor constructors
 * additionally support device public keys, node tokens and public discovery.
 */
import {
  listDeviceWorkspaces,
  listDeviceWorkspaceHistory,
  searchDeviceLogs,
  downloadDeviceHistoryAudio,
  aggregateDeviceTelemetry,
  connectDeviceWifi,
  createApiKey,
  createContact,
  createPeerHTTPClient,
  deleteContact,
  forgetDeviceSavedWifi,
  getApiKey,
  getContact,
  getDevice,
  getDeviceRuntime,
  getDeviceStatus,
  getDeviceTelemetryLatest,
  getDeviceWifi,
  getSelfApiKey,
  listApiKeys,
  listContacts,
  listDeviceSavedWifi,
  playDeviceSound,
  putContact,
  queryDeviceTelemetry,
  rebootDevice,
  revokeApiKey,
  revokeSelfApiKey,
  scanDeviceWifi,
  setDeviceVolume,
} from "@gizclaw/gizclaw/peerhttp";
import type {
  DeviceWorkspace,
  DeviceLogPage,
  PeerRunHistoryListResponse,
  ListDeviceWorkspaceHistoryData,
  SearchDeviceLogsData,
  ApiKey,
  ApiKeyCreateRequest,
  ApiKeyCreateResult,
  ApiKeyList,
  Contact,
  ContactCreateRequest,
  ContactList,
  ContactPutRequest,
  DeviceControlStatus,
  DeviceInfo,
  DevicePlaySoundRequest,
  DeviceRebootRequest,
  DeviceVolumeSetRequest,
  DeviceWifiConnectRequest,
  DeviceWifiSavedList,
  DeviceWifiScanRequest,
  DeviceWifiScanResponse,
  DeviceWifiStatus,
  ErrorResponse,
  PeerHTTPClient,
  PeerStatus,
  PeerTelemetryAggregate,
  PeerTelemetryAggregateResponse,
  PeerTelemetryField,
  PeerTelemetryLatestResponse,
  PeerTelemetryOrder,
  PeerTelemetryRangeResponse,
  Runtime,
} from "@gizclaw/gizclaw/peerhttp";

export type {
  DeviceWorkspace,
  DeviceLogPage,
  PeerRunHistoryListResponse,
  ListDeviceWorkspaceHistoryData,
  SearchDeviceLogsData,
  ApiKey,
  ApiKeyCreateRequest,
  ApiKeyCreateResult,
  ApiKeyList,
  Contact,
  ContactCreateRequest,
  ContactList,
  ContactPutRequest,
  DeviceControlStatus,
  DeviceInfo,
  DevicePlaySoundRequest,
  DeviceRebootRequest,
  DeviceVolumeSetRequest,
  DeviceWifiConnectRequest,
  DeviceWifiSavedList,
  DeviceWifiScanRequest,
  DeviceWifiScanResponse,
  DeviceWifiScanResult,
  DeviceWifiStatus,
  ErrorPayload,
  ErrorResponse,
  PeerHTTPClient,
  PeerStatus,
  PeerTelemetryAggregate,
  PeerTelemetryAggregateResponse,
  PeerTelemetryField,
  PeerTelemetryLatestResponse,
  PeerTelemetryOrder,
  PeerTelemetryRangeResponse,
  Runtime,
} from "@gizclaw/gizclaw/peerhttp";

/**
 * Stable classification of a failed `/gizclaw/v1/*` call.
 *
 * `DEVICE_*` codes are matched on the `error.code` string the Server emits;
 * the remaining kinds are matched on the HTTP status.
 */
export type GizClawControlErrorKind =
  /** `401`: missing, invalid, or revoked API key. */
  | "unauthorized"
  /** `403`: the API key does not authorize this operation. */
  | "forbidden"
  /** `404`: the key, contact, or saved Wi-Fi network does not exist. */
  | "notFound"
  /** `409 DEVICE_OFFLINE`: no active device connection, or rebooting. */
  | "deviceOffline"
  /** `504 DEVICE_TIMEOUT`: the device did not answer within the control timeout. */
  | "deviceTimeout"
  /** `400 DEVICE_REJECTED`: the device rejected the command parameters. */
  | "deviceRejected"
  /** `501 DEVICE_UNSUPPORTED`: the firmware does not implement the method. */
  | "deviceUnsupported"
  /** `502 DEVICE_ERROR`: the device answered with an unexpected RPC error. */
  | "deviceError"
  /** `409` with any other code, such as a duplicate contact. */
  | "conflict"
  /** `400` with any other code. */
  | "invalidRequest"
  /** Any other `5xx`. */
  | "server"
  /** Any other non-2xx status. */
  | "unexpectedStatus"
  /** The request never produced an HTTP response. */
  | "network";

const DEVICE_CODES: Readonly<Record<string, GizClawControlErrorKind>> = {
  DEVICE_OFFLINE: "deviceOffline",
  DEVICE_TIMEOUT: "deviceTimeout",
  DEVICE_REJECTED: "deviceRejected",
  DEVICE_UNSUPPORTED: "deviceUnsupported",
  DEVICE_ERROR: "deviceError",
};

/** Maps a non-2xx status and optional `error.code` to an error kind. */
export function classifyGizClawControlError(
  status: number,
  code?: string,
): GizClawControlErrorKind {
  const deviceKind = code === undefined ? undefined : DEVICE_CODES[code];
  if (deviceKind !== undefined) {
    return deviceKind;
  }
  switch (status) {
    case 400:
      return "invalidRequest";
    case 401:
      return "unauthorized";
    case 403:
      return "forbidden";
    case 404:
      return "notFound";
    case 409:
      return "conflict";
    default:
      return status >= 500 && status < 600 ? "server" : "unexpectedStatus";
  }
}

/** Failure of one `/gizclaw/v1/*` call. */
export class GizClawControlError extends Error {
  readonly kind: GizClawControlErrorKind;
  /** HTTP status, or `undefined` for `network`. */
  readonly status: number | undefined;
  /** `error.code` from the response body when it carried an `ErrorResponse`. */
  readonly code: string | undefined;
  /** `X-Request-ID` response header when the Server set one. */
  readonly requestId: string | undefined;
  /** `error.details` from the response body. */
  readonly details: Readonly<Record<string, unknown>>;

  constructor(
    kind: GizClawControlErrorKind,
    message: string,
    fields: {
      status?: number;
      code?: string;
      requestId?: string;
      details?: Record<string, unknown>;
      cause?: unknown;
    } = {},
  ) {
    super(
      message,
      fields.cause === undefined ? undefined : { cause: fields.cause },
    );
    this.name = "GizClawControlError";
    this.kind = kind;
    this.status = fields.status;
    this.code = fields.code;
    this.requestId = fields.requestId;
    this.details = fields.details ?? {};
  }

  /**
   * Builds the error for one generated-client result whose `response` is
   * missing or not `ok`. `error` is whatever the generated client returned:
   * the decoded body for HTTP failures, or the thrown value for transport
   * failures.
   */
  static fromResult(
    operation: string,
    response: Response | undefined,
    error: unknown,
  ): GizClawControlError {
    if (response === undefined) {
      const message = error instanceof Error ? error.message : String(error);
      return new GizClawControlError("network", `${operation}: ${message}`, {
        cause: error,
      });
    }
    const payload = readErrorPayload(error);
    const requestId = response.headers.get("x-request-id") ?? undefined;
    return new GizClawControlError(
      classifyGizClawControlError(response.status, payload?.code),
      `${operation}: ${payload?.message ?? `HTTP ${response.status}`}`,
      {
        status: response.status,
        code: payload?.code,
        requestId,
        details: payload?.details,
        cause: error,
      },
    );
  }
}

function readErrorPayload(
  value: unknown,
):
  | { code: string; message: string; details?: Record<string, unknown> }
  | undefined {
  if (typeof value !== "object" || value === null || !("error" in value)) {
    return undefined;
  }
  const error = (value as Partial<ErrorResponse>).error;
  if (
    typeof error !== "object" ||
    error === null ||
    typeof error.code !== "string" ||
    typeof error.message !== "string"
  ) {
    return undefined;
  }
  const details =
    typeof error.details === "object" && error.details !== null
      ? (error.details as Record<string, unknown>)
      : undefined;
  return { code: error.code, message: error.message, details };
}

export interface GizClawControlClientOptions {
  /** Server origin, optionally with a path prefix, such as `https://ap.gizclaw.com`. */
  baseUrl: string;
  /** Complete `gizclaw_sk_v1_...` credential. */
  apiKey: string;
  /** Abort all requests from this client, for example when a view unmounts. */
  signal?: AbortSignal;
  /** Fetch implementation; defaults to `globalThis.fetch`. */
  fetch?: typeof fetch;
  /**
   * Allow a plaintext `http` base URL. Every request carries the API key, so
   * this sends the credential in the clear and is only appropriate for a local
   * test deployment.
   */
  allowInsecureTransport?: boolean;
}

export interface GizClawControlApiKeys {
  /** `POST /gizclaw/v1/api-keys`. */
  create(body: ApiKeyCreateRequest): Promise<ApiKeyCreateResult>;
  /** `GET /gizclaw/v1/api-keys`. */
  list(query?: { cursor?: string; limit?: number }): Promise<ApiKeyList>;
  /** `GET /gizclaw/v1/api-keys/self`. */
  getSelf(): Promise<ApiKey>;
  /** `DELETE /gizclaw/v1/api-keys/self`. */
  revokeSelf(): Promise<void>;
  /** `GET /gizclaw/v1/api-keys/{apiKeyName}`. */
  get(apiKeyName: string): Promise<ApiKey>;
  /** `DELETE /gizclaw/v1/api-keys/{apiKeyName}`. */
  revoke(apiKeyName: string): Promise<void>;
}

export interface GizClawControlDevice {
  /** Owned Workspaces, including system Workspaces. */
  listWorkspaces(): Promise<DeviceWorkspace[]>;
  /** Search persisted chat within an owned Workspace. */
  listWorkspaceHistory(
    workspaceId: string,
    query?: ListDeviceWorkspaceHistoryData["query"],
  ): Promise<PeerRunHistoryListResponse>;
  /** Query persistent logs restricted to this device. */
  searchLogs(query: SearchDeviceLogsData["query"]): Promise<DeviceLogPage>;
  /** Download retained Ogg audio; unavailable assets reject with notFound. */
  downloadHistoryAudio(workspaceId: string, historyId: string): Promise<Blob>;
  /** `GET /gizclaw/v1/device`. */
  get(): Promise<DeviceInfo>;
  /** `GET /gizclaw/v1/device/runtime`. */
  getRuntime(): Promise<Runtime>;
  /** `GET /gizclaw/v1/device/status`: the stored snapshot, never contacts the device. */
  getStatus(): Promise<PeerStatus>;
  /** `GET /gizclaw/v1/device/telemetry/latest`; omitted `fields` means every field. */
  getTelemetryLatest(
    fields?: readonly PeerTelemetryField[],
  ): Promise<PeerTelemetryLatestResponse>;
  /** `GET /gizclaw/v1/device/telemetry`. */
  queryTelemetry(query: {
    field: PeerTelemetryField;
    start_time_ms: number;
    end_time_ms: number;
    step_ms?: number;
    limit?: number;
    order?: PeerTelemetryOrder;
  }): Promise<PeerTelemetryRangeResponse>;
  /** `GET /gizclaw/v1/device/telemetry/aggregate`. */
  aggregateTelemetry(query: {
    field: PeerTelemetryField;
    start_time_ms: number;
    end_time_ms: number;
    bucket_ms: number;
    aggregate: PeerTelemetryAggregate;
  }): Promise<PeerTelemetryAggregateResponse>;
  /** `PUT /gizclaw/v1/device/volume`. */
  setVolume(body: DeviceVolumeSetRequest): Promise<DeviceControlStatus>;
  /** `POST /gizclaw/v1/device/actions/play-sound`. */
  playSound(body: DevicePlaySoundRequest): Promise<void>;
  /** `POST /gizclaw/v1/device/actions/reboot`. */
  reboot(body?: DeviceRebootRequest): Promise<void>;
  /** `GET /gizclaw/v1/device/wifi`. */
  getWifi(): Promise<DeviceWifiStatus>;
  /** `POST /gizclaw/v1/device/wifi/scan`. */
  scanWifi(body?: DeviceWifiScanRequest): Promise<DeviceWifiScanResponse>;
  /**
   * `PUT /gizclaw/v1/device/wifi`.
   *
   * Resolving means the device accepted the credentials and began switching
   * networks, not that it joined them. The device goes offline during the
   * switch; poll {@link GizClawControlDevice.getWifi} after it reconnects to
   * observe the outcome.
   */
  connectWifi(body: DeviceWifiConnectRequest): Promise<void>;
  /** `GET /gizclaw/v1/device/wifi/saved`. */
  listSavedWifi(): Promise<DeviceWifiSavedList>;
  /** `DELETE /gizclaw/v1/device/wifi/saved/{ssid}`. */
  forgetSavedWifi(ssid: string): Promise<void>;
}

export interface GizClawControlContacts {
  /** `GET /gizclaw/v1/contacts`. */
  list(query?: { cursor?: string; limit?: number }): Promise<ContactList>;
  /** `POST /gizclaw/v1/contacts`. */
  create(body: ContactCreateRequest): Promise<Contact>;
  /** `GET /gizclaw/v1/contacts/{contactName}`. */
  get(contactName: string): Promise<Contact>;
  /** `PUT /gizclaw/v1/contacts/{contactName}`. */
  put(contactName: string, body: ContactPutRequest): Promise<Contact>;
  /** `DELETE /gizclaw/v1/contacts/{contactName}`. */
  delete(contactName: string): Promise<void>;
}

export interface GizClawControlClient {
  /** Configured generated client for calls this wrapper does not expose. */
  readonly client: PeerHTTPClient;
  readonly apiKeys: GizClawControlApiKeys;
  readonly device: GizClawControlDevice;
  readonly contacts: GizClawControlContacts;
}

interface GeneratedResult<T> {
  data?: T;
  error?: unknown;
  response?: Response;
}

async function unwrap<T>(
  operation: string,
  result: Promise<GeneratedResult<T>>,
): Promise<T> {
  const { data, error, response } = await result;
  if (response === undefined || !response.ok || error !== undefined) {
    throw GizClawControlError.fromResult(operation, response, error);
  }
  return data as T;
}

async function unwrapEmpty(
  operation: string,
  result: Promise<GeneratedResult<unknown>>,
): Promise<void> {
  await unwrap(operation, result);
}

function requireSegment(name: string, value: string): string {
  if (value.length === 0) {
    throw new TypeError(`${name} must not be empty`);
  }
  return value;
}

/**
 * Creates a controller client bound to one API key.
 *
 * Every method resolves with the contract response type and rejects with
 * {@link GizClawControlError} for non-2xx responses and transport failures.
 */
export function createGizClawControlClient(
  options: GizClawControlClientOptions,
): GizClawControlClient {
  if (options.apiKey.length === 0) {
    throw new TypeError("apiKey must not be empty");
  }
  const baseUrl = new URL(options.baseUrl);
  if (baseUrl.protocol !== "https:" && baseUrl.protocol !== "http:") {
    throw new TypeError("baseUrl must be an http(s) URL");
  }
  if (
    baseUrl.protocol !== "https:" &&
    options.allowInsecureTransport !== true
  ) {
    throw new TypeError(
      "baseUrl must use https; set allowInsecureTransport to send the API key over plaintext",
    );
  }
  const client = createPeerHTTPClient({
    baseUrl: baseUrl.href.replace(/\/+$/u, ""),
    auth: options.apiKey,
    fetch: options.fetch,
    throwOnError: false,
  });
  const common = {
    signal: options.signal,
    client,
    throwOnError: false as const,
  };

  return {
    client,
    apiKeys: {
      create: (body) =>
        unwrap("createApiKey", createApiKey({ ...common, body })),
      list: (query) => unwrap("listApiKeys", listApiKeys({ ...common, query })),
      getSelf: () => unwrap("getSelfApiKey", getSelfApiKey(common)),
      revokeSelf: () =>
        unwrapEmpty("revokeSelfApiKey", revokeSelfApiKey(common)),
      get: async (apiKeyName) =>
        unwrap(
          "getApiKey",
          getApiKey({
            ...common,
            path: { apiKeyName: requireSegment("apiKeyName", apiKeyName) },
          }),
        ),
      revoke: async (apiKeyName) =>
        unwrapEmpty(
          "revokeApiKey",
          revokeApiKey({
            ...common,
            path: { apiKeyName: requireSegment("apiKeyName", apiKeyName) },
          }),
        ),
    },
    device: {
      listWorkspaces: () =>
        unwrap("listDeviceWorkspaces", listDeviceWorkspaces(common)),
      listWorkspaceHistory: (workspaceId, query) =>
        unwrap(
          "listDeviceWorkspaceHistory",
          listDeviceWorkspaceHistory({
            ...common,
            path: { workspaceId: requireSegment("workspaceId", workspaceId) },
            query,
          }),
        ),
      searchLogs: (query) =>
        unwrap("searchDeviceLogs", searchDeviceLogs({ ...common, query })),
      downloadHistoryAudio: async (workspaceId, historyId) => {
        const blob = await unwrap(
          "downloadDeviceHistoryAudio",
          downloadDeviceHistoryAudio({
            ...common,
            path: {
              workspaceId: requireSegment("workspaceId", workspaceId),
              historyId: requireSegment("historyId", historyId),
            },
            parseAs: "blob",
          }),
        );
        if (!(blob instanceof Blob))
          throw new TypeError("history audio response must be a Blob");
        return blob;
      },
      get: () => unwrap("getDevice", getDevice(common)),
      getRuntime: () => unwrap("getDeviceRuntime", getDeviceRuntime(common)),
      getStatus: () => unwrap("getDeviceStatus", getDeviceStatus(common)),
      getTelemetryLatest: (fields) =>
        unwrap(
          "getDeviceTelemetryLatest",
          getDeviceTelemetryLatest({
            ...common,
            query:
              fields === undefined || fields.length === 0
                ? undefined
                : { fields: fields.join(",") },
          }),
        ),
      queryTelemetry: (query) =>
        unwrap(
          "queryDeviceTelemetry",
          queryDeviceTelemetry({ ...common, query }),
        ),
      aggregateTelemetry: (query) =>
        unwrap(
          "aggregateDeviceTelemetry",
          aggregateDeviceTelemetry({ ...common, query }),
        ),
      setVolume: (body) =>
        unwrap("setDeviceVolume", setDeviceVolume({ ...common, body })),
      playSound: (body) =>
        unwrapEmpty("playDeviceSound", playDeviceSound({ ...common, body })),
      reboot: (body = {}) =>
        unwrapEmpty("rebootDevice", rebootDevice({ ...common, body })),
      getWifi: () => unwrap("getDeviceWifi", getDeviceWifi(common)),
      scanWifi: (body = {}) =>
        unwrap("scanDeviceWifi", scanDeviceWifi({ ...common, body })),
      connectWifi: (body) =>
        unwrapEmpty(
          "connectDeviceWifi",
          connectDeviceWifi({ ...common, body }),
        ),
      listSavedWifi: () =>
        unwrap("listDeviceSavedWifi", listDeviceSavedWifi(common)),
      forgetSavedWifi: async (ssid) =>
        unwrapEmpty(
          "forgetDeviceSavedWifi",
          forgetDeviceSavedWifi({
            ...common,
            path: { ssid: requireSegment("ssid", ssid) },
          }),
        ),
    },
    contacts: {
      list: (query) =>
        unwrap("listContacts", listContacts({ ...common, query })),
      create: (body) =>
        unwrap("createContact", createContact({ ...common, body })),
      get: async (contactName) =>
        unwrap(
          "getContact",
          getContact({
            ...common,
            path: { contactName: requireSegment("contactName", contactName) },
          }),
        ),
      put: async (contactName, body) =>
        unwrap(
          "putContact",
          putContact({
            ...common,
            path: { contactName: requireSegment("contactName", contactName) },
            body,
          }),
        ),
      delete: async (contactName) =>
        unwrapEmpty(
          "deleteContact",
          deleteContact({
            ...common,
            path: { contactName: requireSegment("contactName", contactName) },
          }),
        ),
    },
  };
}

export {
  createGizClawDiscoveryClient,
  createGizClawPeerMonitorClient,
  createGizClawNodeMonitorClient,
} from "./monitor.ts";
export type { NodeSnapshot, MonitorLog } from "./monitor.ts";
