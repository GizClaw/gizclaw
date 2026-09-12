import { createContext, useContext, useEffect, type ReactNode } from "react";

/** Holds what the current page last published for the assistant to read. */
export type PageViewHolder = { current: unknown };

const PageViewContext = createContext<PageViewHolder | null>(null);

export function PageViewProvider({
  holder,
  children,
}: {
  holder: PageViewHolder;
  children: ReactNode;
}) {
  return (
    <PageViewContext.Provider value={holder}>
      {children}
    </PageViewContext.Provider>
  );
}

/**
 * Publishes a structured, JSON-compatible description of what the page shows.
 * The assistant reads this instead of the DOM. A page that unmounts takes its
 * view with it, so a stale page is never described.
 */
export function usePageView(snapshot: unknown): void {
  const holder = useContext(PageViewContext);
  useEffect(() => {
    if (!holder) return;
    holder.current = snapshot;
    return () => {
      if (holder.current === snapshot) holder.current = undefined;
    };
  });
}
