import type { ReactNode } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

/** Numeric tile: label, measured value, and what the value is measured from. */
export function MetricCard({
  label,
  value,
  note,
  icon,
}: {
  label: string;
  value: string;
  note: string;
  icon?: ReactNode;
}) {
  return (
    <Card className="min-w-0 gap-3">
      <CardHeader>
        <CardTitle className="text-[11px] font-normal text-muted-foreground">
          {label}
        </CardTitle>
        {icon && <span className="text-muted-foreground/70">{icon}</span>}
      </CardHeader>
      <CardContent className="flex flex-col gap-1.5">
        <strong
          className="truncate text-[25px] font-medium tracking-tight tabular-nums"
          title={value}
        >
          {value}
        </strong>
        <small className="text-[10px] break-words text-muted-foreground">
          {note}
        </small>
      </CardContent>
    </Card>
  );
}
