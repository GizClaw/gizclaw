import type { ReactNode } from "react";

import { Table } from "@/components/ui/table";
import { cn } from "@/components/ui/utils";

export function DashboardTable({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}): JSX.Element {
  return (
    <div className="min-w-0 overflow-hidden rounded-md border">
      <Table className={cn("table-fixed", className)}>{children}</Table>
    </div>
  );
}
