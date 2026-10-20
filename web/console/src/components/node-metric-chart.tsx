import { Activity } from "lucide-react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  countText,
  metricTrend,
  type DeviceSample,
  type TrendMode,
} from "@/lib/node-metrics";

export function NodeMetricChart({
  sources,
  mode,
  windowSeconds,
  perMinute,
  single,
}: {
  sources: { role: string; samples: DeviceSample[] }[];
  mode: TrendMode;
  windowSeconds: number;
  perMinute: boolean;
  single: boolean;
}) {
  const points = metricTrend(sources, mode, windowSeconds);
  const keys =
    mode === "online"
      ? single
        ? ["online"]
        : ["server", "edge"]
      : ["rx", "tx"];
  const data = points.map((point) => ({
    ...point,
    rx: point.rx === null ? null : point.rx * (perMinute ? 60 : 1),
    tx: point.tx === null ? null : point.tx * (perMinute ? 60 : 1),
  }));
  if (
    points.filter((point) =>
      mode === "online"
        ? single
          ? point.online !== null
          : point.server !== null || point.edge !== null
        : point.rx !== null || point.tx !== null,
    ).length < 2
  ) {
    return (
      <div
        className="flex h-52 flex-col items-center justify-center gap-3 text-xs text-muted-foreground"
        role="status"
      >
        <Activity size={26} />
        <span>等待至少两次完整的设备指标采样</span>
        <span>不支持、断线与缺失数据留空</span>
      </div>
    );
  }
  const names: Record<string, string> = {
    online: "在线设备",
    server: "Server 在线设备",
    edge: "Edge 接入设备",
    rx: "接收",
    tx: "发送",
  };
  const suffix = mode === "online" ? " 台" : perMinute ? "/min（估算）" : "/s";
  return (
    <div
      className="h-56"
      role="img"
      aria-label={
        mode === "online"
          ? "在线设备趋势"
          : mode === "rtp"
            ? "RTP 包速率趋势"
            : "DataChannel 消息速率趋势"
      }
    >
      <ResponsiveContainer width="100%" height="100%">
        <LineChart
          data={data}
          margin={{ top: 8, right: 12, left: 4, bottom: 0 }}
        >
          <CartesianGrid
            stroke="var(--border)"
            strokeDasharray="3 4"
            vertical={false}
          />
          <XAxis
            dataKey="time"
            minTickGap={70}
            tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
            axisLine={false}
            tickLine={false}
          />
          <YAxis
            width={70}
            allowDecimals={mode !== "online"}
            tickFormatter={(value) => countText(Number(value))}
            tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip
            formatter={(value) => `${countText(Number(value))}${suffix}`}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {keys.map((key, index) => (
            <Line
              key={key}
              dataKey={key}
              name={names[key]}
              stroke={index === 0 ? "var(--chart-rx)" : "var(--chart-tx)"}
              type={mode === "online" ? "stepAfter" : "linear"}
              dot={false}
              connectNulls={false}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
