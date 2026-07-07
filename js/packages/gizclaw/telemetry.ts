import type { WebRTCRPCDataChannel } from "./index.ts";
import {
  encodeTelemetryFrame,
  type BatteryObservation,
  type GnssObservation,
  type NetworkObservation,
  type Observation,
  type SystemObservation,
  type TelemetryFrame,
} from "./generated/telemetry/peer_telemetry.ts";

export type {
  BatteryObservation,
  GnssObservation,
  NetworkObservation,
  Observation,
  SystemObservation,
  TelemetryFrame,
} from "./generated/telemetry/peer_telemetry.ts";

export const GIZCLAW_PROTOCOL_TELEMETRY = 0x11;
export const GIZCLAW_MAX_PACKET_MESSAGE_SIZE = 64 * 1024;
const TELEMETRY_OBSERVATION_BODY_KEYS = ["battery", "gnss", "network", "system"] as const;

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

export function encodeTelemetryPacket(frame: TelemetryFrame): Uint8Array {
  validateTelemetryFrame(frame);
  const stampedFrame = frame.observedAtUnixMs == null || frame.observedAtUnixMs === 0 ? { ...frame, observedAtUnixMs: Date.now() } : frame;
  const body = encodeTelemetryFrame(stampedFrame);
  if (body.length > GIZCLAW_MAX_PACKET_MESSAGE_SIZE - 1) {
    throw new Error(`telemetry packet payload is ${body.length} bytes, maximum is ${GIZCLAW_MAX_PACKET_MESSAGE_SIZE - 1}`);
  }
  const packet = new Uint8Array(body.length + 1);
  packet[0] = GIZCLAW_PROTOCOL_TELEMETRY;
  packet.set(body, 1);
  return packet;
}

export function sendTelemetryPacket(channel: WebRTCRPCDataChannel, frame: TelemetryFrame): void {
  channel.send(encodeTelemetryPacket(frame));
}

export function validateTelemetryFrame(frame: TelemetryFrame): void {
  if (!Array.isArray(frame.observations) || frame.observations.length === 0) {
    throw new Error("telemetry frame must contain at least one observation");
  }
  for (const [index, observation] of frame.observations.entries()) {
    if (observation == null || typeof observation !== "object") {
      throw new Error(`telemetry observation ${index} must be an object`);
    }
    const bodyCount = TELEMETRY_OBSERVATION_BODY_KEYS.reduce((count, key) => count + Number(observation[key] != null), 0);
    if (bodyCount !== 1) {
      throw new Error(`telemetry observation ${index} must have exactly one body, got ${bodyCount}`);
    }
  }
}
