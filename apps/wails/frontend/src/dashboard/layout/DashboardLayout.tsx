import type { ReactNode } from "react";

export function DashboardLayout({
  children,
}: {
  children: ReactNode;
}): JSX.Element {
  return <div className="h-screen overflow-hidden bg-slate-50">{children}</div>;
}
