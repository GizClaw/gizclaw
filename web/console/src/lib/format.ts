export function bytes(value: number): string {
  if (!Number.isFinite(value)) return "—";
  if (value < 1024) return `${Math.round(value)} B`;
  if (value < 1048576) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1073741824) return `${(value / 1048576).toFixed(1)} MiB`;
  return `${(value / 1073741824).toFixed(2)} GiB`;
}

export function duration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  const total = Math.floor(seconds);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  if (hours > 0) return `${hours} 小时 ${minutes} 分`;
  if (minutes > 0) return `${minutes} 分 ${total % 60} 秒`;
  return `${total} 秒`;
}

export function host(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

export type Sample = {
  timestamp: number;
  time: string;
  rx: number;
  tx: number;
};

/** Byte counters are cumulative; a restart lowers them, which reads as zero. */
export function rates(
  previous: { time: number; rx: number; tx: number } | undefined,
  current: { time: number; rx: number; tx: number },
): Sample {
  const seconds = previous ? (current.time - previous.time) / 1000 : 0;
  return {
    timestamp: current.time,
    time: new Date(current.time).toLocaleTimeString(),
    rx:
      previous && seconds > 0
        ? Math.max(0, current.rx - previous.rx) / seconds
        : 0,
    tx:
      previous && seconds > 0
        ? Math.max(0, current.tx - previous.tx) / seconds
        : 0,
  };
}

/**
 * Go zero times arrive as 0001-01-01T00:00:00Z and mean "never", not a date in
 * year 1; unset Unix millisecond stamps arrive as 0.
 */
export function timestamp(value: string | number | undefined): string {
  if (value === undefined || value === "" || value === 0) return "—";
  const parsed = typeof value === "number" ? value : Date.parse(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return "—";
  return new Date(parsed).toLocaleString();
}
