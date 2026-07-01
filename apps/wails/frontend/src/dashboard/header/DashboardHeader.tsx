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
    <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <DashboardHeaderTitle contextName={contextName} eyebrow={eyebrow} subtitle={subtitle} title={title} titleAsHeading={titleAsHeading} />
      <div className="flex flex-wrap items-center gap-2">
        <DashboardMobileNav activeID={activeID} groups={navGroups} onNavigate={onNavigate} />
        <DashboardHeaderActions actions={actions} onSignOut={onSignOut} />
      </div>
    </div>
  );
}
