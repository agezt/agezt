import { useCallback, useEffect, useRef, useState } from "react";
import { ShieldCheck, ShieldAlert, Check, X, ArrowRight } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn } from "@/lib/utils";

// PendingApproval is one HITL request awaiting the operator's decision, as
// returned by /api/approvals.
export interface PendingApproval {
  id?: string;
  capability?: string;
  tool_name?: string;
  reason?: string;
}

// approvalLabel is the one-line summary shown for a pending request. Pure +
// unit-tested.
export function approvalLabel(a: PendingApproval): string {
  const cap = a.capability || a.tool_name || "capability";
  const why = (a.reason || "").trim();
  return why ? `${cap} — ${why}` : cap;
}

// ApprovalsBell is the global pending-approval indicator in the header (M913):
// a HITL request gated by the operator's ask-policy could otherwise sit unseen
// on the Approvals tab. This counts pending requests live (refetching on
// approval.* events), badges the header from EVERY view, and opens a dropdown to
// approve/deny right there — or jump to the full Approvals page.
export function ApprovalsBell() {
  const { subscribe } = useEvents();
  const [pending, setPending] = useState<PendingApproval[]>([]);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  const reload = useCallback(async () => {
    try {
      const d = await getJSON<{ pending?: PendingApproval[] }>("/api/approvals");
      setPending(d.pending || []);
    } catch {
      /* control plane momentarily unavailable — keep the last known list */
    }
  }, []);

  // Initial load + a slow poll as a safety net under the live event refetch.
  useEffect(() => {
    reload();
    const id = setInterval(reload, 15000);
    return () => clearInterval(id);
  }, [reload]);

  // Live: any approval lifecycle event (requested/granted/denied/timeout) changes
  // the pending set — refetch the authoritative list.
  useEffect(
    () =>
      subscribe((e) => {
        if ((e.kind || "").startsWith("approval.")) reload();
      }),
    [subscribe, reload],
  );

  // Close the dropdown on an outside click.
  useEffect(() => {
    if (!open) return;
    function onDown(ev: MouseEvent) {
      if (ref.current && !ref.current.contains(ev.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

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

  const n = pending.length;
  const has = n > 0;

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => (has ? setOpen((v) => !v) : (location.hash = "approvals"))}
        title={has ? `${n} awaiting approval` : "Approvals"}
        aria-label={has ? `${n} requests awaiting approval` : "Approvals"}
        className={cn(
          "relative inline-flex size-8 items-center justify-center rounded-md border transition-colors",
          has ? "border-border text-foreground hover:border-accent" : "border-border text-muted hover:text-foreground",
        )}
      >
        {has ? <ShieldAlert className="size-4 animate-pulse text-warn" /> : <ShieldCheck className="size-4" />}
        {has && (
          <span className="absolute -right-1.5 -top-1.5 inline-flex min-w-4 items-center justify-center rounded-full bg-warn px-1 text-xs font-semibold text-white tabular-nums">
            {n > 99 ? "99+" : n}
          </span>
        )}
      </button>

      {open && has && (
        <div className="absolute right-0 z-50 mt-2 w-80 rounded-lg border border-border bg-card p-2 shadow-lg">
          <div className="mb-1.5 flex items-center justify-between px-1 text-[11px] font-semibold uppercase tracking-normal text-muted">
            <span>{n} awaiting approval</span>
            <button
              onClick={() => {
                setOpen(false);
                location.hash = "approvals";
              }}
              className="inline-flex items-center gap-0.5 text-accent hover:underline"
            >
              Open <ArrowRight className="size-3" />
            </button>
          </div>
          <ul className="max-h-80 space-y-1 overflow-auto">
            {pending.map((a, i) => (
              <li key={a.id || i} className="rounded-md border border-border bg-panel/40 p-2">
                <div className="text-xs text-foreground/90">{approvalLabel(a)}</div>
                {a.id && (
                  <div className="mt-1.5 flex gap-1.5">
                    <button
                      onClick={() => decide(a.id!, "grant")}
                      disabled={busy === a.id}
                      className="inline-flex h-6 flex-1 items-center justify-center gap-1 rounded-md border border-good px-2 text-[11px] text-good transition-colors hover:bg-good hover:text-white disabled:opacity-50"
                    >
                      <Check className="size-3" /> Approve
                    </button>
                    <button
                      onClick={() => decide(a.id!, "deny")}
                      disabled={busy === a.id}
                      className="inline-flex h-6 flex-1 items-center justify-center gap-1 rounded-md border border-bad px-2 text-[11px] text-bad transition-colors hover:bg-bad hover:text-white disabled:opacity-50"
                    >
                      <X className="size-3" /> Deny
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
