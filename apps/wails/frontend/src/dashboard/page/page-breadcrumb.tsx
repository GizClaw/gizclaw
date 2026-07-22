import { ChevronRight, Home } from "lucide-react";
import { Link } from "react-router-dom";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

export type BreadcrumbEntry = {
  href?: string;
  label: string;
};

export function PageBreadcrumb({
  items,
}: {
  items: BreadcrumbEntry[];
}): JSX.Element {
  return (
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link to="/overview">
              <Home className="size-4" />
              <span className="sr-only">Overview</span>
            </Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        {items.map((item, index) => {
          const isLast = index === items.length - 1;
          return (
            <BreadcrumbItem key={`${item.label}:${index}`}>
              <BreadcrumbSeparator>
                <ChevronRight className="size-4" />
              </BreadcrumbSeparator>
              {isLast || item.href === undefined ? (
                <BreadcrumbPage>{item.label}</BreadcrumbPage>
              ) : (
                <BreadcrumbLink asChild>
                  <Link to={item.href}>{item.label}</Link>
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
