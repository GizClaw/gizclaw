import type { ReactNode } from "react";
import { Database } from "lucide-react";

export function DashboardSidebarBrand({
  icon,
  subtitle,
  title,
}: {
  icon?: ReactNode;
  subtitle?: string;
  title: string;
}): JSX.Element {
  return (
    <div className="mb-6 flex items-center gap-3 px-2">
      <div className="flex size-9 items-center justify-center rounded-md bg-primary text-primary-foreground">
        {icon ?? <Database className="size-5" />}
      </div>
      <div className="min-w-0">
        <div className="truncate text-sm font-semibold">{title}</div>
        {subtitle ? <div className="truncate text-xs text-muted-foreground">{subtitle}</div> : null}
      </div>
    </div>
  );
}
