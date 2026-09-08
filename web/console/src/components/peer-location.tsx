import { useEffect, useRef, useState } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { nodeErrorMessage } from "@/lib/api";
import { loadTelemetryRange, type WatchedPeer } from "@/lib/peers";

type Fix = { time: number; lat: number; lon: number };
const RANGES = [1, 6, 24, 72];

/**
 * Reported GNSS fixes only. Latitude and longitude are separate stored series,
 * so a fix exists only where both were sampled in the same step.
 */
function zip(
  latitudes: { time: number; value: number }[],
  longitudes: { time: number; value: number }[],
): Fix[] {
  const byTime = new Map(longitudes.map((point) => [point.time, point.value]));
  const fixes: Fix[] = [];
  for (const point of latitudes) {
    const lon = byTime.get(point.time);
    if (lon === undefined) continue;
    if (Math.abs(point.value) > 90 || Math.abs(lon) > 180) continue;
    fixes.push({ time: point.time, lat: point.value, lon });
  }
  return fixes.sort((a, b) => a.time - b.time);
}

export function PeerLocation({ peer }: { peer: WatchedPeer }) {
  const [hours, setHours] = useState(6);
  const [fixes, setFixes] = useState<Fix[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(true);
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

  useEffect(() => {
    const controller = new AbortController();
    setBusy(true);
    setError("");
    const end = Date.now();
    const start = end - hours * 3600000;
    Promise.all([
      loadTelemetryRange(
        peer.endpoint,
        peer.publicKey,
        "gnss.latitude",
        start,
        end,
        controller.signal,
      ),
      loadTelemetryRange(
        peer.endpoint,
        peer.publicKey,
        "gnss.longitude",
        start,
        end,
        controller.signal,
      ),
    ])
      .then(([latitude, longitude]) => {
        if (controller.signal.aborted) return;
        setFixes(
          zip(
            latitude.points.map((point) => ({
              time: point.observed_at_unix_ms,
              value: point.value,
            })),
            longitude.points.map((point) => ({
              time: point.observed_at_unix_ms,
              value: point.value,
            })),
          ),
        );
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(nodeErrorMessage(failure));
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [peer.endpoint, peer.publicKey, hours]);

  const last = fixes.at(-1);
  return (
    <Card>
      <CardHeader>
        <CardTitle>设备轨迹</CardTitle>
        <CardDescription>
          设备上报的 GNSS 经纬度按时间连成轨迹，不是浏览器定位。地图瓦片来自
          OpenStreetMap，加载时该服务会收到坐标。
        </CardDescription>
        <CardAction>
          <div className="flex items-center gap-1">
            {RANGES.map((value) => (
              <Button
                key={value}
                variant={hours === value ? "secondary" : "ghost"}
                size="sm"
                className="h-6 px-2 text-[10px]"
                onClick={() => setHours(value)}
              >
                {value} 小时
              </Button>
            ))}
          </div>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {error && (
          <Alert>
            <AlertTitle>读取定位失败</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {fixes.length === 0 ? (
          <p className="py-16 text-center text-xs text-muted-foreground">
            {busy ? "正在读取定位采样…" : "这段时间没有上报有效坐标。"}
          </p>
        ) : !online ? (
          <p className="py-16 text-center text-xs text-muted-foreground">
            当前浏览器离线，地图不可用；最后坐标 {last?.lat.toFixed(6)},{" "}
            {last?.lon.toFixed(6)}。
          </p>
        ) : (
          <TrackMap fixes={fixes} />
        )}
        <dl className="grid gap-x-10 gap-y-2 sm:grid-cols-2 xl:grid-cols-4">
          <Field label="轨迹点" value={busy ? "…" : String(fixes.length)} />
          <Field
            label="最后坐标"
            value={
              last ? `${last.lat.toFixed(6)}, ${last.lon.toFixed(6)}` : "—"
            }
          />
          <Field
            label="采样时间"
            value={last ? new Date(last.time).toLocaleString() : "—"}
          />
          <Field
            label="时间跨度"
            value={
              fixes.length > 1
                ? `${Math.round((fixes[fixes.length - 1].time - fixes[0].time) / 60000)} 分钟`
                : "—"
            }
          />
        </dl>
      </CardContent>
    </Card>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 text-xs">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className="m-0 min-w-0 truncate text-right font-mono">{value}</dd>
    </div>
  );
}

function TrackMap({ fixes }: { fixes: Fix[] }) {
  const container = useRef<HTMLDivElement>(null);
  const map = useRef<L.Map>(null);
  const layer = useRef<L.LayerGroup>(null);

  useEffect(() => {
    if (!container.current || map.current) return;
    const instance = L.map(container.current, { attributionControl: true });
    L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
      maxZoom: 19,
      attribution: "© OpenStreetMap contributors",
    }).addTo(instance);
    map.current = instance;
    layer.current = L.layerGroup().addTo(instance);
    // The map fills the remaining viewport, so Leaflet must re-measure whenever
    // the container is resized rather than only when the track changes.
    const observer = new ResizeObserver(() => instance.invalidateSize());
    observer.observe(container.current);
    return () => {
      observer.disconnect();
      instance.remove();
      map.current = null;
      layer.current = null;
    };
  }, []);

  useEffect(() => {
    const instance = map.current;
    const group = layer.current;
    if (!instance || !group) return;
    group.clearLayers();
    const path = fixes.map((fix) => [fix.lat, fix.lon] as [number, number]);
    if (path.length > 1) {
      L.polyline(path, { color: "#cc785c", weight: 3, opacity: 0.85 }).addTo(
        group,
      );
      L.circleMarker(path[0], {
        radius: 5,
        color: "#438e80",
        fillColor: "#438e80",
        fillOpacity: 1,
      })
        .bindTooltip(`起点 ${new Date(fixes[0].time).toLocaleString()}`)
        .addTo(group);
    }
    const last = fixes[fixes.length - 1];
    L.circleMarker(path[path.length - 1], {
      radius: 6,
      color: "#cc785c",
      fillColor: "#cc785c",
      fillOpacity: 1,
    })
      .bindTooltip(
        `最后上报 ${new Date(last.time).toLocaleString()} · ${last.lat.toFixed(6)}, ${last.lon.toFixed(6)}`,
      )
      .addTo(group);
    if (path.length > 1) {
      instance.fitBounds(L.latLngBounds(path), { padding: [24, 24] });
    } else {
      instance.setView(path[0], 15);
    }
    instance.invalidateSize();
  }, [fixes]);

  // Leaflet adds its own classes to the element it mounts into, so it gets a
  // plain inner div that React never rewrites.
  return (
    <div className="h-[calc(100vh-33rem)] max-h-[760px] min-h-72 w-full overflow-hidden rounded-md border border-border">
      <div ref={container} style={{ height: "100%", width: "100%" }} />
    </div>
  );
}
