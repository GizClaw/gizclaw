import type { CreateGiznetWebRtcOfferData } from "@gizclaw/serverpublic";

export const WEBRTC_RPC_DATA_CHANNEL_LABEL = "rpc";
export const WEBRTC_EVENT_DATA_CHANNEL_LABEL = "event";
export const GIZNET_WEBRTC_SIGNALING_PATH = "/giznet/webrtc/v1/offer";
export const LOCAL_BRIDGE_WEBRTC_SIGNALING_PATH = "/webrtc/offer";
export const RPC_VERSION = 1;

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
  send(data: string): void;
};

export type WebRTCRPCDataChannelFactory = {
  createDataChannel(label: string, options?: RTCDataChannelInit): WebRTCRPCDataChannel;
};

export type WebRTCOfferClient = {
  createOffer(): Promise<RTCSessionDescriptionInit>;
  localDescription: RTCSessionDescription | null;
  setLocalDescription(description: RTCSessionDescriptionInit): Promise<void>;
  setRemoteDescription(description: RTCSessionDescriptionInit): Promise<void>;
};

export type ConnectWebRTCOptions = {
  fetch?: typeof fetch;
  offerPath?: string;
  offerURL?: string;
  pc: RTCPeerConnection;
  signal?: AbortSignal;
};

export type PreparedGiznetWebRTCOffer = {
  body: Blob | File;
  clientPublicKey: string;
  nonce: string;
  openAnswer: (encryptedAnswer: Blob) => Promise<string>;
  timestamp: number;
};

export type ConnectGiznetWebRTCOptions = {
  fetch?: typeof fetch;
  pc: RTCPeerConnection;
  prepareOffer: (offerSDP: string) => Promise<PreparedGiznetWebRTCOffer>;
  sendOffer?: (offer: PreparedGiznetWebRTCOffer, signal?: AbortSignal) => Promise<Blob>;
  signal?: AbortSignal;
};

export type WebRTCRPCClientOptions = {
  channelLabel?: string;
  createID?: () => string;
  requestTimeoutMs?: number;
};

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
    this.channelLabel = options.channelLabel ?? WEBRTC_RPC_DATA_CHANNEL_LABEL;
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

  async request<TResult = unknown, TParams = unknown>(request: RPCRequest<TParams>, options: RPCCallOptions = {}): Promise<RPCResponse<TResult>> {
    const channel = this.pc.createDataChannel(`${this.channelLabel}:${request.id}`, { ordered: true });
    channel.binaryType = "arraybuffer";

    const timeoutMs = options.timeoutMs ?? this.requestTimeoutMs;
    const abortSignal = options.signal;

    return new Promise<RPCResponse<TResult>>((resolve, reject) => {
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
        try {
          channel.send(JSON.stringify(request));
        } catch (err) {
          settle(() => reject(err));
        }
      };
      const onMessage = (event: MessageEvent): void => {
        try {
          const response = parseRPCResponse<TResult>(event.data);
          if (response.id != null && response.id !== request.id) {
            throw new Error(`rpc response id mismatch: got ${response.id}, want ${request.id}`);
          }
          settle(() => resolve(response));
        } catch (err) {
          settle(() => reject(err));
        }
      };
      const onError = (): void => {
        settle(() => reject(new Error("WebRTC RPC data channel failed.")));
      };
      const onClose = (): void => {
        settle(() => reject(new Error("WebRTC RPC data channel closed before response.")));
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

export type WorkspaceRunSetRequest = {
  workspace_name: string;
};

export type WorkspaceHistoryPlayRequest = {
  history_id: string;
};

export type WorkspaceHistoryRequest = {
  cursor?: string;
  limit?: number;
  order?: string;
  workspace_name?: string;
};

export type WorkspaceHistoryGetRequest = {
  history_id: string;
  workspace_name: string;
};

export type WorkspaceRecallRequest = {
  limit?: number;
  query: string;
};

export class WorkspaceRPC {
  private readonly client: WebRTCRPCClient;

  constructor(client: WebRTCRPCClient) {
    this.client = client;
  }

  getRunWorkspace<TResult = unknown>(options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult>("server.run.workspace.get", {}, options);
  }

  setRunWorkspace<TResult = unknown>(request: WorkspaceRunSetRequest, options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult, WorkspaceRunSetRequest>("server.run.workspace.set", request, options);
  }

  reloadRunWorkspace<TResult = unknown>(options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult>("server.run.workspace.reload", {}, options);
  }

  listRunWorkspaceHistory<TResult = unknown>(request: WorkspaceHistoryRequest = {}, options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult, WorkspaceHistoryRequest>("server.run.workspace.history", request, options);
  }

  playRunWorkspaceHistory<TResult = unknown>(request: WorkspaceHistoryPlayRequest, options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult, WorkspaceHistoryPlayRequest>("server.run.workspace.history.play", request, options);
  }

  getRunWorkspaceMemoryStats<TResult = unknown>(options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult>("server.run.workspace.memory.stats", {}, options);
  }

  recallRunWorkspaceMemory<TResult = unknown>(request: WorkspaceRecallRequest, options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult, WorkspaceRecallRequest>("server.run.workspace.recall", request, options);
  }

  listWorkspaceHistory<TResult = unknown>(request: WorkspaceHistoryRequest, options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult, WorkspaceHistoryRequest>("server.workspace.history.list", request, options);
  }

  getWorkspaceHistory<TResult = unknown>(request: WorkspaceHistoryGetRequest, options?: RPCCallOptions): Promise<TResult> {
    return this.client.call<TResult, WorkspaceHistoryGetRequest>("server.workspace.history.get", request, options);
  }
}

export async function connectLocalBridgeWebRTC(options: ConnectWebRTCOptions): Promise<RTCPeerConnection> {
  const fetchImpl = options.fetch ?? globalThis.fetch;
  const offerURL = options.offerURL ?? options.offerPath ?? LOCAL_BRIDGE_WEBRTC_SIGNALING_PATH;

  const offer = await options.pc.createOffer();
  await options.pc.setLocalDescription(offer);
  await waitForICEGatheringComplete(options.pc, options.signal);
  const local = options.pc.localDescription;
  if (local == null) {
    throw new Error("WebRTC offer was not created.");
  }

  const response = await fetchImpl(offerURL, {
    body: JSON.stringify({ sdp: local.sdp, type: local.type }),
    headers: { "content-type": "application/json" },
    method: "POST",
    signal: options.signal,
  });
  if (!response.ok) {
    throw new Error(`WebRTC offer failed: ${response.status} ${response.statusText}`);
  }
  const answer = (await response.json()) as RTCSessionDescriptionInit;
  await options.pc.setRemoteDescription(answer);
  return options.pc;
}

export async function connectGiznetWebRTC(options: ConnectGiznetWebRTCOptions): Promise<RTCPeerConnection> {
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
    throw new Error(`WebRTC signaling failed: ${response.status} ${response.statusText}`);
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

export function waitForICEGatheringComplete(pc: RTCPeerConnection, signal?: AbortSignal): Promise<void> {
  if (pc.iceGatheringState === "complete") {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const cleanup = (): void => {
      signal?.removeEventListener("abort", onAbort);
      pc.removeEventListener("icegatheringstatechange", onStateChange);
    };
    const onAbort = (): void => {
      cleanup();
      reject(abortError());
    };
    const onStateChange = (): void => {
      if (pc.iceGatheringState === "complete") {
        cleanup();
        resolve();
      }
    };
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    pc.addEventListener("icegatheringstatechange", onStateChange);
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
