import type { CreateGiznetWebRtcOfferData } from "./generated/serverpublic/types.gen";
import { base58Decode, prepareEncryptedGiznetWebRTCOffer } from "./signaling.ts";
import { encodeTelemetryPacket, type TelemetryFrame } from "./telemetry.ts";
export * from "./telemetry.ts";

export const WEBRTC_RPC_DATA_CHANNEL_LABEL = "rpc";
export const WEBRTC_EVENT_DATA_CHANNEL_LABEL = "event";
export const GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL = "giznet/v1/packet";
export const GIZNET_WEBRTC_SERVICE_DATA_CHANNEL_PREFIX = "giznet/v1/service/";
export const GIZNET_WEBRTC_SIGNALING_PATH = "/webrtc/v1/offer";
export const RPC_VERSION = 1;
export const GIZCLAW_SERVICE_RPC = 0x00;
export const GIZCLAW_SERVICE_SERVER_PUBLIC = 0x01;
export const GIZCLAW_SERVICE_OPENAI = 0x02;
export const GIZCLAW_SERVICE_ADMIN = 0x10;
export const GIZCLAW_SERVICE_AGENT_STREAM = 0x20;
export const GIZCLAW_SERVICE_EVENT = GIZCLAW_SERVICE_AGENT_STREAM;
export const RPC_FRAME_TYPE_EOS = 0;
export const RPC_FRAME_TYPE_JSON = 1;
export const RPC_FRAME_TYPE_BINARY = 2;
export const RPC_FRAME_TYPE_TEXT = 3;
const DATA_CHANNEL_SEND_RETRY_DELAY_MS = 5;
const DATA_CHANNEL_SEND_RETRY_LIMIT = 20;
const giznetPacketDataChannels = new WeakMap<object, WebRTCRPCDataChannel>();

export type RPCID = string;

export type RPCRequest<TParams = unknown> = {
  id: RPCID;
  method: string;
  params?: TParams;
  v: typeof RPC_VERSION;
};

export type RPCErrorBody = {
  code: number;
  data?: unknown;
  message: string;
};

export type RPCResponse<TResult = unknown> = {
  error?: RPCErrorBody;
  id?: RPCID;
  result?: TResult;
  v?: typeof RPC_VERSION;
};

export type RPCBinaryCallResult<TResult = unknown> = {
  body: Uint8Array;
  result: TResult;
};

export type WebRTCRPCDataChannel = {
  addEventListener(type: "open", listener: () => void): void;
  addEventListener(type: "message", listener: (event: MessageEvent) => void): void;
  addEventListener(type: "error", listener: () => void): void;
  addEventListener(type: "close", listener: () => void): void;
  binaryType?: BinaryType;
  close(): void;
  readyState: RTCDataChannelState;
  removeEventListener(type: "open", listener: () => void): void;
  removeEventListener(type: "message", listener: (event: MessageEvent) => void): void;
  removeEventListener(type: "error", listener: () => void): void;
  removeEventListener(type: "close", listener: () => void): void;
  send(data: ArrayBuffer | ArrayBufferView | Blob | string): void;
};

export type WebRTCRPCDataChannelFactory = {
  createDataChannel(label: string, options?: RTCDataChannelInit): WebRTCRPCDataChannel;
};

export type PreparedGiznetWebRTCOffer = {
  body: Blob | File;
  clientPublicKey: string;
  nonce: string;
  openAnswer: (encryptedAnswer: Blob) => Promise<string>;
  timestamp: number;
};

export type ConnectGiznetWebRTCOptions = {
  addAudioTransceiver?: boolean;
  createPacketDataChannel?: boolean;
  fetch?: typeof fetch;
  pc: RTCPeerConnection;
  prepareOffer: (offerSDP: string) => Promise<PreparedGiznetWebRTCOffer>;
  sendOffer?: (offer: PreparedGiznetWebRTCOffer, signal?: AbortSignal) => Promise<Blob>;
  signal?: AbortSignal;
};

export type GiznetServerInfo = {
  endpoint?: string;
  protocol?: string;
  public_key: string;
  signaling_path?: string;
};

export type ServerInfoBootstrapOptions = {
  baseUrl?: string;
  endpoint?: string;
  fetch?: typeof fetch;
  signal?: AbortSignal;
};

export type ConnectGiznetWebRTCFromEndpointOptions = Omit<ConnectGiznetWebRTCOptions, "prepareOffer" | "sendOffer"> & {
  baseUrl?: string;
  clientPrivateKey: Uint8Array;
  clientPublicKey?: Uint8Array | string;
  endpoint?: string;
};

export type WebRTCRPCClientOptions = {
  channelLabel?: string;
  createID?: () => string;
  requestTimeoutMs?: number;
  service?: number;
};

type DataChannelPayload = ArrayBuffer | ArrayBufferView | Blob | string;

export type RPCCallOptions = {
  id?: string;
  signal?: AbortSignal;
  timeoutMs?: number;
};

export class WebRTCRPCError extends Error {
  readonly code: number;
  readonly data?: unknown;
  readonly requestID?: string;

  constructor(error: RPCErrorBody, requestID?: string) {
    super(error.message);
    this.name = "WebRTCRPCError";
    this.code = error.code;
    this.data = error.data;
    this.requestID = requestID;
  }
}

export class WebRTCRPCClient {
  readonly pc: WebRTCRPCDataChannelFactory;
  private readonly channelLabel: string;
  private readonly createID: () => string;
  private readonly requestTimeoutMs: number;

  constructor(pc: WebRTCRPCDataChannelFactory, options: WebRTCRPCClientOptions = {}) {
    this.pc = pc;
    this.channelLabel = options.channelLabel ?? giznetServiceDataChannelLabel(options.service ?? GIZCLAW_SERVICE_RPC);
    this.createID = options.createID ?? defaultRPCID;
    this.requestTimeoutMs = options.requestTimeoutMs ?? 30000;
  }

  async call<TResult = unknown, TParams = unknown>(method: string, params?: TParams, options: RPCCallOptions = {}): Promise<TResult> {
    const id = options.id ?? this.createID();
    const response = await this.request<TResult, TParams>({ id, method, params, v: RPC_VERSION }, options);
    if (response.error != null) {
      throw new WebRTCRPCError(response.error, response.id);
    }
    return response.result as TResult;
  }

  async callBinary<TResult = unknown, TParams = unknown>(method: string, params?: TParams, options: RPCCallOptions = {}): Promise<RPCBinaryCallResult<TResult>> {
    const id = options.id ?? this.createID();
    const response = await this.requestBinary<TResult, TParams>({ id, method, params, v: RPC_VERSION }, options);
    if (response.response.error != null) {
      throw new WebRTCRPCError(response.response.error, response.response.id);
    }
    return {
      body: response.body,
      result: response.response.result as TResult,
    };
  }

  async request<TResult = unknown, TParams = unknown>(request: RPCRequest<TParams>, options: RPCCallOptions = {}): Promise<RPCResponse<TResult>> {
    const channel = this.pc.createDataChannel(this.channelLabel, { ordered: true });
    channel.binaryType = "arraybuffer";

    const timeoutMs = options.timeoutMs ?? this.requestTimeoutMs;
    const abortSignal = options.signal;

    return new Promise<RPCResponse<TResult>>((resolve, reject) => {
      let buffer: Uint8Array<ArrayBufferLike> = new Uint8Array();
      let messageQueue = Promise.resolve();
      let settled = false;
      let timeout: ReturnType<typeof setTimeout> | undefined;

      const settle = (fn: () => void): void => {
        if (settled) {
          return;
        }
        settled = true;
        cleanup();
        try {
          channel.close();
        } catch {
          // Ignore close races from browsers that already closed the channel.
        }
        fn();
      };

      const cleanup = (): void => {
        if (timeout != null) {
          clearTimeout(timeout);
        }
        abortSignal?.removeEventListener("abort", onAbort);
        channel.removeEventListener("open", onOpen);
        channel.removeEventListener("message", onMessage);
        channel.removeEventListener("error", onError);
        channel.removeEventListener("close", onClose);
      };

      const onAbort = (): void => {
        settle(() => reject(abortError()));
      };
      const onOpen = (): void => {
        sendDataChannelMessage(channel, encodeRPCRequest(request), (err) => settle(() => reject(err)));
      };
      const onMessage = (event: MessageEvent): void => {
        messageQueue = messageQueue.then(async () => {
          if (settled) {
            return;
          }
          try {
            const chunk = await messageDataBytes(event.data);
            buffer = appendBytes(buffer, chunk);
            const parsed = tryReadRPCResponse<TResult>(buffer);
            if (parsed == null) {
              return;
            }
            buffer = parsed.rest;
            if (parsed.response.id != null && parsed.response.id !== request.id) {
              throw new Error(`rpc response id mismatch: got ${parsed.response.id}, want ${request.id}`);
            }
            settle(() => resolve(parsed.response));
          } catch (err) {
            settle(() => reject(err));
          }
        });
      };
      const onError = (): void => {
        settle(() => reject(new Error("WebRTC RPC data channel failed.")));
      };
      const onClose = (): void => {
        messageQueue = messageQueue.then(() => {
          if (!settled) {
            settle(() => reject(new Error("WebRTC RPC data channel closed before response.")));
          }
        });
      };

      if (abortSignal?.aborted) {
        settle(() => reject(abortError()));
        return;
      }

      abortSignal?.addEventListener("abort", onAbort, { once: true });
      channel.addEventListener("open", onOpen);
      channel.addEventListener("message", onMessage);
      channel.addEventListener("error", onError);
      channel.addEventListener("close", onClose);

      if (timeoutMs > 0) {
        timeout = setTimeout(() => {
          settle(() => reject(new Error(`WebRTC RPC request timed out after ${timeoutMs}ms.`)));
        }, timeoutMs);
      }
      if (channel.readyState === "open") {
        onOpen();
      } else if (channel.readyState === "closed") {
        onClose();
      }
    });
  }

  async requestBinary<TResult = unknown, TParams = unknown>(
    request: RPCRequest<TParams>,
    options: RPCCallOptions = {},
  ): Promise<{ body: Uint8Array; response: RPCResponse<TResult> }> {
    const channel = this.pc.createDataChannel(this.channelLabel, { ordered: true });
    channel.binaryType = "arraybuffer";

    const timeoutMs = options.timeoutMs ?? this.requestTimeoutMs;
    const abortSignal = options.signal;

    return new Promise<{ body: Uint8Array; response: RPCResponse<TResult> }>((resolve, reject) => {
      let buffer: Uint8Array<ArrayBufferLike> = new Uint8Array();
      let messageQueue = Promise.resolve();
      let settled = false;
      let timeout: ReturnType<typeof setTimeout> | undefined;

      const settle = (fn: () => void): void => {
        if (settled) {
          return;
        }
        settled = true;
        cleanup();
        try {
          channel.close();
        } catch {
          // Ignore close races from browsers that already closed the channel.
        }
        fn();
      };

      const cleanup = (): void => {
        if (timeout != null) {
          clearTimeout(timeout);
        }
        abortSignal?.removeEventListener("abort", onAbort);
        channel.removeEventListener("open", onOpen);
        channel.removeEventListener("message", onMessage);
        channel.removeEventListener("error", onError);
        channel.removeEventListener("close", onClose);
      };

      const onAbort = (): void => {
        settle(() => reject(abortError()));
      };
      const onOpen = (): void => {
        sendDataChannelMessage(channel, encodeRPCRequest(request), (err) => settle(() => reject(err)));
      };
      const onMessage = (event: MessageEvent): void => {
        messageQueue = messageQueue.then(async () => {
          if (settled) {
            return;
          }
          try {
            const chunk = await messageDataBytes(event.data);
            buffer = appendBytes(buffer, chunk);
            const parsed = tryReadRPCBinaryResponse<TResult>(buffer);
            if (parsed == null) {
              return;
            }
            buffer = parsed.rest;
            if (parsed.response.id != null && parsed.response.id !== request.id) {
              throw new Error(`rpc response id mismatch: got ${parsed.response.id}, want ${request.id}`);
            }
            settle(() => resolve({ body: parsed.body, response: parsed.response }));
          } catch (err) {
            settle(() => reject(err));
          }
        });
      };
      const onError = (): void => {
        settle(() => reject(new Error("WebRTC RPC data channel failed.")));
      };
      const onClose = (): void => {
        messageQueue = messageQueue.then(() => {
          if (!settled) {
            settle(() => reject(new Error("WebRTC RPC data channel closed before binary response.")));
          }
        });
      };

      if (abortSignal?.aborted) {
        settle(() => reject(abortError()));
        return;
      }

      abortSignal?.addEventListener("abort", onAbort, { once: true });
      channel.addEventListener("open", onOpen);
      channel.addEventListener("message", onMessage);
      channel.addEventListener("error", onError);
      channel.addEventListener("close", onClose);

      if (timeoutMs > 0) {
        timeout = setTimeout(() => {
          settle(() => reject(new Error(`WebRTC RPC request timed out after ${timeoutMs}ms.`)));
        }, timeoutMs);
      }
      if (channel.readyState === "open") {
        onOpen();
      } else if (channel.readyState === "closed") {
        onClose();
      }
    });
  }
}

export type WebRTCFetchRoute = {
  headers?: HeadersInit;
  method: string;
  params?: unknown;
  status?: number;
};

export type WebRTCFetchRouter = (request: Request) => WebRTCFetchRoute | Promise<WebRTCFetchRoute>;

export type WebRTCFetchOptions = {
  router: WebRTCFetchRouter;
};

export function createWebRTCFetch(client: WebRTCRPCClient, options: WebRTCFetchOptions): typeof fetch {
  return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const request = new Request(input, init);
    const route = await options.router(request);
    const result = await client.call(route.method, route.params, { signal: request.signal });
    const headers = new Headers(route.headers);
    if (!headers.has("content-type")) {
      headers.set("content-type", "application/json");
    }
    return new Response(JSON.stringify(result ?? {}), {
      headers,
      status: route.status ?? 200,
    });
  };
}

export type WebRTCServiceFetchOptions = {
  host?: string;
  requestTimeoutMs?: number;
  service?: number;
};

export function createWebRTCServiceFetch(pc: WebRTCRPCDataChannelFactory, options: WebRTCServiceFetchOptions = {}): typeof fetch {
  const service = options.service ?? GIZCLAW_SERVICE_ADMIN;
  const host = options.host ?? "gizclaw";
  const timeoutMs = options.requestTimeoutMs ?? 30000;
  return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const request = new Request(input, init);
    const channel = pc.createDataChannel(giznetServiceDataChannelLabel(service), { ordered: true });
    channel.binaryType = "arraybuffer";
    const requestBytes = await encodeHTTPRequest(request, host);
    return readHTTPResponse(channel, requestBytes, request.signal, timeoutMs);
  };
}

export function createAdminAPIFetch(pc: WebRTCRPCDataChannelFactory, options: Omit<WebRTCServiceFetchOptions, "service"> = {}): typeof fetch {
  return createWebRTCServiceFetch(pc, { ...options, service: GIZCLAW_SERVICE_ADMIN });
}

export async function connectGiznetWebRTC(options: ConnectGiznetWebRTCOptions): Promise<RTCPeerConnection> {
  prepareGiznetWebRTCPeerConnection(options.pc, options);
  const offer = await options.pc.createOffer();
  await options.pc.setLocalDescription(offer);
  await waitForICEGatheringComplete(options.pc, options.signal);
  const local = options.pc.localDescription;
  if (local == null) {
    throw new Error("WebRTC offer was not created.");
  }

  const prepared = await options.prepareOffer(local.sdp);
  const encryptedAnswer = await (options.sendOffer ?? ((item, signal) => sendGiznetWebRTCOffer(item, { fetch: options.fetch, signal })))(prepared, options.signal);
  const answerSDP = await prepared.openAnswer(encryptedAnswer);
  await options.pc.setRemoteDescription({ sdp: answerSDP, type: "answer" });
  return options.pc;
}

export async function connectGiznetWebRTCFromEndpoint(options: ConnectGiznetWebRTCFromEndpointOptions): Promise<RTCPeerConnection> {
  const serverInfo = await fetchGiznetServerInfo(options);
  const signalingPath = normalizeServerInfoSignalingPath(serverInfo.signaling_path);
  return connectGiznetWebRTC({
    ...options,
    prepareOffer: (offerSDP) =>
      prepareEncryptedGiznetWebRTCOffer(
        {
          clientPrivateKey: options.clientPrivateKey,
          clientPublicKey: options.clientPublicKey,
          serverPublicKey: serverInfo.public_key,
        },
        offerSDP,
      ),
    sendOffer: (offer, signal) =>
      sendGiznetWebRTCOffer(offer, {
        baseUrl: serverInfoBaseURL(options),
        fetch: options.fetch,
        signal,
        url: signalingPath,
      }),
  });
}

export async function fetchGiznetServerInfo(options: ServerInfoBootstrapOptions = {}): Promise<GiznetServerInfo> {
  const fetchImpl = options.fetch ?? globalThis.fetch;
  const response = await fetchImpl(new URL("/server-info", serverInfoBaseURL(options)), { signal: options.signal });
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    const suffix = body.trim() === "" ? "" : `: ${body.trim()}`;
    throw new Error(`server-info failed: ${response.status} ${response.statusText}${suffix}`);
  }
  const serverInfo = (await response.json()) as Partial<GiznetServerInfo>;
  if (serverInfo.protocol != null && serverInfo.protocol !== "gizclaw-webrtc") {
    throw new Error(`server-info protocol = ${serverInfo.protocol}, want gizclaw-webrtc`);
  }
  if (typeof serverInfo.public_key !== "string" || serverInfo.public_key.trim() === "") {
    throw new Error("server-info missing public_key");
  }
  const publicKey = base58Decode(serverInfo.public_key.trim());
  if (publicKey.length !== 32 || publicKey.every((byte) => byte === 0)) {
    throw new Error("server-info invalid public_key");
  }
  return {
    ...serverInfo,
    public_key: serverInfo.public_key.trim(),
    signaling_path: normalizeServerInfoSignalingPath(serverInfo.signaling_path),
  };
}

function serverInfoBaseURL(options: Pick<ServerInfoBootstrapOptions, "baseUrl" | "endpoint">): string {
  if (options.baseUrl != null) {
    return options.baseUrl;
  }
  if (options.endpoint != null) {
    return `http://${options.endpoint}`;
  }
  return typeof location === "undefined" ? "http://gizclaw.local" : location.origin;
}

function normalizeServerInfoSignalingPath(path: string | undefined): string {
  const value = path?.trim() ?? "";
  if (value === "") {
    return GIZNET_WEBRTC_SIGNALING_PATH;
  }
  if (!value.startsWith("/") || value.startsWith("//")) {
    throw new Error(`server-info invalid signaling_path ${value}`);
  }
  return value;
}

export function prepareGiznetWebRTCPeerConnection(
  pc: RTCPeerConnection,
  options: Pick<ConnectGiznetWebRTCOptions, "addAudioTransceiver" | "createPacketDataChannel"> = {},
): void {
  if (options.createPacketDataChannel !== false) {
    const packetDataChannel = pc.createDataChannel(GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL, {
      maxRetransmits: 0,
      ordered: false,
    });
    giznetPacketDataChannels.set(pc, packetDataChannel);
  } else {
    giznetPacketDataChannels.delete(pc);
  }
  if (options.addAudioTransceiver !== false && typeof pc.addTransceiver === "function") {
    pc.addTransceiver("audio", { direction: "sendrecv" });
  }
}

export function getGiznetWebRTCPacketDataChannel(pc: RTCPeerConnection): WebRTCRPCDataChannel | undefined {
  return giznetPacketDataChannels.get(pc);
}

export async function sendGiznetWebRTCTelemetry(pc: RTCPeerConnection, frame: TelemetryFrame): Promise<void> {
  const channel = getGiznetWebRTCPacketDataChannel(pc);
  if (channel == null) {
    throw new Error("giznet WebRTC packet data channel is not available");
  }
  const packet = encodeTelemetryPacket(frame);
  await new Promise<void>((resolve, reject) => {
    sendDataChannelMessage(channel, packet, reject, resolve);
  });
}

export async function sendGiznetWebRTCOffer(
  offer: PreparedGiznetWebRTCOffer,
  options: {
    baseUrl?: string;
    fetch?: typeof fetch;
    signal?: AbortSignal;
    url?: string;
  } = {},
): Promise<Blob> {
  const fetchImpl = options.fetch ?? globalThis.fetch;
  const defaultBaseUrl = typeof location === "undefined" ? "http://gizclaw.local" : location.origin;
  const requestURL = new URL(options.url ?? GIZNET_WEBRTC_SIGNALING_PATH, options.baseUrl ?? defaultBaseUrl);
  const data: CreateGiznetWebRtcOfferData = {
    body: offer.body,
    headers: {
      "X-Giznet-Nonce": offer.nonce,
      "X-Giznet-Public-Key": offer.clientPublicKey,
      "X-Giznet-Timestamp": offer.timestamp,
    },
    url: GIZNET_WEBRTC_SIGNALING_PATH,
  };
  const response = await fetchImpl(requestURL, {
    body: data.body,
    headers: {
      "Content-Type": "application/octet-stream",
      "X-Giznet-Nonce": data.headers["X-Giznet-Nonce"],
      "X-Giznet-Public-Key": data.headers["X-Giznet-Public-Key"],
      "X-Giznet-Timestamp": String(data.headers["X-Giznet-Timestamp"]),
    },
    method: "POST",
    signal: options.signal,
  });
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    const suffix = body.trim() === "" ? "" : `: ${body.trim()}`;
    throw new Error(`WebRTC signaling failed: ${response.status} ${response.statusText}${suffix}`);
  }
  return response.blob();
}

export function parseRPCResponse<TResult = unknown>(data: unknown): RPCResponse<TResult> {
  const text = typeof data === "string" ? data : data instanceof ArrayBuffer ? new TextDecoder().decode(data) : "";
  if (text === "") {
    throw new Error("empty WebRTC RPC response");
  }
  const parsed = JSON.parse(text) as RPCResponse<TResult>;
  if (parsed.error == null && !("result" in parsed)) {
    throw new Error("invalid WebRTC RPC response: missing result or error");
  }
  return parsed;
}

export function giznetServiceDataChannelLabel(service: number): string {
  if (!Number.isSafeInteger(service) || service < 0) {
    throw new Error(`invalid giznet service id: ${service}`);
  }
  return `${GIZNET_WEBRTC_SERVICE_DATA_CHANNEL_PREFIX}${service}`;
}

export function encodeRPCRequest(request: RPCRequest): ArrayBuffer {
  return concatBytes([encodeJSONFrame(request), encodeFrame(RPC_FRAME_TYPE_EOS)]);
}

export function encodeRPCResponse(response: RPCResponse): ArrayBuffer {
  return concatBytes([encodeJSONFrame(response), encodeFrame(RPC_FRAME_TYPE_EOS)]);
}

export function encodeJSONFrame(value: unknown): ArrayBuffer {
  return encodeFrame(RPC_FRAME_TYPE_JSON, new TextEncoder().encode(JSON.stringify(value)));
}

export function encodeFrame(type: number, payload: Uint8Array = new Uint8Array()): ArrayBuffer {
  if (!Number.isInteger(type) || type < 0 || type > 0xffff) {
    throw new Error(`invalid RPC frame type: ${type}`);
  }
  if (payload.length > 0xffff) {
    throw new Error(`RPC frame too large: ${payload.length}`);
  }
  if (type === RPC_FRAME_TYPE_EOS && payload.length !== 0) {
    throw new Error("RPC EOS frame must be empty.");
  }
  const out = new Uint8Array(4 + payload.length);
  const view = new DataView(out.buffer);
  view.setUint16(0, payload.length, true);
  view.setUint16(2, type, true);
  out.set(payload, 4);
  return out.buffer;
}

export function decodeFrames(data: ArrayBuffer | Uint8Array): Array<{ payload: Uint8Array; type: number }> {
  let offset = 0;
  const bytes = data instanceof Uint8Array ? data : new Uint8Array(data);
  const frames: Array<{ payload: Uint8Array; type: number }> = [];
  while (offset < bytes.length) {
    if (bytes.length - offset < 4) {
      throw new Error("incomplete RPC frame header");
    }
    const view = new DataView(bytes.buffer, bytes.byteOffset + offset, 4);
    const length = view.getUint16(0, true);
    const type = view.getUint16(2, true);
    offset += 4;
    if (bytes.length - offset < length) {
      throw new Error("incomplete RPC frame payload");
    }
    if (type === RPC_FRAME_TYPE_EOS && length !== 0) {
      throw new Error("RPC EOS frame must be empty.");
    }
    frames.push({ payload: bytes.slice(offset, offset + length), type });
    offset += length;
  }
  return frames;
}

export async function encodeHTTPRequest(request: Request, host = "gizclaw"): Promise<Uint8Array> {
  const url = new URL(request.url);
  const target = `${url.pathname}${url.search}`;
  const body = new Uint8Array(await request.arrayBuffer());
  const headers = new Headers(request.headers);
  headers.set("Host", host);
  headers.set("Connection", "close");
  headers.delete("Content-Length");
  headers.delete("Transfer-Encoding");
  if (body.length > 0) {
    headers.set("Content-Length", String(body.length));
  }
  const lines = [`${request.method} ${target || "/"} HTTP/1.1`];
  headers.forEach((value, key) => {
    lines.push(`${canonicalHTTPHeaderName(key)}: ${value}`);
  });
  lines.push("", "");
  return new Uint8Array(concatBytes([new TextEncoder().encode(lines.join("\r\n")), body]));
}

export type ParsedHTTPResponse = {
  body: Uint8Array;
  headers: Headers;
  status: number;
  statusText: string;
};

export function tryParseHTTPResponse(buffer: Uint8Array<ArrayBufferLike>, closed = false): ParsedHTTPResponse | null {
  const headerEnd = indexOfBytes(buffer, CRLFCRLF);
  if (headerEnd < 0) {
    return null;
  }
  const headerText = new TextDecoder().decode(buffer.slice(0, headerEnd));
  const lines = headerText.split("\r\n");
  const statusLine = lines.shift() ?? "";
  const match = /^HTTP\/\d(?:\.\d)?\s+(\d{3})(?:\s+(.*))?$/.exec(statusLine);
  if (match == null) {
    throw new Error(`invalid HTTP response status line: ${statusLine}`);
  }
  const headers = new Headers();
  for (const line of lines) {
    if (line === "") {
      continue;
    }
    const idx = line.indexOf(":");
    if (idx < 0) {
      throw new Error(`invalid HTTP response header: ${line}`);
    }
    headers.append(line.slice(0, idx).trim(), line.slice(idx + 1).trim());
  }
  const rawBody = buffer.slice(headerEnd + CRLFCRLF.length);
  const transferEncoding = headers.get("transfer-encoding") ?? "";
  if (/\bchunked\b/i.test(transferEncoding)) {
    const parsed = tryDecodeChunkedBody(rawBody);
    if (parsed == null) {
      return null;
    }
    return {
      body: parsed,
      headers,
      status: Number(match[1]),
      statusText: match[2] ?? "",
    };
  }
  const lengthText = headers.get("content-length");
  if (lengthText != null && lengthText !== "") {
    const length = Number.parseInt(lengthText, 10);
    if (!Number.isSafeInteger(length) || length < 0) {
      throw new Error(`invalid HTTP response content-length: ${lengthText}`);
    }
    if (rawBody.length < length) {
      return null;
    }
    return {
      body: rawBody.slice(0, length),
      headers,
      status: Number(match[1]),
      statusText: match[2] ?? "",
    };
  }
  if (!closed) {
    return null;
  }
  return {
    body: rawBody,
    headers,
    status: Number(match[1]),
    statusText: match[2] ?? "",
  };
}

function tryReadRPCResponse<TResult>(
  buffer: Uint8Array<ArrayBufferLike>,
): { response: RPCResponse<TResult>; rest: Uint8Array<ArrayBufferLike> } | null {
  let offset = 0;
  let response: RPCResponse<TResult> | undefined;
  for (;;) {
    if (buffer.length - offset < 4) {
      return null;
    }
    const view = new DataView(buffer.buffer, buffer.byteOffset + offset, 4);
    const length = view.getUint16(0, true);
    const type = view.getUint16(2, true);
    if (buffer.length - offset - 4 < length) {
      return null;
    }
    offset += 4;
    const payload = buffer.slice(offset, offset + length);
    offset += length;
    if (type === RPC_FRAME_TYPE_EOS) {
      if (length !== 0) {
        throw new Error("RPC EOS frame must be empty.");
      }
      if (response == null) {
        throw new Error("RPC response EOS before JSON frame.");
      }
      return { response, rest: buffer.slice(offset) };
    }
    if (type !== RPC_FRAME_TYPE_JSON) {
      throw new Error(`rpc: expected JSON frame, got type ${type}`);
    }
    if (response != null) {
      throw new Error("RPC response contains multiple JSON frames.");
    }
    response = parseRPCResponse<TResult>(new TextDecoder().decode(payload));
  }
}

function tryReadRPCBinaryResponse<TResult>(
  buffer: Uint8Array<ArrayBufferLike>,
): { body: Uint8Array; response: RPCResponse<TResult>; rest: Uint8Array<ArrayBufferLike> } | null {
  let offset = 0;
  let response: RPCResponse<TResult> | undefined;
  const body: Uint8Array[] = [];
  for (;;) {
    if (buffer.length - offset < 4) {
      return null;
    }
    const view = new DataView(buffer.buffer, buffer.byteOffset + offset, 4);
    const length = view.getUint16(0, true);
    const type = view.getUint16(2, true);
    if (buffer.length - offset - 4 < length) {
      return null;
    }
    offset += 4;
    const payload = buffer.slice(offset, offset + length);
    offset += length;
    if (type === RPC_FRAME_TYPE_EOS) {
      if (length !== 0) {
        throw new Error("RPC EOS frame must be empty.");
      }
      if (response == null) {
        throw new Error("RPC binary response EOS before JSON frame.");
      }
      return { body: concatByteArrays(body), response, rest: buffer.slice(offset) };
    }
    if (response == null) {
      if (type !== RPC_FRAME_TYPE_JSON) {
        throw new Error(`rpc: expected JSON frame, got type ${type}`);
      }
      response = parseRPCResponse<TResult>(new TextDecoder().decode(payload));
      continue;
    }
    if (type !== RPC_FRAME_TYPE_BINARY) {
      throw new Error(`rpc: expected binary frame, got type ${type}`);
    }
    body.push(copyBytes(payload));
  }
}

export function waitForICEGatheringComplete(pc: RTCPeerConnection, signal?: AbortSignal): Promise<void> {
  if (pc.iceGatheringState === "complete") {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    let settled = false;
    const cleanup = (): void => {
      signal?.removeEventListener("abort", onAbort);
      pc.removeEventListener("icegatheringstatechange", onStateChange);
    };
    const complete = (): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      resolve();
    };
    const onAbort = (): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      reject(abortError());
    };
    const onStateChange = (): void => {
      if (pc.iceGatheringState === "complete") {
        complete();
      }
    };
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    pc.addEventListener("icegatheringstatechange", onStateChange);
    if (pc.iceGatheringState === "complete") {
      complete();
    }
  });
}

const CRLF = new TextEncoder().encode("\r\n");
const CRLFCRLF = new TextEncoder().encode("\r\n\r\n");

function readHTTPResponse(channel: WebRTCRPCDataChannel, requestBytes: Uint8Array, signal?: AbortSignal, timeoutMs = 30000): Promise<Response> {
  return new Promise<Response>((resolve, reject) => {
    let buffer: Uint8Array<ArrayBufferLike> = new Uint8Array();
    let messageQueue = Promise.resolve();
    let settled = false;
    let timeout: ReturnType<typeof setTimeout> | undefined;

    const settle = (fn: () => void): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      try {
        channel.close();
      } catch {
        // Ignore close races.
      }
      fn();
    };
    const cleanup = (): void => {
      if (timeout != null) {
        clearTimeout(timeout);
      }
      signal?.removeEventListener("abort", onAbort);
      channel.removeEventListener("open", onOpen);
      channel.removeEventListener("message", onMessage);
      channel.removeEventListener("error", onError);
      channel.removeEventListener("close", onClose);
    };
    const resolveParsed = (closed = false): boolean => {
      const parsed = tryParseHTTPResponse(buffer, closed);
      if (parsed == null) {
        return false;
      }
      settle(() =>
        resolve(
          new Response(arrayBufferFromBytes(parsed.body), {
            headers: parsed.headers,
            status: parsed.status,
            statusText: parsed.statusText,
          }),
        ),
      );
      return true;
    };
    const onAbort = (): void => {
      settle(() => reject(abortError()));
    };
    const onOpen = (): void => {
      sendDataChannelMessage(channel, requestBytes, (err) => settle(() => reject(err)));
    };
    const onMessage = (event: MessageEvent): void => {
      messageQueue = messageQueue.then(async () => {
        if (settled) {
          return;
        }
        try {
          buffer = appendBytes(buffer, await messageDataBytes(event.data));
          resolveParsed(false);
        } catch (err) {
          settle(() => reject(err));
        }
      });
    };
    const onError = (): void => {
      settle(() => reject(new Error("WebRTC HTTP service data channel failed.")));
    };
    const onClose = (): void => {
      messageQueue = messageQueue.then(() => {
        try {
          if (!settled && !resolveParsed(true)) {
            settle(() => reject(new Error("WebRTC HTTP service data channel closed before response.")));
          }
        } catch (err) {
          settle(() => reject(err));
        }
      });
    };

    if (signal?.aborted) {
      settle(() => reject(abortError()));
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    channel.addEventListener("open", onOpen);
    channel.addEventListener("message", onMessage);
    channel.addEventListener("error", onError);
    channel.addEventListener("close", onClose);
    if (timeoutMs > 0) {
      timeout = setTimeout(() => {
        settle(() => reject(new Error(`WebRTC HTTP service request timed out after ${timeoutMs}ms.`)));
      }, timeoutMs);
    }
    if (channel.readyState === "open") {
      onOpen();
    } else if (channel.readyState === "closed") {
      onClose();
    }
  });
}

function defaultRPCID(): string {
  return `webrtc-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function abortError(): Error {
  if (typeof DOMException !== "undefined") {
    return new DOMException("The operation was aborted.", "AbortError");
  }
  const err = new Error("The operation was aborted.");
  err.name = "AbortError";
  return err;
}

function sendDataChannelMessage(
  channel: WebRTCRPCDataChannel,
  payload: DataChannelPayload,
  onError: (err: unknown) => void,
  onSent: () => void = () => {},
): void {
  let retries = 0;
  const send = (): void => {
    if (channel.readyState === "open") {
      try {
        channel.send(payload);
        onSent();
      } catch (err) {
        onError(err);
      }
      return;
    }
    if (channel.readyState === "connecting" && retries < DATA_CHANNEL_SEND_RETRY_LIMIT) {
      retries += 1;
      setTimeout(send, DATA_CHANNEL_SEND_RETRY_DELAY_MS);
      return;
    }
    onError(new Error(`RTCDataChannel.readyState is ${JSON.stringify(channel.readyState)}, want "open"`));
  };
  send();
}

async function messageDataBytes(data: unknown): Promise<Uint8Array> {
  if (data instanceof ArrayBuffer) {
    return new Uint8Array(data);
  }
  if (ArrayBuffer.isView(data)) {
    return copyBytes(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
  }
  if (typeof Blob !== "undefined" && data instanceof Blob) {
    return new Uint8Array(await data.arrayBuffer());
  }
  if (typeof data === "string") {
    return new TextEncoder().encode(data);
  }
  throw new Error("unsupported WebRTC data channel message type");
}

function appendBytes(left: Uint8Array, right: Uint8Array): Uint8Array {
  if (left.length === 0) {
    return copyBytes(right);
  }
  const out = new Uint8Array(left.length + right.length);
  out.set(left, 0);
  out.set(right, left.length);
  return out;
}

function copyBytes(data: Uint8Array): Uint8Array {
  const out = new Uint8Array(data.byteLength);
  out.set(data);
  return out;
}

function arrayBufferFromBytes(data: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(data.byteLength);
  new Uint8Array(out).set(data);
  return out;
}

function concatByteArrays(parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function concatBytes(parts: Array<ArrayBuffer | Uint8Array>): ArrayBuffer {
  const arrays = parts.map((part) => (part instanceof Uint8Array ? part : new Uint8Array(part)));
  const out = new Uint8Array(arrays.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of arrays) {
    out.set(part, offset);
    offset += part.length;
  }
  return out.buffer;
}

function indexOfBytes(haystack: Uint8Array, needle: Uint8Array, start = 0): number {
  if (needle.length === 0) {
    return start;
  }
  outer: for (let i = Math.max(0, start); i <= haystack.length - needle.length; i += 1) {
    for (let j = 0; j < needle.length; j += 1) {
      if (haystack[i + j] !== needle[j]) {
        continue outer;
      }
    }
    return i;
  }
  return -1;
}

function tryDecodeChunkedBody(data: Uint8Array): Uint8Array | null {
  const chunks: Uint8Array[] = [];
  let offset = 0;
  for (;;) {
    const lineEnd = indexOfBytes(data, CRLF, offset);
    if (lineEnd < 0) {
      return null;
    }
    const line = new TextDecoder().decode(data.slice(offset, lineEnd));
    const sizeText = line.split(";", 1)[0]?.trim() ?? "";
    const size = Number.parseInt(sizeText, 16);
    if (!Number.isSafeInteger(size) || size < 0) {
      throw new Error(`invalid HTTP chunk size: ${line}`);
    }
    offset = lineEnd + CRLF.length;
    if (size === 0) {
      const trailerEnd = indexOfBytes(data, CRLFCRLF, offset);
      const hasEmptyTrailer = data.length >= offset + CRLF.length && data[offset] === 13 && data[offset + 1] === 10;
      if (trailerEnd >= 0 || hasEmptyTrailer) {
        return new Uint8Array(concatBytes(chunks));
      }
      return null;
    }
    if (data.length < offset + size + CRLF.length) {
      return null;
    }
    chunks.push(data.slice(offset, offset + size));
    offset += size;
    if (data[offset] !== 13 || data[offset + 1] !== 10) {
      throw new Error("invalid HTTP chunk terminator");
    }
    offset += CRLF.length;
  }
}

function canonicalHTTPHeaderName(name: string): string {
  return name
    .split("-")
    .map((part) => (part === "" ? part : `${part.slice(0, 1).toUpperCase()}${part.slice(1).toLowerCase()}`))
    .join("-");
}
