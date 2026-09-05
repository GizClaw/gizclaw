import { getNodeMonitor } from "./generated/monitor/index.ts";
import {
  createClient,
  createConfig,
} from "./generated/monitor/client/index.ts";
import { createGizClawControlClient, GizClawControlError } from "./index.ts";
import type { GizClawControlClientOptions } from "./index.ts";
import type { NodeSnapshot } from "./generated/monitor/types.gen.ts";
import {
  createPeerHTTPClient,
  findPublicKeysBySn,
  findPublicKeysByImei,
} from "@gizclaw/gizclaw/peerhttp";

export type {
  NodeSnapshot,
  MonitorLog,
} from "./generated/monitor/types.gen.ts";

/** Public, unauthenticated identifier lookup; duplicate identifiers return every match. */
export function createGizClawDiscoveryClient(
  options: Omit<GizClawControlClientOptions, "apiKey">,
) {
  const client = createPeerHTTPClient({
    baseUrl: monitorBaseUrl(options),
    fetch: options.fetch,
  });
  return {
    async findBySn(sn: string): Promise<string[]> {
      const result = await findPublicKeysBySn({
        client,
        signal: options.signal,
        path: { sn },
      });
      if (!result.response?.ok || !result.data)
        throw GizClawControlError.fromResult(
          "findPublicKeysBySn",
          result.response,
          result.error,
        );
      return result.data.public_keys;
    },
    async findByImei(tac: string, serial: string): Promise<string[]> {
      const result = await findPublicKeysByImei({
        client,
        signal: options.signal,
        path: { tac, serial },
      });
      if (!result.response?.ok || !result.data)
        throw GizClawControlError.fromResult(
          "findPublicKeysByImei",
          result.response,
          result.error,
        );
      return result.data.public_keys;
    },
  };
}

function monitorBaseUrl(
  options: Pick<
    GizClawControlClientOptions,
    "baseUrl" | "allowInsecureTransport"
  >,
): string {
  const base = new URL(options.baseUrl);
  if (
    base.protocol !== "https:" &&
    !(base.protocol === "http:" && options.allowInsecureTransport)
  ) {
    throw new TypeError(
      "monitor baseUrl requires HTTPS or explicit allowInsecureTransport",
    );
  }
  if (base.username || base.password || base.search || base.hash) {
    throw new TypeError(
      "monitor baseUrl must not contain credentials, query or fragment",
    );
  }
  return base.toString().replace(/\/$/, "");
}

/** Device monitoring with a public key; the owning Server enforces runtime permissions. */
export function createGizClawPeerMonitorClient(
  options: Omit<GizClawControlClientOptions, "apiKey"> & { publicKey: string },
) {
  const key = options.publicKey.replace(/^gizclaw_pk_/, "");
  if (!key) throw new TypeError("publicKey must not be empty");
  return createGizClawControlClient({ ...options, apiKey: `gizclaw_pk_${key}` })
    .device;
}

/** Process-local monitoring with an independent Monitor Token. */
export function createGizClawNodeMonitorClient(
  options: Omit<GizClawControlClientOptions, "apiKey"> & { token: string },
): { get(): Promise<NodeSnapshot> } {
  const baseUrl = monitorBaseUrl(options);
  if (!options.token) throw new TypeError("monitor token must not be empty");
  const client = createClient(
    createConfig({ baseUrl, fetch: options.fetch, auth: options.token }),
  );
  return {
    async get() {
      const result = await getNodeMonitor({
        client,
        signal: options.signal,
        throwOnError: false,
      });
      if (
        !result.response?.ok ||
        result.error !== undefined ||
        result.data === undefined
      ) {
        throw GizClawControlError.fromResult(
          "getNodeMonitor",
          result.response,
          result.error,
        );
      }
      return result.data;
    },
  };
}
