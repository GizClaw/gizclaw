import {
  FirmwareRPC,
  GameplayRPC,
  PeerResourceRPC,
  SocialRPC,
  WebRTCRPCClient,
  WorkspaceRPC,
  connectGiznetWebRTC,
  sendGiznetWebRTCOffer,
} from "@gizclaw/webrtc";
import { base64Decode, prepareEncryptedGiznetWebRTCOffer } from "@gizclaw/webrtc/signaling";
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
  pets: PlayResourceRow[];
  rewards: PlayResourceRow[];
  runWorkspace?: PlayWorkspaceState;
  warnings: string[];
  wallet?: PlayResourceRow;
  walletTransactions: PlayResourceRow[];
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
  id: string;
  raw?: unknown;
  subtitle?: string;
  title: string;
  updated_at?: string;
}

export interface PlayMemoryStats {
  raw?: unknown;
  total?: number;
}

export interface PlayMemoryRecall {
  hits: PlayResourceRow[];
  raw?: unknown;
}

export async function connectPlaySession(runtime: RuntimeContext): Promise<PlaySession> {
  if (runtime.context == null) {
    throw new Error("Play WebRTC session requires a selected context.");
  }
  if (!runtime.private_key_base64) {
    throw new Error("Play WebRTC session requires injected private key material.");
  }
  if (!runtime.signaling_url) {
    throw new Error("Play WebRTC session requires a signaling URL.");
  }
  const pc = new RTCPeerConnection();
  await connectGiznetWebRTC({
    pc,
    prepareOffer: (offerSDP) =>
      prepareEncryptedGiznetWebRTCOffer(
        {
          clientPrivateKey: base64Decode(runtime.private_key_base64 ?? ""),
          clientPublicKey: runtime.context?.local_public_key,
          serverPublicKey: runtime.context?.server_public_key ?? "",
        },
        offerSDP,
      ),
    sendOffer: (offer, signal) => sendGiznetWebRTCOffer(offer, { signal, url: runtime.signaling_url }),
  });
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

export function createPlayDataClientFromPeerConnection(pc: RTCPeerConnection): PlayDataClient {
  const rpc = new WebRTCRPCClient(pc);
  return createRPCPlayDataClient({
    firmware: new FirmwareRPC(rpc),
    gameplay: new GameplayRPC(rpc),
    resources: new PeerResourceRPC(rpc),
    social: new SocialRPC(rpc),
    workspace: new WorkspaceRPC(rpc),
  });
}

export function createRPCPlayDataClient({
  firmware,
  gameplay,
  resources,
  social,
  workspace,
}: {
  firmware: FirmwareRPC;
  gameplay: GameplayRPC;
  resources: PeerResourceRPC;
  social: SocialRPC;
  workspace: WorkspaceRPC;
}): PlayDataClient {
  return {
    async loadSnapshot(): Promise<PlaySnapshot> {
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
        credentials,
        pets,
        rewards,
        wallet,
        walletTransactions,
      ] = await Promise.all([
        captureCall("server.run.workspace.get", () => workspace.getRunWorkspace()),
        captureCall("server.run.workspace.history", () => workspace.listRunWorkspaceHistory({ limit: 30 })),
        captureCall("server.run.workspace.memory.stats", () => workspace.getRunWorkspaceMemoryStats()),
        captureCall("server.contact.list", () => social.listContacts()),
        captureCall("server.friend.list", () => social.listFriends()),
        captureCall("server.friend_group.list", () => social.listFriendGroups()),
        captureCall("server.firmware.list", () => firmware.listFirmwares()),
        captureCall("server.workspace.list", () => resources.listWorkspaces()),
        captureCall("server.workflow.list", () => resources.listWorkflows()),
        captureCall("server.model.list", () => resources.listModels()),
        captureCall("server.credential.list", () => resources.listCredentials()),
        captureCall("server.pet.list", () => gameplay.listPets()),
        captureCall("server.reward.list", () => gameplay.listRewards()),
        captureCall("server.wallet.get", () => gameplay.getWallet()),
        captureCall("server.wallet.transactions.list", () => gameplay.listWalletTransactions()),
      ]);
      return {
        contacts: listItems(contacts.value).map((item) => itemToResourceRow(item, "contact")),
        credentials: listItems(credentials.value).map((item) => itemToResourceRow(item, "credential")),
        firmwares: listItems(firmwares.value).map((item) => itemToResourceRow(item, "firmware")),
        friendGroups: listItems(friendGroups.value).map((item) => itemToResourceRow(item, "friend-group")),
        friends: listItems(friends.value).map((item) => itemToResourceRow(item, "friend")),
        history: listItems(history.value).map(itemToHistoryRow),
        memoryStats: memoryStatsToRow(memoryStats.value),
        models: listItems(models.value).map((item) => itemToResourceRow(item, "model")),
        pets: listItems(pets.value).map((item) => itemToResourceRow(item, "pet")),
        rewards: listItems(rewards.value).map((item) => itemToResourceRow(item, "reward")),
        runWorkspace: workspaceState(runWorkspace.value),
        wallet: isRecord(wallet.value) ? itemToResourceRow(wallet.value, "wallet") : undefined,
        walletTransactions: listItems(walletTransactions.value).map((item) => itemToResourceRow(item, "wallet-transaction")),
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
          credentials,
          pets,
          rewards,
          wallet,
          walletTransactions,
        ].flatMap((item) => (item.warning ? [item.warning] : [])),
        workflows: listItems(workflows.value).map((item) => itemToResourceRow(item, "workflow")),
        workspaces: listItems(workspaces.value).map((item) => itemToResourceRow(item, "workspace")),
      };
    },
    playHistory(historyID: string): Promise<unknown> {
      return workspace.playRunWorkspaceHistory({ history_id: historyID });
    },
    async recallMemory(query: string): Promise<PlayMemoryRecall> {
      const raw = await workspace.recallRunWorkspaceMemory({ limit: 8, query });
      return {
        hits: listItems(raw).map((item) => itemToResourceRow(item, "memory")),
        raw,
      };
    },
    reloadWorkspace(): Promise<unknown> {
      return workspace.reloadRunWorkspace();
    },
    setWorkspace(workspaceName: string): Promise<unknown> {
      return workspace.setRunWorkspace({ workspace_name: workspaceName });
    },
  };
}

export function getInjectedPlayDataClient(): PlayDataClient | undefined {
  return window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__;
}

async function captureCall<T>(label: string, fn: () => Promise<T>): Promise<{ value?: T; warning?: string }> {
  try {
    return { value: await fn() };
  } catch (err) {
    return { warning: `${label}: ${errorMessage(err)}` };
  }
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
    workspace_name: stringValue(value.workspace_name) ?? stringValue(value.workspaceName),
  };
}

function memoryStatsToRow(value: unknown): PlayMemoryStats | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    raw: value,
    total: numberValue(value.total) ?? numberValue(value.count) ?? numberValue(value.entries),
  };
}

function listItems(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value;
  }
  if (isRecord(value)) {
    for (const key of ["items", "data", "resources", "history", "entries", "hits", "messages"]) {
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
    text: stringValue(record.text) ?? stringValue(record.transcript) ?? stringValue(record.content),
    type: stringValue(record.type) ?? stringValue(record.role),
    updated_at: stringValue(record.updated_at) ?? stringValue(record.created_at) ?? stringValue(record.time),
  };
}

function itemToResourceRow(item: unknown, prefix: string): PlayResourceRow {
  const record = isRecord(item) ? item : {};
  const metadata = isRecord(record.metadata) ? record.metadata : {};
  const id =
    stringValue(record.id) ??
    stringValue(record.name) ??
    stringValue(record.public_key) ??
    stringValue(record.friend_public_key) ??
    stringValue(record.friend_group_id) ??
    stringValue(record.group_id) ??
    stringValue(metadata.name) ??
    `${prefix}-${hashJSON(item)}`;
  const title = stringValue(record.title) ?? stringValue(record.display_name) ?? stringValue(record.name) ?? stringValue(metadata.name) ?? id;
  return {
    id,
    raw: item,
    subtitle:
      relationSubtitle(record) ??
      stringValue(record.description) ??
      stringValue(record.role) ??
      stringValue(record.my_role) ??
      stringValue(record.status),
    title,
    updated_at: stringValue(record.updated_at) ?? stringValue(record.created_at),
  };
}

function relationSubtitle(record: Record<string, unknown>): string | undefined {
  const owner = stringValue(record.owner_public_key) ?? stringValue(record.ownerPublicKey);
  const friend = stringValue(record.friend_public_key) ?? stringValue(record.friendPublicKey);
  if (owner != null && friend != null) {
    return `${owner} <-> ${friend}`;
  }
  return undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
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
