import { useCallback, useEffect, useState } from "react";
import { getJSON, type ReadPath, type ReadArgsPath, type NativePath, type RuntimePath } from "@/app/api";

interface PanelState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  reload: () => void;
}

// usePanel fetches a read-only control-plane view and exposes a manual reload.
// Panels are cheap reads proxied through the daemon; the live feed drives most
// freshness, so a light reload button + mount fetch is enough.
//
// The path is typed to the console's surface: a panel cannot ask for a path
// the daemon does not answer, and the caller's literal is checked against the
// catalogue at the call site.
export function usePanel<T = any>(
  path: ReadPath | ReadArgsPath | NativePath,
  params?: Record<string, string>,
): PanelState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const key = params ? JSON.stringify(params) : "";

  const reload = useCallback(() => {
    setLoading(true);
    // The path arrives as a prop, so it cannot be checked against the
    // catalogue again here — the prop's own type is the catalogue's.
    getJSON<T>(path as RuntimePath, params)
      .then((d) => {
        setData(d);
        setError(null);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, key]);

  useEffect(() => {
    reload();
  }, [reload]);

  return { data, error, loading, reload };
}
