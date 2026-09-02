// Scenario clients: an ephemeral WebRTC device peer built on
// `@gizclaw/gizclaw`, and the Public HTTP surface reached through
// `@gizclaw/gizclaw-control`.
import { randomBytes } from "node:crypto";

import { x25519 } from "@noble/curves/ed25519.js";
import wrtc from "@roamhq/wrtc";
import {
  connectGiznetWebRTCFromEndpoint,
  GizClawDeviceControlError,
  type GizClawDeviceStatus,
  type GizClawPeerRPCHandlers,
} from "@gizclaw/gizclaw";
import { base58Encode } from "@gizclaw/gizclaw/signaling";
import { createPeerRPCClient, type PeerRPCClient } from "@gizclaw/gizclaw/rpc";
import {
  createGizClawControlClient,
  type GizClawControlClient,
} from "@gizclaw/gizclaw-control";

import type { ClientSpec, Step } from "./document.ts";
import type { Variables } from "./variables.ts";

const CONNECT_TIMEOUT_MS = 30_000;
const RPC_TIMEOUT_MS = 30_000;

export type HTTPStepResult = { body: unknown; status: number };

// httpBaseURL turns a client access point into an HTTP origin, dropping any
// path, query and fragment.
export function httpBaseURL(accessPoint: string): string {
  const trimmed = accessPoint.trim();
  if (trimmed === "") {
    throw new Error("invalid access point");
  }
  const withScheme = trimmed.includes("://") ? trimmed : `http://${trimmed}`;
  const parsed = new URL(withScheme);
  if (parsed.host === "") {
    throw new Error(`invalid access point ${accessPoint}`);
  }
  return `${parsed.protocol}//${parsed.host}`;
}

function bearerToken(headers: Record<string, string>): string | undefined {
  for (const [name, value] of Object.entries(headers)) {
    if (name.toLowerCase() !== "authorization") {
      continue;
    }
    const match = /^Bearer\s+(.+)$/u.exec(value);
    return match?.[1];
  }
  return undefined;
}

// scriptedDeviceControlError reads the {error_code, error_message} form a
// scenario uses to make a device provider fail.
function scriptedDeviceControlError(
  response: unknown,
): GizClawDeviceControlError | undefined {
  if (
    response == null ||
    typeof response !== "object" ||
    Array.isArray(response)
  ) {
    return undefined;
  }
  const record = response as Record<string, unknown>;
  if (!Object.hasOwn(record, "error_code")) {
    return undefined;
  }
  const code = record["error_code"];
  if (typeof code !== "number" || !Number.isInteger(code)) {
    throw new Error("error_code must be an integer");
  }
  const message = record["error_message"];
  return new GizClawDeviceControlError(
    code,
    typeof message === "string" ? message : "",
  );
}

function asObject(value: unknown): Record<string, unknown> {
  return value != null && typeof value === "object" && !Array.isArray(value)
    ? { ...(value as Record<string, unknown>) }
    : {};
}

export class ScenarioClient {
  private readonly controlClients = new Map<string, GizClawControlClient>();

  readonly name: string;
  readonly endpoint: string;
  readonly fingerprint: string;
  readonly rpc: PeerRPCClient;
  readonly inbound: Map<string, number>;
  private readonly pc: wrtc.RTCPeerConnection;

  private constructor(
    name: string,
    endpoint: string,
    fingerprint: string,
    pc: wrtc.RTCPeerConnection,
    rpc: PeerRPCClient,
    inbound: Map<string, number>,
  ) {
    this.name = name;
    this.endpoint = endpoint;
    this.fingerprint = fingerprint;
    this.pc = pc;
    this.rpc = rpc;
    this.inbound = inbound;
  }

  // connect brings up one ephemeral device peer with every client.* provider
  // the document's steps script for it installed before signaling starts.
  static async connect(
    name: string,
    spec: ClientSpec,
    steps: Step[],
    variables: Variables,
  ): Promise<ScenarioClient> {
    const endpoint = variables.resolveString(spec.access_point, "access_point");
    const privateKey = new Uint8Array(randomBytes(32));
    const fingerprint = base58Encode(x25519.getPublicKey(privateKey)).slice(
      0,
      12,
    );
    const inbound = new Map<string, number>();
    const handlers = buildHandlers(name, steps, variables, inbound);
    const pc = new wrtc.RTCPeerConnection();
    try {
      await connectGiznetWebRTCFromEndpoint({
        clientPrivateKey: privateKey,
        endpoint,
        pc: pc as unknown as RTCPeerConnection,
        peerRPCHandlers: handlers,
        signal: AbortSignal.timeout(CONNECT_TIMEOUT_MS),
      });
    } catch (error) {
      pc.close();
      throw error;
    }
    const rpc = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
      requestTimeoutMs: RPC_TIMEOUT_MS,
    });
    const client = new ScenarioClient(
      name,
      endpoint,
      fingerprint,
      pc,
      rpc,
      inbound,
    );
    if (spec.registration_token != null && spec.registration_token !== "") {
      const token = variables.resolveString(
        spec.registration_token,
        "registration_token",
      );
      await rpc.call("server.register", { token });
    }
    return client;
  }

  // callRPC sends one unary Peer RPC and returns the decoded response object.
  async callRPC(
    method: string,
    params: unknown,
    signal?: AbortSignal,
  ): Promise<unknown> {
    return (await this.rpc.call(
      method as Parameters<PeerRPCClient["call"]>[0],
      params as never,
      { signal },
    )) as unknown;
  }

  // callHTTP sends one Public HTTP request through the control SDK so the
  // request building, bearer injection and response decoding under test are
  // the ones a controller app would use.
  async callHTTP(
    method: "GET" | "POST" | "PUT" | "DELETE",
    pathWithQuery: string,
    headers: Record<string, string>,
    body: unknown,
    signal?: AbortSignal,
  ): Promise<HTTPStepResult> {
    const token = bearerToken(headers);
    const control = this.controlFor(token);
    const extraHeaders = Object.fromEntries(
      Object.entries(headers).filter(
        ([name]) => name.toLowerCase() !== "authorization",
      ),
    );
    const result = (await control.client.request({
      body: body === undefined ? undefined : body,
      headers: extraHeaders,
      method,
      security:
        token == null ? undefined : [{ scheme: "bearer", type: "http" }],
      signal,
      throwOnError: false,
      url: pathWithQuery,
    })) as { data?: unknown; error?: unknown; response?: Response };
    const response = result.response;
    if (response == null) {
      throw new Error("http request produced no response");
    }
    // A 204 carries no body; the Go runner leaves such a step value unset so
    // captures and expectations are skipped.
    if (response.status === 204) {
      return { body: undefined, status: response.status };
    }
    return {
      body: response.ok ? result.data : result.error,
      status: response.status,
    };
  }

  private controlFor(token: string | undefined): GizClawControlClient {
    const key = token ?? "";
    const existing = this.controlClients.get(key);
    if (existing != null) {
      return existing;
    }
    const created = createGizClawControlClient({
      apiKey: token == null || token === "" ? "unauthenticated" : token,
      baseUrl: httpBaseURL(this.endpoint),
    });
    this.controlClients.set(key, created);
    return created;
  }

  close(): void {
    this.pc.close();
  }
}

// buildHandlers turns the document's client_rpc steps for one client into the
// device-side providers the SDK installs, and counts every inbound call.
function buildHandlers(
  clientName: string,
  steps: Step[],
  variables: Variables,
  inbound: Map<string, number>,
): GizClawPeerRPCHandlers {
  const handlers: GizClawPeerRPCHandlers = { deviceControl: {} };
  const control = handlers.deviceControl!;
  const count = (method: string): void => {
    inbound.set(method, (inbound.get(method) ?? 0) + 1);
  };
  // A device always answers client.info.get, even when no step scripts it, so
  // the server sees the same device surface as it does from the Go and Dart
  // runners.
  let deviceInfo: Record<string, unknown> = {};
  handlers.deviceInfo = () => {
    count("client.info.get");
    return deviceInfo as never;
  };

  for (const step of steps) {
    if (step.client !== clientName || step.client_rpc == null) {
      continue;
    }
    const method = step.client_rpc.method;
    const scripted = variables.resolve(step.client_rpc.response ?? null);
    const failure = scriptedDeviceControlError(scripted);
    const scriptedObject = asObject(scripted);
    inbound.set(method, 0);

    switch (method) {
      case "client.info.get":
        deviceInfo = scriptedObject;
        break;
      case "client.identifiers.get":
        handlers.deviceIdentifiers = () => {
          count(method);
          return scriptedObject as never;
        };
        break;
      case "client.device.status.get":
        control.status = () => {
          count(method);
          if (failure != null) throw failure;
          return scriptedObject as GizClawDeviceStatus;
        };
        break;
      case "client.device.volume.set":
        // The scripted status is echoed with the requested level and mute
        // state so an HTTP round trip can assert them.
        control.setVolume = (level, muted) => {
          count(method);
          if (failure != null) throw failure;
          return {
            ...scriptedObject,
            muted,
            volume: level,
          } as GizClawDeviceStatus;
        };
        break;
      case "client.device.sound.play":
        control.playSound = () => {
          count(method);
          if (failure != null) throw failure;
        };
        break;
      case "client.device.reboot":
        control.reboot = () => {
          count(method);
          if (failure != null) throw failure;
        };
        break;
      case "client.wifi.status.get":
        control.wifiStatus = () => {
          count(method);
          if (failure != null) throw failure;
          return scriptedObject as { connected: boolean };
        };
        break;
      case "client.wifi.saved.list":
        control.savedWifi = () => {
          count(method);
          if (failure != null) throw failure;
          const networks = scriptedObject["networks"];
          return Array.isArray(networks)
            ? (networks as { ssid: string }[])
            : [];
        };
        break;
      case "client.wifi.saved.forget":
        control.forgetWifi = () => {
          count(method);
          if (failure != null) throw failure;
        };
        break;
      default:
        throw new Error(`unsupported client RPC ${method}`);
    }
  }
  return handlers;
}
