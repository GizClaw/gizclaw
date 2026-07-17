import {
  type AdminHTTPClient,
  createAdminAPIClient,
  listAclPolicyBindings,
  listAclRoles,
  listAclViews,
  listContacts,
  listCredentials,
  listDashScopeTenants,
  listFirmwares,
  listFriendGroups,
  listFriends,
  listGeminiTenants,
  listMiniMaxTenants,
  listModels,
  listOpenAiTenants,
  listPeers,
  listVoices,
  listVolcTenants,
  listWorkflows,
  listWorkspaces,
} from "@gizclaw/gizclaw/admin";
import { connectGiznetWebRTCFromEndpoint } from "@gizclaw/gizclaw";
import type { WebRTCRPCDataChannelFactory } from "@gizclaw/gizclaw";
import { base64Decode } from "@gizclaw/gizclaw/signaling";
import type { RuntimeContext } from "../runtime/types";

export interface AdminDataClient {
  listSections(): Promise<AdminSection[]>;
}

export interface AdminSession extends AdminDataClient {
  close(): void;
}

export type AdminPeerSessionState =
  | { status: "connecting" | "ready" | "reconnecting" }
  | { error: Error; status: "failed" };

type AdminPeerConnector = (signal: AbortSignal) => Promise<RTCPeerConnection>;

export class AdminPeerSessionManager {
  readonly #connect: AdminPeerConnector;
  readonly #disconnectedGraceMS: number;
  readonly #onState: (state: AdminPeerSessionState) => void;
  #closed = false;
  #connection?: RTCPeerConnection;
  #connectController?: AbortController;
  #disconnectedTimer?: ReturnType<typeof setTimeout>;
  #generation = 0;
  #recovery?: Promise<RTCPeerConnection>;
  #terminalError?: Error;

  constructor({
    connect,
    disconnectedGraceMS = 2_000,
    onState = () => undefined,
  }: {
    connect: AdminPeerConnector;
    disconnectedGraceMS?: number;
    onState?: (state: AdminPeerSessionState) => void;
  }) {
    this.#connect = connect;
    this.#disconnectedGraceMS = disconnectedGraceMS;
    this.#onState = onState;
  }

  async start(): Promise<RTCPeerConnection> {
    this.#onState({ status: "connecting" });
    return this.#replace(undefined, false);
  }

  async connection(): Promise<RTCPeerConnection> {
    if (this.#terminalError != null) {
      throw this.#terminalError;
    }
    if (this.#closed) {
      throw new Error("Admin WebRTC session is closed.");
    }
    if (this.#connection != null && !isUnusableAdminConnection(this.#connection)) {
      return this.#connection;
    }
    return this.recover(this.#connection);
  }

  recover(expected?: RTCPeerConnection): Promise<RTCPeerConnection> {
    if (this.#terminalError != null) {
      return Promise.reject(this.#terminalError);
    }
    if (this.#closed) {
      return Promise.reject(new Error("Admin WebRTC session is closed."));
    }
    if (expected != null && expected !== this.#connection && this.#connection != null && !isUnusableAdminConnection(this.#connection)) {
      return Promise.resolve(this.#connection);
    }
    if (this.#recovery != null) {
      return this.#recovery;
    }
    return this.#replace(expected, true);
  }

  fail(error: unknown): Error {
    const normalized = toError(error);
    if (this.#closed) {
      return normalized;
    }
    this.#terminalError = normalized;
    this.#generation += 1;
    this.#connectController?.abort();
    this.#clearDisconnectedTimer();
    const connection = this.#connection;
    this.#connection = undefined;
    connection?.close();
    this.#onState({ error: normalized, status: "failed" });
    return normalized;
  }

  close(): void {
    if (this.#closed) {
      return;
    }
    this.#closed = true;
    this.#generation += 1;
    this.#connectController?.abort();
    this.#clearDisconnectedTimer();
    const connection = this.#connection;
    this.#connection = undefined;
    connection?.close();
  }

  #replace(expected: RTCPeerConnection | undefined, recovering: boolean): Promise<RTCPeerConnection> {
    if (recovering) {
      this.#onState({ status: "reconnecting" });
    }
    this.#clearDisconnectedTimer();
    const previous = this.#connection;
    this.#connection = undefined;
    previous?.close();
    const generation = ++this.#generation;
    const controller = new AbortController();
    this.#connectController = controller;
    const recovery = this.#connect(controller.signal)
      .then((connection) => {
        if (this.#closed || generation !== this.#generation) {
          connection.close();
          throw new Error("Admin WebRTC session was superseded.");
        }
        this.#connection = connection;
        this.#observe(connection, generation);
        this.#terminalError = undefined;
        this.#onState({ status: "ready" });
        return connection;
      })
      .catch((error: unknown) => {
        if (!this.#closed && generation === this.#generation) {
          this.fail(error);
        }
        throw toError(error);
      })
      .finally(() => {
        if (this.#connectController === controller) {
          this.#connectController = undefined;
        }
        if (this.#recovery === recovery) {
          this.#recovery = undefined;
        }
      });
    this.#recovery = recovery;
    return recovery;
  }

  #observe(connection: RTCPeerConnection, generation: number): void {
    connection.addEventListener("connectionstatechange", () => {
      if (this.#closed || generation !== this.#generation || connection !== this.#connection) {
        return;
      }
      if (connection.connectionState === "failed" || connection.connectionState === "closed") {
        void this.recover(connection).catch(() => undefined);
        return;
      }
      if (connection.connectionState === "disconnected") {
        this.#clearDisconnectedTimer();
        this.#disconnectedTimer = setTimeout(() => {
          this.#disconnectedTimer = undefined;
          if (generation === this.#generation && connection === this.#connection && connection.connectionState === "disconnected") {
            void this.recover(connection).catch(() => undefined);
          }
        }, this.#disconnectedGraceMS);
        return;
      }
      this.#clearDisconnectedTimer();
    });
  }

  #clearDisconnectedTimer(): void {
    if (this.#disconnectedTimer != null) {
      clearTimeout(this.#disconnectedTimer);
      this.#disconnectedTimer = undefined;
    }
  }
}

function isUnusableAdminConnection(connection: RTCPeerConnection): boolean {
  return connection.connectionState === "closed" || connection.connectionState === "failed";
}

function toError(error: unknown): Error {
  return error instanceof Error ? error : new Error(String(error));
}

export interface AdminSection {
  description: string;
  key: string;
  rows: AdminRow[];
  title: string;
}

export interface AdminRow {
  id: string;
  raw?: unknown;
  status?: string;
  subtitle?: string;
  title: string;
  updated_at?: string;
}

type ListFn = (options: Record<string, unknown>) => Promise<unknown>;

interface SectionSpec {
  description: string;
  key: string;
  list: ListFn;
  title: string;
}

const sectionSpecs: SectionSpec[] = [
  { description: "Registered peers and runtime metadata.", key: "peers", list: listPeers as ListFn, title: "Peers" },
  { description: "Workspace workflow definitions.", key: "workflows", list: listWorkflows as ListFn, title: "Workflows" },
  { description: "Workspace instances visible to admin.", key: "workspaces", list: listWorkspaces as ListFn, title: "Workspaces" },
  { description: "Model catalog entries.", key: "models", list: listModels as ListFn, title: "Models" },
  { description: "Credential resources.", key: "credentials", list: listCredentials as ListFn, title: "Credentials" },
  { description: "Voice catalog entries.", key: "voices", list: listVoices as ListFn, title: "Voices" },
  { description: "Firmware records and artifacts.", key: "firmwares", list: listFirmwares as ListFn, title: "Firmwares" },
  { description: "Global contact resources.", key: "contacts", list: listContacts as ListFn, title: "Contacts" },
  { description: "Friend pair resources.", key: "friends", list: listFriends as ListFn, title: "Friends" },
  { description: "Friend group resources.", key: "friend-groups", list: listFriendGroups as ListFn, title: "Friend Groups" },
  { description: "ACL views.", key: "acl-views", list: listAclViews as ListFn, title: "ACL Views" },
  { description: "ACL roles.", key: "acl-roles", list: listAclRoles as ListFn, title: "ACL Roles" },
  { description: "ACL policy bindings.", key: "acl-policy-bindings", list: listAclPolicyBindings as ListFn, title: "ACL Policy Bindings" },
  { description: "Gemini provider tenants.", key: "gemini-tenants", list: listGeminiTenants as ListFn, title: "Gemini Tenants" },
  { description: "DashScope provider tenants.", key: "dashscope-tenants", list: listDashScopeTenants as ListFn, title: "DashScope Tenants" },
  { description: "OpenAI provider tenants.", key: "openai-tenants", list: listOpenAiTenants as ListFn, title: "OpenAI Tenants" },
  { description: "MiniMax provider tenants.", key: "minimax-tenants", list: listMiniMaxTenants as ListFn, title: "MiniMax Tenants" },
  { description: "Volc provider tenants.", key: "volc-tenants", list: listVolcTenants as ListFn, title: "Volc Tenants" },
];

export function createAdminDataClientFromPeerConnection(pc: WebRTCRPCDataChannelFactory): AdminDataClient {
  return createGeneratedAdminDataClient(createAdminAPIClient(pc));
}

export async function connectAdminPeerConnection(runtime: RuntimeContext, signal?: AbortSignal): Promise<RTCPeerConnection> {
  if (runtime.context == null) {
    throw new Error("Admin WebRTC session requires a selected context.");
  }
  if (!runtime.private_key_base64) {
    throw new Error("Admin WebRTC session requires injected private key material.");
  }
  if (!runtime.context.endpoint) {
    throw new Error("Admin WebRTC session requires a server endpoint.");
  }
  const pc = new RTCPeerConnection();
  const controller = new AbortController();
  const abort = () => controller.abort(signal?.reason);
  if (signal?.aborted) {
    abort();
  } else {
    signal?.addEventListener("abort", abort, { once: true });
  }
  const timeout = window.setTimeout(() => controller.abort(), 15_000);
  try {
    await connectGiznetWebRTCFromEndpoint({
      addAudioTransceiver: false,
      clientPrivateKey: base64Decode(runtime.private_key_base64),
      clientPublicKey: runtime.context.local_public_key,
      createPacketDataChannel: true,
      endpoint: runtime.context.endpoint,
      pc,
      signal: controller.signal,
    });
    return pc;
  } catch (error: unknown) {
    pc.close();
    throw error;
  } finally {
    window.clearTimeout(timeout);
    signal?.removeEventListener("abort", abort);
  }
}

export async function connectAdminSession(runtime: RuntimeContext): Promise<AdminSession> {
  const pc = await connectAdminPeerConnection(runtime);
  const client = createGeneratedAdminDataClient(createAdminAPIClient(pc));
  return {
    close() {
      pc.close();
    },
    listSections: () => client.listSections(),
  };
}

export function createGeneratedAdminDataClient(client: AdminHTTPClient): AdminDataClient {
  return {
    async listSections(): Promise<AdminSection[]> {
      const sections: AdminSection[] = [];
      for (const spec of sectionSpecs) {
        const data = await spec.list({
          client,
          responseStyle: "data",
          throwOnError: true,
        });
        sections.push({
          description: spec.description,
          key: spec.key,
          rows: listItems(data).map((item) => itemToRow(item, spec.key)),
          title: spec.title,
        });
      }
      return sections;
    },
  };
}

export function getInjectedAdminDataClient(): AdminDataClient | undefined {
  return window.__GIZCLAW_DESKTOP_TEST_ADMIN_CLIENT__;
}

function listItems(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value;
  }
  if (isRecord(value)) {
    const items = value.items;
    if (Array.isArray(items)) {
      return items;
    }
    const resources = value.resources;
    if (Array.isArray(resources)) {
      return resources;
    }
  }
  return [];
}

function itemToRow(item: unknown, section: string): AdminRow {
  const record = isRecord(item) ? item : {};
  const metadata = isRecord(record.metadata) ? record.metadata : {};
  const spec = isRecord(record.spec) ? record.spec : {};
  const id =
    stringValue(record.id) ??
    stringValue(record.name) ??
    stringValue(record.public_key) ??
    stringValue(record.publicKey) ??
    stringValue(record.relation_id) ??
    stringValue(record.group_id) ??
    stringValue(metadata.name) ??
    stringValue(spec.name) ??
    `${section}-${hashJSON(item)}`;
  const title = stringValue(record.display_name) ?? stringValue(record.title) ?? stringValue(record.name) ?? stringValue(metadata.name) ?? id;
  const subtitle =
    relationSubtitle(record) ??
    stringValue(record.description) ??
    stringValue(spec.description) ??
    stringValue(record.provider) ??
    stringValue(record.kind);
  return {
    id,
    raw: item,
    status: stringValue(record.status) ?? stringValue(record.role) ?? stringValue(record.my_role),
    subtitle,
    title,
    updated_at: stringValue(record.updated_at) ?? stringValue(record.updatedAt) ?? stringValue(metadata.updated_at),
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
    __GIZCLAW_DESKTOP_TEST_ADMIN_CLIENT__?: AdminDataClient;
  }
}
