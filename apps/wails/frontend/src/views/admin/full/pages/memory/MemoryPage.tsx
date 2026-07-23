import type { ComponentType } from "react";
import { Activity, Database, MemoryStick } from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

import { EmptyState } from "@/dashboard";
import { PageHeader, PageSummaryCard } from "@/dashboard";

function FutureModule({
  description,
  icon: Icon,
  title,
}: {
  description: string;
  icon: ComponentType<{ className?: string }>;
  title: string;
}): JSX.Element {
  return (
    <div className="flex gap-3 rounded-lg border bg-background p-4">
      <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-primary/5 text-primary">
        <Icon className="size-4" />
      </div>
      <div className="space-y-1">
        <div className="text-sm font-semibold">{title}</div>
        <p className="text-sm leading-6 text-muted-foreground">{description}</p>
      </div>
    </div>
  );
}

export function MemoryPage(): JSX.Element {
  return (
    <div className="space-y-6">
      <PageHeader
        items={[{ href: "/overview", label: "Overview" }, { label: "Memory" }]}
      />

      <PageSummaryCard
        description="Reserved diagnostics area for future operational surfaces (cache, KV, jobs) without redesigning navigation."
        eyebrow="System"
        title="Memory"
      />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Memory module</CardTitle>
            <CardDescription>
              This screen exists so future operational areas have a real home in
              the sidebar.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <EmptyState
              description="The shell is ready for memory, cache, and store diagnostics without redesigning navigation again."
              title="Coming soon"
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Planned surfaces</CardTitle>
            <CardDescription>
              Examples of modules that can slot into this admin console next.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <FutureModule
              description="Active sessions, peer state, and cache inspection."
              icon={MemoryStick}
              title="In-memory state"
            />
            <FutureModule
              description="Store backends, persistence health, and data debugging."
              icon={Database}
              title="KV stores"
            />
            <FutureModule
              description="Background refreshes, resource updates, and execution traces."
              icon={Activity}
              title="Jobs"
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
