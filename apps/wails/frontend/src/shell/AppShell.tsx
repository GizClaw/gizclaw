import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, FormEvent } from "react";
import QRCode from "qrcode";
import {
  Activity,
  ArrowUpRight,
  ChevronLeft,
  CircleStop,
  Cloud,
  Laptop,
  FolderOpen,
  Maximize2,
  Minus,
  Pencil,
  Play,
  Plus,
  Search,
  Server,
  Smartphone,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";

import { setLocale, useMessages } from "../i18n";
import { getDesktopAPI } from "../lib/runtime/desktop";
import type { PodInput, PodSummary } from "../lib/runtime/types";
import { DesktopDialog, DesktopDialogTitle } from "./DesktopDialog";
import { HomeCard } from "./HomeCard";
import { ManageListItem } from "./ManageListItem";
import { NeatWaves } from "./NeatWaves";

export function AppShell() {
  const api = useMemo(() => getDesktopAPI(), []);
  const t = useMessages();
  const [pods, setPods] = useState<PodSummary[]>([]);
  const [selected, setSelected] = useState<PodSummary | null>(null);
  const [creating, setCreating] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
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
        <header className="home-heading">
          <h1 className="home-title">GizClaw</h1>
          <p className="home-subtitle">{t("tagline")}</p>
        </header>
        <div className="pod-grid" aria-label={t("pods")}>
          <MobileAppCard onOpen={() => setMobileOpen(true)} />
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
              index={index + 1}
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

      {mobileOpen ? (
        <MobileAppDialog onClose={() => setMobileOpen(false)} />
      ) : null}
      {selected ? (
        <PodDetail
          api={api}
          pod={selected}
          onChange={replacePod}
          onClose={() => setSelected(null)}
          onDelete={async () => {
            setError("");
            try {
              await api.DeletePod(selected.id);
              setPods((current) =>
                current.filter((pod) => pod.id !== selected.id),
              );
              setSelected(null);
            } catch (reason) {
              setError(errorMessage(reason));
            }
          }}
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

function MobileAppCard({ onOpen }: { onOpen(): void }) {
  const t = useMessages();
  return (
    <HomeCard
      className="mobile-app-card"
      aria-label={t("mobileAppPromo")}
      copyClassName="mobile-app-copy"
      description={t("mobileAppHint")}
      footer={
        <span className="mobile-app-platforms" aria-hidden="true">
          <span>
            <b>iOS</b> TestFlight
          </span>
          <span>
            <b>Android</b> Google Play
          </span>
        </span>
      }
      onClick={onOpen}
      title="GizClaw Mobile"
      top={
        <>
          <span className="mobile-app-icon">
            <Smartphone size={18} />
          </span>
          <span className="mode-chip">Mobile</span>
          <span className="mobile-app-status">{t("comingSoon")}</span>
        </>
      }
    />
  );
}

function MobileAppDialog({ onClose }: { onClose(): void }) {
  const t = useMessages();
  const [platform, setPlatform] = useState<"ios" | "android">("ios");
  const channel = platform === "ios" ? "TestFlight" : "Google Play Beta";
  const platformName = platform === "ios" ? "iOS" : "Android";
  const payload = `GizClaw Mobile\n${platformName} / ${channel}\nComing soon`;
  return (
    <DesktopDialog className="mobile-download-dialog" onClose={onClose}>
      {(close) => (
        <>
          <header className="mobile-download-header">
            <div>
              <DesktopDialogTitle>
                <h2>GizClaw Mobile</h2>
              </DesktopDialogTitle>
              <p>{t("mobileDownloadHint")}</p>
            </div>
            <button
              aria-label={t("close")}
              className="icon-button close-button"
              onClick={close}
              title={t("close")}
              type="button"
            >
              <X size={20} />
            </button>
          </header>
          <div className="mobile-platform-switch" role="group">
            <button
              aria-pressed={platform === "ios"}
              className={platform === "ios" ? "selected" : ""}
              onClick={() => setPlatform("ios")}
              type="button"
            >
              <b>iOS</b>
              <span>TestFlight</span>
            </button>
            <button
              aria-pressed={platform === "android"}
              className={platform === "android" ? "selected" : ""}
              onClick={() => setPlatform("android")}
              type="button"
            >
              <b>Android</b>
              <span>Google Play</span>
            </button>
          </div>
          <div className="mobile-download-qr">
            <QRCodeImage label={t("mobileDownloadQRCode")} payload={payload} />
            <strong>{channel}</strong>
            <span>{t("comingSoon")}</span>
          </div>
          <p className="mobile-download-note">{t("mobileDownloadPreview")}</p>
        </>
      )}
    </DesktopDialog>
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
    <HomeCard
      className={`pod-card pod-card-${pod.valid ? pod.mode : "invalid"}`}
      description={
        !pod.valid
          ? pod.error
          : pod.local
            ? running
              ? t("running")
              : t("stopped")
            : `${remoteCount} ${remoteCount === 1 ? t("server") : t("servers")}`
      }
      footer={
        pod.valid ? (
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
        ) : null
      }
      onClick={onOpen}
      style={
        {
          animationDelay: `${Math.min(index, 8) * 55}ms`,
          "--card-hue": hue,
          "--card-hue-alt": (hue + 42) % 360,
        } as CSSProperties
      }
      title={pod.name}
      top={
        <>
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
        </>
      }
    />
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
  onDelete(): Promise<void>;
  onEdit(): void;
  onError(reason: unknown): void;
  onReveal(): void;
  run(action: () => Promise<PodSummary>): Promise<void>;
}) {
  const t = useMessages();
  const [managing, setManaging] = useState(false);
  const [query, setQuery] = useState("");
  const [serverEditor, setServerEditor] = useState<PodServer | "new" | null>(
    null,
  );
  const servers = (pod.remote?.servers ?? []).filter((server) => {
    const matchesQuery = `${server.id} ${server.name} ${server.endpoint}`
      .toLowerCase()
      .includes(query.toLowerCase());
    return matchesQuery;
  });
  const detailSubtitle = pod.local
    ? preferredLANAddress(pod.local.lan_addresses)
    : pod.id;
  return (
    <DesktopDialog
      className={`pod-dialog pod-dialog-${pod.mode} ${managing ? "is-managing" : ""}`}
      onClose={onClose}
    >
      {(close) => (
        <>
        <div className="dialog-aurora" />
        <header className="pod-dialog-header">
          {managing && pod.valid ? (
            <button
              aria-label={t("shareServer")}
              className="icon-button detail-back-button"
              onClick={() => setManaging(false)}
              title={t("shareServer")}
              type="button"
            >
              <ChevronLeft size={18} />
            </button>
          ) : null}
          <div className="pod-dialog-heading">
            <DesktopDialogTitle>
              {pod.valid ? (
                <h2>
                  <button
                    className="pod-name-button"
                    onClick={onEdit}
                    title={t("renameServer")}
                    type="button"
                  >
                    {pod.name}
                  </button>
                </h2>
              ) : (
                <h2>{pod.name}</h2>
              )}
            </DesktopDialogTitle>
          </div>
          <span className="pod-header-meta">
            {detailSubtitle || pod.description || pod.id}
          </span>
          <button
            aria-label={t("close")}
            className="icon-button close-button"
            onClick={close}
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
                  onDelete={() => {
                    if (window.confirm(t("confirmDelete"))) void onDelete();
                  }}
                  onError={onError}
                  pod={pod}
                  run={run}
                />
              }
              managing={managing}
              front={
                <PodShareFace
                  endpoint={preferredLANAddress(pod.local.lan_addresses)}
                  onManage={() => setManaging(true)}
                  onPlay={() =>
                    openBrowserLaunch(api.OpenPlay(pod.id), onError)
                  }
                  pod={pod}
                  publicKey={pod.local.server_public_key ?? ""}
                />
              }
            />
          ) : (
            <PodDetailPages
              back={
                <RemoteManageFace
                  api={api}
                  onAddServer={() => setServerEditor("new")}
                  onDelete={() => {
                    if (window.confirm(t("confirmDelete"))) void onDelete();
                  }}
                  onEditServer={setServerEditor}
                  onError={onError}
                  onQuery={setQuery}
                  pod={pod}
                  query={query}
                  servers={servers}
                />
              }
              managing={managing}
              front={
                <PodShareFace
                  endpoint={pod.remote!.access_point.endpoint}
                  onManage={() => setManaging(true)}
                  onPlay={() =>
                    openBrowserLaunch(api.OpenPlay(pod.id), onError)
                  }
                  pod={pod}
                  publicKey={pod.remote!.access_point.public_key ?? ""}
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
        </>
      )}
    </DesktopDialog>
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
}: {
  endpoint: string;
  onManage(): void;
  onPlay(): void;
  pod: PodSummary;
  publicKey: string;
}) {
  const t = useMessages();
  const payload = useMemo(
    () => serverDeepLink(endpoint, pod.name, pod.mode, publicKey),
    [endpoint, pod.mode, pod.name, publicKey],
  );
  return (
    <div className="share-face-layout">
      <div className="share-qr">
        {endpoint ? (
          <QRCodeImage label={t("serverQRCode")} payload={payload} />
        ) : (
          <div className="qr-code qr-unavailable">{t("noLANAddress")}</div>
        )}
        <span>{t("scanToAddServer")}</span>
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
  onDelete,
  onError,
  pod,
  run,
}: {
  api: ReturnType<typeof getDesktopAPI>;
  onDelete(): void;
  onError(reason: unknown): void;
  pod: PodSummary;
  run(action: () => Promise<PodSummary>): Promise<void>;
}) {
  const t = useMessages();
  const local = pod.local!;
  return (
    <div className="manage-face local-manage-face">
      <ManageListItem
        as="section"
        className="local-status-card"
        description={t("localServer")}
        icon={<Server size={22} />}
        iconClassName={`local-status-icon ${local.process.state === "running" ? "running" : ""}`}
        title={
          local.process.state === "running" ? t("running") : t("stopped")
        }
        trailing={
          <button
            className={`local-status-action ${
              local.process.state === "running"
                ? "stop-action"
                : "start-action"
            }`}
            onClick={() =>
              void run(() =>
                local.process.state === "running"
                  ? api.StopLocalServer(pod.id)
                  : api.StartLocalServer(pod.id),
              )
            }
            type="button"
          >
            {local.process.state === "running" ? (
              <CircleStop size={14} />
            ) : (
              <Play size={14} />
            )}
            <span>
              {local.process.state === "running" ? t("stop") : t("start")}
            </span>
          </button>
        }
      />
      <ManageListItem
        as="button"
        className="local-admin-action"
        description={t("openInBrowser")}
        icon={<Server size={22} />}
        iconClassName="local-admin-icon"
        onClick={() =>
          openBrowserLaunch(api.OpenAdmin(pod.id, "local"), onError)
        }
        title="Admin"
        trailing={<ArrowUpRight size={15} />}
      />
      <button className="pod-delete-action" onClick={onDelete} type="button">
        <Trash2 size={14} />
        {t("deletePod")}
      </button>
    </div>
  );
}

function RemoteManageFace({
  api,
  onAddServer,
  onDelete,
  onEditServer,
  onError,
  onQuery,
  pod,
  query,
  servers,
}: {
  api: ReturnType<typeof getDesktopAPI>;
  onAddServer(): void;
  onDelete(): void;
  onEditServer(server: PodServer): void;
  onError(reason: unknown): void;
  onQuery(value: string): void;
  pod: PodSummary;
  query: string;
  servers: PodServer[];
}) {
  const t = useMessages();
  return (
    <div className="manage-face remote-manage-face">
      <span className="mode-chip remote-manage-label">
        {t("manageServers")}
      </span>
      <div className="remote-list-tools">
        <label>
          <Search size={15} />
          <input
            aria-label={t("searchServers")}
            onChange={(event) => onQuery(event.target.value)}
            placeholder={t("searchServers")}
            value={query}
          />
        </label>
      </div>
      <button className="server-add-card" onClick={onAddServer} type="button">
        <span>
          <Plus size={17} />
        </span>
        {t("addServer")}
      </button>
      {servers.length ? (
        <VirtualServerList
          onAdmin={(server) =>
            openBrowserLaunch(api.OpenAdmin(pod.id, server.id), onError)
          }
          onEdit={onEditServer}
          resetKey={`${pod.id}\u0000${query}`}
          servers={servers}
        />
      ) : (
        <div className="no-servers">{t("noServers")}</div>
      )}
      <button className="pod-delete-action" onClick={onDelete} type="button">
        <Trash2 size={14} />
        {t("deletePod")}
      </button>
    </div>
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
  const rowHeight = 68;
  const viewportHeight = 300;
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
            <span
              className={`server-health-dot server-health-${server.health.state}`}
            />
            <div>
              <strong>{server.name}</strong>
              <small>
                {server.id} · {server.endpoint}
              </small>
            </div>
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
              className={`row-action server-admin-action ${server.admin_configured ? "configured" : ""}`}
              onClick={() =>
                server.admin_configured ? onAdmin(server) : onEdit(server)
              }
              title={
                server.admin_configured ? t("openAdmin") : t("configureAdmin")
              }
              type="button"
            >
              Admin
              <ArrowUpRight size={14} />
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

type EditableServer = Pick<PodServer, "id" | "name" | "endpoint"> & {
  admin_private_key?: string;
};

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
  const [adminPrivateKey, setAdminPrivateKey] = useState("");
  const [saving, setSaving] = useState(false);
  return (
    <DesktopDialog
      className="secret-dialog server-editor-dialog"
      nested
      onClose={onClose}
    >
      {(close) => (
        <form
          className="desktop-dialog-form"
          onSubmit={(event) => {
            event.preventDefault();
            setSaving(true);
            void onSave({
              id: server?.id ?? "",
              name: name.trim(),
              endpoint: endpoint.trim(),
              admin_private_key: adminPrivateKey.trim() || undefined,
            }).finally(() => setSaving(false));
          }}
        >
          <header>
            <div>
              <span className="mode-chip">
                {server ? t("editServer") : t("addServer")}
              </span>
              <DesktopDialogTitle>
                <h3>{server?.name || t("server")}</h3>
              </DesktopDialogTitle>
            </div>
            <button
              aria-label={t("close")}
              className="icon-button"
              onClick={close}
              title={t("close")}
              type="button"
            >
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
            <Field
              label={t("adminPrivateKey")}
              onChange={setAdminPrivateKey}
              placeholder={
                server?.admin_configured
                  ? t("keepAdminPrivateKey")
                  : t("pasteAdminPrivateKey")
              }
              secret
              value={adminPrivateKey}
              wide
            />
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
            <button className="secondary-action" onClick={close} type="button">
              {t("cancel")}
            </button>
            <button className="primary-action" disabled={saving} type="submit">
              {t("saveConfiguration")}
            </button>
          </footer>
        </form>
      )}
    </DesktopDialog>
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
      admin_private_key: server.admin_private_key,
    })),
  };
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
    <DesktopDialog className="create-dialog compact-dialog" onClose={onClose}>
      {(close) => (
        <form
          className="desktop-dialog-form"
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
              <DesktopDialogTitle>
                <h2>{t("addPod")}</h2>
              </DesktopDialogTitle>
            </div>
          )}
          <button
            aria-label={t("close")}
            className="icon-button"
            onClick={close}
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
              <DesktopDialogTitle>
                <h2>{t("connectRemote")}</h2>
              </DesktopDialogTitle>
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
      )}
    </DesktopDialog>
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
  const [accessPoint, setAccessPoint] = useState(
    initial.remote?.access_point.endpoint ?? "",
  );

  async function submit(event: FormEvent) {
    event.preventDefault();
    const base = {
      version: 1 as const,
      id: initial.id,
      name: name.trim(),
      description: initial.description ?? "",
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
    <DesktopDialog className="create-dialog settings-dialog" onClose={onClose}>
      {(close) => (
        <form
          className="desktop-dialog-form"
          onSubmit={(event) => void submit(event)}
        >
        <header>
          <div>
            <span className="mode-chip">{t("editPod")}</span>
            <DesktopDialogTitle>
              <h2>{initial.name}</h2>
            </DesktopDialogTitle>
          </div>
          <button
            aria-label={t("close")}
            className="icon-button"
            onClick={close}
            title={t("close")}
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
            onClick={close}
            type="button"
          >
            {t("cancel")}
          </button>
          <button className="primary-action" type="submit">
            {t("saveConfiguration")}
          </button>
        </footer>
        </form>
      )}
    </DesktopDialog>
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
      <NeatWaves />
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

function serverDeepLink(
  endpoint: string,
  name: string,
  mode: PodSummary["mode"],
  publicKey: string,
) {
  const path = encodeURIComponent(endpoint)
    .replaceAll("%3A", ":")
    .replaceAll("%5B", "[")
    .replaceAll("%5D", "]");
  const query = new URLSearchParams({ name, mode });
  if (publicKey) query.set("public_key", publicKey);
  return `gizclaw://ap/${path}?${query}`;
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : String(reason);
}

function openBrowserLaunch(
  launch: Promise<string>,
  onError: (reason: unknown) => void,
) {
  void launch
    .then((url) => {
      if (!url) throw new Error("Desktop browser launch URL is empty");
      if (!window.runtime?.BrowserOpenURL)
        throw new Error("System browser integration is unavailable");
      window.runtime.BrowserOpenURL(url);
    })
    .catch(onError);
}
