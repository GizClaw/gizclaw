import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

export type DashboardNavItem<ID extends string = string> = {
  badge?: ReactNode;
  icon: LucideIcon;
  id: ID;
  label: string;
};

export type DashboardNavGroup<ID extends string = string> = {
  items: Array<DashboardNavItem<ID>>;
  label?: string;
};
