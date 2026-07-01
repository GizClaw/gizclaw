import { RefreshCw } from "lucide-react";

import { DashboardActionButton } from "./DashboardActionButton";
import { cn } from "@/components/ui/utils";

export function DashboardPager({
  canNext,
  canPrevious,
  loading,
  onNext,
  onPrevious,
  onRefresh,
  pageIndex,
}: {
  canNext: boolean;
  canPrevious: boolean;
  loading: boolean;
  onNext: () => void;
  onPrevious: () => void;
  onRefresh: () => void;
  pageIndex: number;
}): JSX.Element {
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">Page {pageIndex}</span>
      <DashboardActionButton aria-label="Refresh" disabled={loading} onClick={onRefresh}>
        <RefreshCw className={cn("size-4", loading && "animate-spin")} />
      </DashboardActionButton>
      <DashboardActionButton disabled={loading || !canPrevious} onClick={onPrevious}>
        Prev
      </DashboardActionButton>
      <DashboardActionButton disabled={loading || !canNext} onClick={onNext}>
        Next
      </DashboardActionButton>
    </div>
  );
}
