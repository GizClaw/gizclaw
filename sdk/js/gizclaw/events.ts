import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import {
  PeerEventSchema,
  PeerEventType,
  StreamKind,
  type FriendGroupUpdated,
  type FriendRelationshipUpdated,
  type PeerEvent,
  type EventError,
  type WorkspaceHistoryUpdated,
} from "./generated/events/peer_event_pb.js";
import {
  encodeFrame,
  RPC_FRAME_TYPE_BINARY,
  RPC_FRAME_TYPE_EOS,
} from "./index.ts";

export * from "./generated/events/peer_event_pb.js";

export const GIZCLAW_SERVICE_PEER_EVENTS = 0x20;
export const PEER_EVENT_VERSION = 1;
const FRAME_HEADER_SIZE = 4;
const MAX_FRAME_PAYLOAD_SIZE = 0xffff;

export type DecodedPeerStreamEvent = {
  errorCode?: string;
  errorMessage?: string;
  errorRetryable?: boolean;
  friendGroupUpdated?: FriendGroupUpdated;
  friendRelationshipUpdated?: FriendRelationshipUpdated;
  kind?: "text" | "audio" | "video" | "mixed";
  label?: string;
  lastUpdatedAt?: string;
  mimeType?: string;
  streamId?: string;
  text?: string;
  type:
    | "unknown"
    | "bos"
    | "eos"
    | "text.delta"
    | "text.done"
    | "workspace.history.updated"
    | "friend.relationship.updated"
    | "friend_group.updated";
  workspaceHistoryUpdated?: WorkspaceHistoryUpdated;
};

export function createPeerEvent(
  value: Omit<PeerEvent, "$typeName">,
): PeerEvent {
  const event = create(PeerEventSchema, value);
  validatePeerEvent(event);
  return event;
}

export function beginPeerStream(input: {
  kind: StreamKind;
  label?: string;
  mimeType?: string;
  streamId: string;
}): PeerEvent {
  return create(PeerEventSchema, {
    version: PEER_EVENT_VERSION,
    type: PeerEventType.BOS,
    payload: {
      case: "bos",
      value: {
        streamId: input.streamId,
        kind: input.kind,
        label: input.label ?? "",
        mimeType: input.mimeType ?? "",
      },
    },
  });
}

export function endPeerStream(input: {
  error?: Omit<EventError, "$typeName">;
  kind: StreamKind;
  label?: string;
  mimeType?: string;
  streamId: string;
}): PeerEvent {
  return create(PeerEventSchema, {
    version: PEER_EVENT_VERSION,
    type: PeerEventType.EOS,
    payload: {
      case: "eos",
      value: {
        streamId: input.streamId,
        kind: input.kind,
        label: input.label ?? "",
        mimeType: input.mimeType ?? "",
        error: input.error,
      },
    },
  });
}

export function encodePeerEvent(event: PeerEvent): Uint8Array {
  validatePeerEvent(event);
  return toBinary(PeerEventSchema, event);
}

export function decodePeerEvent(bytes: Uint8Array): PeerEvent {
  const event = fromBinary(PeerEventSchema, bytes);
  validateReceivedPeerEvent(event);
  return event;
}

export function encodePeerEventFrame(event: PeerEvent): ArrayBuffer {
  return encodeFrame(RPC_FRAME_TYPE_BINARY, encodePeerEvent(event));
}

export class PeerEventFrameDecoder {
  private buffer = new Uint8Array();

  push(data: ArrayBuffer | Uint8Array): DecodedPeerStreamEvent[] {
    const incoming = data instanceof Uint8Array ? data : new Uint8Array(data);
    const merged = new Uint8Array(this.buffer.length + incoming.length);
    merged.set(this.buffer);
    merged.set(incoming, this.buffer.length);
    this.buffer = merged;
    const out: DecodedPeerStreamEvent[] = [];
    let offset = 0;
    while (this.buffer.length - offset >= FRAME_HEADER_SIZE) {
      const frameStart = offset;
      const view = new DataView(
        this.buffer.buffer,
        this.buffer.byteOffset + offset,
        FRAME_HEADER_SIZE,
      );
      const length = view.getUint16(0, true);
      const type = view.getUint16(2, true);
      if (this.buffer.length - offset < FRAME_HEADER_SIZE + length) break;
      const payloadStart = offset + FRAME_HEADER_SIZE;
      const payload = this.buffer.slice(payloadStart, payloadStart + length);
      offset = payloadStart + length;
      if (type === RPC_FRAME_TYPE_EOS) {
        if (length !== 0) {
          this.discardFrame(frameStart, offset);
          throw new Error("Peer Event EOS frame must be empty.");
        }
        continue;
      }
      if (type !== RPC_FRAME_TYPE_BINARY) {
        this.discardFrame(frameStart, offset);
        throw new Error(`expected Peer Event binary frame, got type ${type}`);
      }
      if (length > MAX_FRAME_PAYLOAD_SIZE) {
        throw new Error(`Peer Event frame is too large: ${length}`);
      }
      try {
        out.push(peerStreamEventView(decodePeerEvent(payload)));
      } catch (error) {
        // The full invalid frame has already been delimited. Drop it before
        // surfacing the protocol error so a caller that chooses to continue
        // cannot get permanently wedged on the same bytes.
        this.discardFrame(frameStart, offset);
        throw error;
      }
    }
    this.buffer = this.buffer.slice(offset);
    return out;
  }

  private discardFrame(start: number, end: number): void {
    const kept = new Uint8Array(start + this.buffer.length - end);
    kept.set(this.buffer.subarray(0, start));
    kept.set(this.buffer.subarray(end), start);
    this.buffer = kept;
  }

  finish(): void {
    if (this.buffer.length !== 0) {
      throw new Error("incomplete Peer Event frame");
    }
  }
}

export function validatePeerEvent(event: PeerEvent): void {
  if (event.version !== PEER_EVENT_VERSION) {
    throw new Error(
      `unsupported Peer Event version ${event.version}, want ${PEER_EVENT_VERSION}`,
    );
  }
  const expectedCase = eventCase(event.type);
  if (expectedCase == null) {
    throw new Error(`unsupported Peer Event type ${event.type}`);
  }
  if (event.payload.case !== expectedCase) {
    throw new Error(
      `Peer Event type ${event.type} requires ${expectedCase} payload, got ${event.payload.case ?? "none"}`,
    );
  }
  validateResourceIdentifiers(event);
}

export function peerStreamEventView(event: PeerEvent): DecodedPeerStreamEvent {
  validateReceivedPeerEvent(event);
  switch (event.payload.case) {
    case "bos":
      return {
        type: "bos",
        streamId: event.payload.value.streamId,
        kind: streamKindName(event.payload.value.kind),
        label: event.payload.value.label,
        mimeType: event.payload.value.mimeType,
      };
    case "eos":
      return {
        type: "eos",
        streamId: event.payload.value.streamId,
        kind: streamKindName(event.payload.value.kind),
        label: event.payload.value.label,
        mimeType: event.payload.value.mimeType,
        errorCode: event.payload.value.error?.code,
        errorMessage: event.payload.value.error?.message,
        errorRetryable: event.payload.value.error?.retryable,
      };
    case "textDelta":
      return {
        type: "text.delta",
        streamId: event.payload.value.streamId,
        label: event.payload.value.label,
        text: event.payload.value.text,
      };
    case "textDone":
      return {
        type: "text.done",
        streamId: event.payload.value.streamId,
        label: event.payload.value.label,
        text: event.payload.value.text,
      };
    case "workspaceHistoryUpdated":
      return {
        type: "workspace.history.updated",
        lastUpdatedAt: unixMillisISOString(
          event.payload.value.lastUpdatedAtUnixMs,
        ),
        workspaceHistoryUpdated: event.payload.value,
      };
    case "friendRelationshipUpdated":
      return {
        type: "friend.relationship.updated",
        friendRelationshipUpdated: event.payload.value,
      };
    case "friendGroupUpdated":
      return {
        type: "friend_group.updated",
        friendGroupUpdated: event.payload.value,
      };
    default:
      return { type: "unknown" };
  }
}

function unixMillisISOString(value: bigint): string {
  const minDateUnixMillis = -8640000000000000n;
  const maxDateUnixMillis = 8640000000000000n;
  if (value < minDateUnixMillis || value > maxDateUnixMillis) {
    throw new Error(`Peer Event timestamp is out of range: ${value}`);
  }
  return new Date(Number(value)).toISOString();
}

function validateReceivedPeerEvent(event: PeerEvent): void {
  if (event.version !== PEER_EVENT_VERSION) {
    throw new Error(
      `unsupported Peer Event version ${event.version}, want ${PEER_EVENT_VERSION}`,
    );
  }
  const expectedCase = eventCase(event.type);
  if (expectedCase == null) {
    if (event.payload.case != null) {
      throw new Error(
        `future Peer Event type ${event.type} must not use a known payload`,
      );
    }
    return;
  }
  if (event.payload.case !== expectedCase) {
    throw new Error(
      `Peer Event type ${event.type} requires ${expectedCase} payload, got ${event.payload.case ?? "none"}`,
    );
  }
  validateResourceIdentifiers(event);
}

function validateResourceIdentifiers(event: PeerEvent): void {
  switch (event.payload.case) {
    case "bos":
    case "eos":
    case "textDelta":
    case "textDone":
      if (event.payload.value.streamId.trim() === "") {
        throw new Error("stream event requires streamId");
      }
      return;
    case "workspaceHistoryUpdated":
      if (event.payload.value.workspaceName.trim() === "") {
        throw new Error("workspace history event requires workspaceName");
      }
      return;
    case "friendRelationshipUpdated":
      if (
        event.payload.value.peerPublicKey.trim() === "" ||
        event.payload.value.workspaceName.trim() === ""
      ) {
        throw new Error(
          "friend relationship event requires peerPublicKey and workspaceName",
        );
      }
      return;
    case "friendGroupUpdated":
      if (
        event.payload.value.friendGroupId.trim() === "" ||
        event.payload.value.workspaceName.trim() === ""
      ) {
        throw new Error(
          "friend group event requires friendGroupId and workspaceName",
        );
      }
  }
}

function eventCase(type: PeerEventType): PeerEvent["payload"]["case"] | null {
  switch (type) {
    case PeerEventType.BOS:
      return "bos";
    case PeerEventType.EOS:
      return "eos";
    case PeerEventType.TEXT_DELTA:
      return "textDelta";
    case PeerEventType.TEXT_DONE:
      return "textDone";
    case PeerEventType.WORKSPACE_HISTORY_UPDATED:
      return "workspaceHistoryUpdated";
    case PeerEventType.FRIEND_RELATIONSHIP_UPDATED:
      return "friendRelationshipUpdated";
    case PeerEventType.FRIEND_GROUP_UPDATED:
      return "friendGroupUpdated";
    default:
      return null;
  }
}

function streamKindName(kind: StreamKind): DecodedPeerStreamEvent["kind"] {
  switch (kind) {
    case StreamKind.TEXT:
      return "text";
    case StreamKind.AUDIO:
      return "audio";
    case StreamKind.VIDEO:
      return "video";
    case StreamKind.MIXED:
      return "mixed";
    default:
      return undefined;
  }
}
