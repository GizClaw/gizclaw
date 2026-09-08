import * as React from "react";
import { cn } from "@/lib/utils";
export function Alert({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      role="alert"
      data-slot="alert"
      className={cn(
        "grid gap-1.5 rounded-md border border-[#e6bdb2] bg-[#fff0eb] px-4 py-3 text-destructive",
        className,
      )}
      {...props}
    />
  );
}
export function AlertTitle({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-title"
      className={cn("text-xs font-semibold", className)}
      {...props}
    />
  );
}
export function AlertDescription({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-description"
      className={cn("text-xs leading-relaxed break-words", className)}
      {...props}
    />
  );
}
