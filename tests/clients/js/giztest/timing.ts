// Scheduling is executed by the shared Go/C runner. This SDK contract runner
// validates the same fields before accepting and ignoring them; its report
// marks timing_mode as "ignored" so it cannot be mistaken for a load test.
const durationPattern = /^(0|([0-9]+(\.[0-9]+)?(ns|us|µs|μs|ms|s|m|h))+)$/u;
const maxDuration = 9_223_372_036_854_775_807n;
const units: Record<string, bigint> = {
  h: 3_600_000_000_000n,
  m: 60_000_000_000n,
  ms: 1_000_000n,
  ns: 1n,
  s: 1_000_000_000n,
  us: 1000n,
  µs: 1000n,
  μs: 1000n,
};

function duration(value: unknown, field: string): bigint {
  if (typeof value !== "string" || !durationPattern.test(value)) {
    throw new Error(`${field} must be a non-negative duration`);
  }
  let total = 0n;
  for (const match of value.matchAll(
    /([0-9]+)(?:\.([0-9]+))?(ns|us|µs|μs|ms|s|m|h)/gu,
  )) {
    const fraction = match[2] ?? "";
    const scale = 10n ** BigInt(fraction.length);
    total += (BigInt(match[1] + fraction) * units[match[3]]) / scale;
  }
  if (total > maxDuration) {
    throw new Error(`${field} exceeds the duration range`);
  }
  return total;
}

export type TimingFields = {
  start_jitter?: unknown;
  stagger?: unknown;
  step_jitter?: unknown;
  seed?: unknown;
};

export function validateTiming(
  fields: TimingFields,
  repeat: number,
  hasBarrier: boolean,
): void {
  const start =
    fields.start_jitter === undefined
      ? 0n
      : duration(fields.start_jitter, "start_jitter");
  const stagger =
    fields.stagger === undefined ? 0n : duration(fields.stagger, "stagger");
  const step =
    fields.step_jitter === undefined
      ? 0n
      : duration(fields.step_jitter, "step_jitter");
  if (
    fields.seed !== undefined &&
    (typeof fields.seed !== "number" ||
      !Number.isSafeInteger(fields.seed) ||
      fields.seed < 0)
  ) {
    throw new Error("seed must be an integer in 0..9007199254740991");
  }
  if (
    Number.isInteger(repeat) &&
    BigInt(Math.max(0, repeat - 1)) * stagger + (start > 0n ? start - 1n : 0n) >
      maxDuration
  ) {
    throw new Error(
      "(repeat - 1) * stagger + start_jitter exceeds the duration range",
    );
  }
  if (hasBarrier && (start > 0n || stagger > 0n || step > 0n)) {
    throw new Error(
      "barrier cannot be combined with non-zero start_jitter, stagger or step_jitter",
    );
  }
}
