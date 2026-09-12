import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { parseRoute as parseConsoleRoute } from "@gizclaw/assistant/routes";
import { Pause, Play } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfigDialog } from "@/components/config-dialog";
import { LoginPage } from "@/pages/login";
import { OverviewPage } from "@/pages/overview";
import { ServerDetailPage } from "@/pages/server-detail";
import { LogsPage } from "@/pages/logs";
import { PeersPage } from "@/pages/peers";
import { PeerDetailPage } from "@/pages/peer-detail";
import { useFleet } from "@/hooks/use-fleet";
import { useWatchedPeers } from "@/hooks/use-peers";
import { exportConfig, parseConfig, type ConsoleConfig } from "@/lib/config";
import { clearAll, clearConfig, readConfig, saveConfig } from "@/lib/store";
import { peerId } from "@/lib/peers";
import { AssistantButton } from "@/assistant/assistant-button";
import type { ConsoleRuntimeDeps } from "@/assistant/console-runtime";
import { PageViewProvider, type PageViewHolder } from "@/assistant/page-view";

export type Route =
  | { page: "overview" }
  | { page: "server"; id: string }
  | { page: "logs"; query: string }
  | { page: "peers" }
  | { page: "peer"; publicKey: string };

// The route syntax lives in @gizclaw/assistant so the assistant navigates with
// exactly the hashes this router reads.
function parseRoute(hash: string): Route {
  const route = parseConsoleRoute(hash);
  return route.page === "logs"
    ? { page: "logs", query: route.query ?? "" }
    : route;
}

function useHashRoute(): Route {
  const [hash, setHash] = useState(window.location.hash);
  useEffect(() => {
    const update = () => setHash(window.location.hash);
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, []);
  return parseRoute(hash);
}

export function App() {
  const [config, setConfig] = useState<ConsoleConfig | undefined>();
  const [restoring, setRestoring] = useState(true);
  const [storageError, setStorageError] = useState("");
  const [paused, setPaused] = useState(false);
  const [windowSeconds, setWindowSeconds] = useState(120);
  const route = useHashRoute();

  useEffect(() => {
    let cancelled = false;
    readConfig()
      .then((text) => {
        if (cancelled || text === "") return;
        setConfig(parseConfig(text));
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setStorageError(
            error instanceof Error ? error.message : String(error),
          );
        }
      })
      .finally(() => {
        if (!cancelled) setRestoring(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const fleet = useFleet(config?.servers ?? [], 5000, paused);
  // Device polling is slower than node polling: each watched device costs three
  // HTTP requests to the device's owning Server.
  const watch = useWatchedPeers(15000, paused);
  const importPeers = watch.importPeers;

  // The assistant reads the latest console state through refs at call time.
  const latest = useRef({ config, fleet, watch });
  latest.current = { config, fleet, watch };
  const pageView = useRef<PageViewHolder>({ current: undefined }).current;
  const assistantDeps = useMemo<Omit<ConsoleRuntimeDeps, "signal">>(
    () => ({
      config: () => latest.current.config!,
      fleet: () => latest.current.fleet,
      peers: () => latest.current.watch.peers,
      peerStates: () => latest.current.watch.states,
      watch: (peer) => latest.current.watch.add(peer),
      view: pageView,
      window,
      now: Date.now,
    }),
    [pageView],
  );

  const connect = useCallback(
    (next: ConsoleConfig, text: string, remember: boolean) => {
      setConfig(next);
      setStorageError("");
      importPeers(next.peers, next.deviceEndpoint ?? window.location.origin);
      const stored = remember ? saveConfig(text) : clearConfig();
      stored.catch((error: unknown) =>
        setStorageError(error instanceof Error ? error.message : String(error)),
      );
    },
    [importPeers],
  );

  const logout = useCallback(() => {
    setConfig(undefined);
    window.location.hash = "";
    clearAll().catch((error: unknown) =>
      setStorageError(error instanceof Error ? error.message : String(error)),
    );
  }, []);

  const [configOpen, setConfigOpen] = useState(false);
  const configText = config ? exportConfig(config, watch.peers) : "";
  const applyConfig = useCallback(
    (next: ConsoleConfig, text: string) => {
      setConfig(next);
      importPeers(next.peers, next.deviceEndpoint ?? window.location.origin);
      saveConfig(text).catch((error: unknown) =>
        setStorageError(error instanceof Error ? error.message : String(error)),
      );
    },
    [importPeers],
  );

  if (restoring) {
    return (
      <div className="flex min-h-screen items-center justify-center text-xs text-muted-foreground">
        正在读取本地配置…
      </div>
    );
  }
  if (!config) {
    return (
      <>
        {storageError && (
          <div className="mx-auto max-w-3xl px-6 pt-6">
            <Alert>
              <AlertTitle>本地配置不可用</AlertTitle>
              <AlertDescription>{storageError}</AlertDescription>
            </Alert>
          </div>
        )}
        <LoginPage onConnect={connect} />
      </>
    );
  }

  const activeId = route.page === "server" ? route.id : undefined;
  const activePeer =
    route.page === "peer"
      ? watch.peers.find((item) => item.publicKey === route.publicKey)
      : undefined;
  const active = config.servers.find((server) => server.id === activeId);
  return (
    <AppShell
      title={config.name}
      servers={config.servers}
      activeId={active?.id}
      page={route.page === "peer" ? "peers" : route.page}
      breadcrumb={[
        "Console",
        route.page === "logs" ? "日志查询" : active ? active.name : "集群总览",
      ]}
      onExport={() => setConfigOpen(true)}
      onLogout={logout}
    >
      <div className="flex flex-wrap items-center justify-end gap-2">
        <Select
          aria-label="流量时间窗口"
          value={windowSeconds}
          onChange={(event) => setWindowSeconds(Number(event.target.value))}
        >
          <option value={120}>最近 2 分钟</option>
          <option value={600}>最近 10 分钟</option>
          <option value={1800}>最近 30 分钟</option>
        </Select>
        <Button variant="outline" size="sm" onClick={() => setPaused(!paused)}>
          {paused ? <Play size={14} /> : <Pause size={14} />}
          {paused ? "继续轮询" : "暂停轮询"}
        </Button>
      </div>
      {storageError && (
        <Alert>
          <AlertTitle>本地配置保存失败</AlertTitle>
          <AlertDescription>{storageError}</AlertDescription>
        </Alert>
      )}
      {activeId && !active && (
        <Alert>
          <AlertTitle>找不到该节点</AlertTitle>
          <AlertDescription>
            配置中没有 id 为 {activeId} 的节点。
          </AlertDescription>
        </Alert>
      )}
      <AssistantButton
        assistant={config.assistant}
        runtimeDeps={assistantDeps}
        endpoint={config.deviceEndpoint ?? window.location.origin}
        onOpenConfig={() => setConfigOpen(true)}
      />
      <ConfigDialog
        open={configOpen}
        text={configText}
        onOpenChange={setConfigOpen}
        onApply={applyConfig}
      />
      <PageViewProvider holder={pageView}>
        {route.page === "peer" ? (
          activePeer ? (
            <PeerDetailPage
              peer={activePeer}
              state={watch.states[peerId(activePeer)]}
              windowSeconds={windowSeconds}
            />
          ) : (
            <Alert>
              <AlertTitle>找不到该设备</AlertTitle>
              <AlertDescription>
                关注列表里没有这个公钥，请先在设备监控页添加。
              </AlertDescription>
            </Alert>
          )
        ) : route.page === "peers" ? (
          <PeersPage
            config={config}
            peers={watch.peers}
            states={watch.states}
            onAdd={watch.add}
            onRemove={watch.remove}
            storageError={watch.storageError}
          />
        ) : route.page === "logs" ? (
          <LogsPage
            key={route.query}
            peers={watch.peers}
            initialQuery={route.query}
          />
        ) : active ? (
          <ServerDetailPage
            server={active}
            state={fleet[active.id]}
            windowSeconds={windowSeconds}
          />
        ) : (
          <OverviewPage
            config={config}
            fleet={fleet}
            windowSeconds={windowSeconds}
          />
        )}
      </PageViewProvider>
    </AppShell>
  );
}
