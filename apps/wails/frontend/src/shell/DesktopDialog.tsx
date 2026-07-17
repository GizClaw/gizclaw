import { useEffect, useRef, useState } from "react";
import type { PropsWithChildren, ReactNode } from "react";
import { Dialog as DialogPrimitive } from "radix-ui";

export function DesktopDialog({
  children,
  className,
  nested = false,
  onClose,
}: {
  children(close: () => void): ReactNode;
  className: string;
  nested?: boolean;
  onClose(): void;
}) {
  const [open, setOpen] = useState(true);
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (open) return;
    const timer = window.setTimeout(() => onCloseRef.current(), 220);
    return () => window.clearTimeout(timer);
  }, [open]);

  const close = () => setOpen(false);
  return (
    <DialogPrimitive.Root onOpenChange={setOpen} open={open}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className={nested ? "nested-dialog-backdrop" : "dialog-backdrop"}
          data-slot="desktop-dialog-overlay"
        />
        <DialogPrimitive.Content
          aria-describedby={undefined}
          className={`desktop-dialog-content ${nested ? "nested-dialog-content " : ""}${className}`}
          data-slot="desktop-dialog-content"
        >
          {children(close)}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

export function DesktopDialogTitle({ children }: PropsWithChildren) {
  return <DialogPrimitive.Title asChild>{children}</DialogPrimitive.Title>;
}
