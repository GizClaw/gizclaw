import type { DashboardNavGroup } from "../types";
import { DashboardSidebarItem } from "./DashboardSidebarItem";

export function DashboardSidebarNav<ID extends string>({
  activeID,
  groups,
  onNavigate,
}: {
  activeID: ID;
  groups: Array<DashboardNavGroup<ID>>;
  onNavigate(id: ID): void;
}): JSX.Element {
  return (
    <nav className="space-y-4">
      {groups.map((group, groupIndex) => (
        <div className="space-y-1" key={group.label ?? groupIndex}>
          {group.label ? (
            <div className="px-3 pb-1 text-xs font-semibold uppercase text-muted-foreground">
              {group.label}
            </div>
          ) : null}
          {group.items.map((item) => (
            <DashboardSidebarItem
              active={activeID === item.id}
              item={item}
              key={item.id}
              onNavigate={onNavigate}
            />
          ))}
        </div>
      ))}
    </nav>
  );
}
