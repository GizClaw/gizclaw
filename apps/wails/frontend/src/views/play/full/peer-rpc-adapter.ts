import { applyGiznetServerInfoICEServers, fetchGiznetServerInfo, sendGiznetWebRTCOffer } from "@gizclaw/gizclaw";
import {
  RPC_METHODS,
  type ContactObject as RPCContactObject,
  type Firmware as RPCFirmware,
  type FriendGroupInviteTokenGetResponse as RPCFriendGroupInviteTokenGetResponse,
  type FriendGroupMemberMutableRole as RPCFriendGroupMemberMutableRole,
  type FriendGroupMemberObject as RPCFriendGroupMemberObject,
  type FriendGroupObject as RPCFriendGroupObject,
  type FriendInviteTokenGetResponse as RPCFriendInviteTokenGetResponse,
  type FriendObject as RPCFriendObject,
  type Badge as RPCBadge,
  type GameResult as RPCGameResult,
  type GameRuleset as RPCGameRuleset,
  type Pet as RPCPet,
  type PetPresentation as RPCPetPresentation,
  type PeerRPCMethodName,
  type PointsAccount as RPCPointsAccount,
  type PointsTransaction as RPCPointsTransaction,
  type RewardGrant as RPCRewardGrant,
  type PeerRPCClient,
  type PeerRunHistoryEntry as RPCPeerRunHistoryEntry,
  type PeerRunMemoryStatsResponse as RPCPeerRunMemoryStatsResponse,
  type PeerRunRecallHit as RPCPeerRunRecallHit,
  type PeerRunRecallResponse as RPCPeerRunRecallResponse,
  type PeerRunWorkspaceState as RPCPeerRunWorkspaceState,
  type RPCMethodMap,
  type Workspace as RPCWorkspace,
  type WorkspaceParameters as RPCWorkspaceParameters,
} from "@gizclaw/gizclaw/rpc";
import { base64Decode, prepareEncryptedGiznetWebRTCOffer } from "@gizclaw/gizclaw/signaling";
import type { RuntimeContext } from "../../../lib/runtime/types";

type ApiResult<T> = { data?: T; error?: unknown };
type RequestOptions = {
  body?: Record<string, unknown>;
  path?: Record<string, unknown>;
  query?: Record<string, unknown>;
  [key: string]: unknown;
};

let currentRPC: PeerRPCClient | undefined;
let currentDataClient: PlayDataClientLike | undefined;
let currentRuntime: RuntimeContext | undefined;

type PlayDataClientLike = {
  loadSnapshot(): Promise<any>;
  adoptPet?(params: Record<string, unknown>): Promise<unknown>;
  deletePet?(params: Record<string, unknown>): Promise<unknown>;
  drivePet?(params: Record<string, unknown>): Promise<unknown>;
  downloadBadgeDefPixa?(params: Record<string, unknown>): Promise<unknown>;
  downloadPetDefPixa?(params: Record<string, unknown>): Promise<unknown>;
  downloadPetPixa?(params: Record<string, unknown>): Promise<unknown>;
  getBadge?(params: Record<string, unknown>): Promise<unknown>;
  getGameResult?(params: Record<string, unknown>): Promise<unknown>;
  getGameRuleset?(params: Record<string, unknown>): Promise<unknown>;
  getPet?(params: Record<string, unknown>): Promise<unknown>;
  getPetPresentation?(params: Record<string, unknown>): Promise<unknown>;
  getPoints?(params: Record<string, unknown>): Promise<unknown>;
  getPointsTransaction?(params: Record<string, unknown>): Promise<unknown>;
  getRewardGrant?(params: Record<string, unknown>): Promise<unknown>;
  listBadges?(params: Record<string, unknown>): Promise<unknown>;
  listGameResults?(params: Record<string, unknown>): Promise<unknown>;
  listPets?(params: Record<string, unknown>): Promise<unknown>;
  listPointsTransactions?(params: Record<string, unknown>): Promise<unknown>;
  listRewardGrants?(params: Record<string, unknown>): Promise<unknown>;
  playHistory?(historyID: string): Promise<unknown>;
  putPet?(params: Record<string, unknown>): Promise<unknown>;
  recallMemory?(query: string): Promise<unknown>;
  reloadWorkspace?(): Promise<unknown>;
  setWorkspace?(workspaceName: string): Promise<unknown>;
};

export function configurePlayRPCClient(rpc: PeerRPCClient): void {
  currentRPC = rpc;
}

export function clearPlayRPCClient(rpc: PeerRPCClient): void {
  if (currentRPC === rpc) {
    currentRPC = undefined;
  }
}

export function configurePlayDataClient(client: PlayDataClientLike): void {
  currentDataClient = client;
}

export function clearPlayDataClient(client: PlayDataClientLike): void {
  if (currentDataClient === client) {
    currentDataClient = undefined;
  }
}

export function configurePlayRuntime(runtime: RuntimeContext): void {
  currentRuntime = runtime;
}

export function clearPlayRuntime(runtime: RuntimeContext): void {
  if (currentRuntime === runtime) {
    currentRuntime = undefined;
  }
}

export function hasInjectedPlayDataClient(): boolean {
  return currentDataClient != null;
}

async function rpcResult<M extends PeerRPCMethodName>(method: M, params: RPCMethodMap[M]["request"]): Promise<ApiResult<RPCMethodMap[M]["response"]>> {
  if (currentRPC == null) {
    return { error: new Error("Play RPC client is not connected.") };
  }
  try {
    const data = await currentRPC.call(method, params);
    return { data };
  } catch (error) {
    return { error };
  }
}

async function snapshotResult<T = any>(key: string): Promise<ApiResult<T>> {
  if (currentDataClient == null) {
    return { error: new Error("Play data client is not connected.") };
  }
  try {
    const snapshot = await currentDataClient.loadSnapshot();
    return { data: { items: snapshot[key] ?? [] } as T };
  } catch (error) {
    return { error };
  }
}

function params(options?: RequestOptions): Record<string, unknown> {
  return {
    ...(options?.query ?? {}),
    ...(options?.path ?? {}),
    ...(options?.body ?? {}),
  };
}

function callRPC<M extends PeerRPCMethodName>(method: M, options?: RequestOptions): Promise<ApiResult<RPCMethodMap[M]["response"]>> {
  return rpcResult(method, params(options) as RPCMethodMap[M]["request"]);
}

async function injectedResult<T>(method: keyof PlayDataClientLike, options?: RequestOptions): Promise<ApiResult<T>> {
  if (currentDataClient == null) {
    return { error: new Error("Play data client is not connected.") };
  }
  const fn = currentDataClient[method];
  if (typeof fn !== "function") {
    return { error: new Error(`Injected play data client does not implement ${String(method)}.`) };
  }
  try {
    return { data: await (fn as (params: Record<string, unknown>) => Promise<T>)(params(options)) };
  } catch (error) {
    return { error };
  }
}

async function callRPCBinary<M extends PeerRPCMethodName>(method: M, options?: RequestOptions): Promise<ApiResult<{ body: Uint8Array; result: RPCMethodMap[M]["response"] }>> {
  if (currentRPC == null) {
    return { error: new Error("Play RPC client is not connected.") };
  }
  try {
    const data = await currentRPC.callBinary(method, params(options) as RPCMethodMap[M]["request"]);
    return { data };
  } catch (error) {
    return { error };
  }
}

export type ContactObject = RPCContactObject;
export type FriendGroupInviteTokenGetResponse = RPCFriendGroupInviteTokenGetResponse;
export type FriendGroupMemberMutableRole = RPCFriendGroupMemberMutableRole;
export type FriendGroupMemberObject = RPCFriendGroupMemberObject;
export type FriendGroupObject = RPCFriendGroupObject;
export type FriendInviteTokenGetResponse = RPCFriendInviteTokenGetResponse;
export type FriendObject = RPCFriendObject;
export type BadgeObject = RPCBadge;
export type Firmware = RPCFirmware;
export type GameResultObject = RPCGameResult;
export type GameRulesetObject = RPCGameRuleset;
export type PetObject = RPCPet;
export type PetPresentationObject = RPCPetPresentation;
export type PointsAccountObject = RPCPointsAccount;
export type PointsTransactionObject = RPCPointsTransaction;
export type RewardGrantObject = RPCRewardGrant;
export type PeerRunHistoryEntry = RPCPeerRunHistoryEntry;
export type PeerRunMemoryStatsResponse = RPCPeerRunMemoryStatsResponse & {
  updated_at?: string;
};
export type PeerRunRecallHit = RPCPeerRunRecallHit & {
  timestamp?: number | string;
};
export type PeerRunRecallResponse = RPCPeerRunRecallResponse;
export type PlayWorkspaceMode = string;
export type PlayWorkspaceState = RPCPeerRunWorkspaceState & {
  active_workspace_name?: string;
  state?: string;
  workspace_mode?: string;
};
export type PlayVoiceStreamEvent = any;
export type WebRtcSessionDescription = RTCSessionDescriptionInit;
export type Workspace = RPCWorkspace;
export type WorkspaceParameters = RPCWorkspaceParameters;

function normalizeInjectedRecallResponse(value: unknown): PeerRunRecallResponse {
  const record = isRecord(value) ? value : {};
  const rawHits = Array.isArray(record.hits) ? record.hits : [];
  return {
    available: record.available !== false,
    hits: rawHits.map((item, index): PeerRunRecallHit => {
      const hit = isRecord(item) ? item : {};
      const id = String(hit.id ?? hit.source_id ?? `hit-${index}`);
      const snippet = String(hit.snippet ?? hit.text ?? hit.title ?? hit.subtitle ?? "");
      return {
        id,
        score: typeof hit.score === "number" ? hit.score : 0,
        snippet,
        ...(hit.created_at != null ? { created_at: String(hit.created_at) } : {}),
        ...(hit.source_id != null ? { source_id: String(hit.source_id) } : {}),
        ...(hit.source_type != null ? { source_type: String(hit.source_type) } : {}),
        ...(hit.timestamp != null ? { timestamp: hit.timestamp as string | number } : {}),
      };
    }),
    ...(typeof record.message === "string" ? { message: record.message } : {}),
  };
}

function normalizeInjectedWorkspaceState(value: unknown): PlayWorkspaceState {
  const record = isRecord(value) ? value : {};
  const workspaceName = String(record.workspace_name ?? record.active_workspace_name ?? record.name ?? "");
  return {
    workspace_name: workspaceName,
    runtime_state: record.runtime_state === "running" || record.runtime_state === "starting" || record.runtime_state === "stopping" || record.runtime_state === "stopped" || record.runtime_state === "error"
      ? record.runtime_state
      : workspaceName === ""
        ? "stopped"
        : "running",
    ...(record.active_workspace_name != null ? { active_workspace_name: String(record.active_workspace_name) } : {}),
    ...(record.agent_type != null ? { agent_type: String(record.agent_type) } : {}),
    ...(record.history_available != null ? { history_available: Boolean(record.history_available) } : {}),
    ...(record.memory_stats_available != null ? { memory_stats_available: Boolean(record.memory_stats_available) } : {}),
    ...(record.message != null ? { message: String(record.message) } : {}),
    ...(record.mode != null ? { mode: String(record.mode) } : {}),
    ...(record.pending_workspace_name != null ? { pending_workspace_name: String(record.pending_workspace_name) } : {}),
    ...(record.recall_available != null ? { recall_available: Boolean(record.recall_available) } : {}),
    ...(record.selected_workspace_name != null ? { selected_workspace_name: String(record.selected_workspace_name) } : {}),
    ...(record.started_at != null ? { started_at: String(record.started_at) } : {}),
    ...(record.state != null ? { state: String(record.state) } : {}),
    ...(record.updated_at != null ? { updated_at: String(record.updated_at) } : {}),
    ...(record.workflow_name != null ? { workflow_name: String(record.workflow_name) } : {}),
    ...(record.workspace_mode != null ? { workspace_mode: String(record.workspace_mode) } : {}),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export const listPeerContacts = (options?: RequestOptions) => currentDataClient ? snapshotResult("contacts") : callRPC(RPC_METHODS["server.contact.list"], options);
export const createPeerContact = (options: RequestOptions) => callRPC(RPC_METHODS["server.contact.create"], options);
export const putPeerContact = (options: RequestOptions) => callRPC(RPC_METHODS["server.contact.put"], options);
export const deletePeerContact = (options: RequestOptions) => callRPC(RPC_METHODS["server.contact.delete"], options);

export const getPeerFriendInviteToken = () => callRPC(RPC_METHODS["server.friend.invite_token.get"]);
export const createPeerFriendInviteToken = () => callRPC(RPC_METHODS["server.friend.invite_token.create"]);
export const clearPeerFriendInviteToken = () => callRPC(RPC_METHODS["server.friend.invite_token.clear"]);
export const addPeerFriend = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend.add"], options);
export const listPeerFriends = (options?: RequestOptions) => currentDataClient ? snapshotResult("friends") : callRPC(RPC_METHODS["server.friend.list"], options);
export const deletePeerFriend = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend.delete"], options);

export const listPeerFriendGroups = (options?: RequestOptions) => currentDataClient ? snapshotResult("friendGroups") : callRPC(RPC_METHODS["server.friend_group.list"], options);
export const getPeerFriendGroup = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.get"], options);
export const createPeerFriendGroup = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.create"], options);
export const joinPeerFriendGroup = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.join"], options);
export const getPeerFriendGroupInviteToken = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.invite_token.get"], options);
export const createPeerFriendGroupInviteToken = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.invite_token.create"], options);
export const clearPeerFriendGroupInviteToken = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.invite_token.clear"], options);
export const listPeerFriendGroupMembers = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.members.list"], options);
export const addPeerFriendGroupMember = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.members.add"], options);
export const putPeerFriendGroupMember = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.members.put"], options);
export const deletePeerFriendGroupMember = (options: RequestOptions) => callRPC(RPC_METHODS["server.friend_group.members.delete"], options);

export const getPeerRunWorkspace = async () => currentDataClient ? { data: normalizeInjectedWorkspaceState((await currentDataClient.loadSnapshot()).runWorkspace) } : callRPC(RPC_METHODS["server.run.workspace.get"]);
export const setPeerRunWorkspace = async (options: RequestOptions) => currentDataClient ? { data: normalizeInjectedWorkspaceState(await currentDataClient.setWorkspace?.(String(options.body?.workspace_name ?? ""))) } : callRPC(RPC_METHODS["server.run.workspace.set"], options);
export const reloadPeerRunWorkspace = async () => currentDataClient ? { data: normalizeInjectedWorkspaceState(await currentDataClient.reloadWorkspace?.()) } : callRPC(RPC_METHODS["server.run.workspace.reload"]);
export const listPeerRunWorkspaceHistory = async (options?: RequestOptions) => currentDataClient ? { data: { items: (await currentDataClient.loadSnapshot()).history ?? [] } } : callRPC(RPC_METHODS["server.run.workspace.history"], options);
export const playPeerRunWorkspaceHistory = async (options: RequestOptions) => currentDataClient ? { data: await currentDataClient.playHistory?.(String(options.body?.history_id ?? "")) } : callRPC(RPC_METHODS["server.run.workspace.history.play"], options);
export const getPeerRunWorkspaceMemoryStats = async () => currentDataClient ? { data: (await currentDataClient.loadSnapshot()).memoryStats } : callRPC(RPC_METHODS["server.run.workspace.memory.stats"]);
export const recallPeerRunWorkspaceMemory = async (options: RequestOptions) => currentDataClient ? { data: normalizeInjectedRecallResponse(await currentDataClient.recallMemory?.(String(options.body?.query ?? ""))) } : callRPC(RPC_METHODS["server.run.workspace.recall"], options);
export const setPeerRunWorkspaceMode = (options: RequestOptions) => callRPC(RPC_METHODS["server.run.workspace.set"], options);
export const getPeerRunWorkspaceDetails = (options?: RequestOptions) => callRPC(RPC_METHODS["server.workspace.get"], options);
export const putPeerRunWorkspaceDetails = (options: RequestOptions) => callRPC(RPC_METHODS["server.workspace.put"], options);
export const listPeerWorkspaceHistory = (options: RequestOptions) => callRPC(RPC_METHODS["server.workspace.history.list"], options);
export const getPeerWorkspaceHistoryAudio = async (options: RequestOptions): Promise<ApiResult<Blob>> => {
  const result = await callRPCBinary(RPC_METHODS["server.workspace.history.audio.get"], options);
  if (result.error != null || result.data == null) {
    return { error: result.error ?? new Error("Workspace history audio response was empty.") };
  }
  const audio = new Uint8Array(result.data.body.byteLength);
  audio.set(result.data.body);
  return {
    data: new Blob([audio.buffer], {
      type: result.data.result.mime_type || "audio/ogg",
    }),
  };
};

export const listPeerFirmwares = (options?: RequestOptions) => currentDataClient ? snapshotResult("firmwares") : callRPC(RPC_METHODS["server.firmware.list"], options);
export const listPeerWorkspaces = (options?: RequestOptions) => currentDataClient ? snapshotResult("workspaces") : callRPC(RPC_METHODS["server.workspace.list"], options);
export const listPeerWorkflows = (options?: RequestOptions) => currentDataClient ? snapshotResult("workflows") : callRPC(RPC_METHODS["server.workflow.list"], options);
export const listPeerModels = (options?: RequestOptions) => currentDataClient ? snapshotResult("models") : callRPC(RPC_METHODS["server.model.list"], options);
export const listPeerCredentials = (options?: RequestOptions) => currentDataClient ? snapshotResult("credentials") : callRPC(RPC_METHODS["server.credential.list"], options);
export const listPeerVoices = (options?: RequestOptions) => currentDataClient ? snapshotResult("voices") : callRPC(RPC_METHODS["server.voice.list"], options);
export const listClientVoices = listPeerVoices;

export const getPeerGameRuleset = (options?: RequestOptions) => currentDataClient ? injectedResult("getGameRuleset", options) : callRPC(RPC_METHODS["server.game_ruleset.get"], options);
export const listPeerPets = (options?: RequestOptions) => currentDataClient ? injectedResult("listPets", options) : callRPC(RPC_METHODS["server.pet.list"], options);
export const getPeerPet = (options: RequestOptions) => currentDataClient ? injectedResult("getPet", options) : callRPC(RPC_METHODS["server.pet.get"], options);
export const getPeerPetPresentation = (options: RequestOptions) => currentDataClient?.getPetPresentation ? injectedResult("getPetPresentation", options) : callRPC(RPC_METHODS["server.pet.presentation.get"], options);
export const adoptPeerPet = (options: RequestOptions) => currentDataClient ? injectedResult("adoptPet", options) : callRPC(RPC_METHODS["server.pet.adopt"], options);
export const putPeerPet = (options: RequestOptions) => currentDataClient ? injectedResult("putPet", options) : callRPC(RPC_METHODS["server.pet.put"], options);
export const deletePeerPet = (options: RequestOptions) => currentDataClient ? injectedResult("deletePet", options) : callRPC(RPC_METHODS["server.pet.delete"], options);
export const drivePeerPet = (options: RequestOptions) => currentDataClient ? injectedResult("drivePet", options) : callRPC(RPC_METHODS["server.pet.drive"], options);
export const getPeerPetDefPixa = async (options: RequestOptions): Promise<ApiResult<Blob>> => {
  if (currentDataClient != null) {
    const result = await injectedResult<Blob | ArrayBuffer | Uint8Array>("downloadPetDefPixa", options);
    return normalizeInjectedBinary(result);
  }
  const result = await callRPCBinary(RPC_METHODS["server.pet_def.pixa.download"], options);
  return binaryBlobResult(result);
};
export const getPeerPetPixa = async (options: RequestOptions): Promise<ApiResult<Blob>> => {
  if (currentDataClient != null) {
    const result = await injectedResult<Blob | ArrayBuffer | Uint8Array>("downloadPetPixa", options);
    return normalizeInjectedBinary(result);
  }
  const result = await callRPCBinary(RPC_METHODS["server.pet.pixa.download"], options);
  return binaryBlobResult(result);
};
export const getPeerPoints = (options?: RequestOptions) => currentDataClient ? injectedResult("getPoints", options) : callRPC(RPC_METHODS["server.points.get"], options);
export const listPeerPointsTransactions = (options?: RequestOptions) => currentDataClient ? injectedResult("listPointsTransactions", options) : callRPC(RPC_METHODS["server.points.transactions.list"], options);
export const getPeerPointsTransaction = (options: RequestOptions) => currentDataClient ? injectedResult("getPointsTransaction", options) : callRPC(RPC_METHODS["server.points.transactions.get"], options);
export const listPeerBadges = (options?: RequestOptions) => currentDataClient ? injectedResult("listBadges", options) : callRPC(RPC_METHODS["server.badge.list"], options);
export const getPeerBadge = (options: RequestOptions) => currentDataClient ? injectedResult("getBadge", options) : callRPC(RPC_METHODS["server.badge.get"], options);
export const getPeerBadgeDefPixa = async (options: RequestOptions): Promise<ApiResult<Blob>> => {
  if (currentDataClient != null) {
    const result = await injectedResult<Blob | ArrayBuffer | Uint8Array>("downloadBadgeDefPixa", options);
    return normalizeInjectedBinary(result);
  }
  const result = await callRPCBinary(RPC_METHODS["server.badge_def.pixa.download"], options);
  return binaryBlobResult(result);
};
export const listPeerGameResults = (options?: RequestOptions) => currentDataClient ? injectedResult("listGameResults", options) : callRPC(RPC_METHODS["server.game_result.list"], options);
export const getPeerGameResult = (options: RequestOptions) => currentDataClient ? injectedResult("getGameResult", options) : callRPC(RPC_METHODS["server.game_result.get"], options);
export const listPeerRewardGrants = (options?: RequestOptions) => currentDataClient ? injectedResult("listRewardGrants", options) : callRPC(RPC_METHODS["server.reward_grant.list"], options);
export const getPeerRewardGrant = (options: RequestOptions) => currentDataClient ? injectedResult("getRewardGrant", options) : callRPC(RPC_METHODS["server.reward_grant.get"], options);

export const streamPlayableVoices = async (options?: RequestOptions): Promise<{ stream: AsyncGenerator<PlayVoiceStreamEvent> }> => ({
  stream: (async function* () {
    const result = await listPeerVoices(options);
    if (result.error != null || result.data == null) {
      yield { error: result.error instanceof Error ? result.error.message : String(result.error ?? "Voice list failed.") };
      return;
    }
    const items = Array.isArray((result.data as { items?: unknown[] }).items) ? (result.data as { items: unknown[] }).items : [];
    for (const voice of items) {
      yield { voice };
    }
    yield { done: true };
  })(),
});

function binaryBlobResult<T>(result: ApiResult<{ body: Uint8Array; result: T }>): ApiResult<Blob> {
  if (result.error != null || result.data == null) {
    return { error: result.error ?? new Error("Binary response was empty.") };
  }
  const body = new Uint8Array(result.data.body.byteLength);
  body.set(result.data.body);
  return { data: new Blob([body.buffer], { type: "application/octet-stream" }) };
}

function normalizeInjectedBinary(result: ApiResult<Blob | ArrayBuffer | Uint8Array>): ApiResult<Blob> {
  if (result.error != null || result.data == null) {
    return { error: result.error ?? new Error("Injected binary response was empty.") };
  }
  if (result.data instanceof Blob) {
    return { data: result.data };
  }
  if (result.data instanceof ArrayBuffer) {
    return { data: new Blob([result.data], { type: "application/octet-stream" }) };
  }
  const body = new Uint8Array(result.data.byteLength);
  body.set(result.data);
  return { data: new Blob([body], { type: "application/octet-stream" }) };
}

export const createWebRtcOffer = async (_options: RequestOptions): Promise<ApiResult<WebRtcSessionDescription>> => {
  try {
    const runtime = currentRuntime;
    const sdp = String(_options.body?.sdp ?? "");
    const type = String(_options.body?.type ?? "");
    if (type !== "offer" || sdp === "") {
      throw new Error("Workspace voice signaling requires a WebRTC offer SDP.");
    }
    if (runtime?.context == null) {
      throw new Error("Workspace voice signaling requires a selected context.");
    }
    if (!runtime.private_key_base64) {
      throw new Error("Workspace voice signaling requires injected private key material.");
    }
    if (!runtime.context.endpoint) {
      throw new Error("Workspace voice signaling requires a server endpoint.");
    }
    const serverInfo = await fetchGiznetServerInfo({ endpoint: runtime.context.endpoint });
    const offer = await prepareEncryptedGiznetWebRTCOffer(
      {
        clientPrivateKey: base64Decode(runtime.private_key_base64),
        clientPublicKey: runtime.context.local_public_key,
        serverPublicKey: serverInfo.public_key,
      },
      sdp,
    );
    const encryptedAnswer = await sendGiznetWebRTCOffer(offer, {
      baseUrl: `http://${runtime.context.endpoint}`,
      url: serverInfo.signaling_path,
    });
    const answerSDP = await offer.openAnswer(encryptedAnswer);
    return { data: { sdp: answerSDP, type: "answer" } };
  } catch (error) {
    return { error };
  }
};

export async function configureWebRtcPeerConnection(pc: RTCPeerConnection): Promise<void> {
  const runtime = currentRuntime;
  if (runtime?.context == null) {
    throw new Error("Workspace voice signaling requires a selected context.");
  }
  if (!runtime.context.endpoint) {
    throw new Error("Workspace voice signaling requires a server endpoint.");
  }
  const serverInfo = await fetchGiznetServerInfo({ endpoint: runtime.context.endpoint });
  applyGiznetServerInfoICEServers(pc, serverInfo);
}
