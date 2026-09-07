import { useEffect, useMemo, useState } from "react";
import { Search, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { PageHeading } from "@/components/app-shell";
import { LogStream } from "@/components/log-stream";
import { LogRecordDetail } from "@/components/log-detail";
import type { ConsoleConfig } from "@/lib/config";
import type { FleetState } from "@/hooks/use-fleet";
import { matches, parseQuery, type LogRecord } from "@/lib/log-query";
import {
  loadDeviceLogs,
  peerId,
  peerLabel,
  type WatchedPeer,
} from "@/lib/peers";
import { nodeErrorMessage } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

const LEVELS = ["ALL", "ERROR", "WARN", "INFO", "DEBUG"] as const;
const WINDOWS = [
  { label: "最近 5 分钟", seconds: 300 },
  { label: "最近 15 分钟", seconds: 900 },
  { label: "最近 1 小时", seconds: 3600 },
  { label: "本次会话全部", seconds: 0 },
];

// The console polls the nodes and its watched devices itself, so its own
// requests would otherwise dominate the stream.
const SELF_NODE_ROUTE = "/monitor/api/node";
const SELF_DEVICE_ROUTES = new Set([
  "/gizclaw/v1/device",
  "/gizclaw/v1/device/runtime",
  "/gizclaw/v1/device/status",
]);

function isConsoleTraffic(record: LogRecord, watched: Set<string>): boolean {
  const route = record.fields?.route;
  if (route === SELF_NODE_ROUTE) return true;
  return (
    route !== undefined &&
    SELF_DEVICE_ROUTES.has(route) &&
    record.peer_public_key !== undefined &&
    watched.has(record.peer_public_key)
  );
}

export function LogsPage({
  config,
  fleet,
  peers,
  initialQuery,
}: {
  config: ConsoleConfig;
  fleet: FleetState;
  peers: WatchedPeer[];
  initialQuery: string;
}) {
  const [text, setText] = useState(initialQuery);
  const [level, setLevel] = useState<string>("ALL");
  const [node, setNode] = useState<string>("ALL");
  const [windowSeconds, setWindowSeconds] = useState(900);
  const [hideSelf, setHideSelf] = useState(true);
  const [selected, setSelected] = useState<LogRecord | undefined>();
  // Two sources answer different questions: the node snapshots carry live
  // process records with request tracing, a device reads its persisted Log
  // Store on the Server.
  const [source, setSource] = useState("nodes");
  const device = peers.find((peer) => peerId(peer) === source);
  const [deviceRecords, setDeviceRecords] = useState<LogRecord[]>([]);
  const [deviceError, setDeviceError] = useState("");
  const [deviceBusy, setDeviceBusy] = useState(false);

  const all = useMemo(() => {
    if (device) return deviceRecords;
    const merged = config.servers.flatMap(
      (server) => fleet[server.id]?.logs ?? [],
    );
    return merged.sort((a, b) => Date.parse(a.time) - Date.parse(b.time));
  }, [config.servers, fleet, device, deviceRecords]);

  useEffect(() => {
    if (!device) {
      setDeviceRecords([]);
      setDeviceError("");
      return;
    }
    const controller = new AbortController();
    setDeviceBusy(true);
    setDeviceError("");
    const end = Date.now();
    loadDeviceLogs(
      device.endpoint,
      device.publicKey,
      "",
      undefined,
      end - (windowSeconds > 0 ? windowSeconds * 1000 : 24 * 3600000),
      end,
      undefined,
      controller.signal,
    )
      .then((page) => {
        if (controller.signal.aborted) return;
        setDeviceRecords(
          page.items.map((item, index) => ({
            id: index,
            time: new Date(item.time_ms).toISOString(),
            level: item.level,
            message: item.message,
            peer_public_key: device.publicKey,
            fields: { ...item.fields, source: item.source, path: item.path },
            node: peerId(device),
            nodeName: peerLabel(device, undefined),
          })),
        );
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted)
          setDeviceError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setDeviceBusy(false);
      });
    return () => controller.abort();
  }, [device, windowSeconds]);

  const watchedKeys = useMemo(
    () => new Set(peers.map((peer) => peer.publicKey)),
    [peers],
  );
  const query = useMemo(() => parseQuery(text), [text]);
  const cutoff = windowSeconds > 0 ? Date.now() - windowSeconds * 1000 : 0;
  const shown = useMemo(
    () =>
      all.filter((record) => {
        if (!device && node !== "ALL" && record.node !== node) return false;
        if (level !== "ALL" && record.level !== level) return false;
        if (cutoff > 0 && Date.parse(record.time) < cutoff) return false;
        if (hideSelf && isConsoleTraffic(record, watchedKeys)) return false;
        return matches(record, query);
      }),
    [all, device, node, level, cutoff, hideSelf, query, watchedKeys],
  );

  const errors = shown.filter((record) => record.level === "ERROR").length;
  const warnings = shown.filter((record) => record.level === "WARN").length;
  const traced = selected?.fields?.request_id;

  return (
    <>
      <PageHeading
        eyebrow="LOG SEARCH"
        title="日志查询"
        description="跨节点检索进程日志：按字段过滤，点开任意一条查看完整字段，并按 request_id 追踪整个请求。"
      />
      <Card>
        <CardHeader>
          <CardTitle>过滤条件</CardTitle>
          <CardDescription>
            自由文本按全字段匹配；<code>key:value</code> 精确到字段，
            <code>-key:value</code> 排除。例如
            <code>
              {" "}
              status:500 operation:getPeerRuntime -route:/server-info
            </code>
            。
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-64 flex-1">
              <Search
                size={14}
                className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                aria-label="日志过滤"
                value={text}
                spellCheck={false}
                onChange={(event) => setText(event.target.value)}
                placeholder="request_id:… 或 status:500 或任意文本"
                className="pl-8 font-mono"
              />
            </div>
            <Select
              aria-label="数据源"
              value={source}
              onChange={(event) => setSource(event.target.value)}
            >
              <option value="nodes">节点进程日志</option>
              {peers.map((peer) => (
                <option key={peerId(peer)} value={peerId(peer)}>
                  设备 · {peerLabel(peer, undefined)}
                </option>
              ))}
            </Select>
            {!device && (
              <Select
                aria-label="节点"
                value={node}
                onChange={(event) => setNode(event.target.value)}
              >
                <option value="ALL">全部节点</option>
                {config.servers.map((server) => (
                  <option key={server.id} value={server.id}>
                    {server.name}
                  </option>
                ))}
              </Select>
            )}
            <Select
              aria-label="级别"
              value={level}
              onChange={(event) => setLevel(event.target.value)}
            >
              {LEVELS.map((value) => (
                <option key={value}>{value}</option>
              ))}
            </Select>
            <Select
              aria-label="时间范围"
              value={windowSeconds}
              onChange={(event) => setWindowSeconds(Number(event.target.value))}
            >
              {WINDOWS.map((option) => (
                <option key={option.seconds} value={option.seconds}>
                  {option.label}
                </option>
              ))}
            </Select>
          </div>
          <div className="flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
            {!device && (
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={hideSelf}
                  onChange={(event) => setHideSelf(event.target.checked)}
                />
                隐藏 Console 自身的轮询请求（节点快照与关注设备）
              </label>
            )}
            {device && (
              <span>
                Log Store · {peerLabel(device, undefined)}
                {deviceBusy ? " · 查询中" : ""}
              </span>
            )}
            <span>
              命中 {shown.length} / {all.length} 条 · ERROR {errors} · WARN{" "}
              {warnings}
            </span>
            {(text !== "" || level !== "ALL" || node !== "ALL") && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setText("");
                  setLevel("ALL");
                  setNode("ALL");
                }}
              >
                <X size={13} /> 清空过滤
              </Button>
            )}
          </div>
          {deviceError !== "" && (
            <Alert>
              <AlertTitle>设备日志查询失败</AlertTitle>
              <AlertDescription>{deviceError}</AlertDescription>
            </Alert>
          )}
          {traced && (
            <div className="flex flex-wrap items-center gap-2 text-[11px]">
              <Badge variant="outline">追踪 request_id</Badge>
              <code className="font-mono">{traced}</code>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setText(`request_id:${traced}`)}
              >
                只看这个请求
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
      <div className="grid min-h-0 flex-1 gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <LogStream
          records={shown}
          selectedId={selected ? `${selected.node}-${selected.id}` : undefined}
          onSelect={setSelected}
          emptyMessage={
            all.length === 0
              ? device
                ? "这段时间没有该设备的持久化日志。"
                : "还没有收到日志。节点快照每 5 秒送来一次进程记录。"
              : "没有命中的记录。放宽过滤条件或时间范围。"
          }
        />
        <LogRecordDetail
          record={selected}
          onFilter={(clause) => setText(clause)}
          onClose={() => setSelected(undefined)}
        />
      </div>
    </>
  );
}
