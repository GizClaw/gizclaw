import type { DashboardNavGroup } from "../types";
import { cn } from "@/components/ui/utils";

export function DashboardMobileNav<ID extends string>({
  activeID,
  groups,
  onNavigate,
}: {
  activeID: ID;
  groups: Array<DashboardNavGroup<ID>>;
  onNavigate(id: ID): void;
}): JSX.Element {
  const items = groups.flatMap((group) => group.items);
  return (
    <div className="flex max-w-full gap-1 overflow-x-auto rounded-md border bg-background p-1 lg:hidden">
      {items.map((item) => (
        <button
          aria-label={item.label}
          className={cn(
            "flex size-8 shrink-0 items-center justify-center rounded-sm text-muted-foreground",
            activeID === item.id && "bg-accent text-accent-foreground",
          )}
          key={item.id}
          onClick={() => onNavigate(item.id)}
          type="button"
        >
          <item.icon className="size-4" />
        </button>
      ))}
    </div>
  );
}
