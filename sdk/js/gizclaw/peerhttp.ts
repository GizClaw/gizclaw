export * from "./generated/peerhttp/index.ts";
export { client as peerHTTPClient } from "./generated/peerhttp/client.gen.ts";
export {
  createClient as createPeerHTTPClient,
  createConfig as createPeerHTTPConfig,
} from "./generated/peerhttp/client/index.ts";
export type {
  Client as PeerHTTPClient,
  Config as PeerHTTPConfig,
} from "./generated/peerhttp/client/index.ts";
