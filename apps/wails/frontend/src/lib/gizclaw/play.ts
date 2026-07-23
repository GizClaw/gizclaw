import { connectGiznetWebRTCFromEndpoint } from "@gizclaw/gizclaw";
import {
  RPC_METHODS,
  createPeerRPCClient,
  type PeerRPCClient,
} from "@gizclaw/gizclaw/rpc";
import { base64Decode } from "@gizclaw/gizclaw/signaling";
import type { RuntimeContext } from "../runtime/types";

export interface PlayDataClient {
  loadSnapshot(): Promise<PlaySnapshot>;
  playHistory(historyID: string): Promise<unknown>;
  recallMemory(query: string): Promise<PlayMemoryRecall>;
  reloadWorkspace(): Promise<unknown>;
  setWorkspace(workspaceName: string): Promise<unknown>;
}

export interface PlaySession extends PlayDataClient {
  close(): void;
}

export interface PlaySnapshot {
  contacts: PlayResourceRow[];
  credentials: PlayResourceRow[];
  firmwares: PlayResourceRow[];
  friendGroups: PlayResourceRow[];
  friends: PlayResourceRow[];
  history: PlayHistoryRow[];
  memoryStats?: PlayMemoryStats;
  models: PlayResourceRow[];
  runtimeProfiles?: Partial<Record<RuntimeCatalogKey, RuntimeProfileMetadata>>;
  runWorkspace?: PlayWorkspaceState;
  voices: PlayResourceRow[];
  warnings: string[];
  workflows: PlayResourceRow[];
  workspaces: PlayResourceRow[];
}

export interface PlayWorkspaceState {
  mode?: string;
  name?: string;
  state?: string;
  workspace_name?: string;
}

export interface PlayHistoryRow {
  id: string;
  name?: string;
  raw?: unknown;
  text?: string;
  type?: string;
  updated_at?: string;
}

export interface PlayResourceRow {
  [key: string]: unknown;
  alias?: string;
  driver?: string;
  id: string;
  i18n?: unknown;
  raw?: unknown;
  subtitle?: string;
  title: string;
  updated_at?: string;
}

type RuntimeCatalogKey = "models" | "voices" | "workflows" | "workspaces";

interface RuntimeProfileMetadata {
  runtime_profile_name: string;
  runtime_profile_revision: string;
}

interface RuntimeCollectionResult extends RuntimeProfileMetadata {
  items: unknown[];
}

export interface PlayMemoryStats {
  raw?: unknown;
  total?: number;
}

export interface PlayMemoryRecall {
  hits: PlayResourceRow[];
  raw?: unknown;
}

export async function connectPlayPeerConnection(
  runtime: RuntimeContext,
): Promise<RTCPeerConnection> {
  if (runtime.context == null) {
    throw new Error("Play WebRTC session requires a selected context.");
  }
  if (!runtime.private_key_base64) {
    throw new Error(
      "Play WebRTC session requires injected private key material.",
    );
  }
  if (!runtime.context.endpoint) {
    throw new Error("Play WebRTC session requires a server endpoint.");
  }
  const pc = new RTCPeerConnection();
  await connectGiznetWebRTCFromEndpoint({
    clientPrivateKey: base64Decode(runtime.private_key_base64),
    clientPublicKey: runtime.context.local_public_key,
    endpoint: runtime.context.endpoint,
    pc,
  });
  return pc;
}

export async function connectPlaySession(
  runtime: RuntimeContext,
): Promise<PlaySession> {
  const pc = await connectPlayPeerConnection(runtime);
  const client = createPlayDataClientFromPeerConnection(pc);
  return {
    close() {
      pc.close();
    },
    loadSnapshot: () => client.loadSnapshot(),
    playHistory: (historyID) => client.playHistory(historyID),
    recallMemory: (query) => client.recallMemory(query),
    reloadWorkspace: () => client.reloadWorkspace(),
    setWorkspace: (workspaceName) => client.setWorkspace(workspaceName),
  };
}

export function createPlayDataClientFromPeerConnection(
  pc: RTCPeerConnection,
): PlayDataClient {
  return createRPCPlayDataClient(createPeerRPCClient(pc));
}

export function createRPCPlayDataClient(rpc: PeerRPCClient): PlayDataClient {
  return {
    async loadSnapshot(): Promise<PlaySnapshot> {
      const collections = [
        "assistants",
        "translates",
        "raids",
        "story-teller",
        "role-play",
      ] as const;
      const [
        runWorkspace,
        history,
        memoryStats,
        contacts,
        friends,
        friendGroups,
        firmwares,
        workspaces,
        workflows,
        models,
        voices,
      ] = await Promise.all([
        captureCall(RPC_METHODS["server.run.workspace.get"], () =>
          rpc.call(RPC_METHODS["server.run.workspace.get"], {}),
        ),
        captureCall(RPC_METHODS["server.run.workspace.history"], () =>
          rpc.call(RPC_METHODS["server.run.workspace.history"], { limit: 30 }),
        ),
        captureCall(RPC_METHODS["server.run.workspace.memory.stats"], () =>
          rpc.call(RPC_METHODS["server.run.workspace.memory.stats"], {}),
        ),
        captureCall(RPC_METHODS["server.contact.list"], () =>
          rpc.call(RPC_METHODS["server.contact.list"], {}),
        ),
        captureCall(RPC_METHODS["server.friend.list"], () =>
          rpc.call(RPC_METHODS["server.friend.list"], {}),
        ),
        captureCall(RPC_METHODS["server.friend_group.list"], () =>
          rpc.call(RPC_METHODS["server.friend_group.list"], {}),
        ),
        captureCall(RPC_METHODS["server.firmware.get"], () =>
          rpc.call(RPC_METHODS["server.firmware.get"], {}),
        ),
        captureCall(RPC_METHODS["server.workspace.list"], async () => ({
          ...(await collectCollections(
            await Promise.all(
              collections.map((collection) =>
                collectCollectionPages(
                  (params) =>
                    rpc.call(RPC_METHODS["server.workspace.list"], params),
                  collection,
                  true,
                ),
              ),
            ),
          )),
        })),
        captureCall(RPC_METHODS["server.workflow.list"], async () => ({
          ...(await collectCollections(
            await Promise.all(
              collections.map((collection) =>
                collectCollectionPages(
                  (params) =>
                    rpc.call(RPC_METHODS["server.workflow.list"], params),
                  collection,
                  true,
                ),
              ),
            ),
          )),
        })),
        captureCall(RPC_METHODS["server.model.list"], () =>
          rpc.call(RPC_METHODS["server.model.list"], {}),
        ),
        captureCall(RPC_METHODS["server.voice.list"], () =>
          rpc.call(RPC_METHODS["server.voice.list"], {}),
        ),
      ]);
      return {
        contacts: listItems(contacts.value).map((item) =>
          itemToResourceRow(item, "contact"),
        ),
        credentials: [],
        firmwares:
          firmwares.value == null
            ? []
            : [itemToResourceRow(firmwares.value, "firmware")],
        friendGroups: listItems(friendGroups.value).map((item) =>
          itemToResourceRow(item, "friend-group"),
        ),
        friends: listItems(friends.value).map((item) =>
          itemToResourceRow(item, "friend"),
        ),
        history: listItems(history.value).map(itemToHistoryRow),
        memoryStats: memoryStatsToRow(memoryStats.value),
        models: listItems(models.value).map((item) =>
          itemToResourceRow(item, "model"),
        ),
        runtimeProfiles: runtimeProfileMetadata({
          models: models.value,
          voices: voices.value,
          workflows: workflows.value,
          workspaces: workspaces.value,
        }),
        runWorkspace: workspaceState(runWorkspace.value),
        voices: listItems(voices.value).map((item) =>
          itemToResourceRow(item, "voice"),
        ),
        warnings: [
          runWorkspace,
          history,
          memoryStats,
          contacts,
          friends,
          friendGroups,
          firmwares,
          workspaces,
          workflows,
          models,
          voices,
        ].flatMap((item) => (item.warning ? [item.warning] : [])),
        workflows: listItems(workflows.value).map((item) =>
          itemToResourceRow(item, "workflow"),
        ),
        workspaces: listItems(workspaces.value).map((item) =>
          itemToResourceRow(item, "workspace"),
        ),
      };
    },
    playHistory(historyID: string): Promise<unknown> {
      return rpc.call(RPC_METHODS["server.run.workspace.history.play"], {
        history_id: historyID,
      });
    },
    async recallMemory(query: string): Promise<PlayMemoryRecall> {
      const raw = await rpc.call(RPC_METHODS["server.run.workspace.recall"], {
        limit: 8,
        query,
      });
      return {
        hits: listItems(raw).map((item) => itemToResourceRow(item, "memory")),
        raw,
      };
    },
    reloadWorkspace(): Promise<unknown> {
      return rpc.call(RPC_METHODS["server.run.workspace.reload"], {});
    },
    setWorkspace(workspaceName: string): Promise<unknown> {
      return rpc.call(RPC_METHODS["server.run.workspace.set"], {
        workspace_name: workspaceName,
      });
    },
  };
}

export function getInjectedPlayDataClient(): PlayDataClient | undefined {
  return window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__;
}

async function captureCall<T>(
  label: string,
  fn: () => Promise<T>,
): Promise<{ value?: T; warning?: string }> {
  try {
    return { value: await fn() };
  } catch (err) {
    return { warning: `${label}: ${errorMessage(err)}` };
  }
}

async function collectCollectionPages(
  call: (params: { collection: string; cursor?: string }) => Promise<unknown>,
  collection: string,
  missingCollectionIsEmpty = false,
): Promise<RuntimeCollectionResult> {
  const items: unknown[] = [];
  const seenCursors = new Set<string>();
  let cursor: string | undefined;
  let runtimeProfileName = "";
  let runtimeProfileRevision = "";
  for (;;) {
    let page: unknown;
    try {
      page = await call(
        cursor == null ? { collection } : { collection, cursor },
      );
    } catch (err) {
      if (cursor == null && missingCollectionIsEmpty && isNotFoundError(err)) {
        return {
          items: [],
          runtime_profile_name: "",
          runtime_profile_revision: "",
        };
      }
      throw err;
    }
    const metadata = profileMetadata(page);
    if (
      runtimeProfileRevision !== "" &&
      (metadata.runtime_profile_name !== runtimeProfileName ||
        metadata.runtime_profile_revision !== runtimeProfileRevision)
    ) {
      throw new Error(
        `${collection}: runtime profile changed while loading pages`,
      );
    }
    runtimeProfileName = metadata.runtime_profile_name;
    runtimeProfileRevision = metadata.runtime_profile_revision;
    items.push(...listItems(page));
    if (!isRecord(page) || page.has_next !== true) {
      return {
        items,
        runtime_profile_name: runtimeProfileName,
        runtime_profile_revision: runtimeProfileRevision,
      };
    }
    const nextCursor = stringValue(page.next_cursor);
    if (nextCursor == null || seenCursors.has(nextCursor)) {
      throw new Error(`${collection}: invalid pagination cursor`);
    }
    seenCursors.add(nextCursor);
    cursor = nextCursor;
  }
}

function collectCollections(
  results: RuntimeCollectionResult[],
): RuntimeCollectionResult {
  const items: unknown[] = [];
  let runtimeProfileName = "";
  let runtimeProfileRevision = "";
  for (const result of results) {
    if (result.runtime_profile_revision !== "") {
      if (
        runtimeProfileRevision !== "" &&
        (result.runtime_profile_name !== runtimeProfileName ||
          result.runtime_profile_revision !== runtimeProfileRevision)
      ) {
        throw new Error("runtime profile changed while loading collections");
      }
      runtimeProfileName = result.runtime_profile_name;
      runtimeProfileRevision = result.runtime_profile_revision;
    }
    items.push(...result.items);
  }
  return {
    items,
    runtime_profile_name: runtimeProfileName,
    runtime_profile_revision: runtimeProfileRevision,
  };
}

function runtimeProfileMetadata(
  values: Record<RuntimeCatalogKey, unknown>,
): Partial<Record<RuntimeCatalogKey, RuntimeProfileMetadata>> {
  const metadata: Partial<Record<RuntimeCatalogKey, RuntimeProfileMetadata>> =
    {};
  for (const [key, value] of Object.entries(values) as Array<
    [RuntimeCatalogKey, unknown]
  >) {
    const profile = profileMetadata(value);
    if (profile.runtime_profile_revision !== "") metadata[key] = profile;
  }
  return metadata;
}

function profileMetadata(value: unknown): RuntimeProfileMetadata {
  const record = isRecord(value) ? value : {};
  return {
    runtime_profile_name: stringValue(record.runtime_profile_name) ?? "",
    runtime_profile_revision:
      stringValue(record.runtime_profile_revision) ?? "",
  };
}

function isNotFoundError(err: unknown): boolean {
  return isRecord(err) && (err.code === 404 || err.code === "404");
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function workspaceState(value: unknown): PlayWorkspaceState | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    mode: stringValue(value.mode),
    name: stringValue(value.name),
    state: stringValue(value.state),
    workspace_name:
      stringValue(value.workspace_name) ?? stringValue(value.workspaceName),
  };
}

function memoryStatsToRow(value: unknown): PlayMemoryStats | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    raw: value,
    total:
      numberValue(value.total) ??
      numberValue(value.count) ??
      numberValue(value.entries),
  };
}

function listItems(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value;
  }
  if (isRecord(value)) {
    for (const key of [
      "items",
      "data",
      "resources",
      "history",
      "entries",
      "hits",
      "messages",
    ]) {
      const items = value[key];
      if (Array.isArray(items)) {
        return items;
      }
    }
  }
  return [];
}

function itemToHistoryRow(item: unknown): PlayHistoryRow {
  const record = isRecord(item) ? item : {};
  const id =
    stringValue(record.history_id) ??
    stringValue(record.id) ??
    stringValue(record.message_id) ??
    stringValue(record.name) ??
    `history-${hashJSON(item)}`;
  return {
    id,
    name: stringValue(record.name),
    raw: item,
    text:
      stringValue(record.text) ??
      stringValue(record.transcript) ??
      stringValue(record.content),
    type: stringValue(record.type) ?? stringValue(record.role),
    updated_at:
      stringValue(record.updated_at) ??
      stringValue(record.created_at) ??
      stringValue(record.time),
  };
}

function itemToResourceRow(item: unknown, prefix: string): PlayResourceRow {
  const record = isRecord(item) ? item : {};
  const metadata = isRecord(record.metadata) ? record.metadata : {};
  const id =
    stringValue(record.alias) ??
    stringValue(record.id) ??
    stringValue(record.name) ??
    stringValue(record.public_key) ??
    stringValue(record.friend_public_key) ??
    stringValue(record.friend_group_id) ??
    stringValue(record.group_id) ??
    stringValue(metadata.name) ??
    `${prefix}-${hashJSON(item)}`;
  const title =
    stringValue(record.title) ??
    stringValue(record.display_name) ??
    localizedDisplayName(record.i18n) ??
    stringValue(record.alias) ??
    stringValue(record.name) ??
    stringValue(metadata.name) ??
    id;
  return {
    ...record,
    alias: stringValue(record.alias),
    driver: stringValue(record.driver),
    id,
    i18n: record.i18n,
    raw: item,
    subtitle:
      relationSubtitle(record) ??
      stringValue(record.description) ??
      stringValue(record.role) ??
      stringValue(record.my_role) ??
      stringValue(record.status),
    title,
    updated_at:
      stringValue(record.updated_at) ?? stringValue(record.created_at),
  };
}

function localizedDisplayName(value: unknown): string | undefined {
  if (!isRecord(value)) return undefined;
  for (const locale of ["en", "zh-CN"]) {
    const translation = value[locale];
    if (isRecord(translation)) {
      const displayName = stringValue(translation.display_name);
      if (displayName != null) return displayName;
    }
  }
  return undefined;
}

function relationSubtitle(record: Record<string, unknown>): string | undefined {
  const owner =
    stringValue(record.owner_public_key) ?? stringValue(record.ownerPublicKey);
  const friend =
    stringValue(record.friend_public_key) ??
    stringValue(record.friendPublicKey);
  if (owner != null && friend != null) {
    return `${owner} <-> ${friend}`;
  }
  return undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value != null && !Array.isArray(value);
}

function hashJSON(value: unknown): string {
  const text = JSON.stringify(value);
  let hash = 0;
  for (let i = 0; i < text.length; i += 1) {
    hash = (hash * 31 + text.charCodeAt(i)) >>> 0;
  }
  return hash.toString(16).padStart(8, "0");
}

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__?: PlayDataClient;
  }
}
