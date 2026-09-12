import { useEffect, useMemo, useRef, useState } from "react";
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
import {
  matches,
  parseQuery,
  summarize,
  type LogRecord,
} from "@/lib/log-query";
import {
  loadDeviceLogs,
  peerId,
  peerLabel,
  type WatchedPeer,
} from "@/lib/peers";
import { nodeErrorMessage } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { usePageView } from "@/assistant/page-view";

const LEVELS = ["ALL", "ERROR", "WARN", "INFO", "DEBUG"] as const;
const WINDOWS = [
  { label: "最近 5 分钟", seconds: 300 },
  { label: "最近 15 分钟", seconds: 900 },
  { label: "最近 1 小时", seconds: 3600 },
  { label: "最近 24 小时", seconds: 24 * 60 * 60 },
];

export function LogsPage({
  peers,
  initialQuery,
}: {
  peers: WatchedPeer[];
  initialQuery: string;
}) {
  const [text, setText] = useState(initialQuery);
  const [level, setLevel] = useState<string>("ALL");
  const [windowSeconds, setWindowSeconds] = useState(900);
  const [selected, setSelected] = useState<LogRecord | undefined>();
  // A query that names a device (from the peers page or the assistant) opens
  // that device's logs; otherwise the first watched device is the source.
  // The source is derived on every render, so a named device that joins the
  // watch list after the page opens is still picked up; a device the user
  // picks explicitly wins while it stays watched.
  const [chosen, setChosen] = useState<string>();
  const wanted = useMemo(
    () =>
      parseQuery(initialQuery).clauses.find(
        (clause) =>
          !clause.negate &&
          (clause.key === "peer_public_key" || clause.key === "peer"),
      )?.value,
    [initialQuery],
  );
  const named = wanted
    ? peers.find((peer) => peer.publicKey.toLowerCase() === wanted)
    : undefined;
  const fallback = named ?? peers[0];
  const source =
    chosen !== undefined && peers.some((peer) => peerId(peer) === chosen)
      ? chosen
      : fallback
        ? peerId(fallback)
        : "";
  const device = peers.find((peer) => peerId(peer) === source);
  const [deviceRecords, setDeviceRecords] = useState<LogRecord[]>([]);
  const [deviceError, setDeviceError] = useState("");
  const [deviceBusy, setDeviceBusy] = useState(false);
  const [deviceCursor, setDeviceCursor] = useState<string>();
  const [deviceNext, setDeviceNext] = useState<string>();
  const deviceRecordCount = useRef(0);

  const all = deviceRecords;

  // The Log Store filters server-side, so the supported filters travel with the
  // query instead of being applied to one page after the fact. Free text is
  // sent whole; field clauses stay local because the store has no such syntax.
  const deviceQuery = useMemo(() => parseQuery(text).text.join(" "), [text]);
  const deviceLevel =
    level === "DEBUG" ||
    level === "INFO" ||
    level === "WARN" ||
    level === "ERROR"
      ? level
      : undefined;

  useEffect(() => {
    setDeviceRecords([]);
    setDeviceCursor(undefined);
    setDeviceNext(undefined);
  }, [source, deviceQuery, deviceLevel, windowSeconds]);

  useEffect(() => {
    if (!device) {
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
      deviceQuery,
      deviceLevel,
      end - windowSeconds * 1000,
      end,
      deviceCursor,
      controller.signal,
    )
      .then((page) => {
        if (controller.signal.aborted) return;
        const offset =
          deviceCursor === undefined ? 0 : deviceRecordCount.current;
        const records = page.items.map((item, index) => ({
          id: offset + index,
          time: new Date(item.time_ms).toISOString(),
          level: item.level,
          message: item.message,
          peer_public_key: device.publicKey,
          fields: { ...item.fields, source: item.source, path: item.path },
          node: peerId(device),
          nodeName: peerLabel(device, undefined),
        }));
        setDeviceRecords((current) => {
          const next =
            deviceCursor === undefined ? records : [...current, ...records];
          deviceRecordCount.current = next.length;
          return next;
        });
        setDeviceNext(page.end.has_next ? page.end.next_cursor : undefined);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted)
          setDeviceError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setDeviceBusy(false);
      });
    return () => controller.abort();
  }, [device, deviceQuery, deviceLevel, windowSeconds, deviceCursor]);

  const query = useMemo(() => parseQuery(text), [text]);
  const cutoff = Date.now() - windowSeconds * 1000;
  const shown = useMemo(
    () =>
      all.filter((record) => {
        if (level !== "ALL" && record.level !== level) return false;
        if (cutoff > 0 && Date.parse(record.time) < cutoff) return false;
        return matches(record, query);
      }),
    [all, level, cutoff, query],
  );

  const errors = shown.filter((record) => record.level === "ERROR").length;
  const warnings = shown.filter((record) => record.level === "WARN").length;
  usePageView({
    page: "日志查询",
    query: text,
    level,
    window_seconds: windowSeconds,
    source_device: device?.publicKey,
    loaded: all.length,
    shown: shown.length,
    errors,
    warnings,
    records: shown.slice(0, 50).map((record) => ({
      time: record.time,
      level: record.level,
      summary: summarize(record),
      message: record.message,
      error: record.error,
    })),
  });
  const traced = selected?.fields?.request_id;

  return (
    <>
      <PageHeading
        eyebrow="LOG SEARCH"
        title="日志查询"
        description="查询设备的持久化日志，查看完整字段，并按 request_id 追踪请求。"
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
              onChange={(event) => setChosen(event.target.value)}
            >
              <option value="">选择设备</option>
              {peers.map((peer) => (
                <option key={peerId(peer)} value={peerId(peer)}>
                  设备 · {peerLabel(peer, undefined)}
                </option>
              ))}
            </Select>
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
            {device && (
              <span>
                Log Store · {peerLabel(device, undefined)} ·
                文本与级别由服务端过滤，key:value 子句仅在已加载的记录上生效
                {deviceBusy ? " · 查询中" : ""}
              </span>
            )}
            {device && deviceNext !== undefined && (
              <Button
                variant="outline"
                size="sm"
                disabled={deviceBusy}
                onClick={() => setDeviceCursor(deviceNext)}
              >
                加载更早的记录
              </Button>
            )}
            <span>
              命中 {shown.length} / {all.length} 条 · ERROR {errors} · WARN{" "}
              {warnings}
            </span>
            {(text !== "" || level !== "ALL") && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setText("");
                  setLevel("ALL");
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
                : "请选择设备查询持久化日志；没有设备时，请先添加关注设备。"
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
