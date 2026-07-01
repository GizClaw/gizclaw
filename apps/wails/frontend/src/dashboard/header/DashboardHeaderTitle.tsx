export function DashboardHeaderTitle({
  contextName,
  eyebrow,
  titleAsHeading = false,
  subtitle,
  title,
}: {
  contextName?: string;
  eyebrow?: string;
  titleAsHeading?: boolean;
  subtitle?: string;
  title: string;
}): JSX.Element {
  return (
    <div className="min-w-0">
      {eyebrow ? <div className="text-xs font-semibold uppercase text-muted-foreground">{eyebrow}</div> : null}
      {titleAsHeading ? (
        <h1 className="truncate text-2xl font-semibold tracking-tight">{title}</h1>
      ) : (
        <div className="truncate text-2xl font-semibold tracking-tight">{title}</div>
      )}
      <div className="mt-0.5 truncate text-xs text-muted-foreground">
        {subtitle ?? (contextName == null || contextName === "" ? "No context selected" : `Context: ${contextName}`)}
      </div>
    </div>
  );
}
