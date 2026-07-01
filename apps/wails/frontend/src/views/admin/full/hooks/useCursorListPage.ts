import { useCallback, useEffect, useRef, useState } from "react";

import { toMessage } from "../components/api";

export const RESOURCE_PAGE_LIMIT = 50;

type CursorListResult<T> = {
  hasNext: boolean;
  items: T[];
  nextCursor: string | null;
};

type CursorPageLoader<T> = (query: { cursor?: string; limit: number }) => Promise<CursorListResult<T>>;

type CursorListPageState<T> = {
  cursor: string | null;
  error: string;
  hasNext: boolean;
  history: Array<string | null>;
  items: T[];
  loading: boolean;
  nextCursor: string | null;
};

export function useCursorListPage<T>(loader: CursorPageLoader<T>): {
  error: string;
  hasNext: boolean;
  items: T[];
  loading: boolean;
  nextPage: () => void;
  pageNumber: number;
  prevPage: () => void;
  refresh: () => Promise<void>;
  reset: () => void;
} {
  const loaderRef = useRef(loader);
  loaderRef.current = loader;

  const [state, setState] = useState<CursorListPageState<T>>({
    cursor: null,
    error: "",
    hasNext: false,
    history: [],
    items: [],
    loading: true,
    nextCursor: null,
  });

  const loadPage = useCallback(
    async (cursor: string | null, history: Array<string | null>) => {
      setState((current) => ({ ...current, error: "", loading: true }));
      try {
        const page = await loaderRef.current({
          cursor: cursor ?? undefined,
          limit: RESOURCE_PAGE_LIMIT,
        });
        setState({
          cursor,
          error: "",
          hasNext: page.hasNext,
          history,
          items: page.items,
          loading: false,
          nextCursor: page.nextCursor,
        });
      } catch (error) {
        setState((current) => ({
          ...current,
          error: toMessage(error),
          loading: false,
        }));
      }
    },
    [],
  );

  useEffect(() => {
    void loadPage(null, []);
  }, [loadPage]);

  const refresh = useCallback(async () => {
    await loadPage(state.cursor, state.history);
  }, [loadPage, state.cursor, state.history]);

  const reset = useCallback(() => {
    void loadPage(null, []);
  }, [loadPage]);

  const nextPage = useCallback(() => {
    if (state.nextCursor === null) {
      return;
    }
    void loadPage(state.nextCursor, [...state.history, state.cursor]);
  }, [loadPage, state.cursor, state.history, state.nextCursor]);

  const prevPage = useCallback(() => {
    if (state.history.length === 0) {
      return;
    }
    const previousCursor = state.history[state.history.length - 1] ?? null;
    void loadPage(previousCursor, state.history.slice(0, -1));
  }, [loadPage, state.history]);

  return {
    error: state.error,
    hasNext: state.hasNext,
    items: state.items,
    loading: state.loading,
    nextPage,
    pageNumber: state.history.length + 1,
    prevPage,
    refresh,
    reset,
  };
}
