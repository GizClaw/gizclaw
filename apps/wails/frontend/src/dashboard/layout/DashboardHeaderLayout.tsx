import type { ReactNode } from "react";

export function DashboardHeaderLayout({
  children,
}: {
  children: ReactNode;
}): JSX.Element {
  return (
    <header className="shrink-0 border-b bg-background px-4 py-4 sm:px-6">
      {children}
    </header>
  );
}
