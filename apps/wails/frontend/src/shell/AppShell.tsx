import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, FormEvent } from "react";
import QRCode from "qrcode";
import {
  Activity,
  ArrowUpRight,
  ChevronLeft,
  CircleStop,
  Cloud,
  Copy,
  Cpu,
  Globe2,
  Laptop,
  FolderOpen,
  Maximize2,
  MoreHorizontal,
  Minus,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Search,
  Server,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";

import { setLocale, useMessages } from "../i18n";
import { getDesktopAPI } from "../lib/runtime/desktop";
import type { PodInput, PodSummary } from "../lib/runtime/types";

export function AppShell() {
  const api = useMemo(() => getDesktopAPI(), []);
  const t = useMessages();
  const [pods, setPods] = useState<PodSummary[]>([]);
  const [selected, setSelected] = useState<PodSummary | null>(null);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<PodSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const refresh = () =>
      api
        .Bootstrap()
        .then(async (state) => {
          setLocale(state.locale);
          setPods(state.pods);
          const checked = await Promise.all(
            state.pods.map((pod) =>
              api.RefreshPodHealth(pod.id).catch(() => pod),
            ),
          );
          setPods(checked);
        })
        .catch((reason) => setError(errorMessage(reason)))
        .finally(() => setLoading(false));
    void refresh();
    const cancel = window.runtime?.EventsOn?.(
      "desktop:open-pod",
      (id: string) => {
        api
          .RefreshPodHealth(id)
          .then((pod) => {
            replacePod(pod);
            setSelected(pod);
          })
          .catch(() =>
            api
              .Bootstrap()
              .then((state) => {
                setLocale(state.locale);
                const pod = state.pods.find((candidate) => candidate.id === id);
                if (pod) {
                  setPods(state.pods);
                  setSelected(pod);
                }
              })
              .catch((reason) => setError(errorMessage(reason))),
          );
      },
    );
    const onFocus = () => {
      if (!document.hidden) void refresh();
    };
    window.addEventListener("focus", onFocus);
    return () => {
      cancel?.();
      window.removeEventListener("focus", onFocus);
    };
  }, [api]);

  function replacePod(next: PodSummary) {
    setPods((current) =>
      current.map((pod) => (pod.id === next.id ? next : pod)),
    );
    setSelected(next);
  }

  async function act(action: () => Promise<PodSummary>) {
    setError("");
    try {
      replacePod(await action());
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }

  async function create(input: PodInput) {
    setError("");
    try {
      const pod = await api.CreatePod(input);
      setPods((current) => [...current, pod]);
      setCreating(false);
      setSelected(pod);
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }

  async function update(input: PodInput) {
    setError("");
    try {
      const pod = await api.UpdatePod(input);
      replacePod(pod);
      setEditing(null);
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }

  async function deletePod(id: string) {
    setError("");
    try {
      await api.DeletePod(id);
      setPods((current) => current.filter((pod) => pod.id !== id));
      setSelected(null);
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }

  function openPod(pod: PodSummary) {
    setSelected(pod);
    void api
      .RefreshPodHealth(pod.id)
      .then(replacePod)
      .catch((reason) => setError(errorMessage(reason)));
  }

  return (
    <main className="desktop-shell">
      <AmbientBackground />
      <div className="window-drag-surface" data-wails-drag />
      <WindowControls />

      {error ? (
        <div className="error-toast">
          <Activity size={15} />
          <span>{error}</span>
          <button
            aria-label={t("close")}
            onClick={() => setError("")}
            type="button"
          >
            <X size={14} />
          </button>
        </div>
      ) : null}

      <section
        className={`pod-canvas ${!loading && pods.length === 0 ? "pod-canvas-empty" : ""}`}
      >
        <div className="pod-grid" aria-label={t("pods")}>
          {loading ? (
            <>
              <span className="pod-skeleton" />
              <span className="pod-skeleton" />
              <span className="pod-skeleton" />
            </>
          ) : null}
          {pods.map((pod, index) => (
            <PodCard
              key={pod.id}
              pod={pod}
              index={index}
              onOpen={() => openPod(pod)}
            />
          ))}
          <button
            className="add-pod-card"
            aria-label={t("addPod")}
            onClick={() => setCreating(true)}
            title={t("addPod")}
            type="button"
          >
            <Plus size={30} strokeWidth={1.7} />
          </button>
        </div>
      </section>

      {selected ? (
        <PodDetail
          api={api}
          pod={selected}
          onChange={replacePod}
          onClose={() => setSelected(null)}
          onDelete={() => void deletePod(selected.id)}
          onEdit={() => setEditing(selected)}
          onError={(reason) => setError(errorMessage(reason))}
          onReveal={() =>
            api
              .RevealPod(selected.id)
              .catch((reason) => setError(errorMessage(reason)))
          }
          run={act}
        />
      ) : null}
      {creating ? (
        <CreatePodDialog onClose={() => setCreating(false)} onSave={create} />
      ) : null}
      {editing ? (
        <PodSettingsDialog
          initial={editing}
          onClose={() => setEditing(null)}
          onSave={update}
        />
      ) : null}
    </main>
  );
}

function WindowControls() {
  const t = useMessages();
  return (
    <div className="window-controls" aria-label={t("windowControls")}>
      <button
        aria-label={t("closeWindow")}
        className="window-control window-close"
        onClick={() => window.runtime?.WindowHide?.()}
        title={t("closeWindow")}
        type="button"
      />
      <button
        aria-label={t("minimizeWindow")}
        className="window-control window-minimize"
        onClick={() => window.runtime?.WindowMinimise?.()}
        title={t("minimizeWindow")}
        type="button"
      >
        <Minus size={9} strokeWidth={2.4} />
      </button>
      <button
        aria-label={t("maximizeWindow")}
        className="window-control window-maximize"
        onClick={() => window.runtime?.WindowToggleMaximise?.()}
        title={t("maximizeWindow")}
        type="button"
      >
        <Maximize2 size={7} strokeWidth={2.2} />
      </button>
    </div>
  );
}

function PodCard({
  pod,
  index,
  onOpen,
}: {
  pod: PodSummary;
  index: number;
  onOpen(): void;
}) {
  const t = useMessages();
  const remoteCount = pod.remote?.servers.length ?? 0;
  const adminCount =
    pod.remote?.servers.filter((server) => server.admin_configured).length ?? 0;
  const running = pod.local?.process.state === "running";
  const online = running || pod.remote?.access_point.state === "reachable";
  const hue = stableHue(pod.id);
  const mode = !pod.valid
    ? t("invalid")
    : pod.mode === "local"
      ? t("local")
      : t("remote");
  return (
    <button
      className={`pod-card pod-card-${pod.valid ? pod.mode : "invalid"}`}
      onClick={onOpen}
      style={
        {
          animationDelay: `${Math.min(index, 8) * 55}ms`,
          "--card-hue": hue,
          "--card-hue-alt": (hue + 42) % 360,
        } as CSSProperties
      }
      type="button"
    >
      <span className="pod-card-top">
        <span className="mode-icon">
          {!pod.valid ? (
            <Activity size={18} />
          ) : pod.mode === "local" ? (
            <Laptop size={18} />
          ) : (
            <Cloud size={18} />
          )}
        </span>
        <span className="mode-chip">{mode}</span>
        <span className={`health-pulse ${online ? "online" : ""}`} />
      </span>
      <span className="pod-card-copy">
        <strong>{pod.name}</strong>
        <small>
          {!pod.valid
            ? pod.error
            : pod.local
              ? running
                ? t("running")
                : t("stopped")
              : `${remoteCount} ${remoteCount === 1 ? t("server") : t("servers")}`}
        </small>
      </span>
      {pod.valid ? (
        <span className="pod-card-capabilities">
          <span
            className={
              pod.local?.admin_configured || adminCount > 0 ? "enabled" : ""
            }
          >
            <Server size={12} /> Admin
          </span>
          <span className={pod.play_configured ? "enabled" : ""}>
            <Sparkles size={12} /> Play
          </span>
        </span>
      ) : null}
    </button>
  );
}

function PodDetail({
  api,
  pod,
  onChange,
  onClose,
  onDelete,
  onEdit,
  onError,
  onReveal,
  run,
}: {
  api: ReturnType<typeof getDesktopAPI>;
  pod: PodSummary;
  onChange(pod: PodSummary): void;
  onClose(): void;
  onDelete(): void;
  onEdit(): void;
  onError(reason: unknown): void;
  onReveal(): void;
  run(action: () => Promise<PodSummary>): Promise<void>;
}) {
  const t = useMessages();
  const [closing, setClosing] = useState(false);
  const [managing, setManaging] = useState(false);
  const [query, setQuery] = useState("");
  const [healthFilter, setHealthFilter] = useState("all");
  const [serverEditor, setServerEditor] = useState<PodServer | "new" | null>(
    null,
  );
  const servers = (pod.remote?.servers ?? []).filter((server) => {
    const matchesQuery = `${server.id} ${server.name} ${server.endpoint}`
      .toLowerCase()
      .includes(query.toLowerCase());
    const matchesHealth =
      healthFilter === "all" || server.health.state === healthFilter;
    return matchesQuery && matchesHealth;
  });
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !serverEditor) setClosing(true);
    };
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  }, [serverEditor]);
  useEffect(() => {
    if (!closing) return;
    const timer = window.setTimeout(onClose, 240);
    return () => window.clearTimeout(timer);
  }, [closing, onClose]);
  return (
    <div
      className={`dialog-backdrop ${closing ? "dialog-closing" : ""}`}
      role="presentation"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) setClosing(true);
      }}
      onAnimationEnd={(event) => {
        if (closing && event.currentTarget === event.target) onClose();
      }}
    >
      <section
        className={`pod-dialog pod-dialog-${pod.mode} ${managing ? "is-managing" : ""}`}
        aria-modal="true"
        role="dialog"
      >
        <div className="dialog-aurora" />
        <header className="pod-dialog-header">
          <span className="mode-icon large">
            {pod.mode === "local" ? (
              <Cpu size={24} />
            ) : pod.mode === "remote" ? (
              <Globe2 size={24} />
            ) : (
              <Activity size={24} />
            )}
          </span>
          <div>
            <span className="mode-chip">
              {pod.mode === "local"
                ? t("local")
                : pod.mode === "remote"
                  ? t("remote")
                  : t("invalid")}
            </span>
            <div className="dialog-title-line">
              <h2>{pod.name}</h2>
              {pod.valid ? (
                <button
                  aria-label={t("renameServer")}
                  className="rename-button"
                  onClick={onEdit}
                  title={t("renameServer")}
                  type="button"
                >
                  <Pencil size={13} />
                </button>
              ) : null}
            </div>
            <p>{pod.description || pod.id}</p>
          </div>
          <details className="pod-menu">
            <summary
              aria-label={t("podActions")}
              className="icon-button"
              title={t("podActions")}
            >
              <MoreHorizontal size={18} />
            </summary>
            <div>
              {pod.valid ? (
                <button onClick={onEdit} type="button">
                  <Pencil size={14} />
                  {t("edit")}
                </button>
              ) : null}
              <button onClick={onReveal} type="button">
                <FolderOpen size={14} />
                {t("reveal")}
              </button>
              <button
                className="danger"
                onClick={() => {
                  if (window.confirm(`${t("confirmDelete")}\n${pod.name}`))
                    onDelete();
                }}
                type="button"
              >
                <Trash2 size={14} />
                {t("deletePod")}
              </button>
            </div>
          </details>
          <button
            aria-label={t("close")}
            className="icon-button close-button"
            onClick={() => setClosing(true)}
            title={t("close")}
            type="button"
          >
            <X size={20} />
          </button>
        </header>
        <div className="pod-dialog-body">
          {!pod.valid ? (
            <div className="invalid-detail">
              <Activity size={26} />
              <h3>{t("invalid")}</h3>
              <p>{pod.error}</p>
              <button
                className="secondary-action"
                onClick={onReveal}
                type="button"
              >
                <FolderOpen size={15} />
                {t("reveal")}
              </button>
            </div>
          ) : pod.local ? (
            <PodDetailPages
              back={
                <LocalManageFace
                  api={api}
                  onError={onError}
                  onShare={() => setManaging(false)}
                  pod={pod}
                  run={run}
                />
              }
              managing={managing}
              front={
                <PodShareFace
                  endpoint={preferredLANAddress(pod.local.lan_addresses)}
                  onManage={() => setManaging(true)}
                  onPlay={() => api.OpenPlay(pod.id).catch(onError)}
                  pod={pod}
                  publicKey={pod.local.server_public_key ?? ""}
                  state={pod.local.process.state}
                />
              }
            />
          ) : (
            <PodDetailPages
              back={
                <RemoteManageFace
                  api={api}
                  healthFilter={healthFilter}
                  onAddServer={() => setServerEditor("new")}
                  onEditServer={setServerEditor}
                  onError={onError}
                  onShare={() => setManaging(false)}
                  onHealthFilter={setHealthFilter}
                  onQuery={setQuery}
                  pod={pod}
                  query={query}
                  run={run}
                  servers={servers}
                />
              }
              managing={managing}
              front={
                <PodShareFace
                  endpoint={pod.remote!.access_point.endpoint}
                  onManage={() => setManaging(true)}
                  onPlay={() => api.OpenPlay(pod.id).catch(onError)}
                  pod={pod}
                  publicKey={pod.remote!.access_point.public_key ?? ""}
                  state={pod.remote!.access_point.state}
                />
              }
            />
          )}
        </div>
        {serverEditor ? (
          <ServerEditorDialog
            server={serverEditor === "new" ? undefined : serverEditor}
            onClose={() => setServerEditor(null)}
            onDelete={
              serverEditor === "new"
                ? undefined
                : async () => {
                    if (!window.confirm(t("confirmDeleteServer"))) return;
                    try {
                      const next = await api.UpdatePod(
                        podInputWithServers(
                          pod,
                          pod.remote!.servers.filter(
                            (server) => server.id !== serverEditor.id,
                          ),
                        ),
                      );
                      onChange(next);
                      setServerEditor(null);
                    } catch (reason) {
                      onError(reason);
                    }
                  }
            }
            onSave={async (draft) => {
              const nextServers =
                serverEditor === "new"
                  ? [...pod.remote!.servers, draft]
                  : pod.remote!.servers.map((server) =>
                      server.id === serverEditor.id ? draft : server,
                    );
              try {
                const next = await api.UpdatePod(
                  podInputWithServers(pod, nextServers),
                );
                onChange(next);
                setServerEditor(null);
              } catch (reason) {
                onError(reason);
              }
            }}
          />
        ) : null}
      </section>
    </div>
  );
}

type PodServer = NonNullable<PodSummary["remote"]>["servers"][number];

function preferredLANAddress(addresses: string[]) {
  return (
    addresses.find((address) =>
      /^192\.168\.\d+\.(?!0:)\d+:\d+$/.test(address),
    ) ??
    addresses.find((address) => !address.startsWith("[")) ??
    addresses[0] ??
    ""
  );
}

function PodDetailPages({
  back,
  managing,
  front,
}: {
  back: React.ReactNode;
  managing: boolean;
  front: React.ReactNode;
}) {
  return (
    <div className={`pod-detail-stage ${managing ? "is-managing" : ""}`}>
      <section
        aria-hidden={managing}
        className="pod-detail-page pod-share-page"
        inert={managing}
      >
        {front}
      </section>
      <section
        aria-hidden={!managing}
        className="pod-detail-page pod-manage-page"
        inert={!managing}
      >
        {back}
      </section>
    </div>
  );
}

function PodShareFace({
  endpoint,
  onManage,
  onPlay,
  pod,
  publicKey,
  state,
}: {
  endpoint: string;
  onManage(): void;
  onPlay(): void;
  pod: PodSummary;
  publicKey: string;
  state: string;
}) {
  const t = useMessages();
  const payload = useMemo(
    () =>
      JSON.stringify({
        version: 1,
        type: "gizclaw-server",
        mode: pod.mode,
        name: pod.name,
        endpoint,
        ...(publicKey ? { server_public_key: publicKey } : {}),
      }),
    [endpoint, pod.mode, pod.name, publicKey],
  );
  return (
    <div className="share-face-layout">
      <div className="qr-card">
        {endpoint ? (
          <QRCodeImage label={t("serverQRCode")} payload={payload} />
        ) : (
          <div className="qr-code qr-unavailable">{t("noLANAddress")}</div>
        )}
        <span>{t("scanToAddServer")}</span>
      </div>
      <div className="share-summary">
        <div className="share-status-line">
          <small>{pod.local ? t("localServer") : t("accessPoint")}</small>
          <Status state={state} />
        </div>
        <div className="share-endpoint share-endpoint-primary">
          <small>{pod.local ? t("lanAddress") : t("accessPoint")}</small>
          <code>{endpoint || t("noLANAddress")}</code>
        </div>
        <div className="share-actions">
          <button
            className="primary-action share-play"
            onClick={onPlay}
            type="button"
          >
            <Sparkles size={17} />
            {t("openPlay")}
          </button>
          <button className="secondary-action" onClick={onManage} type="button">
            <Server size={16} />
            {pod.local ? t("serverControls") : t("manageServers")}
          </button>
        </div>
      </div>
    </div>
  );
}

function QRCodeImage({ label, payload }: { label: string; payload: string }) {
  const [source, setSource] = useState("");
  useEffect(() => {
    let active = true;
    void QRCode.toDataURL(payload, {
      errorCorrectionLevel: "M",
      margin: 2,
      width: 360,
      color: { dark: "#111218", light: "#ffffff" },
    }).then((value) => {
      if (active) setSource(value);
    });
    return () => {
      active = false;
    };
  }, [payload]);
  return (
    <div className="qr-code" data-qr-payload={payload}>
      {source ? (
        <img alt={label} src={source} />
      ) : (
        <span className="qr-placeholder" />
      )}
    </div>
  );
}

function LocalManageFace({
  api,
  onError,
  onShare,
  pod,
  run,
}: {
  api: ReturnType<typeof getDesktopAPI>;
  onError(reason: unknown): void;
  onShare(): void;
  pod: PodSummary;
  run(action: () => Promise<PodSummary>): Promise<void>;
}) {
  const t = useMessages();
  const local = pod.local!;
  return (
    <div className="manage-face local-manage-face">
      <div className="face-toolbar local-face-toolbar">
        <span className="mode-chip">{t("serverControls")}</span>
        <button className="secondary-action" onClick={onShare} type="button">
          <ChevronLeft size={16} />
          {t("shareServer")}
        </button>
      </div>
      <div className="local-control-grid">
        <section className="local-status-card">
          <span
            className={`local-status-icon ${local.process.state === "running" ? "running" : ""}`}
          >
            <Server size={25} />
          </span>
          <div>
            <small>{t("localServer")}</small>
            <strong>
              {local.process.state === "running" ? t("running") : t("stopped")}
            </strong>
          </div>
          <div className="local-listen-address">
            <small>{t("listenAddress")}</small>
            <code>0.0.0.0:{local.port}</code>
          </div>
        </section>
        <div className="local-action-stack">
          <div className="local-power-actions">
            <button
              className="local-control-action start-action"
              disabled={local.process.state === "running"}
              onClick={() => void run(() => api.StartLocalServer(pod.id))}
              type="button"
            >
              <Play size={17} />
              <span>{t("start")}</span>
            </button>
            <button
              className="local-control-action stop-action"
              disabled={local.process.state !== "running"}
              onClick={() => void run(() => api.StopLocalServer(pod.id))}
              type="button"
            >
              <CircleStop size={17} />
              <span>{t("stop")}</span>
            </button>
          </div>
          <button
            className="local-admin-action"
            onClick={() => api.OpenAdmin(pod.id, "local").catch(onError)}
            type="button"
          >
            <span className="local-admin-icon">
              <Server size={18} />
            </span>
            <span>
              <strong>Admin</strong>
              <small>{t("openInBrowser")}</small>
            </span>
            <ArrowUpRight size={15} />
          </button>
        </div>
      </div>
    </div>
  );
}

function RemoteManageFace({
  api,
  healthFilter,
  onAddServer,
  onEditServer,
  onError,
  onShare,
  onHealthFilter,
  onQuery,
  pod,
  query,
  run,
  servers,
}: {
  api: ReturnType<typeof getDesktopAPI>;
  healthFilter: string;
  onAddServer(): void;
  onEditServer(server: PodServer): void;
  onError(reason: unknown): void;
  onShare(): void;
  onHealthFilter(value: string): void;
  onQuery(value: string): void;
  pod: PodSummary;
  query: string;
  run(action: () => Promise<PodSummary>): Promise<void>;
  servers: PodServer[];
}) {
  const t = useMessages();
  return (
    <div className="manage-face remote-manage-face">
      <div className="face-toolbar">
        <div>
          <span className="mode-chip">{t("remote")}</span>
          <h3>{t("manageServers")}</h3>
        </div>
        <button className="secondary-action" onClick={onShare} type="button">
          <ChevronLeft size={16} />
          {t("shareServer")}
        </button>
      </div>
      <div className="remote-toolbar manage-remote-toolbar">
        <label>
          <Search size={16} />
          <input
            aria-label={t("searchServers")}
            onChange={(event) => onQuery(event.target.value)}
            placeholder={t("searchServers")}
            value={query}
          />
        </label>
        <select
          aria-label={t("healthFilter")}
          onChange={(event) => onHealthFilter(event.target.value)}
          value={healthFilter}
        >
          <option value="all">{t("allStates")}</option>
          <option value="reachable">{t("reachable")}</option>
          <option value="unreachable">{t("unreachable")}</option>
          <option value="invalid-response">{t("invalid-response")}</option>
        </select>
        <button
          aria-label={t("refresh")}
          className="secondary-action compact-action"
          onClick={() => void run(() => api.RefreshPodHealth(pod.id))}
          title={t("refresh")}
          type="button"
        >
          <RefreshCw size={15} />
        </button>
        <button className="primary-action" onClick={onAddServer} type="button">
          <Plus size={15} /> {t("addServer")}
        </button>
      </div>
      {servers.length ? (
        <VirtualServerList
          onAdmin={(server) => api.OpenAdmin(pod.id, server.id).catch(onError)}
          onEdit={onEditServer}
          resetKey={`${pod.id}\u0000${query}\u0000${healthFilter}`}
          servers={servers}
        />
      ) : (
        <div className="no-servers">{t("noServers")}</div>
      )}
    </div>
  );
}

function CopyValueButton({ label, value }: { label: string; value: string }) {
  const t = useMessages();
  const [copied, setCopied] = useState(false);
  return (
    <button
      aria-label={label}
      className={`copy-value-button ${copied ? "copied" : ""}`}
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1200);
        });
      }}
      title={copied ? t("copied") : label}
      type="button"
    >
      <Copy size={14} />
    </button>
  );
}

function VirtualServerList({
  onAdmin,
  onEdit,
  resetKey,
  servers,
}: {
  onAdmin(server: PodServer): void;
  onEdit(server: PodServer): void;
  resetKey: string;
  servers: PodServer[];
}) {
  const t = useMessages();
  const viewport = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const rowHeight = 88;
  const viewportHeight = 284;
  const overscan = 5;
  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const end = Math.min(
    servers.length,
    Math.ceil((scrollTop + viewportHeight) / rowHeight) + overscan,
  );
  useEffect(() => {
    if (viewport.current) viewport.current.scrollTop = 0;
    setScrollTop(0);
  }, [resetKey]);
  return (
    <div
      className="server-list virtual-server-list"
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      ref={viewport}
    >
      <div style={{ height: servers.length * rowHeight, position: "relative" }}>
        {servers.slice(start, end).map((server, offset) => (
          <div
            className="server-row virtual-server-row"
            key={server.id}
            style={{
              transform: `translateY(${(start + offset) * rowHeight}px)`,
            }}
          >
            <span className="server-orb">
              <Server size={17} />
            </span>
            <div>
              <strong>{server.name}</strong>
              <small>{server.endpoint}</small>
              {server.admin_public_key ? (
                <span className="server-admin-key">
                  <code>{shortPublicKey(server.admin_public_key)}</code>
                  <CopyValueButton
                    label={t("copyAdminPublicKey")}
                    value={server.admin_public_key}
                  />
                </span>
              ) : null}
            </div>
            <Status state={server.health.state} />
            <button
              aria-label={t("edit")}
              className="row-icon-action"
              onClick={() => onEdit(server)}
              title={t("edit")}
              type="button"
            >
              <Pencil size={14} />
            </button>
            <button
              className="row-action"
              onClick={() => onAdmin(server)}
              type="button"
            >
              {server.admin_configured ? t("openAdmin") : t("configureAdmin")}
              <ArrowUpRight size={14} />
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

function shortPublicKey(value: string) {
  return value.length > 18 ? `${value.slice(0, 10)}…${value.slice(-7)}` : value;
}

type EditableServer = Pick<PodServer, "id" | "name" | "endpoint">;

function ServerEditorDialog({
  onClose,
  onDelete,
  onSave,
  server,
}: {
  onClose(): void;
  onDelete?: () => Promise<void>;
  onSave(server: EditableServer): Promise<void>;
  server?: PodServer;
}) {
  const t = useMessages();
  const [name, setName] = useState(server?.name ?? "");
  const [endpoint, setEndpoint] = useState(server?.endpoint ?? "");
  const [saving, setSaving] = useState(false);
  return (
    <div className="nested-dialog-backdrop">
      <form
        className="secret-dialog server-editor-dialog"
        onSubmit={(event) => {
          event.preventDefault();
          setSaving(true);
          void onSave({
            id: server?.id ?? "",
            name: name.trim(),
            endpoint: endpoint.trim(),
          }).finally(() => setSaving(false));
        }}
      >
        <header>
          <div>
            <span className="mode-chip">
              {server ? t("editServer") : t("addServer")}
            </span>
            <h3>{server?.name || t("server")}</h3>
          </div>
          <button className="icon-button" onClick={onClose} type="button">
            <X size={18} />
          </button>
        </header>
        <div className="form-grid">
          <Field
            label={t("serverName")}
            onChange={setName}
            placeholder={t("optionalName")}
            value={name}
            wide
          />
          <Field
            label={t("serverEndpoint")}
            onChange={setEndpoint}
            placeholder="115.191.6.117:9820"
            required
            value={endpoint}
            wide
          />
        </div>
        <div className="generated-admin-key">
          <div>
            <small>{t("adminPublicKey")}</small>
            {server?.admin_public_key ? (
              <code>{server.admin_public_key}</code>
            ) : (
              <span>{t("adminKeyGeneratedAfterSave")}</span>
            )}
          </div>
          {server?.admin_public_key ? (
            <CopyValueButton
              label={t("copyAdminPublicKey")}
              value={server.admin_public_key}
            />
          ) : null}
        </div>
        <footer>
          {onDelete ? (
            <button
              className="danger-action"
              disabled={saving}
              onClick={() => void onDelete()}
              type="button"
            >
              <Trash2 size={14} /> {t("removeServer")}
            </button>
          ) : null}
          <span />
          <button className="secondary-action" onClick={onClose} type="button">
            {t("cancel")}
          </button>
          <button className="primary-action" disabled={saving} type="submit">
            {t("saveConfiguration")}
          </button>
        </footer>
      </form>
    </div>
  );
}

function podInputWithServers(
  pod: PodSummary,
  servers: EditableServer[],
): PodInput {
  return {
    version: 1,
    id: pod.id,
    name: pod.name,
    description: pod.description,
    remote_access_point: pod.remote!.access_point.endpoint,
    remote_servers: servers.map((server) => ({
      id: server.id,
      name: server.name,
      endpoint: server.endpoint,
    })),
  };
}

function Status({ state }: { state: string }) {
  const t = useMessages();
  const key = (
    ["reachable", "unreachable", "checking", "invalid-response"] as const
  ).find((value) => value === state);
  const processKey = (
    ["running", "stopped", "stopping", "failed"] as const
  ).find((value) => value === state);
  return (
    <span className={`status status-${state}`}>
      <i />
      {processKey ? t(processKey) : key ? t(key) : state}
    </span>
  );
}

function CreatePodDialog({
  onClose,
  onSave,
}: {
  onClose(): void;
  onSave(input: PodInput): Promise<void>;
}) {
  const t = useMessages();
  const [mode, setMode] = useState<"choose" | "remote">("choose");
  const [accessPoint, setAccessPoint] = useState("");
  const [saving, setSaving] = useState(false);
  const [closing, setClosing] = useState(false);
  useEffect(() => {
    if (!closing) return;
    const timer = window.setTimeout(onClose, 240);
    return () => window.clearTimeout(timer);
  }, [closing, onClose]);

  async function createLocal() {
    setSaving(true);
    try {
      await onSave({
        version: 1,
        name: t("localPodDefaultName"),
        local_server: { port: 0 },
      });
    } finally {
      setSaving(false);
    }
  }

  async function createRemote(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      await onSave({
        version: 1,
        name: t("remotePodDefaultName"),
        remote_access_point: accessPoint.trim(),
        remote_servers: [],
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <div
      className={`dialog-backdrop ${closing ? "dialog-closing" : ""}`}
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) setClosing(true);
      }}
      onAnimationEnd={(event) => {
        if (closing && event.currentTarget === event.target) onClose();
      }}
    >
      <form
        className="create-dialog compact-dialog"
        onSubmit={(event) => void createRemote(event)}
      >
        <header>
          {mode === "remote" ? (
            <button
              className="icon-button"
              onClick={() => setMode("choose")}
              type="button"
            >
              <ChevronLeft size={18} />
            </button>
          ) : (
            <div>
              <span className="mode-chip">{t("newEnvironment")}</span>
              <h2>{t("addPod")}</h2>
            </div>
          )}
          <button
            aria-label={t("close")}
            className="icon-button"
            onClick={() => setClosing(true)}
            title={t("close")}
            type="button"
          >
            <X size={18} />
          </button>
        </header>
        {mode === "choose" ? (
          <div className="create-mode-grid">
            <button
              disabled={saving}
              onClick={() => void createLocal()}
              type="button"
            >
              <span>
                <Laptop size={24} />
              </span>
              <strong>{t("local")}</strong>
              <small>{t("localCreateHint")}</small>
            </button>
            <button
              disabled={saving}
              onClick={() => setMode("remote")}
              type="button"
            >
              <span>
                <Cloud size={24} />
              </span>
              <strong>{t("remote")}</strong>
              <small>{t("remoteCreateHint")}</small>
            </button>
          </div>
        ) : (
          <div className="remote-create-step">
            <div>
              <span className="mode-chip">{t("remote")}</span>
              <h2>{t("connectRemote")}</h2>
            </div>
            <Field
              label={t("accessPoint")}
              onChange={setAccessPoint}
              placeholder="ap.dev.gizclaw.com:9820"
              required
              value={accessPoint}
              wide
            />
            <button className="primary-action" disabled={saving} type="submit">
              {t("create")}
            </button>
          </div>
        )}
      </form>
    </div>
  );
}

function PodSettingsDialog({
  initial,
  onClose,
  onSave,
}: {
  initial: PodSummary;
  onClose(): void;
  onSave(input: PodInput): Promise<void>;
}) {
  const t = useMessages();
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description ?? "");
  const [accessPoint, setAccessPoint] = useState(
    initial.remote?.access_point.endpoint ?? "",
  );
  const [closing, setClosing] = useState(false);
  useEffect(() => {
    if (!closing) return;
    const timer = window.setTimeout(onClose, 240);
    return () => window.clearTimeout(timer);
  }, [closing, onClose]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const base = {
      version: 1 as const,
      id: initial.id,
      name: name.trim(),
      description: description.trim(),
    };
    if (initial.local) {
      await onSave({
        ...base,
        local_server: { port: initial.local.port },
      });
      return;
    }
    await onSave({
      ...base,
      remote_access_point: accessPoint.trim(),
      remote_servers: initial.remote!.servers.map((server) => ({
        id: server.id,
        name: server.name,
        endpoint: server.endpoint,
      })),
    });
  }

  return (
    <div
      className={`dialog-backdrop ${closing ? "dialog-closing" : ""}`}
      onAnimationEnd={(event) => {
        if (closing && event.currentTarget === event.target) onClose();
      }}
    >
      <form
        className="create-dialog settings-dialog"
        onSubmit={(event) => void submit(event)}
      >
        <header>
          <div>
            <span className="mode-chip">{t("editPod")}</span>
            <h2>{initial.name}</h2>
          </div>
          <button
            className="icon-button"
            onClick={() => setClosing(true)}
            type="button"
          >
            <X size={18} />
          </button>
        </header>
        <div className="form-grid">
          <Field
            label={t("name")}
            onChange={setName}
            placeholder={t("name")}
            required
            value={name}
            wide
          />
          <Field
            label={t("description")}
            onChange={setDescription}
            placeholder={t("descriptionPlaceholder")}
            value={description}
            wide
          />
          {initial.remote ? (
            <Field
              label={t("accessPoint")}
              onChange={setAccessPoint}
              placeholder="ap.dev.gizclaw.com:9820"
              required
              value={accessPoint}
              wide
            />
          ) : null}
        </div>
        <footer>
          <button
            className="secondary-action"
            onClick={() => setClosing(true)}
            type="button"
          >
            {t("cancel")}
          </button>
          <button className="primary-action" type="submit">
            {t("saveConfiguration")}
          </button>
        </footer>
      </form>
    </div>
  );
}

function Field({
  disabled = false,
  label,
  onChange,
  placeholder,
  required = false,
  secret = false,
  value,
  wide = false,
}: {
  disabled?: boolean;
  label: string;
  onChange(value: string): void;
  placeholder: string;
  required?: boolean;
  secret?: boolean;
  value: string;
  wide?: boolean;
}) {
  return (
    <label className={wide ? "field-wide" : ""}>
      <span>{label}</span>
      <input
        autoComplete="off"
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        required={required}
        type={secret ? "password" : "text"}
        value={value}
      />
    </label>
  );
}

function AmbientBackground() {
  return (
    <div className="ambient-background" aria-hidden="true">
      <span className="ambient-glow ambient-glow-one" />
      <span className="ambient-glow ambient-glow-two" />
      <svg
        className="ambient-flow-lines"
        preserveAspectRatio="none"
        viewBox="0 0 1440 900"
      >
        <path d="M-120 720 C 180 510, 320 870, 640 610 S 1080 280, 1560 450" />
        <path d="M-180 540 C 210 270, 400 690, 710 430 S 1140 80, 1580 260" />
        <path d="M40 980 C 230 650, 520 760, 760 520 S 1010 250, 1510 110" />
        <path d="M-100 270 C 260 130, 440 410, 730 260 S 1120 -20, 1510 160" />
        <path d="M280 950 C 500 720, 750 810, 940 600 S 1220 350, 1510 520" />
      </svg>
      <span className="ambient-noise" />
    </div>
  );
}

function stableHue(value: string) {
  let hash = 0;
  for (const character of value)
    hash = (hash * 31 + character.charCodeAt(0)) | 0;
  return 190 + (Math.abs(hash) % 105);
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : String(reason);
}
