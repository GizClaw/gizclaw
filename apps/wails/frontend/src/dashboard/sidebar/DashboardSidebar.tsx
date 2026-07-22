import type { ReactNode } from "react";

import type { DashboardNavGroup } from "../types";
import { DashboardSidebarBrand } from "./DashboardSidebarBrand";
import { DashboardSidebarFooter } from "./DashboardSidebarFooter";
import { DashboardSidebarNav } from "./DashboardSidebarNav";

export function DashboardSidebar<ID extends string>({
  activeID,
  brandIcon,
  brandSubtitle,
  brandTitle,
  contextName,
  navGroups,
  onNavigate,
}: {
  activeID: ID;
  brandIcon?: ReactNode;
  brandSubtitle?: string;
  brandTitle: string;
  contextName?: string;
  navGroups: Array<DashboardNavGroup<ID>>;
  onNavigate(id: ID): void;
}): JSX.Element {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <DashboardSidebarBrand
        icon={brandIcon}
        subtitle={brandSubtitle}
        title={brandTitle}
      />
      <div className="min-h-0 flex-1 overflow-y-auto pr-1">
        <DashboardSidebarNav
          activeID={activeID}
          groups={navGroups}
          onNavigate={onNavigate}
        />
      </div>
      <DashboardSidebarFooter contextName={contextName} />
    </div>
  );
}
