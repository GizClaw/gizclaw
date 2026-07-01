import type { DashboardNavItem } from "../types";
import { cn } from "@/components/ui/utils";

export function DashboardSidebarItem<ID extends string>({
  active,
  item,
  onNavigate,
}: {
  active: boolean;
  item: DashboardNavItem<ID>;
  onNavigate(id: ID): void;
}): JSX.Element {
  return (
    <button
      className={cn(
        "flex h-9 w-full items-center justify-between rounded-md px-3 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground",
        active && "bg-accent text-accent-foreground",
      )}
      onClick={() => onNavigate(item.id)}
      type="button"
    >
      <span className="inline-flex min-w-0 items-center gap-2">
        <item.icon className="size-4 shrink-0" />
        <span className="truncate">{item.label}</span>
      </span>
      {item.badge}
    </button>
  );
}
