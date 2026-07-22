import type { ReactNode } from "react";

import type { DashboardNavGroup } from "../types";
import { DashboardHeaderActions } from "./DashboardHeaderActions";
import { DashboardHeaderTitle } from "./DashboardHeaderTitle";
import { DashboardMobileNav } from "./DashboardMobileNav";

export function DashboardHeader<ID extends string>({
  actions,
  activeID,
  contextName,
  eyebrow,
  navGroups,
  onNavigate,
  onSignOut,
  subtitle,
  title,
  titleAsHeading,
}: {
  actions?: ReactNode;
  activeID: ID;
  contextName?: string;
  eyebrow?: string;
  navGroups: Array<DashboardNavGroup<ID>>;
  onNavigate(id: ID): void;
  onSignOut(): Promise<void>;
  subtitle?: string;
  title: string;
  titleAsHeading?: boolean;
}): JSX.Element {
  return (
    <div className="grid min-w-0 grid-cols-1 gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
      <DashboardHeaderTitle
        contextName={contextName}
        eyebrow={eyebrow}
        subtitle={subtitle}
        title={title}
        titleAsHeading={titleAsHeading}
      />
      <div className="flex min-w-0 flex-wrap items-center gap-2 lg:justify-end">
        <DashboardMobileNav
          activeID={activeID}
          groups={navGroups}
          onNavigate={onNavigate}
        />
        <DashboardHeaderActions actions={actions} onSignOut={onSignOut} />
      </div>
    </div>
  );
}
