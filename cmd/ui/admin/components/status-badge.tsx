import * as React from "react";

import { Badge, type BadgeProps } from "@/components/ui/badge";

interface StatusBadgeProps extends Omit<BadgeProps, "children" | "variant"> {
  status: string;
}

const StatusBadge = React.forwardRef<HTMLSpanElement, StatusBadgeProps>(
  ({ status, ...props }, ref) => {
    if (status === "active") {
      return (
        <Badge ref={ref} variant="success" {...props}>
          Active
        </Badge>
      );
    }
    if (status === "blocked") {
      return (
        <Badge ref={ref} variant="destructive" {...props}>
          Blocked
        </Badge>
      );
    }
    return (
      <Badge ref={ref} variant="outline" {...props}>
        {status}
      </Badge>
    );
  },
);
StatusBadge.displayName = "StatusBadge";

export { StatusBadge };
