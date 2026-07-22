import { Outlet } from "react-router-dom";
import { useLocation, useNavigate } from "react-router-dom";

import { DashboardShell } from "@/dashboard";
import { adminActiveNavID, adminNavGroups, adminNavTitle } from "../admin-nav";

export function AdminLayout({
  contextName,
  onSignOut,
}: {
  contextName?: string;
  onSignOut(): Promise<void>;
}): JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();
  const activeID = adminActiveNavID(location.pathname);

  return (
    <DashboardShell
      activeID={activeID}
      brandSubtitle="Admin Console"
      brandTitle="GizClaw"
      contextName={contextName}
      eyebrow="Admin"
      navGroups={adminNavGroups}
      onNavigate={(path) => navigate(path)}
      onSignOut={onSignOut}
      title={adminNavTitle(location.pathname)}
    >
      <Outlet />
    </DashboardShell>
  );
}
