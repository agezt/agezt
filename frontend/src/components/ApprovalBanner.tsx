import { useCallback, useEffect, useState } from "react";
import { ShieldAlert, ShieldCheck, Check, X, ArrowRight, Clock } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

// PendingApproval is one HITL request awaiting the operator's decision, as
// returned by /api/approvals.
export interface PendingApproval {
  id?: string;
  capability?: string;
  tool_name?: string;
  reason?: string;
  ts_unix_ms?: number;
}

// approvalLabel is the one-line summary shown for a pending request.
export function approvalLabel(a: PendingApproval): string {
  const cap = a.capability || a.tool_name || "capability";
  const why = (a.reason || "").trim();
  return why ? `${cap} — ${why}` : cap;
}

// ApprovalBanner is an in-chat sticky panel that appears whenever there are
// pending HITL approvals. Unlike ApprovalsBell (a header icon), this is embedded
// directly in the chat view so it is impossible to miss while composing or
// reading — eliminating the "didn't see the approval" problem.
//
// Security note: the banner is READ-ONLY for display; all approval decisions are
// routed through /api/decide which is gated by the daemon's own auth layer.
// Prompt injection cannot fake approval outcomes because the approval IDs are
// server-generated UUIDs that cannot be predicted or forged by the agent.
export function ApprovalBanner({ className }: { className?: string }) {
  const { subscribe } = useEvents();
  const [pending, setPending] = useState<PendingApproval[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [dismissed, setDismissed] = useState(false);

  const reload = useCallback(async () => {
    try {
      const d = await getJSON<{ pending?: PendingApproval[] }>("/api/approvals");
      const items = d.pending || [];
      setPending(items);
      // Auto-show the banner again if new approvals appear after being dismissed
      if (items.length > 0) setDismissed(false);
    } catch {
      /* control plane momentarily unavailable — keep the last known list */
    }
  }, []);

  // Initial load + a slow poll as a safety net.
  useEffect(() => {
    reload();
    const id = setInterval(reload, 8000);
    return () => clearInterval(id);
  }, [reload]);

  // Live: any approval lifecycle event changes the pending set.
  useEffect(
    () =>
      subscribe((e) => {
        if ((e.kind || "").startsWith("approval.")) reload();
      }),
    [subscribe, reload],
  );

  async function decide(id: string, decision: "grant" | "deny") {
    setBusy(id);
    try {
      await postAction("/api/decide", { id, decision });
      setPending((prev) => prev.filter((a) => a.id !== id));
    } catch {
      /* surfaced by the row staying; a reload will reconcile */
    } finally {
      setBusy(null);
      reload();
    }
  }

  if (pending.length === 0 || dismissed) return null;

  const n = pending.length;

  return (
    <div
      className={cn(
        "relative rounded-lg border border-warn/40 bg-warn/10 p-3",
        className,
      )}
      role="alert"
      aria-live="polite"
    >
      {/* Header */}
      <div className="mb-2 flex items-center gap-2">
        <ShieldAlert className="size-4 shrink-0 animate-pulse text-warn" />
        <span className="text-sm font-semibold text-warn">
          {n} approval{n !== 1 ? "s" : ""} needed
        </span>
        <span className="ml-auto text-[11px] text-muted">
          Agent is paused until you decide
        </span>
        <button
          onClick={() => setDismissed(true)}
          title="Dismiss (approvals still pending — check the bell icon)"
          className="ml-1 shrink-0 rounded p-0.5 text-muted transition-colors hover:text-foreground"
        >
          <X className="size-3.5" />
        </button>
      </div>

      {/* Approval list */}
      <ul className="space-y-2">
        {pending.map((a, i) => (
          <li
            key={a.id || i}
            className="flex flex-col gap-1.5 rounded-md border border-border/60 bg-panel/60 p-2.5"
          >
            <div className="flex items-start gap-2">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5 flex-wrap">
                  <Badge variant="warn" className="text-[11px]">
                    {a.capability || a.tool_name || "?"}
                  </Badge>
                  {a.ts_unix_ms && (
                    <span className="inline-flex items-center gap-0.5 text-[10px] text-muted">
                      <Clock className="size-2.5" />
                      {fmtTime(a.ts_unix_ms)}
                    </span>
                  )}
                </div>
                {a.reason && (
                  <p className="mt-0.5 text-xs text-muted">{a.reason}</p>
                )}
              </div>
            </div>

            {/* Approve / Deny buttons */}
            {a.id && (
              <div className="flex gap-2">
                <button
                  onClick={() => decide(a.id!, "grant")}
                  disabled={busy === a.id}
                  className={cn(
                    "inline-flex h-7 flex-1 items-center justify-center gap-1.5 rounded-md border border-good px-2 text-xs font-medium transition-colors",
                    "bg-good/10 text-good hover:bg-good hover:text-white disabled:opacity-50",
                  )}
                >
                  <Check className="size-3.5" />
                  Approve
                </button>
                <button
                  onClick={() => decide(a.id!, "deny")}
                  disabled={busy === a.id}
                  className={cn(
                    "inline-flex h-7 flex-1 items-center justify-center gap-1.5 rounded-md border border-bad px-2 text-xs font-medium transition-colors",
                    "bg-bad/10 text-bad hover:bg-bad hover:text-white disabled:opacity-50",
                  )}
                >
                  <X className="size-3.5" />
                  Deny
                </button>
              </div>
            )}
          </li>
        ))}
      </ul>

      {/* Footer hint */}
      <div className="mt-2 flex items-center justify-between text-[10px] text-muted">
        <span>
          Decisions are logged in the{" "}
          <a
            href="#approvals"
            className="inline-flex items-center gap-0.5 text-accent hover:underline"
            onClick={() => setDismissed(true)}
          >
            Approvals page <ArrowRight className="size-2.5" />
          </a>
        </span>
        <span className="flex items-center gap-1">
          <ShieldCheck className="size-2.5" />
          Secured by daemon auth — prompt injection cannot forge approvals
        </span>
      </div>
    </div>
  );
}
