import type { CreateGiznetWebRtcOfferData } from "./generated/peerhttp/types.gen";
import { RPC_METHOD_IDS } from "./generated/rpc/method-map.ts";
import {
  decodeRPCRequestPayload,
  decodeRPCResponsePayload,
  encodeRPCRequestPayload,
  encodeRPCResponsePayload,
  type PingRequest,
  type SpeechSynthesizeRequest,
  type SpeechSynthesizeResponse,
  type SpeechTranscribeRequest,
  type SpeechTranscribeResponse,
  type SpeedTestRequest,
} from "./generated/rpc/payload-codec.ts";
import { base58Decode, prepareEncryptedGiznetWebRTCOffer } from "./signaling.ts";
import { encodeTelemetryPacket, type TelemetryFrame } from "./telemetry.ts";
export * from "./telemetry.ts";

export const WEBRTC_RPC_DATA_CHANNEL_LABEL = "rpc";
export const WEBRTC_EVENT_DATA_CHANNEL_LABEL = "event";
export const GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL = "giznet/v1/packet";
export const GIZNET_WEBRTC_SERVICE_DATA_CHANNEL_PREFIX = "giznet/v1/service/";
export const GIZNET_WEBRTC_SIGNALING_PATH = "/webrtc/v1/offer";
export const RPC_VERSION = 1;
export const GIZCLAW_SERVICE_PEER_RPC = 0x00;
export const GIZCLAW_SERVICE_PEER_HTTP = 0x01;
export const GIZCLAW_SERVICE_PEER_OPENAI = 0x02;
export const GIZCLAW_SERVICE_ADMIN_HTTP = 0x10;
export const GIZCLAW_EVENT_STREAM_AGENT = 0x20;
export const GIZCLAW_SERVICE_EDGE_RPC = 0x31;
export const GIZCLAW_MEDIA_STREAM_OPUS = "audio/opus";
export const GIZCLAW_PACKET_STAMPED_OPUS = 0x10;
export const RPC_FRAME_TYPE_EOS = 0;
export const RPC_FRAME_TYPE_JSON = 1;
export const RPC_FRAME_TYPE_BINARY = 2;
export const RPC_FRAME_TYPE_TEXT = 3;
export const SPEECH_TRANSCRIPTION_REQUEST_TIMEOUT_MS = 80000;
export const SPEECH_SYNTHESIS_REQUEST_TIMEOUT_MS = 125000;
const RPC_MAX_FRAME_PAYLOAD_SIZE = 0xffff;
const RPC_MAX_ENVELOPE_SIZE = RPC_MAX_FRAME_PAYLOAD_SIZE * 16;
const PACKET_DATA_CHANNEL_OPEN_TIMEOUT_MS = 30000;
const RPC_SPEED_TEST_FRAME_SIZE = 32 * 1024;
const RPC_SPEED_TEST_MAX_CONTENT_LENGTH = 1 << 30;
const RPC_BODY_BUFFER_HIGH_WATER_MARK = 1024 * 1024;
const SERVICE_DATA_CHANNEL_MESSAGE_CHUNK_SIZE = 1400;
const SERVICE_DATA_CHANNEL_BUFFER_HIGH_WATER_MARK = 1024 * 1024;
const SERVICE_DATA_CHANNEL_BUFFER_LOW_WATER_MARK = 256 * 1024;
const SERVICE_DATA_CHANNEL_WRITE_TIMEOUT_MS = 30000;
const giznetPacketDataChannels = new WeakMap<object, WebRTCRPCDataChannel>();
const giznetRPCServers = new WeakSet<object>();
const rpcMethodNamesByID = new Map<number, string>(Object.entries(RPC_METHOD_IDS).map(([name, id]) => [id, name]));

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

export type RPCStreamingCallResult<TResult> = {
  body: AsyncIterable<Uint8Array>;
  cancel: () => void;
  result: TResult;
};

export type WebRTCRPCDataChannel = {
  addEventListener(type: "open", listener: () => void): void;
  addEventListener(type: "message", listener: (event: MessageEvent) => void): void;
  addEventListener(type: "error", listener: () => void): void;
  addEventListener(type: "close", listener: () => void): void;
  addEventListener(type: "bufferedamountlow", listener: () => void): void;
  binaryType?: BinaryType;
  bufferedAmount?: number;
  bufferedAmountLowThreshold?: number;
  close(): void;
  label?: string;
  readyState: RTCDataChannelState;
  removeEventListener(type: "open", listener: () => void): void;
  removeEventListener(type: "message", listener: (event: MessageEvent) => void): void;
  removeEventListener(type: "error", listener: () => void): void;
  removeEventListener(type: "close", listener: () => void): void;
  removeEventListener(type: "bufferedamountlow", listener: () => void): void;
  send(data: ArrayBuffer | ArrayBufferView | Blob | string): void;
};

export type WebRTCRPCDataChannelFactory = {
  createDataChannel(label: string, options?: RTCDataChannelInit): WebRTCRPCDataChannel;
};

export type WebRTCRPCDataChannelServer = {
  addEventListener(type: "datachannel", listener: (event: { channel: WebRTCRPCDataChannel }) => void): void;
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
  iceGatheringTimeoutMs?: number;
  pc: RTCPeerConnection;
  prepareOffer: (offerSDP: string) => Promise<PreparedGiznetWebRTCOffer>;
  sendOffer?: (offer: PreparedGiznetWebRTCOffer, signal?: AbortSignal) => Promise<Blob>;
  signal?: AbortSignal;
};

export type SendGiznetWebRTCTelemetryOptions = {
  signal?: AbortSignal;
  timeoutMs?: number;
};

export type GiznetServerInfo = {
  endpoint?: string;
  ice_servers?: RTCIceServer[];
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
    this.channelLabel = options.channelLabel ?? giznetServiceDataChannelLabel(options.service ?? GIZCLAW_SERVICE_PEER_RPC);
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

  transcribeSpeech(
    params: SpeechTranscribeRequest,
    audio: AsyncIterable<Uint8Array> | Iterable<Uint8Array>,
    options: RPCCallOptions = {},
  ): Promise<SpeechTranscribeResponse> {
    return rpcUploadCall<SpeechTranscribeResponse>(
      this.pc,
      this.channelLabel,
      "server.speech.transcribe",
      params,
      audio,
      {
        ...options,
        id: options.id ?? this.createID(),
        timeoutMs: options.timeoutMs ?? Math.max(this.requestTimeoutMs, SPEECH_TRANSCRIPTION_REQUEST_TIMEOUT_MS),
      },
    );
  }

  synthesizeSpeech(
    params: SpeechSynthesizeRequest,
    options: RPCCallOptions = {},
  ): Promise<RPCStreamingCallResult<SpeechSynthesizeResponse>> {
    return rpcStreamingResponseCall<SpeechSynthesizeResponse>(
      this.pc,
      this.channelLabel,
      "server.speech.synthesize",
      params,
      {
        ...options,
        id: options.id ?? this.createID(),
        timeoutMs: options.timeoutMs ?? Math.max(this.requestTimeoutMs, SPEECH_SYNTHESIS_REQUEST_TIMEOUT_MS),
      },
    );
  }

  async request<TResult = unknown, TParams = unknown>(request: RPCRequest<TParams>, options: RPCCallOptions = {}): Promise<RPCResponse<TResult>> {
    const channel = this.pc.createDataChannel(this.channelLabel, { ordered: true });
    channel.binaryType = "arraybuffer";
    let encodedRequest: ArrayBuffer;
    try {
      encodedRequest = encodeRPCRequest(request);
    } catch (err) {
      try {
        channel.close();
      } catch {
        // Ignore close races from browsers that already closed the channel.
      }
      throw err;
    }

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
        sendDataChannelMessage(
          channel,
          encodedRequest,
          (err) => settle(() => reject(err)),
          { signal: abortSignal, timeoutMs },
        );
      };
      const onMessage = (event: MessageEvent): void => {
        messageQueue = messageQueue.then(async () => {
          if (settled) {
            return;
          }
          try {
            const chunk = await messageDataBytes(event.data);
            buffer = appendBytes(buffer, chunk);
            const parsed = tryReadRPCResponse<TResult>(buffer, request.method);
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
    let encodedRequest: ArrayBuffer;
    try {
      encodedRequest = encodeRPCRequest(request);
    } catch (err) {
      try {
        channel.close();
      } catch {
        // Ignore close races from browsers that already closed the channel.
      }
      throw err;
    }

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
        sendDataChannelMessage(
          channel,
          encodedRequest,
          (err) => settle(() => reject(err)),
          { signal: abortSignal, timeoutMs },
        );
      };
      const onMessage = (event: MessageEvent): void => {
        messageQueue = messageQueue.then(async () => {
          if (settled) {
            return;
          }
          try {
            const chunk = await messageDataBytes(event.data);
            buffer = appendBytes(buffer, chunk);
            const parsed = tryReadRPCBinaryResponse<TResult>(buffer, request.method);
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

async function rpcUploadCall<TResult>(
  pc: WebRTCRPCDataChannelFactory,
  channelLabel: string,
  method: string,
  params: unknown,
  body: AsyncIterable<Uint8Array> | Iterable<Uint8Array>,
  options: Required<Pick<RPCCallOptions, "id" | "timeoutMs">> & RPCCallOptions,
): Promise<TResult> {
  const channel = pc.createDataChannel(channelLabel, { ordered: true });
  channel.binaryType = "arraybuffer";
  const request: RPCRequest = {
    id: options.id,
    method,
    params,
    v: RPC_VERSION,
  };
  const response = readRPCUnaryResponse<TResult>(
    channel,
    method,
    request.id,
    options.signal,
    options.timeoutMs,
  );
  const responseOutcome = response.promise.then((value) => ({ kind: "response" as const, value }));
  const iterator = Symbol.asyncIterator in body
    ? body[Symbol.asyncIterator]()
    : body[Symbol.iterator]();
  let uploadComplete = false;
  try {
    await waitForDataChannelOpen(channel, options.signal, options.timeoutMs);
    const requestEnvelope = encodeRPCRequestEnvelope(request);
    await sendInboundRPCFrames(channel, encodeRPCEnvelopeFrames(requestEnvelope), options);
    if (requestEnvelope.length > RPC_MAX_FRAME_PAYLOAD_SIZE) {
      await sendInboundRPCFrames(channel, [encodeFrame(RPC_FRAME_TYPE_EOS)], options);
    }
    for (;;) {
      const outcome = await Promise.race([
        Promise.resolve(iterator.next()).then((result) => ({ kind: "chunk" as const, result })),
        responseOutcome,
      ]);
      if (outcome.kind === "response") {
        return outcome.value;
      }
      if (outcome.result.done) break;
      const chunk = outcome.result.value;
      if (!(chunk instanceof Uint8Array) || chunk.length === 0) {
        throw new Error("speech audio chunks must be non-empty Uint8Array values");
      }
      for (let offset = 0; offset < chunk.length; offset += RPC_MAX_FRAME_PAYLOAD_SIZE) {
        await sendInboundRPCFrames(channel, [
          encodeFrame(
            RPC_FRAME_TYPE_BINARY,
            chunk.subarray(offset, offset + RPC_MAX_FRAME_PAYLOAD_SIZE),
          ),
        ], options);
      }
    }
    uploadComplete = true;
    await sendInboundRPCFrames(channel, [encodeFrame(RPC_FRAME_TYPE_EOS)], options);
    return await response.promise;
  } catch (error) {
    response.cancel(error);
    await response.promise.catch(() => undefined);
    try {
      channel.close();
    } catch {
      // Ignore close races while unwinding a failed upload.
    }
    throw error;
  } finally {
    if (!uploadComplete && iterator.return != null) {
      await Promise.resolve(iterator.return()).catch(() => undefined);
    }
  }
}

function readRPCUnaryResponse<TResult>(
  channel: WebRTCRPCDataChannel,
  method: string,
  requestID: string,
  signal: AbortSignal | undefined,
  timeoutMs: number,
): { cancel: (reason: unknown) => void; promise: Promise<TResult> } {
  let cancel!: (reason: unknown) => void;
  const promise = new Promise<TResult>((resolve, reject) => {
    let buffer: Uint8Array<ArrayBufferLike> = new Uint8Array();
    let queue = Promise.resolve();
    let settled = false;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    const cleanup = (): void => {
      if (timeout != null) clearTimeout(timeout);
      signal?.removeEventListener("abort", onAbort);
      channel.removeEventListener("message", onMessage);
      channel.removeEventListener("error", onError);
      channel.removeEventListener("close", onClose);
    };
    const settle = (action: () => void): void => {
      if (settled) return;
      settled = true;
      cleanup();
      try {
        channel.close();
      } catch {
        // Ignore close races after a terminal response.
      }
      action();
    };
    const fail = (error: unknown): void =>
      settle(() => reject(error instanceof Error ? error : new Error(String(error))));
    cancel = fail;
    const onAbort = (): void => fail(abortError());
    const onError = (): void => fail(new Error("WebRTC RPC data channel failed."));
    const onClose = (): void => {
      queue = queue.then(() => {
        if (!settled) fail(new Error("WebRTC RPC data channel closed before response EOS."));
      });
    };
    const onMessage = (event: MessageEvent): void => {
      queue = queue.then(async () => {
        if (settled) return;
        try {
          buffer = appendBytes(buffer, await messageDataBytes(event.data));
          const parsed = tryReadRPCResponse<TResult>(buffer, method);
          if (parsed == null) return;
          if (parsed.response.id != null && parsed.response.id !== requestID) {
            throw new Error(`rpc response id mismatch: got ${parsed.response.id}, want ${requestID}`);
          }
          if (parsed.response.error != null) {
            throw new WebRTCRPCError(parsed.response.error, parsed.response.id);
          }
          settle(() => resolve(parsed.response.result as TResult));
        } catch (error) {
          fail(error);
        }
      });
    };
    if (signal?.aborted) {
      fail(abortError());
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    channel.addEventListener("message", onMessage);
    channel.addEventListener("error", onError);
    channel.addEventListener("close", onClose);
    if (timeoutMs > 0) {
      timeout = setTimeout(
        () => fail(new Error(`WebRTC RPC request timed out after ${timeoutMs}ms.`)),
        timeoutMs,
      );
    }
  });
  return { cancel, promise };
}

async function rpcStreamingResponseCall<TResult>(
  pc: WebRTCRPCDataChannelFactory,
  channelLabel: string,
  method: string,
  params: unknown,
  options: Required<Pick<RPCCallOptions, "id" | "timeoutMs">> & RPCCallOptions,
): Promise<RPCStreamingCallResult<TResult>> {
  const channel = pc.createDataChannel(channelLabel, { ordered: true });
  channel.binaryType = "arraybuffer";
  const request: RPCRequest = {
    id: options.id,
    method,
    params,
    v: RPC_VERSION,
  };
  const stream = new RPCBodyStream((reason) => cancel(reason ?? abortError()));
  let buffer: Uint8Array<ArrayBufferLike> = new Uint8Array();
  let response: RPCResponse<TResult> | undefined;
  let envelopeLength = 0;
  const envelopeChunks: Uint8Array[] = [];
  let queue = Promise.resolve();
  let terminal = false;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  let resolveMetadata!: (value: TResult) => void;
  let rejectMetadata!: (reason: unknown) => void;
  const metadata = new Promise<TResult>((resolve, reject) => {
    resolveMetadata = resolve;
    rejectMetadata = reject;
  });
  const cleanup = (): void => {
    if (timeout != null) clearTimeout(timeout);
    options.signal?.removeEventListener("abort", onAbort);
    channel.removeEventListener("message", onMessage);
    channel.removeEventListener("error", onError);
    channel.removeEventListener("close", onClose);
  };
  function cancel(reason: unknown): void {
    if (terminal) return;
    terminal = true;
    cleanup();
    const error = reason instanceof Error ? reason : new Error(String(reason));
    rejectMetadata(error);
    stream.fail(error);
    try {
      channel.close();
    } catch {
      // Ignore close races after cancellation.
    }
  }
  const onAbort = (): void => cancel(abortError());
  const onError = (): void => cancel(new Error("WebRTC RPC data channel failed."));
  const onClose = (): void => {
    queue = queue.then(() => {
      if (!terminal) cancel(new Error("WebRTC RPC body closed without EOS."));
    });
  };
  const acceptEnvelope = (payload: Uint8Array): void => {
    response = decodeRPCResponseEnvelope<TResult>(payload, method);
    if (response.id != null && response.id !== request.id) {
      throw new Error(`rpc response id mismatch: got ${response.id}, want ${request.id}`);
    }
    if (response.error != null) {
      throw new WebRTCRPCError(response.error, response.id);
    }
    resolveMetadata(response.result as TResult);
  };
  const processFrames = (): void => {
    for (;;) {
      const parsed = tryReadFrame(buffer);
      if (parsed == null) return;
      buffer = parsed.rest;
      const { type, payload } = parsed.frame;
      if (response == null) {
        if (type === RPC_FRAME_TYPE_TEXT) {
          envelopeLength += payload.length;
          if (envelopeLength > RPC_MAX_ENVELOPE_SIZE) {
            throw new Error(`RPC protobuf envelope too large: ${envelopeLength}`);
          }
          envelopeChunks.push(copyBytes(payload));
          continue;
        }
        if (type === RPC_FRAME_TYPE_EOS && envelopeChunks.length > 0) {
          acceptEnvelope(concatByteArrays(envelopeChunks));
          continue;
        }
        if (type !== RPC_FRAME_TYPE_BINARY) {
          throw new Error(`rpc: expected protobuf binary frame, got type ${type}`);
        }
        acceptEnvelope(payload);
        continue;
      }
      if (type === RPC_FRAME_TYPE_BINARY) {
        stream.push(copyBytes(payload));
        continue;
      }
      if (type !== RPC_FRAME_TYPE_EOS) {
        throw new Error(`rpc: expected speech body frame, got type ${type}`);
      }
      terminal = true;
      cleanup();
      stream.finish();
      try {
        channel.close();
      } catch {
        // Ignore close races after successful EOS.
      }
      return;
    }
  };
  const onMessage = (event: MessageEvent): void => {
    queue = queue.then(async () => {
      if (terminal) return;
      try {
        buffer = appendBytes(buffer, await messageDataBytes(event.data));
        processFrames();
      } catch (error) {
        cancel(error);
      }
    });
  };
  if (options.signal?.aborted) {
    cancel(abortError());
    return Promise.reject(abortError());
  }
  options.signal?.addEventListener("abort", onAbort, { once: true });
  channel.addEventListener("message", onMessage);
  channel.addEventListener("error", onError);
  channel.addEventListener("close", onClose);
  if (options.timeoutMs > 0) {
    timeout = setTimeout(
      () => cancel(new Error(`WebRTC RPC request timed out after ${options.timeoutMs}ms.`)),
      options.timeoutMs,
    );
  }
  try {
    await waitForDataChannelOpen(channel, options.signal, options.timeoutMs);
    await sendInboundRPCFrames(channel, [
      ...encodeRPCEnvelopeFrames(encodeRPCRequestEnvelope(request)),
      encodeFrame(RPC_FRAME_TYPE_EOS),
    ], options);
    return {
      body: stream,
      cancel: () => cancel(abortError()),
      result: await metadata,
    };
  } catch (error) {
    cancel(error);
    throw error;
  }
}

class RPCBodyStream implements AsyncIterable<Uint8Array> {
  private readonly chunks: Uint8Array[] = [];
  private readonly waiters: Array<{
    reject: (reason: unknown) => void;
    resolve: (value: IteratorResult<Uint8Array>) => void;
  }> = [];
  private bufferedBytes = 0;
  private done = false;
  private error: Error | undefined;
  private readonly onCancel: (reason?: Error) => void;

  constructor(onCancel: (reason?: Error) => void) {
    this.onCancel = onCancel;
  }

  push(chunk: Uint8Array): void {
    if (this.done) return;
    const waiter = this.waiters.shift();
    if (waiter != null) {
      waiter.resolve({ done: false, value: chunk });
      return;
    }
    if (this.bufferedBytes + chunk.length > RPC_BODY_BUFFER_HIGH_WATER_MARK) {
      this.onCancel(new Error("WebRTC RPC body consumer is too slow."));
      return;
    }
    this.chunks.push(chunk);
    this.bufferedBytes += chunk.length;
  }

  finish(): void {
    if (this.done) return;
    this.done = true;
    for (const waiter of this.waiters.splice(0)) {
      waiter.resolve({ done: true, value: undefined });
    }
  }

  fail(error: Error): void {
    if (this.done) return;
    this.done = true;
    this.error = error;
    for (const waiter of this.waiters.splice(0)) waiter.reject(error);
  }

  [Symbol.asyncIterator](): AsyncIterator<Uint8Array> {
    return {
      next: () => {
        const chunk = this.chunks.shift();
        if (chunk != null) {
          this.bufferedBytes -= chunk.length;
          return Promise.resolve({ done: false, value: chunk });
        }
        if (this.error != null) return Promise.reject(this.error);
        if (this.done) return Promise.resolve({ done: true, value: undefined });
        return new Promise((resolve, reject) => {
          this.waiters.push({ resolve, reject });
        });
      },
      return: async () => {
        this.onCancel();
        return { done: true, value: undefined };
      },
    };
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
  const service = options.service ?? GIZCLAW_SERVICE_ADMIN_HTTP;
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
  return createWebRTCServiceFetch(pc, { ...options, service: GIZCLAW_SERVICE_ADMIN_HTTP });
}

export async function connectGiznetWebRTC(options: ConnectGiznetWebRTCOptions): Promise<RTCPeerConnection> {
  prepareGiznetWebRTCPeerConnection(options.pc, options);
  const offer = await options.pc.createOffer();
  await options.pc.setLocalDescription(offer);
  await waitForICEGatheringComplete(options.pc, options.signal, options.iceGatheringTimeoutMs);
  const local = options.pc.localDescription;
  if (local == null) {
    throw new Error("WebRTC offer was not created.");
  }

  const prepared = await options.prepareOffer(local.sdp);
  const encryptedAnswer = await (options.sendOffer ?? ((item, signal) => sendGiznetWebRTCOffer(item, { fetch: options.fetch, signal })))(prepared, options.signal);
  const answerSDP = await prepared.openAnswer(encryptedAnswer);
  await options.pc.setRemoteDescription({ sdp: answerSDP, type: "answer" });
  const packetDataChannel = getGiznetWebRTCPacketDataChannel(options.pc);
  if (packetDataChannel != null) {
    await waitForDataChannelOpen(packetDataChannel, options.signal);
  }
  return options.pc;
}

export async function connectGiznetWebRTCFromEndpoint(options: ConnectGiznetWebRTCFromEndpointOptions): Promise<RTCPeerConnection> {
  const serverInfo = await fetchGiznetServerInfo(options);
  const signalingPath = normalizeServerInfoSignalingPath(serverInfo.signaling_path);
  applyGiznetServerInfoICEServers(options.pc, serverInfo);
  return connectGiznetWebRTC({
    ...options,
    prepareOffer: async (offerSDP) => {
      const prepared = await prepareEncryptedGiznetWebRTCOffer(
        {
          clientPrivateKey: options.clientPrivateKey,
          clientPublicKey: options.clientPublicKey,
          serverPublicKey: serverInfo.public_key,
        },
        offerSDP,
      );
      return {
        ...prepared,
        openAnswer: async (encryptedAnswer) =>
          rewriteGiznetWebRTCAnswerForEndpoint(await prepared.openAnswer(encryptedAnswer), options.endpoint),
      };
    },
    sendOffer: (offer, signal) =>
      sendGiznetWebRTCOffer(offer, {
        baseUrl: serverInfoBaseURL(options),
        fetch: options.fetch,
        signal,
        url: signalingPath,
      }),
  });
}

export function rewriteGiznetWebRTCAnswerForEndpoint(answerSDP: string, endpoint?: string): string {
  const target = loopbackICEEndpoint(endpoint);
  if (target == null) {
    return answerSDP;
  }
  return answerSDP.replace(/^a=candidate:.*$/gm, (line) => {
    const trailingCR = line.endsWith("\r") ? "\r" : "";
    const fields = line.slice(0, line.length - trailingCR.length).split(/\s+/);
    if (fields.length < 8 || fields[6]?.toLowerCase() !== "typ" || fields[7]?.toLowerCase() !== "host") {
      return line;
    }
    fields[4] = target.host;
    fields[5] = target.port;
    return `${fields.join(" ")}${trailingCR}`;
  });
}

function loopbackICEEndpoint(endpoint?: string): { host: string; port: string } | undefined {
  const value = endpoint?.trim() ?? "";
  if (value === "") {
    return undefined;
  }
  let parsed: URL;
  try {
    parsed = new URL(`http://${value}`);
  } catch {
    return undefined;
  }
  const host = parsed.hostname.replace(/^\[(.*)\]$/, "$1");
  const isLoopback = host === "localhost" || host === "::1" || /^127(?:\.\d{1,3}){3}$/.test(host);
  if (!isLoopback || parsed.port === "") {
    return undefined;
  }
  return { host, port: parsed.port };
}

export function applyGiznetServerInfoICEServers(pc: RTCPeerConnection, serverInfo: Pick<GiznetServerInfo, "ice_servers">): void {
  if (serverInfo.ice_servers == null || serverInfo.ice_servers.length === 0 || typeof pc.setConfiguration !== "function") {
    return;
  }
  const current = pc.getConfiguration?.() ?? {};
  pc.setConfiguration({ ...current, iceServers: serverInfo.ice_servers });
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
    ice_servers: normalizeServerInfoICEServers((serverInfo as { ice_servers?: unknown }).ice_servers),
    public_key: serverInfo.public_key.trim(),
    signaling_path: normalizeServerInfoSignalingPath(serverInfo.signaling_path),
  };
}

function normalizeServerInfoICEServers(servers: unknown): RTCIceServer[] | undefined {
  if (servers == null) {
    return undefined;
  }
  if (!Array.isArray(servers)) {
    throw new Error("server-info invalid ice_servers: expected an array");
  }
  return servers.map((server, serverIndex) => {
    if (typeof server !== "object" || server == null || Array.isArray(server)) {
      throw new Error(`server-info invalid ice_servers[${serverIndex}]: expected an object`);
    }
    const value = server as Record<string, unknown>;
    const rawURLs = Array.isArray(value.urls) ? value.urls : typeof value.urls === "string" ? [value.urls] : null;
    if (rawURLs == null || rawURLs.length === 0) {
      throw new Error(`server-info invalid ice_servers[${serverIndex}].urls`);
    }
    const urls = rawURLs.map((rawURL, urlIndex) => {
      if (typeof rawURL !== "string") {
        throw new Error(`server-info invalid ice_servers[${serverIndex}].urls[${urlIndex}]`);
      }
      const url = rawURL.trim();
      if (!/^(?:stun|stuns|turn|turns):\S+$/i.test(url)) {
        throw new Error(`server-info invalid ice_servers[${serverIndex}].urls[${urlIndex}]`);
      }
      return url;
    });
    if (value.username != null && typeof value.username !== "string") {
      throw new Error(`server-info invalid ice_servers[${serverIndex}].username`);
    }
    if (value.credential != null && typeof value.credential !== "string") {
      throw new Error(`server-info invalid ice_servers[${serverIndex}].credential`);
    }
    return {
      credential: value.credential as string | undefined,
      urls,
      username: value.username as string | undefined,
    };
  });
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
  serveGiznetWebRTCRPC(pc);
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

// serveGiznetWebRTCRPC installs the built-in server for RPC streams initiated
// by the remote GizClaw server. Installation is idempotent for each peer
// connection and handles all.ping plus all.speed_test.run.
export function serveGiznetWebRTCRPC(pc: WebRTCRPCDataChannelServer): void {
  if (giznetRPCServers.has(pc)) {
    return;
  }
  giznetRPCServers.add(pc);
  pc.addEventListener("datachannel", (event) => {
    const channel = event.channel;
    if (channel.label !== giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC)) {
      return;
    }
    handleInboundRPCDataChannel(channel);
  });
}

export function getGiznetWebRTCPacketDataChannel(pc: RTCPeerConnection): WebRTCRPCDataChannel | undefined {
  return giznetPacketDataChannels.get(pc);
}

export async function sendGiznetWebRTCTelemetry(
  pc: RTCPeerConnection,
  frame: TelemetryFrame,
  options: SendGiznetWebRTCTelemetryOptions = {},
): Promise<void> {
  const channel = getGiznetWebRTCPacketDataChannel(pc);
  if (channel == null) {
    throw new Error("giznet WebRTC packet data channel is not available");
  }
  const packet = encodeTelemetryPacket(frame);
  await waitForDataChannelOpen(channel, options.signal, options.timeoutMs ?? PACKET_DATA_CHANNEL_OPEN_TIMEOUT_MS);
  channel.send(packet);
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

export function parseRPCResponse<TResult = unknown>(data: ArrayBuffer | Uint8Array, method: string): RPCResponse<TResult> {
  const bytes = data instanceof Uint8Array ? data : new Uint8Array(data);
  const parsed = tryReadRPCResponse<TResult>(bytes, method);
  if (parsed == null) {
    throw new Error("incomplete WebRTC RPC response");
  }
  if (parsed.rest.length !== 0) {
    throw new Error("WebRTC RPC response contains trailing bytes");
  }
  return parsed.response;
}

export function giznetServiceDataChannelLabel(service: number): string {
  if (!Number.isSafeInteger(service) || service < 0) {
    throw new Error(`invalid giznet service id: ${service}`);
  }
  return `${GIZNET_WEBRTC_SERVICE_DATA_CHANNEL_PREFIX}${service}`;
}

export function encodeRPCRequest(request: RPCRequest): ArrayBuffer {
  return concatBytes([...encodeRPCEnvelopeFrames(encodeRPCRequestEnvelope(request)), encodeFrame(RPC_FRAME_TYPE_EOS)]);
}

export function encodeRPCResponse(response: RPCResponse, method: string): ArrayBuffer {
  return concatBytes([...encodeRPCEnvelopeFrames(encodeRPCResponseEnvelope(response, method)), encodeFrame(RPC_FRAME_TYPE_EOS)]);
}

export function encodeJSONFrame(value: unknown): ArrayBuffer {
  return encodeFrame(RPC_FRAME_TYPE_JSON, new TextEncoder().encode(JSON.stringify(value)));
}

function encodeRPCRequestEnvelope(request: RPCRequest): Uint8Array {
  const method = RPC_METHOD_IDS[request.method as keyof typeof RPC_METHOD_IDS];
  if (method == null) {
    throw new Error(`unknown RPC method: ${request.method}`);
  }
  const writer = new ProtoWriter();
  writer.string(1, request.id);
  writer.uint32(2, method);
  if (request.params !== undefined) {
    writer.bytes(3, encodeRPCRequestPayload(request.method, request.params));
  }
  return writer.finish();
}

function encodeRPCResponseEnvelope(response: RPCResponse, method: string): Uint8Array {
  const writer = new ProtoWriter();
  if (response.id != null) {
    writer.string(1, response.id);
  }
  if (response.error != null) {
    const error = new ProtoWriter();
    error.int32(1, response.error.code);
    error.string(2, response.error.message);
    writer.bytes(3, error.finish());
  } else {
    writer.bytes(2, encodeRPCResponsePayload(method, response.result ?? {}));
  }
  return writer.finish();
}

function encodeRPCEnvelopeFrames(envelope: Uint8Array): ArrayBuffer[] {
  if (envelope.length <= RPC_MAX_FRAME_PAYLOAD_SIZE) {
    return [encodeFrame(RPC_FRAME_TYPE_BINARY, envelope)];
  }
  const frames: ArrayBuffer[] = [];
  for (let offset = 0; offset < envelope.length; offset += RPC_MAX_FRAME_PAYLOAD_SIZE) {
    frames.push(encodeFrame(RPC_FRAME_TYPE_TEXT, envelope.slice(offset, offset + RPC_MAX_FRAME_PAYLOAD_SIZE)));
  }
  return frames;
}

function decodeRPCResponseEnvelope<TResult>(payload: Uint8Array, method: string): RPCResponse<TResult> {
  const reader = new ProtoReader(payload);
  const response: RPCResponse<TResult> = { v: RPC_VERSION };
  while (!reader.done()) {
    const field = reader.field();
    switch (field.number) {
      case 1:
        response.id = reader.string(field);
        break;
      case 2: {
        const body = reader.bytes(field);
        response.result = decodeRPCResponsePayload(method, body) as TResult;
        break;
      }
      case 3:
        response.error = decodeRPCError(reader.bytes(field));
        break;
      default:
        reader.skip(field);
        break;
    }
  }
  if (response.error == null && !("result" in response)) {
    throw new Error("invalid WebRTC RPC response: missing result or error");
  }
  return response;
}

function decodeRPCRequestEnvelope(payload: Uint8Array): RPCRequest {
  const reader = new ProtoReader(payload);
  let id = "";
  let methodID: number | undefined;
  let paramsPayload: Uint8Array | undefined;
  while (!reader.done()) {
    const field = reader.field();
    switch (field.number) {
      case 1:
        id = reader.string(field);
        break;
      case 2:
        methodID = reader.int32(field);
        break;
      case 3:
        paramsPayload = reader.bytes(field);
        break;
      default:
        reader.skip(field);
        break;
    }
  }
  if (id === "" || methodID == null) {
    throw new Error("invalid WebRTC RPC request: missing id or method");
  }
  const method = rpcMethodNamesByID.get(methodID) ?? `unknown:${methodID}`;
  return {
    id,
    method,
    ...(paramsPayload == null || !rpcMethodNamesByID.has(methodID)
      ? {}
      : { params: decodeRPCRequestPayload(method, paramsPayload) }),
    v: RPC_VERSION,
  };
}

function handleInboundRPCDataChannel(channel: WebRTCRPCDataChannel): void {
  channel.binaryType = "arraybuffer";
  let buffer: Uint8Array<ArrayBufferLike> = new Uint8Array();
  let messageQueue = Promise.resolve();
  let request: RPCRequest | undefined;
  let envelopeLength = 0;
  const envelopeChunks: Uint8Array[] = [];
  let uploaded = 0;
  let ignoreBody = false;
  let closed = false;

  const cleanup = (): void => {
    channel.removeEventListener("message", onMessage);
    channel.removeEventListener("close", onClose);
    channel.removeEventListener("error", onError);
  };
  const close = (): void => {
    if (closed) {
      return;
    }
    closed = true;
    cleanup();
    try {
      channel.close();
    } catch {
      // Ignore close races from an already closed remote stream.
    }
  };
  const fail = (): void => close();
  const sendResponse = (response: RPCResponse, method: string): Promise<void> =>
    sendInboundRPCFrames(channel, [...encodeRPCEnvelopeFrames(encodeRPCResponseEnvelope(response, method)), encodeFrame(RPC_FRAME_TYPE_EOS)]);

  const finishPing = (pingRequest: RPCRequest): void => {
    const params = pingRequest.params as PingRequest | undefined;
    const response = params == null
      ? rpcErrorResponse(pingRequest.id, -32602, "missing params")
      : { id: pingRequest.id, result: { server_time: Date.now() }, v: RPC_VERSION } satisfies RPCResponse;
    ignoreBody = true;
    void sendResponse(response, pingRequest.method).catch(fail);
  };

  const startRequest = (next: RPCRequest): void => {
    request = next;
    switch (next.method) {
      case "all.ping":
        return;
      case "all.speed_test.run": {
        const params = validSpeedTestParams(next.params);
        if (params == null) {
          ignoreBody = true;
          void sendResponse(rpcErrorResponse(next.id, -32602, "invalid params"), next.method).catch(fail);
          return;
        }
        void sendInboundSpeedTestResponse(channel, next.id, params).catch(fail);
        return;
      }
      default:
        ignoreBody = true;
        void sendResponse(rpcErrorResponse(next.id, -32601, `unsupported method: ${next.method}`), next.method).catch(fail);
    }
  };

  const handleFrame = (frame: { payload: Uint8Array; type: number }): void => {
    if (request == null) {
      if (frame.type === RPC_FRAME_TYPE_TEXT) {
        envelopeLength += frame.payload.length;
        if (envelopeLength > RPC_MAX_ENVELOPE_SIZE) {
          throw new Error(`RPC protobuf envelope too large: ${envelopeLength}`);
        }
        envelopeChunks.push(copyBytes(frame.payload));
        return;
      }
      if (frame.type === RPC_FRAME_TYPE_BINARY) {
        if (envelopeChunks.length > 0) {
          throw new Error("RPC request contains multiple protobuf frames.");
        }
        startRequest(decodeRPCRequestEnvelope(frame.payload));
        return;
      }
      if (frame.type === RPC_FRAME_TYPE_EOS && envelopeChunks.length > 0) {
        const continuedRequest = decodeRPCRequestEnvelope(concatByteArrays(envelopeChunks));
        startRequest(continuedRequest);
        if (continuedRequest.method === "all.ping") {
          finishPing(continuedRequest);
        }
        return;
      }
      throw new Error(`rpc: expected protobuf request frame, got type ${frame.type}`);
    }

    if (ignoreBody) {
      return;
    }
    if (request.method === "all.ping") {
      if (frame.type !== RPC_FRAME_TYPE_EOS) {
        throw new Error(`rpc: expected ping EOS frame, got type ${frame.type}`);
      }
      finishPing(request);
      return;
    }
    if (request.method === "all.speed_test.run") {
      if (frame.type === RPC_FRAME_TYPE_BINARY) {
        uploaded += frame.payload.length;
        return;
      }
      if (frame.type !== RPC_FRAME_TYPE_EOS) {
        throw new Error(`rpc: expected speed-test binary frame, got type ${frame.type}`);
      }
      const params = request.params as SpeedTestRequest;
      if (uploaded !== params.up_content_length) {
        throw new Error(`rpc: speed test upload length mismatch: got ${uploaded} want ${params.up_content_length}`);
      }
      ignoreBody = true;
    }
  };

  const drainFrames = (): void => {
    for (;;) {
      const parsed = tryReadFrame(buffer);
      if (parsed == null) {
        return;
      }
      buffer = parsed.rest;
      handleFrame(parsed.frame);
    }
  };
  const onMessage = (event: MessageEvent): void => {
    messageQueue = messageQueue.then(async () => {
      if (closed) {
        return;
      }
      buffer = appendBytes(buffer, await messageDataBytes(event.data));
      drainFrames();
    }).catch(fail);
  };
  const onClose = (): void => {
    closed = true;
    cleanup();
  };
  const onError = (): void => fail();

  channel.addEventListener("message", onMessage);
  channel.addEventListener("close", onClose);
  channel.addEventListener("error", onError);
}

function validSpeedTestParams(value: unknown): SpeedTestRequest | null {
  if (value == null || typeof value !== "object") {
    return null;
  }
  const params = value as Partial<SpeedTestRequest>;
  if (
    !Number.isSafeInteger(params.up_content_length)
    || !Number.isSafeInteger(params.down_content_length)
    || (params.up_content_length ?? -1) < 0
    || (params.down_content_length ?? -1) < 0
    || (params.up_content_length ?? 0) > RPC_SPEED_TEST_MAX_CONTENT_LENGTH
    || (params.down_content_length ?? 0) > RPC_SPEED_TEST_MAX_CONTENT_LENGTH
  ) {
    return null;
  }
  return params as SpeedTestRequest;
}

function rpcErrorResponse(id: string, code: number, message: string): RPCResponse {
  return { error: { code, message }, id, v: RPC_VERSION };
}

async function sendInboundSpeedTestResponse(channel: WebRTCRPCDataChannel, id: string, params: SpeedTestRequest): Promise<void> {
  const response: RPCResponse = {
    id,
    result: {
      down_content_length: params.down_content_length,
      up_content_length: params.up_content_length,
    },
    v: RPC_VERSION,
  };
  const responseEnvelope = encodeRPCResponseEnvelope(response, "all.speed_test.run");
  await sendInboundRPCFrames(channel, encodeRPCEnvelopeFrames(responseEnvelope));
  if (responseEnvelope.length > RPC_MAX_FRAME_PAYLOAD_SIZE) {
    await sendInboundRPCFrames(channel, [encodeFrame(RPC_FRAME_TYPE_EOS)]);
  }
  const chunk = new Uint8Array(RPC_SPEED_TEST_FRAME_SIZE);
  for (let offset = 0; offset < params.down_content_length; offset += chunk.length) {
    const size = Math.min(chunk.length, params.down_content_length - offset);
    await sendInboundRPCFrames(channel, [encodeFrame(RPC_FRAME_TYPE_BINARY, chunk.subarray(0, size))]);
  }
  await sendInboundRPCFrames(channel, [encodeFrame(RPC_FRAME_TYPE_EOS)]);
}

function sendInboundRPCFrames(
  channel: WebRTCRPCDataChannel,
  frames: ArrayBuffer[],
  options: ServiceDataChannelWriteOptions = {},
): Promise<void> {
  return serviceDataChannelWriter(channel).write(frames, options);
}

function decodeRPCError(payload: Uint8Array): RPCErrorBody {
  const reader = new ProtoReader(payload);
  const error: RPCErrorBody = { code: 0, message: "" };
  while (!reader.done()) {
    const field = reader.field();
    switch (field.number) {
      case 1:
        error.code = reader.int32(field);
        break;
      case 2:
        error.message = reader.string(field);
        break;
      default:
        reader.skip(field);
        break;
    }
  }
  return error;
}

export function encodeFrame(type: number, payload: Uint8Array = new Uint8Array()): ArrayBuffer {
  if (!Number.isInteger(type) || type < 0 || type > 0xffff) {
    throw new Error(`invalid RPC frame type: ${type}`);
  }
  if (payload.length > RPC_MAX_FRAME_PAYLOAD_SIZE) {
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

function tryReadFrame(
  buffer: Uint8Array<ArrayBufferLike>,
): { frame: { payload: Uint8Array; type: number }; rest: Uint8Array<ArrayBufferLike> } | null {
  if (buffer.length < 4) {
    return null;
  }
  const view = new DataView(buffer.buffer, buffer.byteOffset, 4);
  const length = view.getUint16(0, true);
  const type = view.getUint16(2, true);
  if (buffer.length < 4 + length) {
    return null;
  }
  if (type === RPC_FRAME_TYPE_EOS && length !== 0) {
    throw new Error("RPC EOS frame must be empty.");
  }
  return {
    frame: { payload: buffer.slice(4, 4 + length), type },
    rest: buffer.slice(4 + length),
  };
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
  method: string,
): { response: RPCResponse<TResult>; rest: Uint8Array<ArrayBufferLike> } | null {
  let offset = 0;
  let response: RPCResponse<TResult> | undefined;
  const envelopeChunks: Uint8Array[] = [];
  let envelopeLength = 0;
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
      if (response == null && envelopeChunks.length > 0) {
        response = decodeRPCResponseEnvelope<TResult>(concatByteArrays(envelopeChunks), method);
      }
      if (response == null) {
        throw new Error("RPC response EOS before protobuf frame.");
      }
      return { response, rest: buffer.slice(offset) };
    }
    if (type === RPC_FRAME_TYPE_TEXT) {
      if (response != null) {
        throw new Error("RPC response contains continuation after protobuf frame.");
      }
      envelopeLength += payload.length;
      if (envelopeLength > RPC_MAX_ENVELOPE_SIZE) {
        throw new Error(`RPC protobuf envelope too large: ${envelopeLength}`);
      }
      envelopeChunks.push(copyBytes(payload));
      continue;
    }
    if (type !== RPC_FRAME_TYPE_BINARY) {
      throw new Error(`rpc: expected protobuf binary frame, got type ${type}`);
    }
    if (response != null || envelopeChunks.length > 0) {
      throw new Error("RPC response contains multiple protobuf frames.");
    }
    response = decodeRPCResponseEnvelope<TResult>(payload, method);
  }
}

function tryReadRPCBinaryResponse<TResult>(
  buffer: Uint8Array<ArrayBufferLike>,
  method: string,
): { body: Uint8Array; response: RPCResponse<TResult>; rest: Uint8Array<ArrayBufferLike> } | null {
  let offset = 0;
  let response: RPCResponse<TResult> | undefined;
  const envelopeChunks: Uint8Array[] = [];
  let envelopeLength = 0;
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
      if (response == null && envelopeChunks.length > 0) {
        response = decodeRPCResponseEnvelope<TResult>(concatByteArrays(envelopeChunks), method);
        if (response.error != null) {
          return { body: concatByteArrays(body), response, rest: buffer.slice(offset) };
        }
        continue;
      }
      if (response == null) {
        throw new Error("RPC binary response EOS before protobuf frame.");
      }
      return { body: concatByteArrays(body), response, rest: buffer.slice(offset) };
    }
    if (response == null) {
      if (type === RPC_FRAME_TYPE_TEXT) {
        envelopeLength += payload.length;
        if (envelopeLength > RPC_MAX_ENVELOPE_SIZE) {
          throw new Error(`RPC protobuf envelope too large: ${envelopeLength}`);
        }
        envelopeChunks.push(copyBytes(payload));
        continue;
      }
      if (type !== RPC_FRAME_TYPE_BINARY) {
        throw new Error(`rpc: expected protobuf binary frame, got type ${type}`);
      }
      if (envelopeChunks.length > 0) {
        throw new Error("RPC binary response contains multiple protobuf frames.");
      }
      response = decodeRPCResponseEnvelope<TResult>(payload, method);
      continue;
    }
    if (type !== RPC_FRAME_TYPE_BINARY) {
      throw new Error(`rpc: expected binary frame, got type ${type}`);
    }
    body.push(copyBytes(payload));
  }
}

type ProtoField = {
  number: number;
  wireType: number;
};

class ProtoWriter {
  private readonly chunks: number[] = [];

  uint32(field: number, value: number): void {
    this.tag(field, 0);
    this.varint(BigInt(value >>> 0));
  }

  int32(field: number, value: number): void {
    this.tag(field, 0);
    this.varint(BigInt.asUintN(64, BigInt(value | 0)));
  }

  string(field: number, value: string): void {
    this.bytes(field, new TextEncoder().encode(value));
  }

  bytes(field: number, value: Uint8Array): void {
    this.tag(field, 2);
    this.varint(BigInt(value.length));
    for (const byte of value) {
      this.chunks.push(byte);
    }
  }

  finish(): Uint8Array {
    return Uint8Array.from(this.chunks);
  }

  private tag(field: number, wireType: number): void {
    this.varint(BigInt((field << 3) | wireType));
  }

  private varint(value: bigint): void {
    let next = value;
    while (next > 0x7fn) {
      this.chunks.push(Number((next & 0x7fn) | 0x80n));
      next >>= 7n;
    }
    this.chunks.push(Number(next));
  }
}

class ProtoReader {
  private readonly data: Uint8Array;
  private offset = 0;

  constructor(data: Uint8Array) {
    this.data = data;
  }

  done(): boolean {
    return this.offset >= this.data.length;
  }

  field(): ProtoField {
    const tag = Number(this.varint());
    return { number: tag >>> 3, wireType: tag & 0x7 };
  }

  int32(field: ProtoField): number {
    this.expect(field, 0);
    return Number(BigInt.asIntN(32, this.varint()));
  }

  string(field: ProtoField): string {
    return new TextDecoder().decode(this.bytes(field));
  }

  bytes(field: ProtoField): Uint8Array {
    this.expect(field, 2);
    const length = Number(this.varint());
    if (!Number.isSafeInteger(length) || length < 0 || this.data.length - this.offset < length) {
      throw new Error("invalid protobuf bytes length");
    }
    const out = this.data.slice(this.offset, this.offset + length);
    this.offset += length;
    return out;
  }

  skip(field: ProtoField): void {
    switch (field.wireType) {
      case 0:
        this.varint();
        return;
      case 2:
        this.bytes(field);
        return;
      default:
        throw new Error(`unsupported protobuf wire type: ${field.wireType}`);
    }
  }

  private expect(field: ProtoField, wireType: number): void {
    if (field.wireType !== wireType) {
      throw new Error(`unexpected protobuf wire type ${field.wireType} for field ${field.number}`);
    }
  }

  private varint(): bigint {
    let shift = 0n;
    let value = 0n;
    for (;;) {
      if (this.offset >= this.data.length) {
        throw new Error("truncated protobuf varint");
      }
      const byte = this.data[this.offset++];
      value |= BigInt(byte & 0x7f) << shift;
      if ((byte & 0x80) === 0) {
        return value;
      }
      shift += 7n;
      if (shift > 70n) {
        throw new Error("protobuf varint too long");
      }
    }
  }
}

export function waitForICEGatheringComplete(pc: RTCPeerConnection, signal?: AbortSignal, timeoutMs = 10_000): Promise<void> {
  if (pc.iceGatheringState === "complete") {
    return Promise.resolve();
  }
  if (!Number.isFinite(timeoutMs) || timeoutMs < 0) {
    return Promise.reject(new Error("ICE gathering timeout must be a non-negative finite number."));
  }
  return new Promise((resolve, reject) => {
    let settled = false;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    const cleanup = (): void => {
      if (timeout != null) {
        clearTimeout(timeout);
      }
      signal?.removeEventListener("abort", onAbort);
      pc.removeEventListener("icecandidate", onCandidate);
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
    const fail = (err: Error): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      reject(err);
    };
    const onAbort = (): void => {
      fail(abortError());
    };
    const onStateChange = (): void => {
      if (pc.iceGatheringState === "complete") {
        complete();
      }
    };
    const onCandidate = (event: RTCPeerConnectionIceEvent): void => {
      if (event.candidate == null) {
        complete();
      }
    };
    const onTimeout = (): void => {
      if (/^a=candidate:.*\btyp\s+relay\b/im.test(pc.localDescription?.sdp ?? "")) {
        complete();
        return;
      }
      fail(new Error(`ICE gathering timed out after ${timeoutMs}ms before producing a relay candidate.`));
    };
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    pc.addEventListener("icecandidate", onCandidate);
    pc.addEventListener("icegatheringstatechange", onStateChange);
    timeout = setTimeout(onTimeout, timeoutMs);
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
      sendDataChannelMessage(
        channel,
        requestBytes,
        (err) => settle(() => reject(err)),
        { signal, timeoutMs },
      );
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

function waitForDataChannelOpen(channel: WebRTCRPCDataChannel, signal?: AbortSignal, timeoutMs = 30000): Promise<void> {
  if (channel.readyState === "open") {
    return Promise.resolve();
  }
  if (channel.readyState === "closed") {
    return Promise.reject(new Error(`RTCDataChannel.readyState is ${JSON.stringify(channel.readyState)}, want "open"`));
  }
  return new Promise((resolve, reject) => {
    let timeout: ReturnType<typeof setTimeout> | undefined;
    let settled = false;
    const cleanup = (): void => {
      if (timeout != null) {
        clearTimeout(timeout);
      }
      signal?.removeEventListener("abort", onAbort);
      channel.removeEventListener("open", onOpen);
      channel.removeEventListener("close", onClose);
      channel.removeEventListener("error", onError);
    };
    const settle = (fn: () => void): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      fn();
    };
    const onOpen = (): void => {
      if (channel.readyState === "open") {
        settle(resolve);
      }
    };
    const onClose = (): void => {
      settle(() => reject(new Error("RTCDataChannel closed before opening")));
    };
    const onError = (): void => {
      settle(() => reject(new Error("RTCDataChannel failed before opening")));
    };
    const onAbort = (): void => {
      settle(() => reject(abortError()));
    };
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
    channel.addEventListener("open", onOpen);
    channel.addEventListener("close", onClose);
    channel.addEventListener("error", onError);
    if (timeoutMs > 0) {
      timeout = setTimeout(() => {
        settle(() => reject(new Error(`RTCDataChannel did not open within ${timeoutMs}ms`)));
      }, timeoutMs);
    }
    onOpen();
  });
}

type ServiceDataChannelWriteOptions = {
  signal?: AbortSignal;
  timeoutMs?: number;
};

const serviceDataChannelWriters = new WeakMap<object, ServiceDataChannelWriter>();

class ServiceDataChannelWriter {
  private tail: Promise<void> = Promise.resolve();
  private backpressured = false;
  private readonly channel: WebRTCRPCDataChannel;

  constructor(channel: WebRTCRPCDataChannel) {
    this.channel = channel;
    channel.bufferedAmountLowThreshold = SERVICE_DATA_CHANNEL_BUFFER_LOW_WATER_MARK;
  }

  write(payloads: readonly DataChannelPayload[], options: ServiceDataChannelWriteOptions = {}): Promise<void> {
    const timeoutMs = options.timeoutMs ?? SERVICE_DATA_CHANNEL_WRITE_TIMEOUT_MS;
    if (!Number.isFinite(timeoutMs) || timeoutMs < 0) {
      return Promise.reject(new Error("service data channel write timeout must be a non-negative finite number"));
    }
    const deadline = timeoutMs === 0 ? Number.POSITIVE_INFINITY : Date.now() + timeoutMs;
    const run = this.tail.then(() => this.writeNow(payloads, options.signal, deadline));
    this.tail = run.catch(() => undefined);
    return run;
  }

  private async writeNow(
    payloads: readonly DataChannelPayload[],
    signal: AbortSignal | undefined,
    deadline: number,
  ): Promise<void> {
    let sent = false;
    try {
      for (const payload of payloads) {
        const bytes = await dataChannelPayloadBytes(payload);
        for (let offset = 0; offset < bytes.length; offset += SERVICE_DATA_CHANNEL_MESSAGE_CHUNK_SIZE) {
          await this.waitForBudget(signal, deadline);
          this.assertWritable(signal, deadline);
          this.channel.send(bytes.subarray(offset, offset + SERVICE_DATA_CHANNEL_MESSAGE_CHUNK_SIZE));
          sent = true;
        }
      }
    } catch (error) {
      if (sent) {
        try {
          this.channel.close();
        } catch {
          // Ignore close races after a partial logical write.
        }
      }
      throw error;
    }
  }

  private async waitForBudget(signal: AbortSignal | undefined, deadline: number): Promise<void> {
    for (;;) {
      this.assertWritable(signal, deadline);
      const amount = this.channel.bufferedAmount ?? 0;
      if (this.backpressured) {
        if (amount <= SERVICE_DATA_CHANNEL_BUFFER_LOW_WATER_MARK) {
          this.backpressured = false;
          return;
        }
      } else if (amount < SERVICE_DATA_CHANNEL_BUFFER_HIGH_WATER_MARK) {
        return;
      } else {
        this.backpressured = true;
      }
      await this.waitForLowWater(signal, deadline);
    }
  }

  private waitForLowWater(signal: AbortSignal | undefined, deadline: number): Promise<void> {
    return new Promise((resolve, reject) => {
      let timeout: ReturnType<typeof setTimeout> | undefined;
      let settled = false;
      const cleanup = (): void => {
        if (timeout != null) clearTimeout(timeout);
        signal?.removeEventListener("abort", onAbort);
        this.channel.removeEventListener("bufferedamountlow", onLow);
        this.channel.removeEventListener("close", onClose);
        this.channel.removeEventListener("error", onError);
      };
      const settle = (fn: () => void): void => {
        if (settled) return;
        settled = true;
        cleanup();
        fn();
      };
      const onLow = (): void => {
        if ((this.channel.bufferedAmount ?? 0) <= SERVICE_DATA_CHANNEL_BUFFER_LOW_WATER_MARK) {
          settle(resolve);
        }
      };
      const onAbort = (): void => settle(() => reject(abortError()));
      const onClose = (): void => settle(() => reject(new Error("service data channel closed while writing")));
      const onError = (): void => settle(() => reject(new Error("service data channel failed while writing")));
      signal?.addEventListener("abort", onAbort, { once: true });
      this.channel.addEventListener("bufferedamountlow", onLow);
      this.channel.addEventListener("close", onClose);
      this.channel.addEventListener("error", onError);
      const remaining = deadline - Date.now();
      if (Number.isFinite(deadline)) {
        if (remaining <= 0) {
          settle(() => reject(new Error("service data channel write timed out")));
          return;
        }
        timeout = setTimeout(
          () => settle(() => reject(new Error("service data channel write timed out"))),
          remaining,
        );
      }
      if (signal?.aborted) {
        onAbort();
      } else if (this.channel.readyState !== "open") {
        onClose();
      } else {
        onLow();
      }
    });
  }

  private assertWritable(signal: AbortSignal | undefined, deadline: number): void {
    if (signal?.aborted) throw abortError();
    if (Date.now() >= deadline) throw new Error("service data channel write timed out");
    if (this.channel.readyState !== "open") {
      throw new Error(`RTCDataChannel.readyState is ${JSON.stringify(this.channel.readyState)}, want "open"`);
    }
  }
}

function serviceDataChannelWriter(channel: WebRTCRPCDataChannel): ServiceDataChannelWriter {
  let writer = serviceDataChannelWriters.get(channel);
  if (writer == null) {
    writer = new ServiceDataChannelWriter(channel);
    serviceDataChannelWriters.set(channel, writer);
  }
  return writer;
}

function sendDataChannelMessage(
  channel: WebRTCRPCDataChannel,
  payload: DataChannelPayload,
  onError: (err: unknown) => void,
  options: ServiceDataChannelWriteOptions = {},
): void {
  void serviceDataChannelWriter(channel).write([payload], options).catch(onError);
}

async function dataChannelPayloadBytes(payload: DataChannelPayload): Promise<Uint8Array> {
  if (payload instanceof ArrayBuffer) return new Uint8Array(payload);
  if (ArrayBuffer.isView(payload)) return new Uint8Array(payload.buffer, payload.byteOffset, payload.byteLength);
  if (typeof payload === "string") return new TextEncoder().encode(payload);
  return new Uint8Array(await payload.arrayBuffer());
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
