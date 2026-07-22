export function DashboardSidebarFooter({
  contextName,
}: {
  contextName?: string;
}): JSX.Element {
  return (
    <div className="mt-auto border-t pt-4">
      <div className="min-w-0 px-2">
        <div className="text-xs font-semibold uppercase text-muted-foreground">
          Context
        </div>
        <div
          className="mt-1 truncate text-sm font-medium text-foreground"
          title={contextName ?? ""}
        >
          {contextName == null || contextName === ""
            ? "No context"
            : contextName}
        </div>
      </div>
    </div>
  );
}
