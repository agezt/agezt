import { Brain, ArrowUpRight, Share2 } from "lucide-react";
import { fmtAgo, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { type MemoryRecord } from "@/lib/agentdetail";

export function MemoryTab({
  records,
  scope,
  busy,
  onAction,
  onManage,
}: {
  records: MemoryRecord[] | null;
  scope: string;
  busy: boolean;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
  ) => void;
  onManage: (view: string) => void;
}) {
  if (!records) return <SkeletonList count={4} lines={2} />;
  if (records.length === 0)
    return (
      <EmptyState
        icon={Brain}
        title="No private memory yet"
        hint={`Records this agent writes to its private scope (${scope}) appear here. Shared knowledge lives in the Memory page.`}
      />
    );
  return (
    <div className="space-y-2">
      <div className="text-xs uppercase tracking-normal text-muted">
        private to scope{" "}
        <span className="font-mono text-foreground/80">{scope}</span> ·{" "}
        {records.length} record(s)
      </div>
      <ul className="space-y-2">
        {records.map((r) => (
          <li
            key={r.id}
            className="rounded-lg border border-border bg-panel/30 p-2.5"
          >
            <div className="flex flex-wrap items-center gap-2">
              {r.type && (
                <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-accent">
                  {r.type}
                </span>
              )}
              {r.subject && (
                <span className="text-xs font-medium">{r.subject}</span>
              )}
              {typeof r.confidence === "number" && (
                <span className="text-xs text-muted">
                  conf {r.confidence.toFixed(2)}
                </span>
              )}
              <span className="ml-auto flex items-center gap-2">
                <span className="font-mono text-xs text-muted">
                  {fmtAgo(r.last_seen_ms || r.created_ms)}
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy}
                  title="Promote to shared memory"
                  onClick={() =>
                    r.id &&
                    onAction(
                      "/api/memory/promote",
                      { id: r.id },
                      "promoted to shared",
                    )
                  }
                >
                  <Share2 className="size-3.5" />
                </Button>
              </span>
            </div>
            {r.content && (
              <div className="mt-1 whitespace-pre-wrap text-[11px] text-muted">
                {clip(r.content, 280)}
              </div>
            )}
          </li>
        ))}
      </ul>
      <Button variant="ghost" size="sm" onClick={() => onManage("memory")}>
        Open Memory <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

