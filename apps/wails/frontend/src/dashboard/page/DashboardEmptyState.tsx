import * as React from "react";

import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { cn } from "@/components/ui/utils";

interface DashboardEmptyStateProps extends React.HTMLAttributes<HTMLDivElement> {
  description: string;
  title: string;
}

const DashboardEmptyState = React.forwardRef<
  HTMLDivElement,
  DashboardEmptyStateProps
>(({ className, description, title, ...props }, ref) => (
  <Empty
    ref={ref}
    className={cn("min-h-56 border bg-muted/20", className)}
    {...props}
  >
    <EmptyHeader>
      <EmptyTitle className="text-base">{title}</EmptyTitle>
      <EmptyDescription>{description}</EmptyDescription>
    </EmptyHeader>
  </Empty>
));
DashboardEmptyState.displayName = "DashboardEmptyState";

export { DashboardEmptyState };
export { DashboardEmptyState as EmptyState };
