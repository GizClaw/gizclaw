import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  ArrowDownLeft,
  ArrowUpRight,
  Box,
  ChevronRight,
  CircleHelp,
  Copy,
  KeyRound,
  LogOut,
  Pause,
  Play,
  Search,
  Server,
  ShieldCheck,
  Terminal,
  Wifi,
} from "lucide-react";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  CartesianGrid,
  XAxis,
  YAxis,
  Tooltip,
} from "recharts";
import { Button } from "./components/ui/button";
import {
  monitorErrorMessage,
  bytes,
  controlPeer,
  findPeers,
  loadNode,
  loadPeer,
  rates,
  type LogEntry,
  type NodeSnapshot,
  type PeerSnapshot,
  type Sample,
} from "./api";
import "./style.css";
import {
  Fields,
  RawData,
  TelemetryPanel,
  LocationPanel,
  WorkflowsPanel,
  PersistentLogsPanel,
} from "./device-panels";
import {
  readConnection,
  saveConnection,
  clearConnection,
} from "./connection-store";

function Logs({ entries }: { entries: LogEntry[] }) {
  const [filter, setFilter] = useState("");
  const [level, setLevel] = useState("ALL");
  const [follow, setFollow] = useState(true);
  const [scroll, setScroll] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  const filtered = entries.filter(
    (e) =>
      (level === "ALL" || e.level === level) &&
      `${e.message} ${e.error ?? ""} ${e.peer_public_key ?? ""}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
  );
  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [entries, follow]);
  const start = Math.max(0, Math.floor(scroll / 26) - 4);
  const shown = filtered.slice(start, start + 28);
  return (
    <section className="logs">
      <div className="log-tools">
        <span>
          <Terminal size={16} /> 实时日志 <small>{filtered.length} 条</small>
        </span>
        <input
          aria-label="筛选日志"
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
            setScroll(0);
            if (box.current) box.current.scrollTop = 0;
          }}
          placeholder="搜索消息或设备公钥…"
        />
        <select
          aria-label="日志级别"
          value={level}
          onChange={(e) => {
            setLevel(e.target.value);
            setScroll(0);
            if (box.current) box.current.scrollTop = 0;
          }}
        >
          {["ALL", "DEBUG", "INFO", "WARN", "ERROR"].map((x) => (
            <option key={x}>{x}</option>
          ))}
        </select>
        <button onClick={() => setFollow(!follow)}>
          {follow ? "跟随中" : "自动跟随"}
        </button>
      </div>
      <div
        className="log-body"
        ref={box}
        onScroll={(e) => setScroll(e.currentTarget.scrollTop)}
      >
        {filtered.length === 0 ? (
          <div className="log-empty">
            暂无匹配日志。只显示当前进程实际记录的数据。
          </div>
        ) : (
          <div style={{ height: filtered.length * 26, position: "relative" }}>
            {shown.map((e, i) => (
              <div
                className="log-line"
                key={e.id}
                style={{
                  position: "absolute",
                  top: (start + i) * 26,
                  left: 0,
                  right: 0,
                }}
              >
                <time>{new Date(e.time).toLocaleTimeString()}</time>
                <b className={`level-${e.level}`}>{e.level}</b>
                {e.peer_public_key && (
                  <code className="log-peer" title={e.peer_public_key}>
                    {e.peer_public_key}
                  </code>
                )}
                <span title={`${e.message} ${e.error ?? ""}`}>
                  {e.message} {e.error}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
      <footer>服务端进程日志 · 最多保留 500 条 · 重启后清空</footer>
    </section>
  );
}
function Metric({
  label,
  value,
  note,
  icon,
}: {
  label: string;
  value: string;
  note: string;
  icon: React.ReactNode;
}) {
  return (
    <div className="metric">
      <div>
        {label}
        {icon}
      </div>
      <strong>{value}</strong>
      <small>{note}</small>
    </div>
  );
}
function savedTab(isNode: boolean): string {
  try {
    const t = localStorage.getItem(
      isNode ? "monitor-node-tab" : "monitor-peer-tab",
    );
    if (
      t &&
      (isNode
        ? ["logs", "config"]
        : [
            "overview",
            "workflows",
            "traffic",
            "telemetry",
            "location",
            "logs",
            "config",
            "actions",
          ]
      ).includes(t)
    )
      return t;
  } catch {
    /* Display preference storage may be disabled. */
  }
  return isNode ? "logs" : "overview";
}
export function App() {
  const isNode = window.location.pathname !== "/monitor/peer";
  const [credential, setCredential] = useState("");
  const [input, setInput] = useState("");
  const [kind, setKind] = useState("sn");
  const [serial, setSerial] = useState("");
  const [matches, setMatches] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [node, setNode] = useState<NodeSnapshot>();
  const [peer, setPeer] = useState<PeerSnapshot>();
  const [samples, setSamples] = useState<Sample[]>([]);
  const [paused, setPaused] = useState(false);
  const [tab, setTab] = useState(() => savedTab(isNode));
  const [windowSize, setWindowSize] = useState(120);
  useEffect(() => {
    try {
      localStorage.setItem(
        isNode ? "monitor-node-tab" : "monitor-peer-tab",
        tab,
      );
    } catch {
      /* Display preferences are optional. */
    }
  }, [isNode, tab]);
  const connectionName = isNode ? "node" : "peer";
  const restoreVersion = useRef(0);
  useEffect(() => {
    let active = true;
    const version = restoreVersion.current;
    void readConnection(connectionName)
      .then((value) => {
        if (active && version === restoreVersion.current) setCredential(value);
      })
      .catch((e) => {
        if (active) setNotice(String(e));
      });
    return () => {
      active = false;
    };
  }, [connectionName]);
  async function remember(value: string) {
    const version = ++restoreVersion.current;
    try {
      await saveConnection(connectionName, value);
    } catch (e) {
      if (version === restoreVersion.current)
        setNotice(`连接可用，但无法本地保存：${String(e)}`);
    }
    return version === restoreVersion.current;
  }
  const [last, setLast] = useState<Date>();
  const [volume, setVolume] = useState(50);
  const formAbort = useRef<AbortController | undefined>(undefined);
  const controlsAbort = useRef<AbortController | undefined>(undefined);
  useEffect(
    () => () => {
      restoreVersion.current++;
      formAbort.current?.abort();
      controlsAbort.current?.abort();
    },
    [],
  );
  useEffect(() => {
    if (!credential || paused) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const controller = new AbortController();
    let previous: { time: number; rx: number; tx: number } | undefined;
    async function poll() {
      try {
        const now = Date.now();
        let rx: number | undefined, tx: number | undefined;
        if (isNode) {
          const data = await loadNode(credential, controller.signal);
          if (stopped) return;
          setNode(data);
          rx = data.transport.rx_bytes;
          tx = data.transport.tx_bytes;
        } else {
          const data = await loadPeer(credential, controller.signal);
          if (stopped) return;
          setPeer(data);
          rx = data.runtime.rx_bytes;
          tx = data.runtime.tx_bytes;
        }
        if (rx !== undefined && tx !== undefined) {
          const current = { time: now, rx, tx };
          const sample = rates(previous, current);
          previous = current;
          setSamples((old) => [...old.slice(-1799), sample]);
        } else {
          previous = undefined;
          setSamples([]);
        }
        setError("");
        setLast(new Date());
      } catch (e) {
        if (!stopped) {
          setError(monitorErrorMessage(e));
          setNode(undefined);
          setPeer(undefined);
          setSamples([]);
          setLast(undefined);
          previous = undefined;
        }
      } finally {
        if (!stopped) timer = setTimeout(() => void poll(), 1000);
      }
    }
    void poll();
    return () => {
      stopped = true;
      controller.abort();
      clearTimeout(timer);
    };
  }, [credential, isNode, paused]);
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setNotice("");
    setBusy(true);
    formAbort.current?.abort();
    const controller = new AbortController();
    formAbort.current = controller;
    try {
      if (isNode) {
        const data = await loadNode(input.trim(), controller.signal);
        setNode(data);
        if (!(await remember(input.trim()))) return;
        if (controller.signal.aborted) return;
        setCredential(input.trim());
        setInput("");
      } else {
        const keys = await findPeers(
          kind,
          input.trim(),
          serial.trim(),
          controller.signal,
        );
        setMatches(keys);
        if (keys.length === 1) await select(keys[0]);
      }
    } catch (e) {
      if (!controller.signal.aborted) setError(monitorErrorMessage(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  async function select(key: string) {
    if (!(await remember(key))) return;
    setCredential(key);
    setNode(undefined);
    setPeer(undefined);
    setSamples([]);
    setError("");
    setPaused(false);
  }
  async function disconnect() {
    restoreVersion.current++;
    formAbort.current?.abort();
    setNotice("");
    const clearing = clearConnection(connectionName);
    setBusy(false);
    setCredential("");
    setNode(undefined);
    setPeer(undefined);
    setSamples([]);
    setLast(undefined);
    setError("");
    setPaused(false);
    controlsAbort.current?.abort();
    try {
      await clearing;
    } catch (e) {
      setNotice(`已断开连接，但清除本地记录失败：${String(e)}`);
    }
  }
  async function action(name: "reboot" | "volume") {
    if (
      name === "reboot" &&
      !window.confirm("确认重启这台设备？连接将暂时断开。")
    )
      return;
    setBusy(true);
    setNotice("");
    controlsAbort.current?.abort();
    const c = new AbortController();
    controlsAbort.current = c;
    try {
      await controlPeer(credential, name, volume, c.signal);
      setNotice("设备已确认操作。");
    } catch (e) {
      if (!c.signal.aborted) setError(monitorErrorMessage(e));
    } finally {
      if (!c.signal.aborted) setBusy(false);
    }
  }
  const connected = isNode ? !!node : !!peer;
  const logs = node?.logs ?? peer?.logs ?? [];
  const rx = node?.transport.rx_bytes ?? peer?.runtime.rx_bytes;
  const tx = node?.transport.tx_bytes ?? peer?.runtime.tx_bytes;
  const latest = samples.at(-1);
  return (
    <div className="app">
      <aside>
        <a className="brand" href="/monitor/node">
          <span className="brand-mark">✳</span> GizClaw
          <span className="brand-sub">MONITOR</span>
        </a>
        <div className="nav-label">OBSERVABILITY</div>
        <nav>
          <a className={isNode ? "selected" : ""} href="/monitor/node">
            <Server size={18} /> 节点监控 <ChevronRight size={14} />
          </a>
          <a className={!isNode ? "selected" : ""} href="/monitor/peer">
            <Box size={18} /> 设备监控 <ChevronRight size={14} />
          </a>
        </nav>
        <div className="rail-note">
          <ShieldCheck size={19} />
          <strong>数据来自当前节点</strong>
          <p>节点使用独立 Monitor Token。设备访问由所属 Server 检查权限。</p>
        </div>
        <div className="rail-bottom">
          GizClaw Console <span>01</span>
        </div>
      </aside>
      <div className="workspace">
        <header>
          <span>
            Monitor <ChevronRight size={13} /> {isNode ? "Node" : "Peer"}
          </span>
          <span className="host">{window.location.host}</span>
        </header>
        <main>
          <div className="heading">
            <div>
              <div className="eyebrow">
                {isNode
                  ? "YOUR NODE, IN REAL TIME"
                  : "A CLOSER LOOK AT YOUR DEVICE"}
              </div>
              <h1>{isNode ? "节点监控" : "设备监控"}</h1>
              <p>
                {isNode
                  ? "连接、流量与运行日志，尽在此处。"
                  : "从设备身份开始，查看状态、流量和配置。"}
              </p>
            </div>
            <div className="live">
              <i className={connected && !paused && !error ? "green" : ""} />
              {paused
                ? "已暂停"
                : connected && !error
                  ? "实时更新"
                  : credential
                    ? "连接中"
                    : "尚未连接"}
            </div>
          </div>
          {!credential ? (
            <section className="connect panel">
              <div className="connect-icon">
                {isNode ? <KeyRound size={25} /> : <Search size={25} />}
              </div>
              <div>
                <h2>{isNode ? "连接到这个节点" : "查找你的设备"}</h2>
                <p>
                  {isNode
                    ? "输入此节点的 Monitor Token，只读查看运行信息。"
                    : "使用 SN、IMEI 或公钥查找。重复标识会列出所有匹配设备。"}
                </p>
              </div>
              <form onSubmit={(e) => void submit(e)}>
                {!isNode && (
                  <select
                    aria-label="查询类型"
                    value={kind}
                    onChange={(e) => {
                      setKind(e.target.value);
                      setMatches(null);
                    }}
                  >
                    <option value="sn">SN</option>
                    <option value="imei">IMEI</option>
                    <option value="key">Public Key</option>
                  </select>
                )}
                <input
                  aria-label={isNode ? "Monitor Token" : "设备标识"}
                  type={isNode ? "password" : "text"}
                  autoComplete="off"
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  placeholder={
                    isNode
                      ? "gizclaw_mk_…"
                      : kind === "imei"
                        ? "TAC"
                        : kind === "sn"
                          ? "输入设备 SN"
                          : "输入设备公钥"
                  }
                  required
                />
                {!isNode && kind === "imei" && (
                  <input
                    aria-label="IMEI Serial"
                    value={serial}
                    onChange={(e) => setSerial(e.target.value)}
                    placeholder="Serial"
                    required
                  />
                )}
                <Button disabled={busy || !input.trim()}>
                  {busy ? "正在连接…" : isNode ? "连接节点" : "查找设备"}
                  <ChevronRight size={15} />
                </Button>
              </form>
              <small>
                <ShieldCheck size={13} />
                {isNode
                  ? "连接信息加密保存在当前浏览器，刷新后恢复；退出可清除。"
                  : "无需 API Key；设备需自行开启 readonly 或 fullcontrol。"}
              </small>
              {matches && (
                <div className="matches">
                  {matches.length === 0 ? (
                    <p>没有匹配设备。</p>
                  ) : (
                    matches.map((key) => (
                      <button key={key} onClick={() => void select(key)}>
                        <Box size={16} />
                        <code>{key}</code>
                        <ChevronRight size={14} />
                      </button>
                    ))
                  )}
                </div>
              )}
            </section>
          ) : (
            <div className="identity">
              <span className="identity-icon">
                {isNode ? <Server size={20} /> : <Box size={20} />}
              </span>
              <div>
                <strong>
                  {isNode
                    ? (node?.role ?? "Node")
                    : typeof peer?.info.name === "string"
                      ? peer.info.name
                      : "Device"}
                </strong>
                <code>
                  {isNode ? (node?.public_key ?? "等待节点响应") : credential}
                </code>
                {!isNode && peer && (
                  <div className="identity-details">
                    <Fields value={peer.info.identifiers ?? {}} />
                    <Fields value={peer.info.hardware ?? {}} />
                  </div>
                )}
              </div>
              <Button
                variant="ghost"
                aria-label="复制公钥"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(isNode ? (node?.public_key ?? "") : credential)
                    .then(
                      () => setNotice("公钥已复制"),
                      () => setNotice("复制失败，请手动选择公钥"),
                    );
                }}
              >
                <Copy size={15} />
              </Button>
              <span className="permission">
                {isNode ? "只读" : (peer?.runtime.debug_mode ?? "等待权限检查")}
              </span>
              <Button
                variant="outline"
                onClick={() => {
                  if (paused) setSamples([]);
                  setPaused(!paused);
                }}
              >
                {paused ? <Play size={14} /> : <Pause size={14} />}{" "}
                {paused ? "继续" : "暂停"}
              </Button>
              <Button variant="ghost" onClick={() => void disconnect()}>
                <LogOut size={15} /> 退出并清除
              </Button>
            </div>
          )}
          {error && (
            <div className="alert" role="alert">
              <strong>无法读取数据</strong>
              <span>{error}</span>
              <small>
                检查凭证、节点配置和设备调试模式。重连成功后将自动恢复。
              </small>
            </div>
          )}
          {notice && (
            <div className="notice" role="status">
              {notice}
            </div>
          )}
          <div className="tabs" role="tablist" aria-label="监控详情">
            {(isNode
              ? ["logs", "config"]
              : [
                  "overview",
                  "workflows",
                  "traffic",
                  "telemetry",
                  "location",
                  "logs",
                  "config",
                  "actions",
                ]
            ).map((t) => (
              <button
                role="tab"
                aria-selected={tab === t}
                key={t}
                onClick={() => setTab(t)}
              >
                {
                  {
                    overview: "概览",
                    workflows: "Workflows",
                    traffic: "实时流量",
                    location: "定位",
                    logs: "运行日志",
                    config: "配置与状态",
                    telemetry: "Telemetry",
                    actions: "设备操作",
                  }[t]
                }
              </button>
            ))}
            <span>
              {last ? `更新于 ${last.toLocaleTimeString()}` : "等待数据"}
            </span>
          </div>
          <div className="metrics">
            <Metric
              label={isNode ? "WebRTC 连接" : "设备状态"}
              value={
                isNode
                  ? node
                    ? String(node.transport.connections)
                    : "—"
                  : peer
                    ? peer.runtime.online
                      ? "在线"
                      : "离线"
                    : "—"
              }
              note={isNode ? "当前活跃关联" : "所属 Server 报告"}
              icon={<Wifi size={16} />}
            />
            <Metric
              label={isNode ? "接收速率" : "设备上行"}
              value={
                connected && latest && samples.length > 1
                  ? `${bytes(latest.rx)}/s`
                  : "—"
              }
              note={rx !== undefined ? `累计 ${bytes(rx)}` : "等待采样"}
              icon={<ArrowDownLeft size={17} />}
            />
            <Metric
              label={isNode ? "发送速率" : "设备下行"}
              value={
                connected && latest && samples.length > 1
                  ? `${bytes(latest.tx)}/s`
                  : "—"
              }
              note={tx !== undefined ? `累计 ${bytes(tx)}` : "等待采样"}
              icon={<ArrowUpRight size={17} />}
            />
            <Metric
              label={isNode ? "运行时间" : "调试权限"}
              value={
                isNode
                  ? node
                    ? `${Math.floor(node.uptime_seconds / 60)} 分钟`
                    : "—"
                  : (peer?.runtime.debug_mode ?? "—")
              }
              note={
                isNode
                  ? node
                    ? `${node.goroutines} goroutines · ${bytes(node.heap_bytes)} 堆内存`
                    : "当前进程"
                  : "由设备自行设定"
              }
              icon={<Activity size={16} />}
            />
          </div>
          {(isNode || tab === "traffic" || tab === "overview") && (
            <section className="panel traffic">
              <div className="panel-title">
                <div>
                  <h2>流量趋势</h2>
                  <p>应用负载字节，不含 ICE / DTLS 开销 · 本次连接采样</p>
                  <select
                    aria-label="流量时间窗口"
                    value={windowSize}
                    onChange={(e) => setWindowSize(Number(e.target.value))}
                  >
                    <option value={120}>最近 2 分钟</option>
                    <option value={600}>最近 10 分钟</option>
                    <option value={1800}>最近 30 分钟</option>
                  </select>
                </div>
                <div className="legend">
                  <span className="rx-dot" /> {isNode ? "接收" : "上行"}{" "}
                  <span className="tx-dot" /> {isNode ? "发送" : "下行"}{" "}
                  <span className="range">实时 / 1s</span>
                </div>
              </div>
              {samples.length > 1 ? (
                <div className="chart">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart
                      data={samples.filter(
                        (sample) =>
                          sample.timestamp >=
                          (samples.at(-1)?.timestamp ?? 0) - windowSize * 1000,
                      )}
                      margin={{ top: 10, right: 12, left: 8, bottom: 0 }}
                    >
                      <CartesianGrid
                        stroke="#e6dfd8"
                        strokeDasharray="3 4"
                        vertical={false}
                      />
                      <XAxis
                        dataKey="time"
                        tick={{ fontSize: 11, fill: "#8e8b82" }}
                        minTickGap={70}
                        axisLine={false}
                        tickLine={false}
                      />
                      <YAxis
                        width={85}
                        tickFormatter={(value) => `${bytes(Number(value))}/s`}
                        tick={{ fontSize: 11, fill: "#8e8b82" }}
                        axisLine={false}
                        tickLine={false}
                      />
                      <Tooltip
                        formatter={(value) => `${bytes(Number(value))}/s`}
                      />
                      <Area
                        name={isNode ? "接收" : "设备上行"}
                        type="monotone"
                        dataKey="rx"
                        stroke="#438e80"
                        fill="#5db8a6"
                        fillOpacity={0.1}
                        isAnimationActive={false}
                      />
                      <Area
                        name={isNode ? "发送" : "设备下行"}
                        type="monotone"
                        dataKey="tx"
                        stroke="#cc785c"
                        fill="#cc785c"
                        fillOpacity={0.08}
                        isAnimationActive={false}
                      />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              ) : (
                <div className="chart-empty">
                  <Activity size={32} />
                  <strong>
                    {credential ? "等待流量采样" : "连接后显示实时流量"}
                  </strong>
                  <span>这里仅展示实际测量的数据</span>
                </div>
              )}
            </section>
          )}
          {tab === "logs" && isNode && (
            <>
              <Logs entries={logs} />
              {!isNode && (
                <p className="source-note">
                  <CircleHelp size={14} />{" "}
                  此处为服务端记录的设备相关日志，不是固件串口日志。
                </p>
              )}
            </>
          )}
          {!isNode && connected && credential && tab === "logs" && (
            <PersistentLogsPanel key={credential} credential={credential} />
          )}
          {!isNode &&
            connected &&
            credential &&
            (tab === "workflows" || tab === "overview") && (
              <WorkflowsPanel key={credential} credential={credential} />
            )}
          {tab === "config" && (
            <section className="panel">
              <h2>{isNode ? "节点运行快照" : "设备信息与状态"}</h2>
              <Fields
                value={
                  isNode
                    ? node
                    : peer
                      ? {
                          info: peer.info,
                          runtime: peer.runtime,
                          status: peer.status,
                        }
                      : {}
                }
              />
              <RawData value={isNode ? node : peer} />
            </section>
          )}
          {tab === "telemetry" && (
            <TelemetryPanel key={credential} peer={peer} />
          )}
          {tab === "location" && <LocationPanel peer={peer} />}
          {tab === "actions" && (
            <section className="panel actions">
              <h2>设备控制</h2>
              <p>仅 fullcontrol 且设备在线时可用。Server 会再次检查权限。</p>
              <label>
                音量{" "}
                <input
                  aria-label="音量"
                  type="range"
                  min="0"
                  max="100"
                  value={volume}
                  onChange={(e) => setVolume(Number(e.target.value))}
                />
                {volume}
              </label>
              <Button
                disabled={
                  busy ||
                  !!error ||
                  peer?.runtime.debug_mode !== "fullcontrol" ||
                  !peer?.runtime.online
                }
                onClick={() => void action("volume")}
              >
                设置音量
              </Button>
              <Button
                variant="outline"
                disabled={
                  busy ||
                  !!error ||
                  peer?.runtime.debug_mode !== "fullcontrol" ||
                  !peer?.runtime.online
                }
                onClick={() => void action("reboot")}
              >
                重启设备
              </Button>
            </section>
          )}
          <footer className="page-footer">
            <span>
              <ShieldCheck size={13} />{" "}
              {isNode
                ? "节点数据受 Monitor Token 保护"
                : "权限由设备所属 Server 执行"}
            </span>
            <span>GizClaw Monitor</span>
          </footer>
        </main>
      </div>
    </div>
  );
}
const root = document.getElementById("root");
if (root)
  createRoot(root).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
