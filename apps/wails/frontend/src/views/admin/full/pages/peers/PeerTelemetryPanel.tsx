import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  Activity,
  BatteryCharging,
  Cpu,
  MapPinned,
  Radio,
  RefreshCw,
  Route,
  Sigma,
} from "lucide-react";
import maplibregl from "maplibre-gl";
import type { StyleSpecification } from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";

import { expectData, toMessage } from "@/dashboard";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

import {
  aggregatePeerTelemetry,
  getPeerTelemetryLatest,
  queryPeerTelemetry,
  type PeerTelemetryAggregate,
  type PeerTelemetryAggregatePoint,
  type PeerTelemetryField,
  type PeerTelemetryPoint,
  type PeerTelemetryValue,
} from "@gizclaw/gizclaw/admin";

type WindowOption = {
  label: string;
  ms: number;
  stepMs: number;
  bucketMs: number;
};

type TelemetryState = {
  aggregate: PeerTelemetryAggregatePoint[];
  history: PeerTelemetryPoint[];
  latest: PeerTelemetryValue[];
  route: RoutePoint[];
};

type RoutePoint = {
  at: number;
  lat: number;
  lng: number;
};

const latestFields: PeerTelemetryField[] = [
  "battery.percent",
  "battery.charging",
  "battery.voltage_mv",
  "gnss.latitude",
  "gnss.longitude",
  "gnss.altitude_m",
  "gnss.accuracy_m",
  "network.rssi_dbm",
  "network.signal_level",
  "network.connected",
  "system.uptime_seconds",
  "system.free_memory_bytes",
  "system.temperature_c",
];

const fieldOptions: Array<{
  label: string;
  value: PeerTelemetryField;
  unit: string;
}> = [
  { label: "Battery percent", value: "battery.percent", unit: "%" },
  { label: "Battery voltage", value: "battery.voltage_mv", unit: "mV" },
  { label: "GNSS latitude", value: "gnss.latitude", unit: "" },
  { label: "GNSS longitude", value: "gnss.longitude", unit: "" },
  { label: "GNSS altitude", value: "gnss.altitude_m", unit: "m" },
  { label: "GNSS accuracy", value: "gnss.accuracy_m", unit: "m" },
  { label: "Network RSSI", value: "network.rssi_dbm", unit: "dBm" },
  { label: "Network signal", value: "network.signal_level", unit: "" },
  { label: "System uptime", value: "system.uptime_seconds", unit: "s" },
  { label: "Free memory", value: "system.free_memory_bytes", unit: "B" },
  { label: "Temperature", value: "system.temperature_c", unit: "C" },
];

const windowOptions: WindowOption[] = [
  {
    label: "6h",
    ms: 6 * 60 * 60 * 1000,
    stepMs: 2 * 60 * 1000,
    bucketMs: 30 * 60 * 1000,
  },
  {
    label: "24h",
    ms: 24 * 60 * 60 * 1000,
    stepMs: 10 * 60 * 1000,
    bucketMs: 2 * 60 * 60 * 1000,
  },
  {
    label: "7d",
    ms: 7 * 24 * 60 * 60 * 1000,
    stepMs: 60 * 60 * 1000,
    bucketMs: 12 * 60 * 60 * 1000,
  },
];

const aggregateOptions: PeerTelemetryAggregate[] = [
  "avg",
  "min",
  "max",
  "sum",
  "count",
  "last",
];

const offlineRouteMapStyle: StyleSpecification = {
  version: 8,
  sources: {},
  layers: [
    {
      id: "background",
      type: "background",
      paint: { "background-color": "#eef2f7" },
    },
  ],
};

export function PeerTelemetryPanel({
  publicKey,
}: {
  publicKey: string;
}): JSX.Element {
  const [field, setField] = useState<PeerTelemetryField>("battery.percent");
  const [windowMs, setWindowMs] = useState(windowOptions[1].ms);
  const [aggregate, setAggregate] = useState<PeerTelemetryAggregate>("avg");
  const [state, setState] = useState<TelemetryState>({
    aggregate: [],
    history: [],
    latest: [],
    route: [],
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const loadSequenceRef = useRef(0);

  const selectedWindow = useMemo(
    () =>
      windowOptions.find((item) => item.ms === windowMs) ?? windowOptions[1],
    [windowMs],
  );
  const selectedField = useMemo(
    () => fieldOptions.find((item) => item.value === field) ?? fieldOptions[0],
    [field],
  );

  const load = useCallback(async () => {
    const loadSequence = loadSequenceRef.current + 1;
    loadSequenceRef.current = loadSequence;
    const end = Date.now();
    const start = end - selectedWindow.ms;
    setLoading(true);
    setError("");
    try {
      const [latest, history, lat, lng] = await Promise.all([
        expectData(
          getPeerTelemetryLatest({
            path: { publicKey },
            query: { fields: latestFields.join(",") },
          }),
        ),
        expectData(
          queryPeerTelemetry({
            path: { publicKey },
            query: {
              field,
              start_time_ms: start,
              end_time_ms: end,
              step_ms: selectedWindow.stepMs,
              limit: 1000,
              order: "asc",
            },
          }),
        ),
        expectData(
          queryPeerTelemetry({
            path: { publicKey },
            query: {
              field: "gnss.latitude",
              start_time_ms: start,
              end_time_ms: end,
              step_ms: selectedWindow.stepMs,
              limit: 1000,
              order: "asc",
            },
          }),
        ),
        expectData(
          queryPeerTelemetry({
            path: { publicKey },
            query: {
              field: "gnss.longitude",
              start_time_ms: start,
              end_time_ms: end,
              step_ms: selectedWindow.stepMs,
              limit: 1000,
              order: "asc",
            },
          }),
        ),
      ]);
      let aggregatePoints: PeerTelemetryAggregatePoint[] = [];
      try {
        const aggregateData = await expectData(
          aggregatePeerTelemetry({
            path: { publicKey },
            query: {
              field,
              start_time_ms: start,
              end_time_ms: end,
              bucket_ms: selectedWindow.bucketMs,
              aggregate,
            },
          }),
        );
        aggregatePoints = aggregateData.points;
      } catch {
        aggregatePoints = [];
      }
      if (loadSequenceRef.current !== loadSequence) {
        return;
      }
      setState({
        aggregate: aggregatePoints,
        history: history.points,
        latest: latest.values,
        route: pairRoute(lat.points, lng.points),
      });
    } catch (nextError) {
      if (loadSequenceRef.current !== loadSequence) {
        return;
      }
      setError(toMessage(nextError));
      setState({ aggregate: [], history: [], latest: [], route: [] });
    } finally {
      if (loadSequenceRef.current === loadSequence) {
        setLoading(false);
      }
    }
  }, [aggregate, field, publicKey, selectedWindow]);

  useEffect(() => {
    void load();
  }, [load]);

  const latestByField = useMemo(
    () => new Map(state.latest.map((value) => [value.field, value])),
    [state.latest],
  );

  return (
    <div className="space-y-4">
      {error !== "" ? (
        <Alert variant="destructive">
          <Activity className="size-4" />
          <AlertTitle>Telemetry query failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-col gap-3 rounded-lg border bg-card p-3 md:flex-row md:items-center md:justify-between">
        <div className="grid gap-2 sm:grid-cols-[14rem_8rem_9rem]">
          <Select
            onValueChange={(value) => setField(value as PeerTelemetryField)}
            value={field}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {fieldOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            onValueChange={(value) => setWindowMs(Number(value))}
            value={String(windowMs)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {windowOptions.map((option) => (
                <SelectItem key={option.ms} value={String(option.ms)}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            onValueChange={(value) =>
              setAggregate(value as PeerTelemetryAggregate)
            }
            value={aggregate}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {aggregateOptions.map((option) => (
                <SelectItem key={option} value={option}>
                  {option}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          disabled={loading}
          onClick={() => void load()}
          size="sm"
          variant="outline"
        >
          <RefreshCw className="size-4" />
          Reload
        </Button>
      </div>

      {loading ? (
        <div className="grid gap-4 xl:grid-cols-[0.95fr_1.05fr]">
          <Skeleton className="h-72 w-full" />
          <Skeleton className="h-72 w-full" />
          <Skeleton className="h-80 w-full xl:col-span-2" />
        </div>
      ) : (
        <>
          <div className="grid gap-4 xl:grid-cols-[0.95fr_1.05fr]">
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Activity className="size-4" />
                  Latest
                </CardTitle>
                <CardDescription>
                  {formatLatestTime(state.latest)}
                </CardDescription>
              </CardHeader>
              <CardContent className="grid gap-3 sm:grid-cols-2">
                <LatestGroup
                  icon={<BatteryCharging className="size-4" />}
                  items={[
                    ["Percent", latestByField.get("battery.percent"), "%"],
                    ["Charging", latestByField.get("battery.charging"), ""],
                    ["Voltage", latestByField.get("battery.voltage_mv"), "mV"],
                  ]}
                  title="Battery"
                />
                <LatestGroup
                  icon={<MapPinned className="size-4" />}
                  items={[
                    ["Latitude", latestByField.get("gnss.latitude"), ""],
                    ["Longitude", latestByField.get("gnss.longitude"), ""],
                    ["Accuracy", latestByField.get("gnss.accuracy_m"), "m"],
                  ]}
                  title="GNSS"
                />
                <LatestGroup
                  icon={<Radio className="size-4" />}
                  items={[
                    ["RSSI", latestByField.get("network.rssi_dbm"), "dBm"],
                    ["Signal", latestByField.get("network.signal_level"), ""],
                    ["Connected", latestByField.get("network.connected"), ""],
                  ]}
                  title="Network"
                />
                <LatestGroup
                  icon={<Cpu className="size-4" />}
                  items={[
                    ["Uptime", latestByField.get("system.uptime_seconds"), "s"],
                    [
                      "Free memory",
                      latestByField.get("system.free_memory_bytes"),
                      "B",
                    ],
                    [
                      "Temperature",
                      latestByField.get("system.temperature_c"),
                      "C",
                    ],
                  ]}
                  title="System"
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Route className="size-4" />
                  Trajectory
                </CardTitle>
                <CardDescription>
                  {state.route.length === 0
                    ? "No GNSS points"
                    : `${state.route.length} points`}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <TelemetryRouteMap points={state.route} />
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-base">
                  {selectedField.label}
                </CardTitle>
                <CardDescription>
                  {state.history.length} sampled points
                </CardDescription>
              </CardHeader>
              <CardContent>
                <TelemetrySparkline
                  field={selectedField.value}
                  points={state.history}
                  unit={selectedField.unit}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <Sigma className="size-4" />
                  Aggregate
                </CardTitle>
                <CardDescription>
                  {aggregate} / {selectedWindow.label}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <AggregateTable
                  aggregate={aggregate}
                  field={selectedField.value}
                  points={state.aggregate}
                  unit={selectedField.unit}
                />
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}

function LatestGroup({
  icon,
  items,
  title,
}: {
  icon: ReactNode;
  items: Array<
    [label: string, value: PeerTelemetryValue | undefined, unit: string]
  >;
  title: string;
}): JSX.Element {
  return (
    <div className="rounded-lg border bg-muted/20 p-3">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          {icon}
          {title}
        </div>
        <Badge variant="outline">
          {items.filter(([, value]) => value !== undefined).length}
        </Badge>
      </div>
      <dl className="space-y-2">
        {items.map(([label, value, unit]) => (
          <div
            className="flex min-w-0 items-baseline justify-between gap-3"
            key={label}
          >
            <dt className="truncate text-xs text-muted-foreground">{label}</dt>
            <dd className="min-w-fit text-right font-mono text-sm">
              {formatTelemetryValue(value, unit)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

function TelemetrySparkline({
  field,
  points,
  unit,
}: {
  field: PeerTelemetryField;
  points: PeerTelemetryPoint[];
  unit: string;
}): JSX.Element {
  const path = useMemo(() => sparklinePath(points), [points]);
  const latest = points.at(-1);
  const min = minValue(points);
  const max = maxValue(points);
  if (points.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
        No points
      </div>
    );
  }
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-2">
        <MetricPill
          label="Latest"
          value={formatNumber(latest?.value, unit, field)}
        />
        <MetricPill label="Min" value={formatNumber(min, unit, field)} />
        <MetricPill label="Max" value={formatNumber(max, unit, field)} />
      </div>
      <svg
        className="h-56 w-full rounded-lg border bg-background"
        preserveAspectRatio="none"
        viewBox="0 0 100 100"
      >
        <path d={path.area} fill="rgba(16, 185, 129, 0.12)" />
        <path
          d={path.line}
          fill="none"
          stroke="rgb(13, 148, 136)"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="2"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
    </div>
  );
}

function MetricPill({
  label,
  value,
}: {
  label: string;
  value: string;
}): JSX.Element {
  return (
    <div className="rounded-lg border bg-muted/20 px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="truncate font-mono text-sm">{value}</div>
    </div>
  );
}

function TelemetryRouteMap({ points }: { points: RoutePoint[] }): JSX.Element {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [mapError, setMapError] = useState("");

  useEffect(() => {
    const container = containerRef.current;
    if (container === null || points.length === 0) {
      return;
    }
    if (!webGLAvailable()) {
      setMapError("WebGL unavailable");
      return;
    }
    setMapError("");
    let map: maplibregl.Map;
    try {
      map = new maplibregl.Map({
        container,
        style: offlineRouteMapStyle,
        center: [points[0].lng, points[0].lat],
        zoom: 12,
        attributionControl: false,
      });
    } catch {
      setMapError("Map unavailable");
      return;
    }
    map.addControl(
      new maplibregl.NavigationControl({ showCompass: false }),
      "top-right",
    );
    map.on("error", () => setMapError("Map rendering unavailable"));
    map.on("load", () => {
      const coordinates = points.map((point) => [point.lng, point.lat]);
      if (coordinates.length > 1) {
        map.addSource("telemetry-route", {
          type: "geojson",
          data: {
            type: "Feature",
            geometry: { type: "LineString", coordinates },
            properties: {},
          },
        });
        map.addLayer({
          id: "telemetry-route-line",
          type: "line",
          source: "telemetry-route",
          paint: {
            "line-color": "#0f766e",
            "line-width": 3,
            "line-opacity": 0.9,
          },
        });
      }
      map.addSource("telemetry-route-points", {
        type: "geojson",
        data: {
          type: "FeatureCollection",
          features: points.map((point) => ({
            type: "Feature",
            geometry: { type: "Point", coordinates: [point.lng, point.lat] },
            properties: { at: point.at },
          })),
        },
      });
      map.addLayer({
        id: "telemetry-route-points",
        type: "circle",
        source: "telemetry-route-points",
        paint: {
          "circle-color": "#f97316",
          "circle-radius": 4,
          "circle-stroke-color": "#ffffff",
          "circle-stroke-width": 1,
        },
      });
      if (points.length > 1) {
        const bounds = new maplibregl.LngLatBounds(
          [points[0].lng, points[0].lat],
          [points[0].lng, points[0].lat],
        );
        points.forEach((point) => bounds.extend([point.lng, point.lat]));
        map.fitBounds(bounds, { padding: 36, maxZoom: 15, duration: 0 });
      }
    });
    return () => map.remove();
  }, [points]);

  if (points.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
        No route
      </div>
    );
  }
  return (
    <div className="relative h-64 overflow-hidden rounded-lg border">
      <div className="h-full w-full" ref={containerRef} />
      {mapError !== "" ? (
        <div className="absolute inset-0 grid place-items-center bg-background/85 p-4 text-center text-sm text-muted-foreground">
          <div>
            <div className="font-medium text-foreground">{mapError}</div>
            <div className="mt-1 font-mono text-xs">
              {formatLatLng(points.at(-1))}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function AggregateTable({
  aggregate,
  field,
  points,
  unit,
}: {
  aggregate: PeerTelemetryAggregate;
  field: PeerTelemetryField;
  points: PeerTelemetryAggregatePoint[];
  unit: string;
}): JSX.Element {
  if (points.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
        No buckets
      </div>
    );
  }
  const valueUnit = aggregate === "count" ? "" : unit;
  const valueField = aggregate === "count" ? undefined : field;
  return (
    <div className="max-h-64 overflow-auto rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Bucket</TableHead>
            <TableHead className="text-right">Value</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {points
            .slice(-12)
            .reverse()
            .map((point) => (
              <TableRow key={point.bucket_start_time_ms}>
                <TableCell>
                  {formatDateTime(point.bucket_start_time_ms)}
                </TableCell>
                <TableCell className="text-right font-mono">
                  {formatNumber(point.value, valueUnit, valueField)}
                </TableCell>
              </TableRow>
            ))}
        </TableBody>
      </Table>
    </div>
  );
}

function pairRoute(
  latPoints: PeerTelemetryPoint[],
  lngPoints: PeerTelemetryPoint[],
): RoutePoint[] {
  const longitudes = new Map(
    lngPoints.map((point) => [point.observed_at_unix_ms, point.value]),
  );
  return latPoints
    .flatMap((latPoint) => {
      const lng = longitudes.get(latPoint.observed_at_unix_ms);
      if (lng === undefined || !validLatLng(latPoint.value, lng)) {
        return [];
      }
      return [{ at: latPoint.observed_at_unix_ms, lat: latPoint.value, lng }];
    })
    .slice(-500);
}

function validLatLng(lat: number, lng: number): boolean {
  return (
    Number.isFinite(lat) &&
    Number.isFinite(lng) &&
    lat >= -90 &&
    lat <= 90 &&
    lng >= -180 &&
    lng <= 180
  );
}

function sparklinePath(points: PeerTelemetryPoint[]): {
  area: string;
  line: string;
} {
  if (points.length === 0) {
    return { area: "", line: "" };
  }
  const min = minValue(points) ?? 0;
  const max = maxValue(points) ?? min + 1;
  const span = max === min ? 1 : max - min;
  const mapped = points.map((point, index) => {
    const x = points.length === 1 ? 50 : (index / (points.length - 1)) * 100;
    const y = 92 - ((point.value - min) / span) * 84;
    return [x, y] as const;
  });
  const line = mapped
    .map(
      ([x, y], index) =>
        `${index === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)}`,
    )
    .join(" ");
  const area = `${line} L ${mapped.at(-1)?.[0].toFixed(2) ?? "100"} 96 L ${mapped[0]?.[0].toFixed(2) ?? "0"} 96 Z`;
  return { area, line };
}

function minValue(points: PeerTelemetryPoint[]): number | undefined {
  return points.reduce<number | undefined>(
    (min, point) =>
      min === undefined || point.value < min ? point.value : min,
    undefined,
  );
}

function maxValue(points: PeerTelemetryPoint[]): number | undefined {
  return points.reduce<number | undefined>(
    (max, point) =>
      max === undefined || point.value > max ? point.value : max,
    undefined,
  );
}

function formatTelemetryValue(
  value: PeerTelemetryValue | undefined,
  unit: string,
): string {
  if (value === undefined) {
    return "-";
  }
  if (
    value.field === "battery.charging" ||
    value.field === "network.connected"
  ) {
    return value.value > 0 ? "Yes" : "No";
  }
  return formatNumber(value.value, unit, value.field);
}

function formatNumber(
  value: number | undefined,
  unit: string,
  field?: PeerTelemetryField,
): string {
  if (value === undefined || !Number.isFinite(value)) {
    return "-";
  }
  if (field !== undefined && isCoordinateField(field)) {
    return value.toFixed(6);
  }
  if (unit === "B") {
    return formatBytes(value);
  }
  const abs = Math.abs(value);
  const precision =
    abs >= 100 || Number.isInteger(value) ? 0 : abs >= 10 ? 1 : 2;
  const suffix = unit === "" ? "" : ` ${unit}`;
  return `${value.toFixed(precision)}${suffix}`;
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(index === 0 || size >= 10 ? 0 : 1)} ${units[index]}`;
}

function formatDateTime(ms: number | undefined): string {
  if (ms === undefined || !Number.isFinite(ms)) {
    return "-";
  }
  return new Date(ms).toLocaleString();
}

function formatLatestTime(values: PeerTelemetryValue[]): string {
  const latest = values.reduce<number | undefined>(
    (max, item) =>
      max === undefined || item.observed_at_unix_ms > max
        ? item.observed_at_unix_ms
        : max,
    undefined,
  );
  return latest === undefined
    ? "No latest values"
    : `Updated ${formatDateTime(latest)}`;
}

function formatLatLng(point: RoutePoint | undefined): string {
  if (point === undefined) {
    return "-";
  }
  return `${point.lat.toFixed(6)}, ${point.lng.toFixed(6)}`;
}

function isCoordinateField(field: PeerTelemetryField): boolean {
  return field === "gnss.latitude" || field === "gnss.longitude";
}

function webGLAvailable(): boolean {
  const canvas = document.createElement("canvas");
  return (
    canvas.getContext("webgl") !== null ||
    canvas.getContext("experimental-webgl") !== null
  );
}
