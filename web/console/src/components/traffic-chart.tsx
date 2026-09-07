import { Activity } from "lucide-react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { bytes, type Sample } from "@/lib/format";

/** Renders measured samples only; an empty window stays visibly empty. */
export function TrafficChart({
  samples,
  windowSeconds,
  height = 220,
  emptyMessage,
}: {
  samples: Sample[];
  windowSeconds: number;
  height?: number;
  emptyMessage: string;
}) {
  const last = samples.at(-1)?.timestamp ?? 0;
  const shown = samples.filter(
    (sample) => sample.timestamp >= last - windowSeconds * 1000,
  );
  if (shown.length < 2) {
    return (
      <div
        className="flex flex-col items-center justify-center gap-3 rounded-md text-muted-foreground/70"
        style={{ height }}
      >
        <Activity size={30} />
        <strong className="text-xs font-normal">{emptyMessage}</strong>
        <span className="text-[10px]">这里仅展示实际测量的数据</span>
      </div>
    );
  }
  return (
    <div style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart
          data={shown}
          margin={{ top: 8, right: 8, left: 4, bottom: 0 }}
        >
          <CartesianGrid
            stroke="var(--border)"
            strokeDasharray="3 4"
            vertical={false}
          />
          <XAxis
            dataKey="time"
            tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
            minTickGap={70}
            axisLine={false}
            tickLine={false}
          />
          <YAxis
            width={80}
            tickFormatter={(value) => `${bytes(Number(value))}/s`}
            tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip formatter={(value) => `${bytes(Number(value))}/s`} />
          <Area
            name="接收"
            type="monotone"
            dataKey="rx"
            stroke="var(--chart-rx)"
            fill="var(--chart-rx)"
            fillOpacity={0.1}
            isAnimationActive={false}
          />
          <Area
            name="发送"
            type="monotone"
            dataKey="tx"
            stroke="var(--chart-tx)"
            fill="var(--chart-tx)"
            fillOpacity={0.08}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

export function TrafficLegend() {
  return (
    <span className="flex items-center gap-2 text-[10px] text-muted-foreground">
      <i className="size-1.5 rounded-full bg-chart-rx" /> 接收
      <i className="ml-2 size-1.5 rounded-full bg-chart-tx" /> 发送
    </span>
  );
}
