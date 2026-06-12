import type { ReactNode } from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

type DeleteConfirmButtonProps = {
  children: ReactNode;
  description?: string;
  disabled?: boolean;
  onConfirm: () => void;
  size?: "default" | "sm";
  title?: string;
};

export function DeleteConfirmButton({
  children,
  description = "This action cannot be undone.",
  disabled = false,
  onConfirm,
  size = "default",
  title = "Delete item?",
}: DeleteConfirmButtonProps): JSX.Element {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button className="border-destructive/40 text-destructive hover:bg-destructive/10" disabled={disabled} size={size} type="button" variant="outline">
          {children}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={onConfirm}>
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
