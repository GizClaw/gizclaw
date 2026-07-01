import type { ReactNode } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DashboardEmptyState } from "../page/DashboardEmptyState";

export function DashboardTableCard({
  actions,
  children,
  emptyDescription,
  emptyTitle,
  error,
  title,
}: {
  actions?: ReactNode;
  children: ReactNode;
  emptyDescription?: string;
  emptyTitle?: string;
  error?: string;
  title: string;
}): JSX.Element {
  return (
    <Card className="max-w-6xl">
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{title}</CardTitle>
        {actions}
      </CardHeader>
      <CardContent>
        {error ? (
          <Alert className="mb-4" variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {emptyTitle ? <DashboardEmptyState description={emptyDescription ?? ""} title={emptyTitle} /> : children}
      </CardContent>
    </Card>
  );
}
