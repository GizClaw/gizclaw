import type {
  AdminContactObject,
  AdminFriendObject,
  AdminFriendGroupObject,
} from "@gizclaw/gizclaw/admin";

import {
  decodeRouteParam,
  encodeRouteParam,
} from "@/views/admin/full/lib/route-param";

import { formatShortKey } from "../../lib/format";

export { decodeRouteParam };

export function friendDetailPath(friend: AdminFriendObject): string {
  return `/social/friends/${encodeRouteParam(friend.owner_public_key)}/${encodeRouteParam(friend.id)}`;
}

export function contactDetailPath(contact: AdminContactObject): string {
  return `/social/contacts/${encodeRouteParam(contact.owner_public_key)}/${encodeRouteParam(contact.id)}`;
}

export function friendRelationID(a: string, b: string): string {
  return [a.trim(), b.trim()].sort().join(":");
}

export function friendGroupDetailPath(group: AdminFriendGroupObject): string {
  return `/social/friend-groups/${encodeRouteParam(group.id)}`;
}

export function socialWorkspaceName(value: string | undefined): string {
  return value?.trim() ? value : "—";
}

export function socialPeerLabel(value: string | undefined): string {
  return value?.trim() ? formatShortKey(value) : "No peer";
}
