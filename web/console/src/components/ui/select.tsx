// Native select styled like the shadcn trigger: the console only needs short,
// static option lists and keeps the bundle free of a popover implementation.
import * as React from "react";
import { cn } from "@/lib/utils";
export function Select({
  className,
  ...props
}: React.ComponentProps<"select">) {
  return (
    <select
      data-slot="select"
      className={cn(
        "h-9 rounded-md border border-input bg-card px-2.5 text-xs text-foreground",
        className,
      )}
      {...props}
    />
  );
}
