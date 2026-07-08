import { GIZCLAW_SERVICE_PEER_RPC, WebRTCRPCClient } from "./index.ts";
import type { RPCBinaryCallResult, RPCCallOptions, WebRTCRPCClientOptions, WebRTCRPCDataChannelFactory } from "./index.ts";
import type { RPCMethodMap, RPCMethodName } from "./generated/rpc/method-map.ts";

export type { RPCMethodMap, RPCMethodName } from "./generated/rpc/method-map.ts";
export { RPC_METHODS } from "./generated/rpc/method-map.ts";

export type PeerRPCClientOptions = Omit<WebRTCRPCClientOptions, "service">;
export type PeerRPCCaller = Pick<WebRTCRPCClient, "call" | "callBinary">;

export class PeerRPCClient {
  private readonly client: PeerRPCCaller;

  constructor(pc: WebRTCRPCDataChannelFactory | PeerRPCCaller, options: PeerRPCClientOptions = {}) {
    this.client =
      isPeerRPCCaller(pc)
        ? pc
        : new WebRTCRPCClient(pc, {
            ...options,
            service: GIZCLAW_SERVICE_PEER_RPC,
          });
  }

  call<M extends RPCMethodName>(
    method: M,
    params: RPCMethodMap[M]["request"],
    options?: RPCCallOptions,
  ): Promise<RPCMethodMap[M]["response"]> {
    return this.client.call<RPCMethodMap[M]["response"], RPCMethodMap[M]["request"]>(method, params, options);
  }

  callBinary<M extends RPCMethodName>(
    method: M,
    params: RPCMethodMap[M]["request"],
    options?: RPCCallOptions,
  ): Promise<RPCBinaryCallResult<RPCMethodMap[M]["response"]>> {
    return this.client.callBinary<RPCMethodMap[M]["response"], RPCMethodMap[M]["request"]>(method, params, options);
  }
}

export function createPeerRPCClient(
  pc: WebRTCRPCDataChannelFactory | PeerRPCCaller,
  options: PeerRPCClientOptions = {},
): PeerRPCClient {
  return new PeerRPCClient(pc, options);
}

function isPeerRPCCaller(value: WebRTCRPCDataChannelFactory | PeerRPCCaller): value is PeerRPCCaller {
  return "call" in value && typeof value.call === "function" && "callBinary" in value && typeof value.callBinary === "function";
}
