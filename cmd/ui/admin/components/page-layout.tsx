import type { ReactNode } from "react";

import { Card, CardContent } from "@/components/ui/card";
import { PageBreadcrumb, type BreadcrumbEntry } from "./page-breadcrumb";
import { cn } from "@/components/ui/utils";

type PageHeaderProps = {
  actions?: ReactNode;
  items: BreadcrumbEntry[];
};

export function PageHeader({ actions, items }: PageHeaderProps): JSX.Element {
  return (
    <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <PageBreadcrumb items={items} />
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}

type PageSummaryCardProps = {
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
  description?: ReactNode;
  eyebrow?: string;
  meta?: ReactNode;
  title: ReactNode;
};

export function PageSummaryCard({
  actions,
  children,
  className,
  description,
  eyebrow,
  meta,
  title,
}: PageSummaryCardProps): JSX.Element {
  return (
    <Card className={cn("border-border/60 bg-background/95 shadow-sm", className)}>
      <CardContent className="flex flex-col gap-4 p-5">
        <div className="flex min-w-0 flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 flex-col gap-2">
            {eyebrow ? <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{eyebrow}</div> : null}
            <h1 className="text-2xl font-semibold tracking-tight text-foreground lg:text-3xl">{title}</h1>
            {description ? <div className="max-w-3xl text-sm leading-6 text-muted-foreground lg:text-base">{description}</div> : null}
          </div>
          {meta || actions ? (
            <div className="flex shrink-0 flex-wrap items-center gap-2">
              {meta}
              {actions}
            </div>
          ) : null}
        </div>
        {children}
      </CardContent>
    </Card>
  );
}
