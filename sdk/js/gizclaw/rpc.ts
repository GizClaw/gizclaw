import {
  GIZCLAW_SERVICE_EDGE_RPC,
  GIZCLAW_SERVICE_PEER_RPC,
  WebRTCRPCClient,
} from "./index.ts";
import type {
  RPCBinaryCallResult,
  RPCCallOptions,
  RPCStreamingCallResult,
  WebRTCRPCClientOptions,
  WebRTCRPCDataChannelFactory,
} from "./index.ts";
import type {
  RPCMethodMap as GeneratedRPCMethodMap,
  RPCMethodName as GeneratedRPCMethodName,
} from "./generated/rpc/method-map.ts";
import type * as RPCPayload from "./generated/rpc/payload-codec.ts";

export type * from "./generated/rpc/payload-codec.ts";
export { RPC_METHODS } from "./generated/rpc/method-map.ts";

type WithRequired<T, K extends keyof T> = Omit<T, K> & Required<Pick<T, K>>;
type Override<T, U> = Omit<T, keyof U> & U;

export type FriendGroupMemberMutableRole = "member" | "admin";
export type FriendGroupObject = Omit<
  RPCPayload.FriendGroupObject,
  "my_role"
> & {
  my_role?: string;
};
export type FirmwareSlot = RPCPayload.FirmwareSlot;
export type FirmwareSlots = Required<RPCPayload.FirmwareSlots>;
export type Firmware = Omit<RPCPayload.Firmware, "slots"> & {
  slots: FirmwareSlots;
};
export type FirmwareGetResponse = Firmware;
export type PeerRunRecallHit = Omit<RPCPayload.PeerRunRecallHit, "metadata"> & {
  metadata?: Record<string, unknown>;
};
export type PeerRunRecallRequest = Omit<
  RPCPayload.PeerRunRecallRequest,
  "filters"
> & {
  filters?: Record<string, unknown>;
};
export type PeerRunRecallResponse = Omit<
  RPCPayload.PeerRunRecallResponse,
  "hits"
> & {
  hits: PeerRunRecallHit[];
};
export type ServerRunWorkspaceRecallRequest = PeerRunRecallRequest;
export type ServerRunWorkspaceRecallResponse = PeerRunRecallResponse;

export type RPCMethodName = GeneratedRPCMethodName;
export type RPCMethodMap = Override<
  GeneratedRPCMethodMap,
  {
    "server.firmware.get": Override<
      GeneratedRPCMethodMap["server.firmware.get"],
      {
        response: FirmwareGetResponse;
      }
    >;
    "server.run.workspace.recall": Override<
      GeneratedRPCMethodMap["server.run.workspace.recall"],
      {
        request: ServerRunWorkspaceRecallRequest;
        response: ServerRunWorkspaceRecallResponse;
      }
    >;
  }
>;
export type EdgeRPCMethodName = Extract<
  RPCMethodName,
  "server.peer.lookup" | "server.peer.assign" | "server.route.resolve"
>;
export type StreamingPeerRPCMethodName = Extract<
  RPCMethodName,
  "server.speech.transcribe" | "server.speech.synthesize"
>;
export type PeerRPCMethodName = Exclude<
  RPCMethodName,
  EdgeRPCMethodName | StreamingPeerRPCMethodName
>;

export type PeerRPCClientOptions = Omit<WebRTCRPCClientOptions, "service">;
export type PeerRPCCaller = Pick<
  WebRTCRPCClient,
  "call" | "callBinary" | "transcribeSpeech" | "synthesizeSpeech"
>;
export type EdgeRPCClientOptions = Omit<WebRTCRPCClientOptions, "service">;
export type EdgeRPCCaller = Pick<WebRTCRPCClient, "call" | "callBinary">;

export class PeerRPCClient {
  private readonly client: PeerRPCCaller;

  constructor(
    pc: WebRTCRPCDataChannelFactory | PeerRPCCaller,
    options: PeerRPCClientOptions = {},
  ) {
    if (looksLikeRPCCaller(pc) && !isPeerRPCCaller(pc)) {
      throw new TypeError(
        "Peer RPC caller must implement call, callBinary, transcribeSpeech, and synthesizeSpeech.",
      );
    }
    this.client = isPeerRPCCaller(pc)
      ? pc
      : new WebRTCRPCClient(pc, {
          ...options,
          service: GIZCLAW_SERVICE_PEER_RPC,
        });
  }

  call<M extends PeerRPCMethodName>(
    method: M,
    params: RPCMethodMap[M]["request"],
    options?: RPCCallOptions,
  ): Promise<RPCMethodMap[M]["response"]> {
    return this.client.call<
      RPCMethodMap[M]["response"],
      RPCMethodMap[M]["request"]
    >(method, params, options);
  }

  callBinary<M extends PeerRPCMethodName>(
    method: M,
    params: RPCMethodMap[M]["request"],
    options?: RPCCallOptions,
  ): Promise<RPCBinaryCallResult<RPCMethodMap[M]["response"]>> {
    return this.client.callBinary<
      RPCMethodMap[M]["response"],
      RPCMethodMap[M]["request"]
    >(method, params, options);
  }

  transcribeSpeech(
    params: RPCPayload.SpeechTranscribeRequest,
    audio: AsyncIterable<Uint8Array> | Iterable<Uint8Array>,
    options?: RPCCallOptions,
  ): Promise<RPCPayload.SpeechTranscribeResponse> {
    return this.client.transcribeSpeech(params, audio, options);
  }

  synthesizeSpeech(
    params: RPCPayload.SpeechSynthesizeRequest,
    options?: RPCCallOptions,
  ): Promise<RPCStreamingCallResult<RPCPayload.SpeechSynthesizeResponse>> {
    return this.client.synthesizeSpeech(params, options);
  }
}

export function createPeerRPCClient(
  pc: WebRTCRPCDataChannelFactory | PeerRPCCaller,
  options: PeerRPCClientOptions = {},
): PeerRPCClient {
  return new PeerRPCClient(pc, options);
}

export class EdgeRPCClient {
  private readonly client: EdgeRPCCaller;

  constructor(
    pc: WebRTCRPCDataChannelFactory | EdgeRPCCaller,
    options: EdgeRPCClientOptions = {},
  ) {
    this.client = isEdgeRPCCaller(pc)
      ? pc
      : new WebRTCRPCClient(pc, {
          ...options,
          service: GIZCLAW_SERVICE_EDGE_RPC,
        });
  }

  call<M extends EdgeRPCMethodName>(
    method: M,
    params: RPCMethodMap[M]["request"],
    options?: RPCCallOptions,
  ): Promise<RPCMethodMap[M]["response"]> {
    return this.client.call<
      RPCMethodMap[M]["response"],
      RPCMethodMap[M]["request"]
    >(method, params, options);
  }

  callBinary<M extends EdgeRPCMethodName>(
    method: M,
    params: RPCMethodMap[M]["request"],
    options?: RPCCallOptions,
  ): Promise<RPCBinaryCallResult<RPCMethodMap[M]["response"]>> {
    return this.client.callBinary<
      RPCMethodMap[M]["response"],
      RPCMethodMap[M]["request"]
    >(method, params, options);
  }
}

export function createEdgeRPCClient(
  pc: WebRTCRPCDataChannelFactory | EdgeRPCCaller,
  options: EdgeRPCClientOptions = {},
): EdgeRPCClient {
  return new EdgeRPCClient(pc, options);
}

function isPeerRPCCaller(
  value: WebRTCRPCDataChannelFactory | PeerRPCCaller,
): value is PeerRPCCaller {
  return (
    looksLikeRPCCaller(value) &&
    "transcribeSpeech" in value &&
    typeof value.transcribeSpeech === "function" &&
    "synthesizeSpeech" in value &&
    typeof value.synthesizeSpeech === "function"
  );
}

function looksLikeRPCCaller(
  value: WebRTCRPCDataChannelFactory | PeerRPCCaller,
): boolean {
  return (
    "call" in value &&
    typeof value.call === "function" &&
    "callBinary" in value &&
    typeof value.callBinary === "function"
  );
}

function isEdgeRPCCaller(
  value: WebRTCRPCDataChannelFactory | EdgeRPCCaller,
): value is EdgeRPCCaller {
  return (
    "call" in value &&
    typeof value.call === "function" &&
    "callBinary" in value &&
    typeof value.callBinary === "function"
  );
}
