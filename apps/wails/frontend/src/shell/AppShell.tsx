import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  ArrowUpRight,
  ChevronLeft,
  CircleStop,
  Cloud,
  Cpu,
  Globe2,
  KeyRound,
  Laptop,
  FolderOpen,
  Maximize2,
  MoreHorizontal,
  Minus,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCw,
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
  const mode = !pod.valid
    ? t("invalid")
    : pod.mode === "local"
      ? t("local")
      : t("remote");
  return (
    <button
      className={`pod-card pod-card-${pod.valid ? pod.mode : "invalid"}`}
      onClick={onOpen}
      style={{ animationDelay: `${Math.min(index, 8) * 55}ms` }}
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
  const [query, setQuery] = useState("");
  const [adminFilter, setAdminFilter] = useState<
    "all" | "configured" | "missing"
  >("all");
  const [healthFilter, setHealthFilter] = useState("all");
  const [secretTarget, setSecretTarget] = useState<{
    kind: "admin" | "client";
    serverID?: string;
  } | null>(null);
  const [serverEditor, setServerEditor] = useState<PodServer | "new" | null>(
    null,
  );
  const servers = (pod.remote?.servers ?? []).filter((server) => {
    const matchesQuery = `${server.id} ${server.name} ${server.endpoint}`
      .toLowerCase()
      .includes(query.toLowerCase());
    const matchesAdmin =
      adminFilter === "all" ||
      (adminFilter === "configured"
        ? server.admin_configured
        : !server.admin_configured);
    const matchesHealth =
      healthFilter === "all" || server.health.state === healthFilter;
    return matchesQuery && matchesAdmin && matchesHealth;
  });
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !secretTarget && !serverEditor)
        setClosing(true);
    };
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  }, [secretTarget, serverEditor]);
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
      <section className="pod-dialog" aria-modal="true" role="dialog">
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
            <h2>{pod.name}</h2>
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
            <div className="local-detail">
              <div className="detail-hero">
                <div>
                  <small>{t("localServer")}</small>
                  <strong>0.0.0.0:{pod.local.port}</strong>
                </div>
                <Status state={pod.local.process.state} />
              </div>
              <div className="action-row">
                <button
                  className="primary-action"
                  disabled={pod.local.process.state === "running"}
                  onClick={() => void run(() => api.StartLocalServer(pod.id))}
                  type="button"
                >
                  <Play size={16} />
                  {t("start")}
                </button>
                <button
                  className="secondary-action"
                  disabled={pod.local.process.state !== "running"}
                  onClick={() => void run(() => api.StopLocalServer(pod.id))}
                  type="button"
                >
                  <CircleStop size={16} />
                  {t("stop")}
                </button>
                <button
                  className="secondary-action"
                  onClick={() => void run(() => api.RestartLocalServer(pod.id))}
                  type="button"
                >
                  <RotateCw size={16} />
                  {t("restart")}
                </button>
                <button
                  className="secondary-action"
                  onClick={() => void run(() => api.RefreshPodHealth(pod.id))}
                  type="button"
                >
                  <RefreshCw size={16} />
                  {t("refresh")}
                </button>
              </div>
              {pod.local.lan_addresses.length ? (
                <div className="lan-hints">
                  {pod.local.lan_addresses.map((address) => (
                    <code key={address}>{address}</code>
                  ))}
                </div>
              ) : null}
              <div className="surface-grid">
                <SurfaceCard
                  enabled={pod.local.admin_configured}
                  icon={<Server size={19} />}
                  label="Admin"
                  onConfigure={() => setSecretTarget({ kind: "admin" })}
                  onOpen={() => api.OpenAdmin(pod.id, "local").catch(onError)}
                />
                <SurfaceCard
                  enabled={pod.play_configured}
                  icon={<Sparkles size={19} />}
                  label="Play"
                  onConfigure={() => setSecretTarget({ kind: "client" })}
                  onOpen={() => api.OpenPlay(pod.id).catch(onError)}
                />
              </div>
              {pod.local.process.logs?.length ? (
                <pre className="log-view">
                  {pod.local.process.logs.slice(-20).join("\n")}
                </pre>
              ) : null}
            </div>
          ) : (
            <div className="remote-detail">
              <div className="access-point">
                <div>
                  <small>{t("remoteAccessPoint")}</small>
                  <strong>{pod.remote?.access_point.endpoint}</strong>
                </div>
                <Status state={pod.remote?.access_point.state ?? "checking"} />
              </div>
              <div className="remote-toolbar">
                <label>
                  <Search size={16} />
                  <input
                    aria-label={t("searchServers")}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder={t("searchServers")}
                    value={query}
                  />
                </label>
                <select
                  aria-label={t("adminFilter")}
                  onChange={(event) =>
                    setAdminFilter(event.target.value as typeof adminFilter)
                  }
                  value={adminFilter}
                >
                  <option value="all">{t("allAdmin")}</option>
                  <option value="configured">{t("adminConfigured")}</option>
                  <option value="missing">{t("adminMissing")}</option>
                </select>
                <select
                  aria-label={t("healthFilter")}
                  onChange={(event) => setHealthFilter(event.target.value)}
                  value={healthFilter}
                >
                  <option value="all">{t("allStates")}</option>
                  <option value="reachable">{t("reachable")}</option>
                  <option value="unreachable">{t("unreachable")}</option>
                  <option value="invalid-response">
                    {t("invalid-response")}
                  </option>
                </select>
                <button
                  className="secondary-action"
                  onClick={() => void run(() => api.RefreshPodHealth(pod.id))}
                  type="button"
                >
                  <RefreshCw size={16} />
                  {t("refresh")}
                </button>
                <button
                  className="secondary-action"
                  onClick={() => setServerEditor("new")}
                  type="button"
                >
                  <Plus size={16} />
                  {t("addServer")}
                </button>
                <button
                  className={
                    pod.play_configured ? "primary-action" : "secondary-action"
                  }
                  onClick={() =>
                    pod.play_configured
                      ? api.OpenPlay(pod.id).catch(onError)
                      : setSecretTarget({ kind: "client" })
                  }
                  type="button"
                >
                  <Sparkles size={16} />
                  {pod.play_configured ? t("openPlay") : t("configurePlay")}
                </button>
              </div>
              {servers.length ? (
                <VirtualServerList
                  onAdmin={(server) =>
                    server.admin_configured
                      ? api.OpenAdmin(pod.id, server.id).catch(onError)
                      : setSecretTarget({ kind: "admin", serverID: server.id })
                  }
                  onEdit={(server) => setServerEditor(server)}
                  servers={servers}
                />
              ) : (
                <div className="no-servers">{t("noServers")}</div>
              )}
            </div>
          )}
        </div>
        {secretTarget ? (
          <SecretDialog
            label={
              secretTarget.kind === "client"
                ? t("clientPrivateKey")
                : t("adminPrivateKey")
            }
            onClose={() => setSecretTarget(null)}
            onSave={async (value) => {
              try {
                const next = await api.UpdatePod(
                  podInputWithSecret(pod, secretTarget, value),
                );
                onChange(next);
                setSecretTarget(null);
              } catch (reason) {
                onError(reason);
              }
            }}
          />
        ) : null}
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

function VirtualServerList({
  onAdmin,
  onEdit,
  servers,
}: {
  onAdmin(server: PodServer): void;
  onEdit(server: PodServer): void;
  servers: PodServer[];
}) {
  const t = useMessages();
  const viewport = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const rowHeight = 69;
  const viewportHeight = 430;
  const overscan = 5;
  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const end = Math.min(
    servers.length,
    Math.ceil((scrollTop + viewportHeight) / rowHeight) + overscan,
  );
  useEffect(() => {
    if (viewport.current) viewport.current.scrollTop = 0;
    setScrollTop(0);
  }, [servers]);
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

function SurfaceCard({
  enabled,
  icon,
  label,
  onConfigure,
  onOpen,
}: {
  enabled: boolean;
  icon: React.ReactNode;
  label: string;
  onConfigure(): void;
  onOpen(): void;
}) {
  const t = useMessages();
  return (
    <button
      className="surface-card"
      onClick={enabled ? onOpen : onConfigure}
      type="button"
    >
      <span>{icon}</span>
      <div>
        <strong>{label}</strong>
        <small>
          {enabled
            ? t("openInBrowser")
            : `${t("notConfigured")} · ${t("configure")}`}
        </small>
      </div>
      <ArrowUpRight size={17} />
    </button>
  );
}

function SecretDialog({
  label,
  onClose,
  onSave,
}: {
  label: string;
  onClose(): void;
  onSave(value: string): Promise<void>;
}) {
  const t = useMessages();
  const [value, setValue] = useState("");
  return (
    <div className="nested-dialog-backdrop">
      <form
        className="secret-dialog"
        onSubmit={(event) => {
          event.preventDefault();
          void onSave(value.trim());
        }}
      >
        <header>
          <div>
            <span className="eyebrow">
              <KeyRound size={14} /> {t("writeOnly")}
            </span>
            <h3>{label}</h3>
          </div>
          <button
            aria-label={t("close")}
            className="icon-button"
            onClick={onClose}
            title={t("close")}
            type="button"
          >
            <X size={18} />
          </button>
        </header>
        <p>{t("secretBody")}</p>
        <input
          autoFocus
          onChange={(event) => setValue(event.target.value)}
          placeholder={t("giznetPrivateKey")}
          required
          type="password"
          value={value}
        />
        <footer>
          <button className="secondary-action" onClick={onClose} type="button">
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

function podInputWithSecret(
  pod: PodSummary,
  target: { kind: "admin" | "client"; serverID?: string },
  value: string,
): PodInput {
  const base = {
    version: 1 as const,
    id: pod.id,
    name: pod.name,
    description: pod.description,
    ...(target.kind === "client" ? { client_private_key: value } : {}),
  };
  if (pod.local)
    return {
      ...base,
      local_server: {
        port: pod.local.port,
        ...(target.kind === "admin" ? { admin_private_key: value } : {}),
      },
    };
  return {
    ...base,
    remote_access_point: pod.remote!.access_point.endpoint,
    remote_servers: pod.remote!.servers.map((server) => ({
      id: server.id,
      name: server.name,
      endpoint: server.endpoint,
      ...(target.kind === "admin" && target.serverID === server.id
        ? { admin_private_key: value }
        : {}),
    })),
  };
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
  const [removeKeys, setRemoveKeys] = useState<string[]>([]);
  const [closing, setClosing] = useState(false);
  useEffect(() => {
    if (!closing) return;
    const timer = window.setTimeout(onClose, 240);
    return () => window.clearTimeout(timer);
  }, [closing, onClose]);

  const removableCredentials = [
    ...(initial.play_configured ? [{ id: "client", label: "Play" }] : []),
    ...(initial.local?.admin_configured
      ? [{ id: "admin:local", label: "Admin" }]
      : []),
    ...(initial.remote?.servers
      .filter((server) => server.admin_configured)
      .map((server) => ({
        id: `admin:${server.id}`,
        label: `Admin · ${server.name}`,
      })) ?? []),
  ];

  function toggleRemoval(id: string) {
    if (removeKeys.includes(id)) {
      setRemoveKeys((current) => current.filter((value) => value !== id));
    } else if (window.confirm(t("confirmRemoveCredential"))) {
      setRemoveKeys((current) => [...current, id]);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const base = {
      version: 1 as const,
      id: initial.id,
      name: name.trim(),
      description: description.trim(),
      ...(removeKeys.includes("client") ? { client_private_key: "" } : {}),
    };
    if (initial.local) {
      await onSave({
        ...base,
        local_server: {
          port: initial.local.port,
          ...(removeKeys.includes("admin:local")
            ? { admin_private_key: "" }
            : {}),
        },
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
        ...(removeKeys.includes(`admin:${server.id}`)
          ? { admin_private_key: "" }
          : {}),
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
        {removableCredentials.length ? (
          <section className="credential-removal">
            <small>{t("removeCredentialHint")}</small>
            <div>
              {removableCredentials.map((credential) => (
                <button
                  className={
                    removeKeys.includes(credential.id) ? "selected" : ""
                  }
                  key={credential.id}
                  onClick={() => toggleRemoval(credential.id)}
                  type="button"
                >
                  <KeyRound size={13} />
                  {removeKeys.includes(credential.id)
                    ? `${t("keepCredential")} · ${credential.label}`
                    : `${t("removeCredential")} · ${credential.label}`}
                </button>
              ))}
            </div>
          </section>
        ) : null}
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
      <span className="ambient-noise" />
    </div>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : String(reason);
}
