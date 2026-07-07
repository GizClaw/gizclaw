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
  const body = encodeTelemetryFrame(frame);
  const packet = new Uint8Array(body.length + 1);
  packet[0] = GIZCLAW_PROTOCOL_TELEMETRY;
  packet.set(body, 1);
  return packet;
}

export function sendTelemetryPacket(channel: WebRTCRPCDataChannel, frame: TelemetryFrame): void {
  channel.send(encodeTelemetryPacket(frame));
}
