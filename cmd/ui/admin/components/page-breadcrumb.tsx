import * as React from "react";
import { Link } from "react-router-dom";

import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

type BreadcrumbEntry = {
  href?: string;
  label: string;
};

interface PageBreadcrumbProps extends React.ComponentPropsWithoutRef<typeof Breadcrumb> {
  items: BreadcrumbEntry[];
}

const PageBreadcrumb = React.forwardRef<HTMLElement, PageBreadcrumbProps>(
  ({ items, ...props }, ref) => (
    <Breadcrumb ref={ref} {...props}>
      <BreadcrumbList>
        {items.map((item, index) => {
          const isLast = index === items.length - 1;

          return (
            <BreadcrumbItem key={`${item.label}-${index}`}>
              {item.href && !isLast ? (
                <BreadcrumbLink asChild>
                  <Link to={item.href}>{item.label}</Link>
                </BreadcrumbLink>
              ) : (
                <BreadcrumbPage>{item.label}</BreadcrumbPage>
              )}
              {!isLast ? <BreadcrumbSeparator /> : null}
            </BreadcrumbItem>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  ),
);
PageBreadcrumb.displayName = "PageBreadcrumb";

export { PageBreadcrumb };
export type { BreadcrumbEntry };
