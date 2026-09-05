import React, { useEffect, useState } from "react";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import {
  monitorErrorMessage,
  loadHistoryAudio,
  loadWorkspaces,
  loadHistory,
  loadLogs,
  telemetryValues,
  bytes,
  type PeerSnapshot,
} from "./api";

export function RawData({ value }: { value: unknown }) {
  return (
    <details className="raw-data">
      <summary>原始数据</summary>
      <pre className="json">{JSON.stringify(value, null, 2)}</pre>
    </details>
  );
}
export function Fields({ value }: { value: unknown }) {
  if (value === null || value === undefined)
    return <span className="muted">未上报</span>;
  if (typeof value !== "object") return <span>{String(value)}</span>;
  return (
    <dl className="detail-fields">
      {Object.entries(value).map(([name, item]) => (
        <div key={name}>
          <dt>{name}</dt>
          <dd>
            <Fields value={item} />
          </dd>
        </div>
      ))}
    </dl>
  );
}
const labels: Record<string, [string, string]> = {
  "battery.percent": ["电量", "%"],
  "battery.charging": ["充电状态", ""],
  "battery.voltage_mv": ["电池电压", "mV"],
  "network.rssi_dbm": ["信号强度", "dBm"],
  "network.signal_level": ["信号等级", ""],
  "network.connected": ["网络连接", ""],
  "system.uptime_seconds": ["运行时长", "秒"],
  "system.free_memory_bytes": ["可用内存", ""],
  "system.temperature_c": ["温度", "°C"],
  "gnss.latitude": ["纬度", "°"],
  "gnss.longitude": ["经度", "°"],
  "gnss.altitude_m": ["海拔", "m"],
  "gnss.accuracy_m": ["定位精度", "m"],
};
function valueText(field: string, value: number) {
  if (field === "battery.charging") return value ? "充电中" : "未充电";
  if (field === "network.connected") return value ? "已连接" : "未连接";
  if (field === "system.free_memory_bytes") return bytes(value);
  return value.toLocaleString(undefined, { maximumFractionDigits: 6 });
}
export function TelemetryPanel({ peer }: { peer: PeerSnapshot | undefined }) {
  const parsed = telemetryValues.safeParse(peer?.telemetry);
  const values = parsed.success ? parsed.data.values : [];
  const [selected, setSelected] = useState("battery.percent");
  const [history, setHistory] = useState<
    Record<string, { time: number; value: number }[]>
  >({});
  useEffect(() => {
    const p = telemetryValues.safeParse(peer?.telemetry);
    if (!p.success) {
      setHistory({});
      return;
    }
    setHistory((old) => {
      const next = { ...old };
      for (const v of p.data.values) {
        const items = next[v.field] ?? [];
        if (items.at(-1)?.time !== v.observed_at_unix_ms)
          next[v.field] = [
            ...items.slice(-119),
            { time: v.observed_at_unix_ms, value: v.value },
          ];
      }
      return next;
    });
  }, [peer?.telemetry]);
  const groups = [
    ...new Set(
      [...Object.keys(labels), ...values.map((v) => v.field)].map(
        (f) => f.split(".")[0],
      ),
    ),
  ];
  return (
    <>
      <div className="section-heading">
        <h2>Telemetry 指标</h2>
        <p>
          每个字段保留自己的采样时间；点击数值卡片查看本次打开页面后采集的趋势。
        </p>
      </div>
      {groups.map((group) => (
        <section className="telemetry-group" key={group}>
          <h3>
            {{ battery: "电源", network: "网络", system: "系统", gnss: "定位" }[
              group
            ] ?? group}
          </h3>
          <div className="telemetry-grid">
            {[
              ...new Set([
                ...Object.keys(labels),
                ...values.map((v) => v.field),
              ]),
            ]
              .filter((f) => f.startsWith(group + "."))
              .map((field) => {
                const item = values.find((v) => v.field === field);
                return (
                  <button
                    className={`telemetry-card ${selected === field ? "selected" : ""}`}
                    key={field}
                    onClick={() => setSelected(field)}
                  >
                    <span>{labels[field]?.[0] ?? field}</span>
                    <strong>
                      {item ? valueText(field, item.value) : "未上报"}{" "}
                      <small>
                        {item && field !== "system.free_memory_bytes"
                          ? labels[field]?.[1]
                          : ""}
                      </small>
                    </strong>
                    <code>{field}</code>
                    <small>
                      {item
                        ? `采样于 ${new Date(item.observed_at_unix_ms).toLocaleString()}`
                        : "尚未收到此字段"}
                    </small>
                    {item && Date.now() - item.observed_at_unix_ms > 60000 && (
                      <small className="stale">超过 1 分钟未更新</small>
                    )}
                  </button>
                );
              })}
          </div>
        </section>
      ))}
      <section className="panel">
        <div className="panel-title">
          <h2>{labels[selected]?.[0] ?? selected} · 趋势</h2>
          <span>本次页面采样 · 最多 120 个点</span>
        </div>
        {(history[selected]?.length ?? 0) > 1 ? (
          <div style={{ height: 220 }}>
            <ResponsiveContainer>
              <AreaChart data={history[selected]}>
                <CartesianGrid strokeDasharray="3 5" />
                <XAxis
                  dataKey="time"
                  tickFormatter={(n) => new Date(n).toLocaleTimeString()}
                />
                <YAxis domain={["auto", "auto"]} />
                <Tooltip
                  labelFormatter={(n) => new Date(Number(n)).toLocaleString()}
                />
                <Area
                  type="stepAfter"
                  dataKey="value"
                  stroke="#4d887a"
                  fill="#4d887a22"
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        ) : (
          <p className="empty-detail">等待至少两次设备采样，未生成示例曲线。</p>
        )}
        <RawData value={peer?.telemetry ?? {}} />
      </section>
    </>
  );
}
export function LocationPanel({ peer }: { peer: PeerSnapshot | undefined }) {
  const [online, setOnline] = useState(navigator.onLine !== false);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine !== false);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
    };
  }, []);
  const p = telemetryValues.safeParse(peer?.telemetry);
  const values = p.success ? p.data.values : [];
  const lat = values.find((x) => x.field === "gnss.latitude"),
    lon = values.find((x) => x.field === "gnss.longitude");
  const valid =
    lat && lon && Math.abs(lat.value) <= 90 && Math.abs(lon.value) <= 180;
  const url =
    valid && online
      ? `https://www.openstreetmap.org/export/embed.html?bbox=${Math.max(-180, lon.value - 0.01)},${Math.max(-90, lat.value - 0.01)},${Math.min(180, lon.value + 0.01)},${Math.min(90, lat.value + 0.01)}&layer=mapnik&marker=${lat.value},${lon.value}`
      : undefined;
  return (
    <section className="panel">
      <div className="panel-title">
        <h2>设备定位</h2>
        <span>GNSS · 设备上报</span>
      </div>
      {url ? (
        <iframe
          className="device-map"
          title="设备位置地图"
          src={url}
          loading="lazy"
          referrerPolicy="no-referrer"
          sandbox="allow-scripts allow-same-origin allow-popups"
        />
      ) : (
        <div className="empty-detail">
          {valid && !online
            ? "当前浏览器离线，地图不可用；下方保留最后上报的坐标。"
            : "尚未收到有效经纬度，无法显示设备位置。"}
        </div>
      )}
      <div className="telemetry-grid">
        {[
          "gnss.latitude",
          "gnss.longitude",
          "gnss.accuracy_m",
          "gnss.altitude_m",
        ].map((f) => {
          const v = values.find((x) => x.field === f);
          return (
            <div className="telemetry-card" key={f}>
              <span>{labels[f][0]}</span>
              <strong>
                {v ? v.value : "未上报"} <small>{v ? labels[f][1] : ""}</small>
              </strong>
              <small>
                {v
                  ? new Date(v.observed_at_unix_ms).toLocaleString()
                  : "尚无采样"}
              </small>
            </div>
          );
        })}
      </div>
      <p className="source-note">
        地图使用
        OpenStreetMap；打开定位页时自动向该服务加载已上报坐标。坐标为最后上报值，请核对采样时间，并不代表设备当前位置。地图需要联网，加载失败时仍可查看坐标数值。
      </p>
    </section>
  );
}
export function WorkflowsPanel({ credential }: { credential: string }) {
  const [items, setItems] = useState<
      Awaited<ReturnType<typeof loadWorkspaces>>
    >([]),
    [workflow, setWorkflow] = useState(""),
    [workspace, setWorkspace] = useState("");
  const [page, setPage] = useState<Awaited<ReturnType<typeof loadHistory>>>(),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState(""),
    [cursor, setCursor] = useState<string>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loadingWorkspaces, setLoadingWorkspaces] = useState(true);
  useEffect(() => {
    const c = new AbortController();
    setError("");
    setLoadingWorkspaces(true);
    setItems([]);
    setWorkspace("");
    setPage(undefined);
    void loadWorkspaces(credential, c.signal)
      .then((data) => {
        if (c.signal.aborted) return;
        setItems(data);
        setWorkflow(data[0]?.workflow_id ?? "");
        setWorkspace(data[0]?.id ?? "");
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(monitorErrorMessage(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoadingWorkspaces(false);
      });
    return () => c.abort();
  }, [credential]);
  useEffect(() => {
    setPage(undefined);
    if (!workspace) return;
    const c = new AbortController();
    setBusy(true);
    setError("");
    void loadHistory(credential, workspace, search, cursor, c.signal)
      .then((data) => {
        if (!c.signal.aborted) setPage(data);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(monitorErrorMessage(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setBusy(false);
      });
    return () => c.abort();
  }, [credential, workspace, search, cursor]);
  function select(id: string) {
    setWorkspace(id);
    setCursor(undefined);
    setQuery("");
    setSearch("");
    setPage(undefined);
  }
  const selected = items.find((x) => x.id === workspace);
  return (
    <>
      <div className="section-heading">
        <h2>Workflows 与聊天记录</h2>
        <p>仅展示该设备拥有的 Workspace；聊天记录从持久化 History 读取。</p>
      </div>
      {error && (
        <div role="alert" className="error">
          {error}
        </div>
      )}
      {loadingWorkspaces ? (
        <p className="empty-detail">正在读取 Workflows…</p>
      ) : items.length === 0 && !error ? (
        <p className="empty-detail">尚无可展示的 Workspace。</p>
      ) : (
        <>
          <select
            aria-label="Workflow"
            value={workflow}
            onChange={(e) => {
              setWorkflow(e.target.value);
              select(
                items.find((x) => x.workflow_id === e.target.value)?.id ?? "",
              );
            }}
          >
            {[...new Set(items.map((x) => x.workflow_id))].map((id) => (
              <option key={id}>{id}</option>
            ))}
          </select>
          <div className="workspace-browser">
            <section className="workspace-list">
              <h3>关联 Workspace</h3>
              {items
                .filter((x) => x.workflow_id === workflow)
                .map((x) => (
                  <button
                    className={x.id === workspace ? "selected" : ""}
                    key={x.id}
                    onClick={() => select(x.id)}
                  >
                    <strong>{x.name}</strong>
                    <small>{new Date(x.last_active_at).toLocaleString()}</small>
                  </button>
                ))}
            </section>
            <section className="chat-history">
              <h3>{selected?.name}</h3>
              <form
                className="history-search"
                onSubmit={(e) => {
                  e.preventDefault();
                  setSearch(query);
                  setCursor(undefined);
                }}
              >
                <input
                  aria-label="搜索历史聊天"
                  placeholder="搜索历史聊天内容…"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <button>搜索</button>
              </form>
              {busy ? (
                <p>正在读取历史记录…</p>
              ) : page?.items.length === 0 ? (
                <p className="empty-detail">没有匹配的历史记录。</p>
              ) : (
                page?.items
                  .slice()
                  .reverse()
                  .map((item) => (
                    <article
                      className={`chat-message ${item.type === "gear" ? "user" : "assistant"}`}
                      key={item.name}
                    >
                      <header>
                        <strong>{item.actor_name || item.type}</strong>
                        <time>
                          {new Date(item.created_at).toLocaleString()}
                        </time>
                      </header>
                      <p>{item.text || "此记录没有文本"}</p>
                      {item.replay_available && (
                        <HistoryAudio
                          credential={credential}
                          workspace={workspace}
                          history={item.name}
                        />
                      )}
                    </article>
                  ))
              )}
              {page?.has_next && (
                <button onClick={() => setCursor(page.next_cursor)}>
                  更早的记录
                </button>
              )}
              <p className="source-note">
                当前页按时间顺序展示 · 每页最多 100 条
              </p>
            </section>
            <section className="workspace-context">
              <h3>上下文</h3>
              <Fields value={selected ?? {}} />
            </section>
          </div>
        </>
      )}
    </>
  );
}
type Level = "DEBUG" | "INFO" | "WARN" | "ERROR";
export function PersistentLogsPanel({ credential }: { credential: string }) {
  const [query, setQuery] = useState(""),
    [level, setLevel] = useState<Level | undefined>(),
    [hours, setHours] = useState(1),
    [request, setRequest] = useState({
      query: "",
      level: undefined as Level | undefined,
      start: Date.now() - 3600000,
      end: Date.now(),
      cursor: undefined as string | undefined,
    });
  const [page, setPage] = useState<Awaited<ReturnType<typeof loadLogs>>>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    const c = new AbortController();
    setBusy(true);
    setError("");
    setPage(undefined);
    void loadLogs(
      credential,
      request.query,
      request.level,
      request.start,
      request.end,
      request.cursor,
      c.signal,
    )
      .then((data) => {
        if (!c.signal.aborted) setPage(data);
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(monitorErrorMessage(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setBusy(false);
      });
    return () => c.abort();
  }, [credential, request]);
  return (
    <section className="panel">
      <div className="panel-title">
        <h2>运行日志</h2>
        <span>Log Store · 当前设备</span>
      </div>
      <form
        className="history-search"
        onSubmit={(e) => {
          e.preventDefault();
          const end = Date.now();
          setRequest({
            query,
            level,
            start: end - hours * 3600000,
            end,
            cursor: undefined,
          });
        }}
      >
        <input
          aria-label="搜索历史日志"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="搜索消息或错误…"
        />
        <select
          aria-label="历史日志级别"
          value={level ?? ""}
          onChange={(e) => {
            const value = e.target.value;
            setLevel(
              value === "DEBUG" ||
                value === "INFO" ||
                value === "WARN" ||
                value === "ERROR"
                ? value
                : undefined,
            );
          }}
        >
          <option value="">全部级别</option>
          {["DEBUG", "INFO", "WARN", "ERROR"].map((l) => (
            <option key={l}>{l}</option>
          ))}
        </select>
        <select
          aria-label="日志时间范围"
          value={hours}
          onChange={(e) => setHours(Number(e.target.value))}
        >
          <option value={1}>最近 1 小时</option>
          <option value={24}>最近 24 小时</option>
          <option value={168}>最近 7 天</option>
        </select>
        <button disabled={busy}>查询 / 刷新</button>
      </form>
      {error && (
        <div role="alert" className="error">
          {error}
        </div>
      )}
      <div className="dense-logs" role="log" aria-label="设备日志" tabIndex={0}>
        {busy
          ? "正在读取…"
          : page?.items.length === 0
            ? "没有匹配的日志。"
            : page?.items.map((entry, i) => (
                <div className="dense-log" key={`${entry.time_ms}-${i}`}>
                  <time>{new Date(entry.time_ms).toLocaleString()}</time>
                  <b className={`level-${entry.level}`}>{entry.level}</b>
                  <span>[{entry.path || entry.source}]</span>
                  <span>{entry.message}</span>
                  {entry.fields.error && (
                    <span className="level-ERROR">{entry.fields.error}</span>
                  )}
                </div>
              ))}
      </div>
      {page?.end.has_next && (
        <button
          onClick={() =>
            setRequest((old) => ({ ...old, cursor: page.end.next_cursor }))
          }
        >
          下一页
        </button>
      )}
      <p className="source-note">
        {page?.items.length ?? 0} 条 · 查询时间{" "}
        {new Date(request.start).toLocaleString()} —{" "}
        {new Date(request.end).toLocaleString()}
      </p>
    </section>
  );
}

function HistoryAudio({
  credential,
  workspace,
  history,
}: {
  credential: string;
  workspace: string;
  history: string;
}) {
  const [requested, setRequested] = useState(0),
    [url, setURL] = useState(""),
    [error, setError] = useState("");
  useEffect(() => {
    if (!requested) return;
    const controller = new AbortController();
    let objectURL = "";
    setURL("");
    setError("");
    void loadHistoryAudio(credential, workspace, history, controller.signal)
      .then((blob) => {
        if (controller.signal.aborted) return;
        objectURL = URL.createObjectURL(blob);
        setURL(objectURL);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(monitorErrorMessage(e));
      });
    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [requested, credential, workspace, history]);
  return (
    <div>
      {url ? (
        <audio controls src={url} aria-label="历史聊天音频" />
      ) : (
        <button
          disabled={requested > 0 && !error}
          onClick={() => setRequested((value) => value + 1)}
        >
          {requested && !error ? "正在读取音频…" : "读取音频"}
        </button>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
