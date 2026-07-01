import type { ReactNode } from "react";

export function DashboardMainLayout({ children }: { children: ReactNode }): JSX.Element {
  return <main className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</main>;
}
