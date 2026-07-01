import { Outlet } from "react-router-dom";

import { AppSidebar } from "./AppSidebar";

export function AdminLayout(): JSX.Element {
  return (
    <div className="h-screen overflow-hidden bg-muted/30">
      <div className="grid h-screen lg:grid-cols-[248px_minmax(0,1fr)]">
        <AppSidebar />
        <main className="min-w-0 overflow-y-auto overscroll-contain">
          <div className="mx-auto flex min-h-full w-full max-w-[1400px] flex-col gap-8 px-6 pb-6 lg:px-10 lg:pb-10">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
