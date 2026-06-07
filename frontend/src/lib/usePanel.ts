import { useCallback, useEffect, useState } from "react";
import { getJSON } from "@/lib/api";

interface PanelState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  reload: () => void;
}

// usePanel fetches a read-only control-plane view and exposes a manual reload.
// Panels are cheap reads proxied through the daemon; the live feed drives most
// freshness, so a light reload button + mount fetch is enough.
export function usePanel<T = any>(path: string, params?: Record<string, string>): PanelState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const key = params ? JSON.stringify(params) : "";

  const reload = useCallback(() => {
    setLoading(true);
    getJSON<T>(path, params)
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
