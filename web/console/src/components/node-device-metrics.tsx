import { useState } from "react";
import {
  Radio,
  Network,
  MessagesSquare,
  AudioLines,
  Smartphone,
} from "lucide-react";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Select } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  MonitorSummary,
  type SummaryMetric,
} from "@/components/monitor-summary";
import { NodeMetricChart } from "@/components/node-metric-chart";
import type { ConsoleServer } from "@/lib/config";
import type { ServerState } from "@/hooks/use-fleet";
import {
  countText,
  messageRates,
  rateText,
  transportTotals,
  type TrendMode,
} from "@/lib/node-metrics";

type NodeSource = { server: ConsoleServer; state: ServerState | undefined };
const peerNames = {
  device: "设备连接",
  upstream: "上游连接",
  admin: "管理员连接",
  unknown: "未分类连接",
};

/** Current values only use successful snapshots; aliases of one node count once. */
export function NodeDeviceMetrics({
  nodes,
  windowSeconds,
  single = false,
}: {
  nodes: NodeSource[];
  windowSeconds: number;
  single?: boolean;
}) {
  const [perMinute, setPerMinute] = useState(false);
  const [mode, setMode] = useState<TrendMode>("online");
  const byIdentity = new Map<string, NodeSource>();
  for (const item of nodes) {
    const key = item.state?.snapshot?.public_key ?? item.server.url;
    if (!byIdentity.has(key) || item.state?.status === "online")
      byIdentity.set(key, item);
  }
  const sources = [...byIdentity.values()];
  const live = sources.filter(({ state }) => state?.status === "online");
  const supported = live.flatMap(({ server, state }) =>
    state?.snapshot?.device_metrics
      ? [{ server, state, metrics: state.snapshot.device_metrics }]
      : [],
  );
  const roleOf = (item: NodeSource) =>
    item.state?.snapshot?.role ?? item.server.role;
  const serverNodes = sources.filter((item) => roleOf(item) === "server");
  const edgeNodes = sources.filter((item) => roleOf(item) === "edge");
  const online = (selected: NodeSource[]) => {
    const values = selected.flatMap(({ state }) =>
      state?.status === "online" &&
      state.snapshot?.device_metrics?.online_devices !== undefined
        ? [state.snapshot.device_metrics.online_devices]
        : [],
    );
    return {
      value: values.length ? countText(values.reduce((a, b) => a + b, 0)) : "—",
      coverage: `${values.length}/${selected.length}`,
    };
  };
  const totals = supported.map(({ metrics }) => transportTotals(metrics));
  const sum = (key: keyof ReturnType<typeof transportTotals>) =>
    totals.reduce((total, item) => total + item[key], 0);
  const rates = supported.flatMap(({ state }) =>
    state.deviceRates ? [state.deviceRates] : [],
  );
  const pair = (rx: "messagesRx" | "rtpRx", tx: "messagesTx" | "rtpTx") =>
    rates.length
      ? `${rateText(
          rates.reduce((total, item) => total + item[rx], 0),
          perMinute,
        )} · ${rateText(
          rates.reduce((total, item) => total + item[tx], 0),
          perMinute,
        )}`
      : "—";
  const coverage = `已覆盖 ${supported.length}/${sources.length} 节点`;
  const rateNote = rates.length
    ? `收 / 发 · ${rates.length}/${sources.length} 节点有效${perMinute ? " · 采样平均速率 × 60" : " · 最近采样间隔的平均值"}`
    : supported.length === 0
      ? "等待节点提供消息计数"
      : "等待两次连续有效采样";
  const one = sources[0];
  const oneMetrics =
    single && one?.state?.status === "online"
      ? one.state.snapshot?.device_metrics
      : undefined;
  const unsupported = live.filter(
    ({ state }) => !state?.snapshot?.device_metrics,
  );

  const hasHistory = sources.some(({ state }) =>
    state?.deviceSamples?.some(
      (sample) => sample.onlineDevices !== undefined || sample.rates,
    ),
  );
  if (supported.length === 0 && !hasHistory) {
    return (
      <NodeMetricsEmpty
        total={sources.length}
        connected={live.length}
        unsupported={unsupported.length}
        failed={sources.filter(({ state }) => state?.status === "error").length}
      />
    );
  }
  const summary: SummaryMetric[] = single
    ? [
        {
          label: "在线设备",
          value: online(sources).value,
          note: `${online(sources).coverage} 节点已上报 · 按设备身份去重`,
          icon: <Smartphone size={16} />,
        },
      ]
    : [
        {
          label: "在线设备 · Server",
          value: online(serverNodes).value,
          note: `${online(serverNodes).coverage} Server 已上报`,
          icon: <Smartphone size={16} />,
        },
        {
          label: "接入设备 · Edge",
          value: online(edgeNodes).value,
          note: `${online(edgeNodes).coverage} Edge 已上报 · 与 Server 分开统计`,
          icon: <Smartphone size={16} />,
        },
      ];
  summary.push(
    {
      label: "RTP 通道 · 收 / 发",
      value: supported.length
        ? `${countText(sum("rtpRxTracks"))} / ${countText(sum("rtpTxTracks"))}`
        : "—",
      note: "当前已协商的通道",
      icon: <Radio size={16} />,
    },
    {
      label: "DataChannel 通道",
      value: supported.length
        ? countText(
            sum("packetChannels") +
              sum("serviceChannels") +
              sum("nativeChannels"),
          )
        : "—",
      note: supported.length
        ? `packet ${countText(sum("packetChannels"))} · service ${countText(sum("serviceChannels"))} · native ${countText(sum("nativeChannels"))}`
        : "等待有效快照",
      icon: <Network size={16} />,
    },
    {
      label: "DataChannel 消息 · 收 / 发",
      value: pair("messagesRx", "messagesTx"),
      compact: true,
      note: rateNote,
      icon: <MessagesSquare size={16} />,
    },
    {
      label: "RTP 包 · 收 / 发",
      value: pair("rtpRx", "rtpTx"),
      compact: true,
      note: rateNote,
      icon: <AudioLines size={16} />,
    },
  );
  return (
    <section aria-label="设备与通道监控" className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <Badge
            variant={
              supported.length === sources.length && sources.length > 0
                ? "success"
                : "outline"
            }
          >
            {coverage}
          </Badge>
          {unsupported.length > 0 && (
            <span className="text-xs text-muted-foreground">
              {unsupported.length} 个节点等待升级
            </span>
          )}
          {sources.length > live.length && (
            <span className="text-xs text-muted-foreground">
              {sources.length - live.length} 个节点暂无有效快照
            </span>
          )}
        </div>
        <Select
          aria-label="消息速率单位"
          value={perMinute ? "min" : "s"}
          onChange={(event) => setPerMinute(event.target.value === "min")}
        >
          <option value="s">每秒平均速率</option>
          <option value="min">每分钟数量估算</option>
        </Select>
      </div>
      <MonitorSummary metrics={summary} />
      <Card>
        <CardHeader>
          <CardTitle className="text-[15px]">设备与消息趋势</CardTitle>
          <CardDescription className="text-[11px]">
            最近 {windowSeconds / 60} 分钟 · 本次浏览器会话
          </CardDescription>
          <CardAction>
            <Select
              aria-label="设备趋势指标"
              value={mode}
              onChange={(event) => {
                const next = event.target.value;
                if (next === "online" || next === "messages" || next === "rtp")
                  setMode(next);
              }}
            >
              <option value="online">在线设备</option>
              <option value="messages">DataChannel 消息</option>
              <option value="rtp">RTP 包</option>
            </Select>
          </CardAction>
        </CardHeader>
        <CardContent>
          <NodeMetricChart
            sources={sources.map((item) => ({
              role: roleOf(item),
              samples: item.state?.deviceSamples ?? [],
            }))}
            mode={mode}
            windowSeconds={windowSeconds}
            perMinute={perMinute}
            single={single}
          />
        </CardContent>
      </Card>
      {single && oneMetrics && (
        <Card>
          <CardHeader>
            <CardTitle>连接类型明细</CardTitle>
            <CardDescription>
              Server 经 Edge 接入的设备音频承载在上游 DataChannel，不能计为
              Server 的设备侧 RTP。
            </CardDescription>
          </CardHeader>
          <CardContent className="overflow-x-auto px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="pl-5">类型</TableHead>
                  <TableHead>RTP 收 / 发</TableHead>
                  <TableHead>DataChannel</TableHead>
                  <TableHead>消息 收 / 发</TableHead>
                  <TableHead>RTP 包 收 / 发</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {oneMetrics.transports.map((item) => {
                  const counts = transportTotals(oneMetrics, item.peer_kind);
                  const measured = one?.state?.snapshot
                    ? messageRates(
                        one.state.previousSnapshot,
                        one.state.snapshot,
                        item.peer_kind,
                      )
                    : undefined;
                  return (
                    <TableRow key={item.peer_kind}>
                      <TableCell className="pl-5">
                        {peerNames[item.peer_kind]}
                      </TableCell>
                      <TableCell>
                        {counts.rtpRxTracks} / {counts.rtpTxTracks}
                      </TableCell>
                      <TableCell>
                        {counts.packetChannels +
                          counts.serviceChannels +
                          counts.nativeChannels}
                      </TableCell>
                      <TableCell>
                        {rateText(measured?.messagesRx, perMinute)} ·{" "}
                        {rateText(measured?.messagesTx, perMinute)}
                      </TableCell>
                      <TableCell>
                        {rateText(measured?.rtpRx, perMinute)} ·{" "}
                        {rateText(measured?.rtpTx, perMinute)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
      <details className="text-xs text-muted-foreground">
        <summary className="w-fit cursor-pointer hover:text-foreground">
          统计口径与采样说明
        </summary>
        <div className="mt-3 max-w-3xl space-y-2 leading-relaxed">
          <p>
            Server 与 Edge
            从不同位置观察设备，分别统计，不相加作为全局设备数。通道和消息包含设备、上游及其他连接；节点详情可按连接类型查看。
          </p>
          <p>
            消息速率来自连续采样的计数差，每分钟数量为平均速率乘以 60
            的估算。初次采样、重启和读取失败时留空，缺失节点不补为零。趋势仅保留本次浏览器会话的数据。
          </p>
        </div>
      </details>
    </section>
  );
}

function NodeMetricsEmpty({
  total,
  connected,
  unsupported,
  failed,
}: {
  total: number;
  connected: number;
  unsupported: number;
  failed: number;
}) {
  const title =
    unsupported > 0
      ? "设备指标尚未接入"
      : failed > 0
        ? "暂时无法读取设备指标"
        : "正在等待设备指标";
  const description =
    unsupported > 0
      ? `${unsupported} 个在线节点的当前版本尚未提供设备指标。升级对应节点后，这里会自动开始采集。`
      : failed > 0
        ? "当前节点连接不可用。连接恢复后，控制台会自动继续采集。"
        : "连接节点并完成首次采样后，这里会展示设备与通道数据。";
  return (
    <section aria-label="设备与通道监控">
      <Card className="gap-0 overflow-hidden p-0">
        <div className="grid md:grid-cols-[minmax(0,1fr)_240px]">
          <div className="flex flex-col justify-center px-6 py-6 sm:px-8 sm:py-8">
            <div className="mb-5 flex size-11 items-center justify-center rounded-xl border border-border bg-secondary/60 text-muted-foreground">
              <Radio size={21} />
            </div>
            <h2 className="text-[22px] font-semibold tracking-tight">
              {title}
            </h2>
            <p className="mt-3 max-w-xl text-[13px] leading-7 text-muted-foreground">
              {description}
            </p>
            <div
              className="mt-6 flex flex-wrap gap-x-5 gap-y-3 text-[11px] text-muted-foreground"
              aria-label="采集指标"
            >
              <span className="inline-flex items-center gap-1.5">
                <Smartphone size={13} />
                在线设备
              </span>
              <span className="inline-flex items-center gap-1.5">
                <Radio size={13} />
                RTP 通道
              </span>
              <span className="inline-flex items-center gap-1.5">
                <Network size={13} />
                DataChannel
              </span>
              <span className="inline-flex items-center gap-1.5">
                <MessagesSquare size={13} />
                消息速率
              </span>
            </div>
          </div>
          <div className="grid grid-cols-2 items-center gap-x-6 border-t border-border bg-secondary/35 px-6 py-6 md:block md:border-l md:border-t-0 md:py-7">
            <div>
              <h3 className="text-xs font-medium text-muted-foreground">
                指标覆盖
              </h3>
              <p className="mt-4 text-[36px] font-medium leading-none tracking-tight tabular-nums">
                0
                <span className="ml-2 text-lg font-normal text-muted-foreground/60">
                  / {total}
                </span>
              </p>
              <p className="mt-2 text-[11px] text-muted-foreground">
                个节点已提供设备指标
              </p>
            </div>
            <div
              className="my-5 hidden h-1 rounded-full bg-border md:block"
              aria-hidden="true"
            />
            <dl className="space-y-3 text-xs">
              <div className="flex justify-between gap-3">
                <dt className="text-muted-foreground">可连接节点</dt>
                <dd className="tabular-nums">{connected}</dd>
              </div>
              <div className="flex justify-between gap-3">
                <dt className="text-muted-foreground">等待升级</dt>
                <dd className="tabular-nums">{unsupported}</dd>
              </div>
              {failed > 0 && (
                <div className="flex justify-between gap-3">
                  <dt className="text-muted-foreground">读取失败</dt>
                  <dd className="tabular-nums text-destructive">{failed}</dd>
                </div>
              )}
            </dl>
          </div>
        </div>
      </Card>
    </section>
  );
}
