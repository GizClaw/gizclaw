import type { ReactNode } from "react";

import { DashboardContentLayout } from "./layout/DashboardContentLayout";
import { DashboardHeaderLayout } from "./layout/DashboardHeaderLayout";
import { DashboardLayout } from "./layout/DashboardLayout";
import { DashboardMainLayout } from "./layout/DashboardMainLayout";
import { DashboardSidebarLayout } from "./layout/DashboardSidebarLayout";
import { DashboardHeader } from "./header/DashboardHeader";
import { DashboardSidebar } from "./sidebar/DashboardSidebar";
import type { DashboardNavGroup } from "./types";

export function DashboardShell<ID extends string>({
  actions,
  activeID,
  brandSubtitle,
  brandTitle,
  children,
  contentClassName,
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
  brandSubtitle?: string;
  brandTitle: string;
  children: ReactNode;
  contentClassName?: string;
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
    <DashboardLayout>
      <div className="flex h-screen min-h-0">
        <DashboardSidebarLayout>
          <DashboardSidebar
            activeID={activeID}
            brandSubtitle={brandSubtitle}
            brandTitle={brandTitle}
            contextName={contextName}
            navGroups={navGroups}
            onNavigate={onNavigate}
          />
        </DashboardSidebarLayout>
        <DashboardMainLayout>
          <DashboardHeaderLayout>
            <DashboardHeader
              actions={actions}
              activeID={activeID}
              contextName={contextName}
              eyebrow={eyebrow}
              navGroups={navGroups}
              onNavigate={onNavigate}
              onSignOut={onSignOut}
              subtitle={subtitle}
              title={title}
              titleAsHeading={titleAsHeading}
            />
          </DashboardHeaderLayout>
          <DashboardContentLayout className={contentClassName}>{children}</DashboardContentLayout>
        </DashboardMainLayout>
      </div>
    </DashboardLayout>
  );
}
