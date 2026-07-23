import type { ReactNode } from "react";

import { cn } from "@/components/ui/utils";

export function DashboardContentLayout({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}): JSX.Element {
  return (
    <div
      className={cn(
        "flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto overscroll-contain p-4 sm:p-6",
        className,
      )}
    >
      {children}
    </div>
  );
}
