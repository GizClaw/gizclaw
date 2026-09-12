import { z } from "zod";

// A console configuration lists every Server and Edge node the operator wants
// to watch, together with that node's own Monitor Token. Tokens stay in the
// browser: the console talks to each node directly.
const serverSchema = z
  .object({
    id: z.string().trim().min(1).optional(),
    name: z.string().trim().min(1).optional(),
    url: z.string().trim().min(1).optional(),
    role: z.enum(["server", "edge"]).optional(),
    region: z.string().trim().min(1).optional(),
    monitorToken: z.string().trim().min(1),
  })
  .strict();

const peerSchema = z
  .object({
    publicKey: z.string().trim().min(1),
    label: z.string().trim().optional(),
    endpoint: z.string().trim().min(1).optional(),
  })
  .strict();

const assistantSchema = z
  .object({
    apiKey: z.string().trim().min(1),
    model: z.string().trim().min(1).optional(),
    endpoint: z.string().trim().min(1).optional(),
    contextTokens: z.number().int().min(4_000).max(1_000_000).optional(),
  })
  .strict();

const fileSchema = z
  .object({
    name: z.string().trim().min(1).optional(),
    deviceEndpoint: z.string().trim().min(1).optional(),
    peers: z.array(peerSchema).optional(),
    assistant: assistantSchema.optional(),
    servers: z.union([
      z.array(serverSchema).min(1),
      z.record(z.string(), serverSchema),
    ]),
  })
  .strict();

export type ConsoleServer = {
  id: string;
  name: string;
  url: string;
  role: "server" | "edge" | "unknown";
  region?: string;
  monitorToken: string;
};
export type ConsoleConfig = {
  name: string;
  servers: ConsoleServer[];
  /**
   * Optional override for the access point serving the device APIs. The
   * console is normally served from that access point, so device requests go
   * to its own origin and no address is configured at all.
   */
  deviceEndpoint?: string;
  /** Devices carried by the configuration, used to seed the watch list. */
  peers: ConsoleConfigPeer[];
  /** The diagnostic assistant's model access; absent disables the chat. */
  assistant?: ConsoleAssistant;
};

/**
 * An existing device's API key. The assistant calls /openai/v1 with it and
 * uses that device's RuntimeProfile models; the key also controls that device.
 */
export type ConsoleAssistant = {
  apiKey: string;
  model: string;
  /** The node serving /openai/v1; defaults to the device endpoint. */
  endpoint?: string;
  /**
   * Token budget per turn, matching the model's context window; older turns
   * are compacted beyond it. Defaults to the assistant package's budget.
   */
  contextTokens?: number;
};

export type ConsoleConfigPeer = {
  publicKey: string;
  label: string;
  endpoint?: string;
};

export function isLocal(hostname: string) {
  return (
    hostname === "localhost" ||
    hostname === "127.0.0.1" ||
    hostname === "[::1]" ||
    hostname === "::1"
  );
}

function normalizeUrl(raw: string): string {
  const value = /^https?:\/\//i.test(raw) ? raw : `https://${raw}`;
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error(`${raw} 不是合法地址`);
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new Error(`${raw} 不能包含账号密码、查询参数或片段`);
  }
  if (
    url.protocol !== "https:" &&
    !(url.protocol === "http:" && isLocal(url.hostname))
  ) {
    throw new Error(`${raw} 必须使用 HTTPS（localhost 可用 HTTP）`);
  }
  return url.origin;
}

// The API key format of api/http/peer.json.
const API_KEY = /^gizclaw_sk_v1_[A-Za-z0-9_-]{43}$/;

function checkAPIKey(key: string): string {
  if (!API_KEY.test(key)) {
    throw new Error(
      "assistant.apiKey 必须是以 gizclaw_sk_v1_ 开头的设备 API Key",
    );
  }
  return key;
}

function checkToken(token: string): string {
  if (!token.startsWith("gizclaw_mk_") || token.length < 11 + 32) {
    throw new Error("monitorToken 必须以 gizclaw_mk_ 开头并足够长");
  }
  return token;
}

/** Parses a pasted console configuration; every failure names what to fix. */
export function parseConfig(text: string): ConsoleConfig {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch (error) {
    throw new Error(
      `配置不是合法 JSON：${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const parsed = fileSchema.safeParse(data);
  if (!parsed.success) {
    const issue = parsed.error.issues[0];
    throw new Error(
      `配置字段有误：${issue.path.join(".") || "servers"} ${issue.message}`,
    );
  }
  const entries: (readonly [string, z.infer<typeof serverSchema>])[] =
    Array.isArray(parsed.data.servers)
      ? parsed.data.servers.map(
          (server) => [server.id ?? server.url ?? "", server] as const,
        )
      : Object.entries(parsed.data.servers).map(
          ([key, server]) =>
            [key, { ...server, url: server.url ?? key }] as const,
        );
  if (entries.length === 0)
    throw new Error("配置字段有误：servers 至少需要一个节点");
  const servers: ConsoleServer[] = [];
  const seen = new Set<string>();
  for (const [key, server] of entries) {
    if (!server.url) throw new Error(`${key || "节点"} 缺少 url`);
    const id = (server.id ?? key).trim();
    if (!id) throw new Error("节点缺少 id");
    if (seen.has(id)) throw new Error(`节点 id 重复：${id}`);
    seen.add(id);
    servers.push({
      id,
      name: server.name ?? id,
      url: normalizeUrl(server.url),
      role: server.role ?? "unknown",
      region: server.region,
      monitorToken: checkToken(server.monitorToken),
    });
  }
  return {
    name: parsed.data.name ?? "GizClaw 集群",
    servers,
    peers: (parsed.data.peers ?? []).map((peer) => ({
      publicKey: peer.publicKey.replace(/^gizclaw_pk_/, ""),
      label: peer.label ?? "",
      endpoint:
        peer.endpoint === undefined ? undefined : normalizeUrl(peer.endpoint),
    })),
    deviceEndpoint:
      parsed.data.deviceEndpoint === undefined
        ? undefined
        : normalizeUrl(parsed.data.deviceEndpoint),
    assistant: parsed.data.assistant && {
      apiKey: checkAPIKey(parsed.data.assistant.apiKey),
      model: parsed.data.assistant.model ?? "llm",
      endpoint:
        parsed.data.assistant.endpoint === undefined
          ? undefined
          : normalizeUrl(parsed.data.assistant.endpoint),
      contextTokens: parsed.data.assistant.contextTokens,
    },
  };
}

export const sampleConfig = `{
  "name": "生产集群",
  "servers": [
    {
      "id": "server-1",
      "name": "Server 主节点",
      "role": "server",
      "region": "cn-north",
      "url": "https://server1.example.com",
      "monitorToken": "gizclaw_mk_..."
    },
    {
      "id": "edge-1",
      "name": "Edge 华东",
      "role": "edge",
      "url": "https://edge1.example.com",
      "monitorToken": "gizclaw_mk_..."
    }
  ],
  "assistant": {
    "apiKey": "gizclaw_sk_v1_...",
    "model": "llm"
  }
}`;

/**
 * Rebuilds the pasted configuration, including the personal device list, so an
 * operator can carry the whole console setup to another browser.
 */
export function exportConfig(
  config: ConsoleConfig,
  peers: ConsoleConfigPeer[],
): string {
  return `${JSON.stringify(
    {
      name: config.name,
      ...(config.deviceEndpoint === undefined
        ? {}
        : { deviceEndpoint: config.deviceEndpoint }),
      servers: config.servers.map((server) => ({
        id: server.id,
        name: server.name,
        role: server.role === "unknown" ? undefined : server.role,
        region: server.region,
        url: server.url,
        monitorToken: server.monitorToken,
      })),
      peers: peers.map((peer) => ({
        publicKey: peer.publicKey,
        label: peer.label === "" ? undefined : peer.label,
        endpoint: peer.endpoint,
      })),
      assistant: config.assistant,
    },
    null,
    2,
  )}\n`;
}
