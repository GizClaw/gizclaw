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
import { DashboardActionButton } from "./DashboardActionButton";

type DashboardDeleteButtonProps = {
  children: ReactNode;
  description?: string;
  disabled?: boolean;
  onConfirm: () => void;
  size?: "default" | "sm";
  title?: string;
};

export function DashboardDeleteButton({
  children,
  description = "This action cannot be undone.",
  disabled = false,
  onConfirm,
  size = "default",
  title = "Delete item?",
}: DashboardDeleteButtonProps): JSX.Element {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <DashboardActionButton
          className="border-destructive/40 text-destructive hover:bg-destructive/10"
          disabled={disabled}
          size={size}
        >
          {children}
        </DashboardActionButton>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={onConfirm}
          >
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
