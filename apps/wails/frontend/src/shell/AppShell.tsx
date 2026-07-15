import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  ArrowUpRight,
  CircleStop,
  Cloud,
  Cpu,
  Globe2,
  KeyRound,
  Laptop,
  FolderOpen,
  MoreHorizontal,
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
  Zap,
} from "lucide-react";

import { useMessages } from "../i18n";
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
      <TechBackground />
      <header className="titlebar" data-wails-drag>
        <div className="wordmark">
          <span className="wordmark-orbit">
            <Zap size={15} />
          </span>
          <span>{t("appName")}</span>
        </div>
        <div className="titlebar-status">
          <span className="live-dot" /> {t("tagline")}
        </div>
      </header>

      <section className="pod-home">
        <div className="home-heading">
          <div>
            <p className="eyebrow">
              <Sparkles size={14} /> {t("controlPlane")}
            </p>
            <h1>{t("pods")}</h1>
          </div>
          <button
            className="icon-button"
            onClick={() =>
              void Promise.all(
                pods.map((pod) =>
                  api.RefreshPodHealth(pod.id).catch(() => pod),
                ),
              )
                .then((next) => setPods(next))
                .catch((reason) => setError(errorMessage(reason)))
            }
            title={t("refresh")}
            type="button"
          >
            <RefreshCw size={18} />
          </button>
        </div>

        {error ? (
          <div className="error-banner">
            <Activity size={16} />
            {error}
            <button onClick={() => setError("")} type="button">
              <X size={14} />
            </button>
          </div>
        ) : null}
        {loading ? (
          <div className="loading-grid">
            <span />
            <span />
            <span />
          </div>
        ) : null}

        {!loading && pods.length === 0 ? (
          <div className="empty-state">
            <div className="empty-orbit">
              <Globe2 size={34} />
            </div>
            <h2>{t("emptyTitle")}</h2>
            <p>{t("emptyBody")}</p>
          </div>
        ) : null}

        <div className="pod-grid" aria-label={t("pods")}>
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
            onClick={() => setCreating(true)}
            type="button"
          >
            <span className="add-icon">
              <Plus size={25} />
            </span>
            <strong>{t("addPod")}</strong>
            <small>{t("addPodHint")}</small>
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
        <PodFormDialog onClose={() => setCreating(false)} onSave={create} />
      ) : null}
      {editing ? (
        <PodFormDialog
          initial={editing}
          onClose={() => setEditing(null)}
          onSave={update}
        />
      ) : null}
    </main>
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
  if (!pod.valid)
    return (
      <button
        className="pod-card pod-card-invalid"
        onClick={onOpen}
        style={{ animationDelay: `${index * 70}ms` }}
        type="button"
      >
        <span className="card-glow" />
        <span className="pod-card-top">
          <span className="mode-icon">
            <Activity size={20} />
          </span>
          <span className="mode-chip">{t("invalid")}</span>
          <ArrowUpRight className="open-arrow" size={18} />
        </span>
        <span className="pod-card-copy">
          <strong>{pod.name}</strong>
          <small>{pod.error}</small>
        </span>
        <span className="pod-card-footer">
          <span>
            <Activity size={13} /> {t("invalid")}
          </span>
          <span className="health-pulse" />
        </span>
      </button>
    );
  return (
    <button
      className={`pod-card pod-card-${pod.mode}`}
      onClick={onOpen}
      style={{ animationDelay: `${index * 70}ms` }}
      type="button"
    >
      <span className="card-glow" />
      <span className="pod-card-top">
        <span className="mode-icon">
          {pod.mode === "local" ? <Laptop size={20} /> : <Cloud size={20} />}
        </span>
        <span className="mode-chip">
          {pod.mode === "local" ? t("local") : t("remote")}
        </span>
        <ArrowUpRight className="open-arrow" size={18} />
      </span>
      <span className="pod-card-copy">
        <strong>{pod.name}</strong>
        <small>{pod.description || pod.id}</small>
      </span>
      <span className="pod-card-metrics">
        {pod.local ? (
          <>
            <Metric
              label={`:${pod.local.port}`}
              live={running}
              value={running ? t("running") : t("stopped")}
            />
            <Metric
              label="Admin"
              live={pod.local.admin_configured}
              value={pod.local.admin_configured ? t("ready") : "—"}
            />
          </>
        ) : (
          <>
            <Metric
              label={`${remoteCount} ${remoteCount === 1 ? t("server") : t("servers")}`}
              live={pod.remote?.access_point.state === "reachable"}
              value={`${adminCount} ADMIN`}
            />
            <Metric
              label={t("accessPoint")}
              live={pod.remote?.access_point.state === "reachable"}
              value={t(pod.remote?.access_point.state ?? "checking")}
            />
          </>
        )}
      </span>
      <span className="pod-card-footer">
        <span>
          <KeyRound size={13} />{" "}
          {pod.play_configured ? t("playReady") : t("notConfigured")}
        </span>
        <span
          className={`health-pulse ${running || pod.remote?.access_point.state === "reachable" ? "online" : ""}`}
        />
      </span>
    </button>
  );
}

function Metric({
  label,
  live,
  value,
}: {
  label: string;
  live: boolean;
  value: string;
}) {
  return (
    <span className="metric">
      <small>{label}</small>
      <strong className={live ? "metric-live" : ""}>{value}</strong>
    </span>
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
  const [query, setQuery] = useState("");
  const [adminFilter, setAdminFilter] = useState<
    "all" | "configured" | "missing"
  >("all");
  const [healthFilter, setHealthFilter] = useState("all");
  const [secretTarget, setSecretTarget] = useState<{
    kind: "admin" | "client";
    serverID?: string;
  } | null>(null);
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
  return (
    <div
      className="dialog-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) onClose();
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
            onClick={onClose}
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
      </section>
    </div>
  );
}

type PodServer = NonNullable<PodSummary["remote"]>["servers"][number];

function VirtualServerList({
  onAdmin,
  servers,
}: {
  onAdmin(server: PodServer): void;
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

function PodFormDialog({
  initial,
  onClose,
  onSave,
}: {
  initial?: PodSummary;
  onClose(): void;
  onSave(input: PodInput): Promise<void>;
}) {
  const t = useMessages();
  const [mode, setMode] = useState<"local" | "remote">(
    initial?.mode === "remote" ? "remote" : "local",
  );
  const [form, setForm] = useState({
    id: initial?.id ?? "",
    name: initial?.name ?? "",
    description: initial?.description ?? "",
    port: String(initial?.local?.port ?? 9820),
    accessPoint: initial?.remote?.access_point.endpoint ?? "",
    clientKey: "",
    adminKey: "",
  });
  const [remoteServers, setRemoteServers] = useState(
    initial?.remote?.servers.map((server) => ({
      id: server.id,
      name: server.name,
      endpoint: server.endpoint,
      adminKey: "",
    })) ?? [{ id: "", name: "", endpoint: "", adminKey: "" }],
  );
  const [removeKeys, setRemoveKeys] = useState<string[]>([]);
  const update = (key: keyof typeof form, value: string) =>
    setForm((current) => ({ ...current, [key]: value }));
  async function submit(event: FormEvent) {
    event.preventDefault();
    const base = {
      version: 1 as const,
      id: form.id.trim(),
      name: form.name.trim(),
      description: form.description.trim(),
      ...(form.clientKey.trim()
        ? { client_private_key: form.clientKey.trim() }
        : removeKeys.includes("client")
          ? { client_private_key: "" }
          : {}),
    };
    if (
      initial &&
      (initial.mode !== mode ||
        (initial.remote?.servers.length ?? 0) > remoteServers.length) &&
      !window.confirm(t("confirmTopologyChange"))
    )
      return;
    if (mode === "local")
      await onSave({
        ...base,
        local_server: {
          port: Number(form.port) || 0,
          ...(form.adminKey.trim()
            ? { admin_private_key: form.adminKey.trim() }
            : removeKeys.includes("admin:local")
              ? { admin_private_key: "" }
              : {}),
        },
      });
    else
      await onSave({
        ...base,
        remote_access_point: form.accessPoint.trim(),
        remote_servers: remoteServers.map((server) => ({
          id: server.id.trim(),
          name: server.name.trim(),
          endpoint: server.endpoint.trim(),
          ...(server.adminKey.trim()
            ? { admin_private_key: server.adminKey.trim() }
            : removeKeys.includes(`admin:${server.id}`)
              ? { admin_private_key: "" }
              : {}),
        })),
      });
  }
  const removableCredentials = initial
    ? [
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
      ]
    : [];
  function toggleRemoval(id: string) {
    if (removeKeys.includes(id)) {
      setRemoveKeys((current) => current.filter((value) => value !== id));
      return;
    }
    if (window.confirm(t("confirmRemoveCredential")))
      setRemoveKeys((current) => [...current, id]);
  }
  return (
    <div className="dialog-backdrop">
      <form className="create-dialog" onSubmit={(event) => void submit(event)}>
        <header>
          <div>
            <span className="eyebrow">
              <Plus size={14} /> {initial ? t("editPod") : t("newEnvironment")}
            </span>
            <h2>{initial ? initial.name : t("addPod")}</h2>
          </div>
          <button
            aria-label={t("close")}
            className="icon-button"
            onClick={onClose}
            title={t("close")}
            type="button"
          >
            <X size={20} />
          </button>
        </header>
        <div className="mode-switch">
          <button
            className={mode === "local" ? "active" : ""}
            onClick={() => setMode("local")}
            type="button"
          >
            <Laptop size={17} />
            {t("local")}
          </button>
          <button
            className={mode === "remote" ? "active" : ""}
            onClick={() => setMode("remote")}
            type="button"
          >
            <Cloud size={17} />
            {t("remote")}
          </button>
        </div>
        <div className="form-grid">
          <Field
            disabled={Boolean(initial)}
            label={t("podID")}
            onChange={(value) => update("id", value)}
            placeholder="local-lab"
            required
            value={form.id}
          />
          <Field
            label={t("name")}
            onChange={(value) => update("name", value)}
            placeholder="Local Lab"
            required
            value={form.name}
          />
          <Field
            label={t("description")}
            onChange={(value) => update("description", value)}
            placeholder={t("descriptionPlaceholder")}
            value={form.description}
            wide
          />
          {mode === "local" ? (
            <>
              <Field
                label={t("serverPort")}
                onChange={(value) => update("port", value)}
                placeholder="9820"
                required
                value={form.port}
              />
              <Field
                label={t("adminPrivateKey")}
                onChange={(value) => update("adminKey", value)}
                placeholder={t("optionalWriteOnly")}
                value={form.adminKey}
                secret
              />
            </>
          ) : (
            <>
              <Field
                label={t("accessPoint")}
                onChange={(value) => update("accessPoint", value)}
                placeholder="ap.dev.gizclaw.com:9820"
                required
                value={form.accessPoint}
                wide
              />
              {remoteServers.map((server, index) => (
                <div className="remote-server-form" key={index}>
                  <Field
                    label={`${t("serverID")} ${index + 1}`}
                    onChange={(value) =>
                      setRemoteServers((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index ? { ...item, id: value } : item,
                        ),
                      )
                    }
                    placeholder="beijing-a"
                    required
                    value={server.id}
                  />
                  <Field
                    label={`${t("serverName")} ${index + 1}`}
                    onChange={(value) =>
                      setRemoteServers((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index ? { ...item, name: value } : item,
                        ),
                      )
                    }
                    placeholder="Beijing A"
                    required
                    value={server.name}
                  />
                  <Field
                    label={`${t("serverEndpoint")} ${index + 1}`}
                    onChange={(value) =>
                      setRemoteServers((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index
                            ? { ...item, endpoint: value }
                            : item,
                        ),
                      )
                    }
                    placeholder="115.191.6.117:9820"
                    required
                    value={server.endpoint}
                    wide
                  />
                  <Field
                    label={`${t("adminPrivateKey")} ${index + 1}`}
                    onChange={(value) =>
                      setRemoteServers((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index
                            ? { ...item, adminKey: value }
                            : item,
                        ),
                      )
                    }
                    placeholder={t("optionalWriteOnly")}
                    secret
                    value={server.adminKey}
                    wide
                  />
                  {remoteServers.length > 1 ? (
                    <button
                      className="remove-server"
                      onClick={() =>
                        setRemoteServers((current) =>
                          current.filter((_, itemIndex) => itemIndex !== index),
                        )
                      }
                      type="button"
                    >
                      {t("removeServer")}
                    </button>
                  ) : null}
                </div>
              ))}
              <button
                className="secondary-action add-server"
                onClick={() =>
                  setRemoteServers((current) => [
                    ...current,
                    { id: "", name: "", endpoint: "", adminKey: "" },
                  ])
                }
                type="button"
              >
                <Plus size={15} />
                {t("addAnotherServer")}
              </button>
            </>
          )}
          <Field
            label={t("clientPrivateKey")}
            onChange={(value) => update("clientKey", value)}
            placeholder={t("optionalWriteOnly")}
            value={form.clientKey}
            wide
            secret
          />
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
          <button className="secondary-action" onClick={onClose} type="button">
            {t("cancel")}
          </button>
          <button className="primary-action" type="submit">
            <Plus size={16} />
            {initial ? t("saveConfiguration") : t("create")}
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

function TechBackground() {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const move = (event: PointerEvent) => {
      const x = (event.clientX / window.innerWidth - 0.5) * 18;
      const y = (event.clientY / window.innerHeight - 0.5) * 18;
      ref.current?.style.setProperty("--parallax-x", `${x}px`);
      ref.current?.style.setProperty("--parallax-y", `${y}px`);
    };
    window.addEventListener("pointermove", move, { passive: true });
    return () => window.removeEventListener("pointermove", move);
  }, []);
  return (
    <div className="tech-bg" aria-hidden="true" ref={ref}>
      <div className="grid-plane" />
      <div className="aurora aurora-one" />
      <div className="aurora aurora-two" />
      <div className="noise" />
    </div>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : String(reason);
}
