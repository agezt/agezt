import { useEffect, useState } from "react";
import { getJSON } from "@/app/api";
import { usePanel } from "@/lib/usePanel";

/**
 * useCursorPager is the load-more state machine used by views that page
 * through a control-plane list endpoint (e.g. /api/runs, /api/agents,
 * /api/inbox, /api/board, /api/memory). It mirrors the pattern already
 * shipped with `useRunsPager` but is generic enough to reuse for any
 * endpoint that returns `{ <items>: T[]; next_cursor?: string | null }`.
 *
 * The hook composes the existing `usePanel` (polling, auth, error retry,
 * live-event reload — see lib/usePanel) with a separate `loadMore` that
 * hits the same path with `?cursor=…&limit=…` on demand. The two share
 * the first page: `usePanel` returns the leading rows + the initial
 * `next_cursor`; the pager extends from there.
 *
 * @param path      e.g. "/api/runs"
 * @param itemsKey  field name on the response envelope that holds the
 *                  rows — "runs" for /api/runs, "profiles" for
 *                  /api/agents, "threads" for /api/inbox, etc.
 * @param idKey     field name on each row that uniquely identifies it
 *                  (used for dedup so an apparent re-emission from the
 *                  server can't double-count a row across pages).
 * @param limit     page size; defaults to 50.
 * @param params    extra query params forwarded to every request (e.g.
 *                  `channel` for /api/inbox, `topic` for /api/board).
 */
export interface CursorPagerResult<T> {
  paged: T[];
  error: string | null;
  loading: boolean;
  loadMore: () => Promise<void>;
  loadingMore: boolean;
  moreError: string | null;
  hasMore: boolean;
  reload: () => void;
}

export function useCursorPager<T extends Record<string, unknown>>(
  path: string,
  itemsKey: string,
  idKey: keyof T & string,
  limit: number = 50,
  params?: Record<string, string>,
): CursorPagerResult<T> {
  const query: Record<string, string> = { limit: String(limit), ...(params || {}) };
  const { data, error, loading, reload } = usePanel<{
    [k: string]: unknown;
    next_cursor?: string | null;
  }>(path, query);
  const [paged, setPaged] = useState<T[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<string | null>(null);

  useEffect(() => {
    if (!data) return;
    const items = (data[itemsKey] as T[] | undefined) ?? [];
    setPaged(items);
    setCursor((data.next_cursor as string | undefined) ?? null);
    setMoreError(null);
  }, [data, itemsKey]);

  const loadMore = async () => {
    if (loadingMore) return;
    if (!cursor) return;
    setLoadingMore(true);
    setMoreError(null);
    try {
      const page = await getJSON<{ [k: string]: unknown; next_cursor?: string | null }>(path, {
        ...query,
        cursor,
      });
      const next = (page[itemsKey] as T[] | undefined) ?? [];
      setPaged((cur) => {
        // A row with no id value cannot identify itself. `String(undefined)` is
        // the truthy string "undefined", which would make every id-less row look
        // like a duplicate of the first one and silently drop it (the `id &&`
        // guard below is written to keep such rows, and coercion defeats it).
        // Normalize absent/null to "" so only a present id participates in dedup.
        const dedupKey = (r: T) => (r[idKey] == null ? "" : String(r[idKey]));
        const seen = new Set(cur.map(dedupKey));
        const merged = [...cur];
        for (const r of next) {
          const id = dedupKey(r);
          if (id && seen.has(id)) continue;
          merged.push(r);
          if (id) seen.add(id);
        }
        return merged;
      });
      setCursor((page.next_cursor as string | undefined) ?? null);
    } catch (err) {
      setMoreError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoadingMore(false);
    }
  };

  return {
    paged,
    error,
    loading,
    loadMore,
    loadingMore,
    moreError,
    hasMore: cursor !== null,
    reload,
  };
}

// ───────────────────────── Endpoint-specific wrappers ─────────────────────────
//
// These wrappers are intentionally thin: they pick the (itemsKey, idKey)
// pair for one specific endpoint and forward everything else to
// useCursorPager. Views opt in by importing the named hook instead of
// reaching into the generic helper, so the endpoint-id contract stays
// pinned in one place. When the user asks for pagination on a new
// endpoint, add a wrapper here and a test that exercises the cursor
// chain on a real (or mocked) transport.

// ───────────────────────── /api/agents ─────────────────────────

export interface AgentRow extends Record<string, unknown> {
  slug: string;
}

/**
 * useAgentsPager drives the Agents / AgentPage roster paginator.
 * Cursor encodes (CreatedMS, slug) server-side; the rows are profiles
 * sorted DESC by CreatedMS. The hook returns the paged list, plus
 * `loadMore` for the next page and `hasMore` to drive a Load-50-more
 * footer in the view.
 */
export function useAgentsPager(limit: number = 100) {
  return useCursorPager<AgentRow>(
    "/api/agents",
    "profiles",
    "slug",
    limit,
  );
}

// ───────────────────────── /api/memory ─────────────────────────

export interface MemoryRecordRow extends Record<string, unknown> {
  id: string;
}

/**
 * useMemoryPager drives the Memory view's record list.
 */
export function useMemoryPager(limit: number = 100) {
  return useCursorPager<MemoryRecordRow>(
    "/api/memory",
    "records",
    "id",
    limit,
  );
}

// ─────────────────────── log endpoints (A2 Phase 1 + 2) ───────────────────────
//
// The 5 journal-backed log endpoints all page on the shared ms:seq cursor and
// expose a `seq` field on every row as the dedup id (plan/schedule use their
// natural correlation_id). itemsKey matches each handler's response envelope.

export interface LogRow extends Record<string, unknown> {
  seq: number;
}

/** useWebhookLogPager — /api/webhook_log (delivery attempts). */
export function useWebhookLogPager(limit: number = 50) {
  return useCursorPager<LogRow>("/api/webhook_log", "deliveries", "seq", limit);
}

/** useWardenLogPager — /api/warden_log (sandboxed executions). */
export function useWardenLogPager(limit: number = 50) {
  return useCursorPager<LogRow>("/api/warden_log", "executions", "seq", limit);
}

/** useNetguardLogPager — /api/netguard_log (blocked egress). */
export function useNetguardLogPager(limit: number = 50) {
  return useCursorPager<LogRow>("/api/netguard_log", "blocks", "seq", limit);
}

/** useWorldLogPager — /api/world_log (world-model ops). */
export function useWorldLogPager(limit: number = 50) {
  return useCursorPager<LogRow>("/api/world_log", "ops", "seq", limit);
}

/** useMemoryLogPager — /api/memory_log (memory write/forget ops). */
export function useMemoryLogPager(limit: number = 50) {
  return useCursorPager<LogRow>("/api/memory_log", "ops", "seq", limit);
}

/** ScheduleFiresRow — row shape for /api/schedule/fires. */
export interface ScheduleFiresRow extends Record<string, unknown> {
  correlation_id: string;
}

/** useScheduleFiresPager — /api/schedule/fires (cronjob firings; id = correlation_id). */
export function useScheduleFiresPager(limit: number = 50) {
  return useCursorPager<ScheduleFiresRow>("/api/schedule/fires", "fires", "correlation_id", limit);
}