// Console routes are hash routes; formatRoute and parseRoute must stay the
// exact inverse of parseRoute in web/console/src/App.tsx.

export type ConsoleRoute =
  | { page: "overview" }
  | { page: "server"; id: string }
  | { page: "peers" }
  | { page: "peer"; publicKey: string }
  | { page: "logs"; query?: string };

export function formatRoute(route: ConsoleRoute): string {
  switch (route.page) {
    case "overview":
      return "#/";
    case "server":
      return `#/server/${encodeURIComponent(route.id)}`;
    case "peers":
      return "#/peers";
    case "peer":
      return `#/peer/${encodeURIComponent(route.publicKey)}`;
    case "logs":
      return route.query
        ? `#/logs?q=${encodeURIComponent(route.query)}`
        : "#/logs";
  }
}

export function parseRoute(hash: string): ConsoleRoute {
  const server = /^#\/server\/(.+)$/.exec(hash);
  if (server) return { page: "server", id: decodeURIComponent(server[1]) };
  const logs = /^#\/logs(?:\?q=(.*))?$/.exec(hash);
  if (logs) {
    return logs[1]
      ? { page: "logs", query: decodeURIComponent(logs[1]) }
      : { page: "logs" };
  }
  if (hash === "#/peers") return { page: "peers" };
  const peer = /^#\/peer\/(.+)$/.exec(hash);
  if (peer) return { page: "peer", publicKey: decodeURIComponent(peer[1]) };
  return { page: "overview" };
}
