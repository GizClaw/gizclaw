import {
  GizClawControlError,
  createGizClawNodeMonitorClient,
} from "@gizclaw/gizclaw-control";
import { z } from "zod";
import type { ConsoleServer } from "@/lib/config";
import { isLocal } from "@/lib/config";

export const nodeSchema = z.object({
  public_key: z.string(),
  role: z.string(),
  // Nodes built before the snapshot carried build identity omit both fields.
  version: z.string().optional(),
  build_commit: z.string().optional(),
  time: z.string(),
  uptime_seconds: z.number(),
  goroutines: z.number(),
  heap_bytes: z.number(),
  transport: z.object({
    connections: z.number(),
    services: z.number(),
    inbound_service_channels: z.number().int().nonnegative(),
    rx_bytes: z.number(),
    tx_bytes: z.number(),
  }),
});
export type NodeSnapshot = z.infer<typeof nodeSchema>;

export async function loadNode(
  server: ConsoleServer,
  signal: AbortSignal,
): Promise<NodeSnapshot> {
  const client = createGizClawNodeMonitorClient({
    baseUrl: server.url,
    allowInsecureTransport:
      new URL(server.url).protocol === "http:" &&
      isLocal(new URL(server.url).hostname),
    signal,
    token: server.monitorToken,
  });
  return nodeSchema.parse(await client.get());
}

export function nodeErrorMessage(error: unknown): string {
  if (error instanceof GizClawControlError) {
    return [error.status, error.code, error.message]
      .filter((value) => value !== undefined)
      .join(" · ");
  }
  if (error instanceof TypeError) {
    return `${error.message}（可能是地址不可达、证书不受信任或节点未开放跨域）`;
  }
  return error instanceof Error ? error.message : String(error);
}
