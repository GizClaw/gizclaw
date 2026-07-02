import type { ReactNode } from "react";
import { LogOut } from "lucide-react";

import { Button } from "@/components/ui/button";

export function DashboardHeaderActions({
  actions,
  onSignOut,
}: {
  actions?: ReactNode;
  onSignOut(): Promise<void>;
}): JSX.Element {
  return (
    <div className="flex min-w-0 flex-wrap justify-end gap-2">
      {actions}
      <Button onClick={() => void onSignOut()} size="sm" type="button" variant="outline">
        <LogOut className="size-4" />
        Logout
      </Button>
    </div>
  );
}
