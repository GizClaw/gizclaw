import { Fragment, useEffect, useState } from "react";
import { ChartSpline } from "lucide-react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { nodeErrorMessage } from "@/lib/api";
import {
  loadTelemetry,
  loadTelemetryRange,
  telemetryFields,
  type TelemetryValue,
  type WatchedPeer,
} from "@/lib/peers";
import { bytes } from "@/lib/format";

const LABELS: Record<string, [string, string]> = {
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

/** Each field carries its own observation time; nothing is interpolated. */
export function PeerTelemetry({ peer }: { peer: WatchedPeer }) {
  const [values, setValues] = useState<TelemetryValue[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState<(typeof telemetryFields)[number]>();
  const [rangeHours, setRangeHours] = useState(6);
  const [series, setSeries] = useState<{ time: number; value: number }[]>([]);
  const [stepMs, setStepMs] = useState(0);
  const [seriesBusy, setSeriesBusy] = useState(false);
  const [seriesError, setSeriesError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      try {
        const result = await loadTelemetry(
          peer.endpoint,
          peer.publicKey,
          controller.signal,
        );
        if (controller.signal.aborted) return;
        setValues(result);
        setError("");
      } catch (failure) {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
          timer = setTimeout(() => void poll(), 10000);
        }
      }
    };
    void poll();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [peer.endpoint, peer.publicKey]);

  useEffect(() => {
    if (open === undefined) return;
    const controller = new AbortController();
    setSeriesBusy(true);
    setSeriesError("");
    setSeries([]);
    const end = Date.now();
    loadTelemetryRange(
      peer.endpoint,
      peer.publicKey,
      open,
      end - rangeHours * 3600000,
      end,
      controller.signal,
    )
      .then((range) => {
        if (controller.signal.aborted) return;
        setStepMs(range.step_ms);
        setSeries(
          range.points.map((point) => ({
            time: point.observed_at_unix_ms,
            value: point.value,
          })),
        );
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted)
          setSeriesError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setSeriesBusy(false);
      });
    return () => controller.abort();
  }, [peer.endpoint, peer.publicKey, open, rangeHours]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Telemetry</CardTitle>
        <CardDescription>
          每 10
          秒读取一次最新值，字段各自保留设备上报的采样时间；已经积累到两次以上采样的字段可以展开趋势。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 px-0">
        {error && (
          <div className="px-5">
            <Alert>
              <AlertTitle>Telemetry 读取失败</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        )}
        <div className="mx-5 divide-y divide-border overflow-hidden rounded-md border border-border">
          {/* An ordinary first row names the columns; no separate header style. */}
          <div className="grid grid-cols-[7rem_14rem_6rem_minmax(0,1fr)_5rem] items-baseline gap-3 px-3 py-1.5 text-[10px] text-muted-foreground">
            <span className="whitespace-nowrap">采样时间</span>
            <span>字段</span>
            <span>指标</span>
            <span>值</span>
            <span className="text-right">趋势</span>
          </div>
          {telemetryFields.map((field) => {
            const item = values.find((value) => value.field === field);
            const stale =
              item !== undefined &&
              Date.now() - item.observed_at_unix_ms > 60000;
            const unit =
              item && field !== "system.free_memory_bytes"
                ? (LABELS[field]?.[1] ?? "")
                : "";
            return (
              <Fragment key={field}>
                <div className="grid grid-cols-[7rem_14rem_6rem_minmax(0,1fr)_5rem] items-baseline gap-3 px-3 py-1.5 hover:bg-secondary/60">
                  <time
                    className={cn(
                      "font-mono text-[10px] whitespace-nowrap text-muted-foreground",
                      stale && "text-warning",
                    )}
                  >
                    {item
                      ? `${new Date(item.observed_at_unix_ms).toLocaleTimeString()}${stale ? " 已过期" : ""}`
                      : "尚未收到"}
                  </time>
                  <span className="truncate font-mono text-[11px]">
                    {field}
                  </span>
                  <span className="truncate text-[11px] text-muted-foreground">
                    {LABELS[field]?.[0] ?? ""}
                  </span>
                  <span className="font-mono text-xs tabular-nums">
                    {item ? (
                      <>
                        {valueText(field, item.value)}
                        {unit && (
                          <span className="ml-1 text-[10px] text-muted-foreground">
                            {unit}
                          </span>
                        )}
                      </>
                    ) : loading ? (
                      "…"
                    ) : (
                      <span className="text-muted-foreground">未上报</span>
                    )}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 justify-end gap-1 px-1.5 text-[10px]"
                    aria-label={`${field} 趋势`}
                    onClick={() =>
                      setOpen((current) =>
                        current === field ? undefined : field,
                      )
                    }
                  >
                    <ChartSpline size={11} />
                    趋势
                  </Button>
                </div>
              </Fragment>
            );
          })}
        </div>
      </CardContent>
      <Dialog
        open={open !== undefined}
        onOpenChange={(next) => {
          if (!next) setOpen(undefined);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {open ? (LABELS[open]?.[0] ?? open) : ""} 趋势
            </DialogTitle>
            <DialogDescription>
              {open} · Server 存储的历史采样，按所选区间查询。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
            <span>历史区间</span>
            {[1, 6, 24, 72].map((hours) => (
              <Button
                key={hours}
                variant={rangeHours === hours ? "secondary" : "ghost"}
                size="sm"
                className="h-6 px-2 text-[10px]"
                onClick={() => setRangeHours(hours)}
              >
                {hours} 小时
              </Button>
            ))}
            {seriesBusy && <span>查询中…</span>}
            {seriesError !== "" && (
              <span className="text-destructive">{seriesError}</span>
            )}
          </div>
          {open !== undefined && series.length > 0 ? (
            <FieldChart field={open} series={series} stepMs={stepMs} />
          ) : (
            <p className="py-10 text-center text-xs text-muted-foreground">
              {seriesBusy ? "正在读取历史采样…" : "这段时间没有存储的采样。"}
            </p>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function FieldChart({
  field,
  series,
  stepMs,
}: {
  field: string;
  series: { time: number; value: number }[];
  stepMs: number;
}) {
  return (
    <div className="rounded-md border border-border p-3">
      <p className="mb-2 text-[11px] text-muted-foreground">
        {series.length} 个采样点 · {field}
        {stepMs > 0
          ? ` · 步长 ${stepMs >= 60000 ? `${Math.round(stepMs / 60000)} 分钟` : `${Math.round(stepMs / 1000)} 秒`}（区间越长步长越大）`
          : ""}
      </p>
      <div style={{ height: 180 }}>
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart
            data={series}
            margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
          >
            <CartesianGrid
              stroke="var(--border)"
              strokeDasharray="3 4"
              vertical={false}
            />
            <XAxis
              dataKey="time"
              tickFormatter={(value) => new Date(value).toLocaleTimeString()}
              tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
              minTickGap={60}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              domain={["auto", "auto"]}
              width={60}
              tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip
              labelFormatter={(value) =>
                new Date(Number(value)).toLocaleString()
              }
              formatter={(value) => valueText(field, Number(value))}
            />
            <Area
              name={LABELS[field]?.[0] ?? field}
              type="stepAfter"
              dataKey="value"
              dot={series.length < 12}
              stroke="var(--chart-rx)"
              fill="var(--chart-rx)"
              fillOpacity={0.12}
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
