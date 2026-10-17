import type { ReactNode } from "react";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export type SummaryMetric = {
  label: string;
  value: ReactNode;
  note: string;
  icon: ReactNode;
  tone?: "rx" | "tx";
  compact?: boolean;
};

export function MonitorSummary({ metrics }: { metrics: SummaryMetric[] }) {
  return (
    <Card className="gap-0 overflow-hidden p-0">
      <dl
        className={cn(
          "grid grid-cols-2 gap-px bg-border",
          metrics.length > 4 ? "xl:grid-cols-3" : "xl:grid-cols-4",
        )}
      >
        {metrics.map((metric) => (
          <div
            key={metric.label}
            className={cn(
              "min-w-0 bg-card px-4 py-4 sm:px-6 sm:py-5",
              metrics.length % 2 === 1 && "last:col-span-2",
            )}
          >
            <dt className="flex items-center justify-between gap-3 text-[12px] text-muted-foreground">
              {metric.label}
              <span
                aria-hidden="true"
                className={cn(
                  "text-muted-foreground/60",
                  metric.tone === "rx" && "text-chart-rx",
                  metric.tone === "tx" && "text-chart-tx",
                )}
              >
                {metric.icon}
              </span>
            </dt>
            <dd
              className={cn(
                "mt-3 truncate font-medium leading-tight tracking-tight tabular-nums",
                metric.compact
                  ? "text-lg sm:text-[22px]"
                  : "text-[23px] sm:text-[30px]",
              )}
              title={
                typeof metric.value === "string" ? metric.value : undefined
              }
            >
              {metric.value}
            </dd>
            <dd className="mt-2 text-[11px] leading-relaxed text-muted-foreground">
              {metric.note}
            </dd>
          </div>
        ))}
      </dl>
    </Card>
  );
}
