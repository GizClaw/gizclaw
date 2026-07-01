import type { Client as AdminServiceClient } from "@gizclaw/adminservice/client";
import {
  listAclPolicyBindings,
  listAclRoles,
  listAclViews,
  listContacts,
  listCredentials,
  listFirmwares,
  listFriendGroups,
  listFriends,
  listMiniMaxTenants,
  listModels,
  listOpenAiTenants,
  listPeers,
  listVoices,
  listVolcTenants,
  listWorkflows,
  listWorkspaces,
} from "@gizclaw/adminservice";
import { createAdminAPIClient } from "@gizclaw/webrtc/adminservice";
import { connectGiznetWebRTC, sendGiznetWebRTCOffer } from "@gizclaw/webrtc";
import type { WebRTCRPCDataChannelFactory } from "@gizclaw/webrtc";
import { base64Decode, prepareEncryptedGiznetWebRTCOffer } from "@gizclaw/webrtc/signaling";
import type { RuntimeContext } from "../runtime/types";

export interface AdminDataClient {
  listSections(): Promise<AdminSection[]>;
}

export interface AdminSession extends AdminDataClient {
  close(): void;
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
  { description: "OpenAI provider tenants.", key: "openai-tenants", list: listOpenAiTenants as ListFn, title: "OpenAI Tenants" },
  { description: "MiniMax provider tenants.", key: "minimax-tenants", list: listMiniMaxTenants as ListFn, title: "MiniMax Tenants" },
  { description: "Volc provider tenants.", key: "volc-tenants", list: listVolcTenants as ListFn, title: "Volc Tenants" },
];

export function createAdminDataClientFromPeerConnection(pc: WebRTCRPCDataChannelFactory): AdminDataClient {
  return createGeneratedAdminDataClient(createAdminAPIClient(pc));
}

export async function connectAdminSession(runtime: RuntimeContext): Promise<AdminSession> {
  if (runtime.context == null) {
    throw new Error("Admin WebRTC session requires a selected context.");
  }
  if (!runtime.private_key_base64) {
    throw new Error("Admin WebRTC session requires injected private key material.");
  }
  if (!runtime.signaling_url) {
    throw new Error("Admin WebRTC session requires a signaling URL.");
  }
  const pc = new RTCPeerConnection();
  await connectGiznetWebRTC({
    addAudioTransceiver: false,
    createPacketDataChannel: false,
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
  const client = createGeneratedAdminDataClient(createAdminAPIClient(pc));
  return {
    close() {
      pc.close();
    },
    listSections: () => client.listSections(),
  };
}

export function createGeneratedAdminDataClient(client: AdminServiceClient): AdminDataClient {
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
