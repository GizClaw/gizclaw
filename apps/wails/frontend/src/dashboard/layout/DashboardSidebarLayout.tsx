import type { ReactNode } from "react";

export function DashboardSidebarLayout({ children }: { children: ReactNode }): JSX.Element {
  return <aside className="hidden w-64 shrink-0 border-r bg-background px-4 py-5 lg:block">{children}</aside>;
}
