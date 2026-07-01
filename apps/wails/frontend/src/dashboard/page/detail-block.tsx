import * as React from "react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/components/ui/utils";

type DetailBlockItem = [label: string, value: string | undefined];

interface DetailBlockProps extends React.HTMLAttributes<HTMLDivElement> {
  items: DetailBlockItem[];
  title: string;
}

const DetailBlock = React.forwardRef<HTMLDivElement, DetailBlockProps>(
  ({ className, items, title, ...props }, ref) => (
    <Card ref={ref} className={cn("min-w-0", className)} {...props}>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {items.map(([label, value]) => (
          <div className="flex min-w-0 items-start justify-between gap-4 text-sm" key={label}>
            <span className="shrink-0 text-muted-foreground">{label}</span>
            <span className="block min-w-0 max-w-[min(16rem,65%)] break-words text-right text-foreground">{formatDetailValue(value)}</span>
          </div>
        ))}
      </CardContent>
    </Card>
  ),
);
DetailBlock.displayName = "DetailBlock";

function formatDetailValue(value: string | undefined): string {
  if (value === undefined || value === "") {
    return "-";
  }
  if (!isDateTimeLike(value)) {
    return value;
  }
  const time = Date.parse(value);
  return Number.isNaN(time) ? value : new Date(time).toLocaleString();
}

function isDateTimeLike(value: string): boolean {
  return value.includes("T") || value.endsWith("Z");
}

export { DetailBlock };
export type { DetailBlockItem };
