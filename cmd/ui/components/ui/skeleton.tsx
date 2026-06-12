import * as React from "react";

import { cn } from "./utils";

function Skeleton({ className, ...props }: React.ComponentProps<"div">): JSX.Element {
  return (
    <div
      className={cn("animate-pulse rounded-md bg-accent", className)}
      data-slot="skeleton"
      {...props}
    />
  );
}

export { Skeleton };
