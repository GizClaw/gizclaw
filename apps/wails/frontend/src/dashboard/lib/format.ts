export function formatDashboardDate(
  value: number | string | undefined | null,
): string {
  if (value == null || value === "") {
    return "-";
  }
  const date =
    typeof value === "number"
      ? new Date(value < 10_000_000_000 ? value * 1000 : value)
      : new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString();
}

export function compactDashboardID(value: string): string {
  if (value.length <= 36) {
    return value;
  }
  return `${value.slice(0, 20)}...${value.slice(-8)}`;
}

export function formatDashboardBytes(value: number | undefined | null): string {
  if (value == null || !Number.isFinite(value)) {
    return "-";
  }
  if (value < 1024) {
    return `${Math.round(value)} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KiB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MiB`;
}
