import { type LucideIcon } from "lucide-react";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { Muted } from "@/components/JsonView";
import { fmtDateTime } from "@/lib/utils";

interface LogHistoryPanelProps<T extends Record<string, unknown>> {
  icon: LucideIcon;
  title: string;
  rows: T[];
  loading: boolean;
  loadMore: () => Promise<void>;
  loadingMore: boolean;
  moreError: string | null;
  hasMore: boolean;
  pageSize: number;
  renderRow: (row: T, index: number) => React.ReactNode;
}

/**
 * LogHistoryPanel renders a cursor-paginated log list with a LoadMoreFooter.
 * It standardises the layout (header, list, footer) so each consuming view
 * only needs to provide a row renderer. Used by the 6 log-endpoint views.
 */
export function LogHistoryPanel<T extends Record<string, unknown> & { seq: number }>({
  icon: Icon,
  title,
  rows,
  loading,
  loadMore,
  loadingMore,
  moreError,
  hasMore,
  pageSize,
  renderRow,
}: LogHistoryPanelProps<T>) {
  return (
    <div className="glass rounded-xl p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Icon className="size-3.5" /> {title}
        {!loading && <span className="ml-1 normal-case text-muted/70">({rows.length})</span>}
      </div>
      {loading && rows.length === 0 ? (
        <Muted>loading…</Muted>
      ) : rows.length === 0 && !hasMore ? (
        <Muted>no entries yet</Muted>
      ) : (
        <>
          {rows.length > 0 && (
            <ul className="space-y-1">
              {rows.map((row, i) => (
                <li
                  key={row.seq ?? i}
                  className="flex items-center gap-2 border-b border-border/40 py-1 text-xs last:border-0"
                >
                  {renderRow(row, i)}
                  {row.ts_unix_ms != null && (
                    <span className="ml-auto shrink-0 text-muted">{fmtDateTime(row.ts_unix_ms as number)}</span>
                  )}
                </li>
              ))}
            </ul>
          )}
          <LoadMoreFooter
            hasMore={hasMore}
            loadingMore={loadingMore}
            moreError={moreError}
            onLoadMore={loadMore}
            pageSize={pageSize}
            label={title.toLowerCase()}
          />
        </>
      )}
    </div>
  );
}