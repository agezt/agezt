import { useEffect, useState } from "react";
import { getJSON } from "@/lib/api";
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
        const seen = new Set(cur.map((r) => String(r[idKey])));
        const merged = [...cur];
        for (const r of next) {
          const id = String(r[idKey]);
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

// ───────────────────────── /api/inbox ─────────────────────────

export interface InboxThreadRow extends Record<string, unknown> {
  correlation_id: string;
}

/**
 * useInboxPager drives the Inbox view's thread list. Optional channel
 * filter is forwarded on every request (loadMore preserves it).
 */
export function useInboxPager(channel?: string, limit: number = 50) {
  const params = channel ? { channel } : undefined;
  return useCursorPager<InboxThreadRow>(
    "/api/inbox",
    "threads",
    "correlation_id",
    limit,
    params,
  );
}

// ───────────────────────── /api/board ─────────────────────────

export interface BoardMessageRow extends Record<string, unknown> {
  id: string;
}

/**
 * useBoardPager drives the Board view's message list. Optional topic
 * filter is forwarded on every request.
 */
export function useBoardPager(topic?: string, limit: number = 50) {
  const params = topic ? { topic } : undefined;
  return useCursorPager<BoardMessageRow>(
    "/api/board",
    "messages",
    "id",
    limit,
    params,
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

// ───────────────────────── /api/agents/activity ─────────────────────────

export interface AgentActivityRow extends Record<string, unknown> {
  seq?: number | string;
}

/**
 * useAgentActivityPager drives the per-agent activity timeline (the
 * Audit panel's recent-events feed). Returns events sorted DESC by
 * journal seq; the cursor is a `<seq>` boundary.
 */
export function useAgentActivityPager(ref: string, limit: number = 50) {
  // The seq field on the response can be either a number or a string
  // depending on the wire format (number pre-json-unmarshal, string
  // after); idKey "seq" works for both because we coerce to string in
  // the dedup Set. ref rides on as a query param so loadMore preserves it.
  return useCursorPager<AgentActivityRow>(
    "/api/agents/activity",
    "activity",
    "seq",
    limit,
    { ref },
  );
}

// ───────────────────────── /api/agents/escalations ─────────────────────────

export interface AgentEscalationRow extends Record<string, unknown> {
  message_id: string;
}

/**
 * useAgentEscalationsPager drives the per-agent open escalations list.
 */
export function useAgentEscalationsPager(ref: string, limit: number = 50) {
  return useCursorPager<AgentEscalationRow>(
    "/api/agents/escalations",
    "escalations",
    "message_id",
    limit,
    { ref },
  );
}