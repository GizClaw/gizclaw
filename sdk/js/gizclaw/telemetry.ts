import type { WebRTCRPCDataChannel } from "./index.ts";
import {
  encodeTelemetryFrame,
  type AudioPlayerObservation,
  type BatteryObservation,
  type GnssObservation,
  type KVObservation,
  type NetworkObservation,
  type Observation,
  type OtaObservation,
  type SystemObservation,
  type TelemetryFrame,
} from "./generated/telemetry/peer_telemetry.ts";

export type {
  AudioPlayerObservation,
  BatteryObservation,
  GnssObservation,
  KVObservation,
  NetworkObservation,
  Observation,
  OtaObservation,
  SystemObservation,
  TelemetryFrame,
} from "./generated/telemetry/peer_telemetry.ts";

export { OtaState } from "./generated/telemetry/peer_telemetry.ts";

export const GIZCLAW_EVENT_STREAM_TELEMETRY = 0x40;
export const GIZCLAW_MAX_PACKET_MESSAGE_SIZE = 64 * 1024;
const TELEMETRY_OBSERVATION_BODY_KEYS = [
  "activity",
  "audioplayer",
  "battery",
  "gnss",
  "network",
  "system",
  "ota",
  "kv",
] as const;

export function batteryTelemetry(input: BatteryObservation): Observation {
  return { battery: input };
}

export function gnssTelemetry(input: GnssObservation): Observation {
  return { gnss: input };
}

export function networkTelemetry(input: NetworkObservation): Observation {
  return { network: input };
}

export function systemTelemetry(input: SystemObservation): Observation {
  return { system: input };
}

export function otaTelemetry(input: OtaObservation): Observation {
  return { ota: input };
}

export function encodeTelemetryPacket(frame: TelemetryFrame): Uint8Array {
  validateTelemetryFrame(frame);
  const stampedFrame =
    frame.observedAtUnixMs == null || frame.observedAtUnixMs === 0
      ? { ...frame, observedAtUnixMs: Date.now() }
      : frame;
  const body = encodeTelemetryFrame(stampedFrame);
  if (
    (frame.observations ?? []).some((entry) => entry.kv != null) &&
    body.length > 4096
  ) {
    throw new Error("KV telemetry frame exceeds 4096 encoded bytes");
  }
  if (body.length > GIZCLAW_MAX_PACKET_MESSAGE_SIZE - 1) {
    throw new Error(
      `telemetry packet payload is ${body.length} bytes, maximum is ${GIZCLAW_MAX_PACKET_MESSAGE_SIZE - 1}`,
    );
  }
  const packet = new Uint8Array(body.length + 1);
  packet[0] = GIZCLAW_EVENT_STREAM_TELEMETRY;
  packet.set(body, 1);
  return packet;
}

export function sendTelemetryPacket(
  channel: WebRTCRPCDataChannel,
  frame: TelemetryFrame,
): void {
  channel.send(encodeTelemetryPacket(frame));
}

export function validateTelemetryFrame(frame: TelemetryFrame): void {
  if (!Array.isArray(frame.observations) || frame.observations.length === 0) {
    throw new Error("telemetry frame must contain at least one observation");
  }
  if (frame.observations.filter((entry) => entry?.kv != null).length > 16)
    throw new Error("too many KV entries");
  for (const [index, observation] of frame.observations.entries()) {
    if (observation == null || typeof observation !== "object") {
      throw new Error(`telemetry observation ${index} must be an object`);
    }
    if (observation.kv != null) validateKV(observation.kv);
    const bodyCount = TELEMETRY_OBSERVATION_BODY_KEYS.reduce(
      (count, key) => count + Number(observation[key] != null),
      0,
    );
    if (bodyCount !== 1) {
      throw new Error(
        `telemetry observation ${index} must have exactly one body, got ${bodyCount}`,
      );
    }
  }
}

/** Reports the device's single player state and progress. */
export function audioPlayerTelemetry(
  input: AudioPlayerObservation,
): Observation {
  return { audioplayer: input };
}

const utf8Bytes = (value: string) => new TextEncoder().encode(value).length;
function validateKV(kv: KVObservation): void {
  if (
    typeof kv.key !== "string" ||
    !kv.key.isWellFormed() ||
    utf8Bytes(kv.key) < 1 ||
    utf8Bytes(kv.key) > 64
  )
    throw new Error("invalid KV key");
  if (kv.delete !== undefined) {
    if (kv.delete !== true || kv.jsonValue !== undefined)
      throw new Error("invalid KV deletion");
  } else {
    if (
      typeof kv.jsonValue !== "string" ||
      !kv.jsonValue.isWellFormed() ||
      utf8Bytes(kv.jsonValue) > 1024
    )
      throw new Error("invalid KV JSON value");
    JSON.parse(kv.jsonValue);
  }
}

/** Send a JSON encoding verbatim, preserving large numeric values on the wire. */
export function kvTelemetryJSON(key: string, jsonValue: string): Observation {
  const kv = { key, jsonValue };
  validateKV(kv);
  return { kv };
}
/** Value must be JSON serializable; null, false and zero remain values. */
export function kvTelemetry(key: string, value: unknown): Observation {
  const encoded = JSON.stringify(value, (_key, item: unknown) => {
    if (
      item === undefined ||
      typeof item === "function" ||
      typeof item === "symbol" ||
      typeof item === "bigint" ||
      (typeof item === "number" && !Number.isFinite(item))
    )
      throw new TypeError("KV value is not JSON");
    return item;
  });
  if (encoded === undefined) throw new Error("KV value is not JSON");
  return kvTelemetryJSON(key, encoded);
}
export function deleteKVTelemetry(key: string): Observation {
  const kv = { key, delete: true };
  validateKV(kv);
  return { kv };
}
