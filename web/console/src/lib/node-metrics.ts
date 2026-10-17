import type { NodeDeviceMetrics, NodeSnapshot } from "@/lib/api";

export type TransportTotals = {
  rtpRxTracks: number;
  rtpTxTracks: number;
  packetChannels: number;
  serviceChannels: number;
  nativeChannels: number;
  rtpRx: number;
  rtpTx: number;
  messagesRx: number;
  messagesTx: number;
};
export type MessageRates = Pick<
  TransportTotals,
  "rtpRx" | "rtpTx" | "messagesRx" | "messagesTx"
>;
export type DeviceSample = {
  timestamp: number;
  onlineDevices?: number;
  rates?: MessageRates;
};

export type TrendMode = "online" | "messages" | "rtp";
type TrendSource = { role: string; samples: DeviceSample[] };

/** Missing nodes make a bucket incomplete; never replace missing readings with zero. */
export function metricTrend(
  sources: TrendSource[],
  mode: TrendMode,
  windowSeconds: number,
) {
  const buckets = new Map<number, Map<number, DeviceSample>>();
  sources.forEach((source, index) => {
    for (const sample of source.samples) {
      const time = Math.floor(sample.timestamp / 5000) * 5000;
      const bucket = buckets.get(time) ?? new Map<number, DeviceSample>();
      bucket.set(index, sample);
      buckets.set(time, bucket);
    }
  });
  const recorded = [...buckets.keys()].sort((a, b) => a - b);
  for (let i = 1; i < recorded.length; i++) {
    if (recorded[i] - recorded[i - 1] > 10000) {
      buckets.set(recorded[i - 1] + 5000, new Map());
    }
  }
  const last = Math.max(0, ...buckets.keys());
  return [...buckets]
    .filter(([time]) => time >= last - windowSeconds * 1000)
    .sort(([a], [b]) => a - b)
    .map(([timestamp, samples]) => {
      const sum = (role?: string, rate?: keyof MessageRates) => {
        const selected = sources
          .map((source, index) => ({ source, index }))
          .filter(({ source }) => !role || source.role === role);
        if (selected.length === 0) return null;
        const values = selected.map(({ index }) =>
          rate
            ? samples.get(index)?.rates?.[rate]
            : samples.get(index)?.onlineDevices,
        );
        return values.every((value) => value !== undefined)
          ? values.reduce<number>((total, value) => total + (value ?? 0), 0)
          : null;
      };
      return {
        timestamp,
        time: new Date(timestamp).toLocaleTimeString(),
        server: sum("server"),
        edge: sum("edge"),
        online: sum(),
        rx: sum(undefined, mode === "rtp" ? "rtpRx" : "messagesRx"),
        tx: sum(undefined, mode === "rtp" ? "rtpTx" : "messagesTx"),
      };
    });
}

export function transportTotals(
  metrics: NodeDeviceMetrics,
  peerKind?: string,
): TransportTotals {
  return metrics.transports
    .filter((item) => !peerKind || item.peer_kind === peerKind)
    .reduce(
      (sum, item) => ({
        rtpRxTracks: sum.rtpRxTracks + item.rtp_tracks.rx,
        rtpTxTracks: sum.rtpTxTracks + item.rtp_tracks.tx,
        packetChannels: sum.packetChannels + item.data_channels.packet,
        serviceChannels: sum.serviceChannels + item.data_channels.service,
        nativeChannels: sum.nativeChannels + item.data_channels.native,
        rtpRx: sum.rtpRx + item.rtp_packets_total.rx,
        rtpTx: sum.rtpTx + item.rtp_packets_total.tx,
        messagesRx: sum.messagesRx + item.data_channel_messages_total.rx,
        messagesTx: sum.messagesTx + item.data_channel_messages_total.tx,
      }),
      {
        rtpRxTracks: 0,
        rtpTxTracks: 0,
        packetChannels: 0,
        serviceChannels: 0,
        nativeChannels: 0,
        rtpRx: 0,
        rtpTx: 0,
        messagesRx: 0,
        messagesTx: 0,
      },
    );
}

/** Only consecutive successful snapshots of the same process yield a rate. */
export function messageRates(
  previous: NodeSnapshot | undefined,
  current: NodeSnapshot,
  peerKind?: string,
): MessageRates | undefined {
  if (
    !previous?.device_metrics ||
    !current.device_metrics ||
    previous.public_key !== current.public_key ||
    current.uptime_seconds < previous.uptime_seconds
  )
    return;
  const seconds = (Date.parse(current.time) - Date.parse(previous.time)) / 1000;
  if (!(seconds > 0)) return;
  const oldBoot = Date.parse(previous.time) - previous.uptime_seconds * 1000;
  const newBoot = Date.parse(current.time) - current.uptime_seconds * 1000;
  if (Math.abs(newBoot - oldBoot) > 1000) return;
  const before = transportTotals(previous.device_metrics, peerKind);
  const after = transportTotals(current.device_metrics, peerKind);
  const keys = ["rtpRx", "rtpTx", "messagesRx", "messagesTx"] as const;
  if (keys.some((key) => after[key] < before[key])) return;
  return {
    rtpRx: (after.rtpRx - before.rtpRx) / seconds,
    rtpTx: (after.rtpTx - before.rtpTx) / seconds,
    messagesRx: (after.messagesRx - before.messagesRx) / seconds,
    messagesTx: (after.messagesTx - before.messagesTx) / seconds,
  };
}

export function countText(value: number): string {
  return value.toLocaleString(undefined, { maximumFractionDigits: 1 });
}

export function rateText(
  value: number | undefined,
  perMinute: boolean,
): string {
  return value === undefined
    ? "—"
    : `${countText(value * (perMinute ? 60 : 1))}/${perMinute ? "min" : "s"}`;
}
