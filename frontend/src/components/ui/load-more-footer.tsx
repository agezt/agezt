import { Loader2 } from "lucide-react";

/**
 * LoadMoreFooter — the shared "Load N more / end of list" footer used by
 * every cursor-paginated view (Runs, and the 12 log views wired to the
 * use*LogPager hooks in lib/cursorPager). It renders:
 *
 *   • a "Load N more" button while the pager reports `hasMore`,
 *   • an inline spinner + "loading…" label while a page is in flight,
 *   • a non-blocking error line if the last loadMore() rejected,
 *   • a stable "— end of <label> —" terminal marker once drained.
 *
 * The props mirror the tail of `useCursorPager`'s return value, so a view
 * can spread the hook result straight in:
 *
 *   const { paged, hasMore, loadingMore, moreError, loadMore } = useToolLogPager();
 *   …
 *   <LoadMoreFooter
 *     hasMore={hasMore}
 *     loadingMore={loadingMore}
 *     moreError={moreError}
 *     onLoadMore={loadMore}
 *     pageSize={50}
 *     label="tool log"
 *   />
 *
 * Keeping this in one place means the footer's markup, a11y attributes
 * (aria-busy / role="alert"), and copy stay consistent across all views.
 */
export interface LoadMoreFooterProps {
  hasMore: boolean;
  loadingMore: boolean;
  moreError?: string | null;
  onLoadMore: () => void;
  /** Page size shown in the button copy ("Load 50 more"). Defaults to 50. */
  pageSize?: number;
  /** Noun shown in the terminal marker ("— end of tool log —"). */
  label?: string;
}

export function LoadMoreFooter({
  hasMore,
  loadingMore,
  moreError,
  onLoadMore,
  pageSize = 50,
  label = "list",
}: LoadMoreFooterProps) {
  return (
    <div className="flex flex-col items-center gap-1 border-t border-border/50 px-3 py-3">
      {hasMore ? (
        <>
          <button
            type="button"
            onClick={onLoadMore}
            disabled={loadingMore}
            aria-busy={loadingMore || undefined}
            className="inline-flex items-center gap-2 rounded-md border border-border bg-card px-3 py-1.5 text-xs text-foreground hover:border-accent disabled:opacity-50"
          >
            {loadingMore ? (
              <>
                <Loader2 className="size-3 animate-spin" /> loading…
              </>
            ) : (
              <>Load {pageSize} more</>
            )}
          </button>
          {moreError && (
            <p className="text-[11px] text-bad" role="alert">
              couldn't load more: {moreError}
            </p>
          )}
        </>
      ) : (
        <p className="text-xs text-muted">— end of {label} —</p>
      )}
    </div>
  );
}
