import type { ComponentProps } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/components/ui/utils";

export function DashboardActionButton({
  className,
  size = "sm",
  type = "button",
  variant = "outline",
  ...props
}: ComponentProps<typeof Button>): JSX.Element {
  return (
    <Button
      className={cn(
        "min-w-fit shrink-0 whitespace-nowrap disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none",
        className,
      )}
      size={size}
      type={type}
      variant={variant}
      {...props}
    />
  );
}
