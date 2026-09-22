// Approvals.tsx — full-page human-in-the-loop queue. Day 25 brought this back
// after Day 23 deleted @/views/Approvals. The page is the long-form version of
// the header bell (M913) — same /api/approvals endpoint, same decide action,
// but with filters and history rather than just a counter + dropdown.

import { useCallback, useEffect, useState } from "react";
import { ArrowRight, Check, History, ShieldAlert, ShieldCheck, X } from "lucide-react";
import { getJSON, postAction } from "@/app/api";
import { useEvents } from "@/app/events";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/app/utils";
import { approvalLabel, type PendingApproval } from "@/components/ApprovalsBell";

type Filter = "pending" | "granted" | "denied";

export default function Approvals() {
  const { subscribe } = useEvents();
  const ui = useUI();
  const [pending, setPending] = useState<PendingApproval[]>([]);
  const [history, setHistory] = useState<{ id: string; capability?: string; tool_name?: string; decision: string; ts: number; summary?: string }[]>([]);
  const [filter, setFilter] = useState<Filter>("pending");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const d = await getJSON<{
        pending?: PendingApproval[];
        history?: { id: string; capability?: string; tool_name?: string; decision: string; ts: number; summary?: string }[];
      }>("/api/approvals");
      setPending(d.pending || []);
      setHistory(d.history || []);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);

  useEffect(() => {
    void reload();
    const id = setInterval(reload, 15_000);
    return () => clearInterval(id);
  }, [reload]);

  useEffect(
    () =>
      subscribe((e) => {
        if ((e.kind || "").startsWith("approval.")) void reload();
      }),
    [subscribe, reload],
  );

  async function decide(id: string, decision: "grant" | "deny") {
    setBusy(id);
    try {
      await postAction("/api/decide", { id, decision });
      ui.toast(decision === "grant" ? "Granted" : "Denied", "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const historyFiltered = filter === "pending" ? [] : history.filter((h) => h.decision === filter);

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 p-4">
      <header className="flex flex-wrap items-baseline gap-3">
        <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground">
          <ShieldAlert className="size-5 text-accent" />
          Approvals
        </h2>
        <p className="text-xs text-muted">
          human-in-the-loop queue — capabilities the policy refuses to auto-decide
        </p>
        <div className="ml-auto">
          <Button size="sm" variant="ghost" onClick={() => void reload()}>
            Reload
          </Button>
        </div>
      </header>

      {error && (
        <p className="rounded-md border border-bad/40 bg-bad/10 px-3 py-2 text-xs text-bad">{error}</p>
      )}

      <section className="glass rounded-2xl p-4">
        <h3 className="flex items-center justify-between text-xs font-semibold uppercase tracking-wider text-muted">
          <span>Pending</span>
          <Badge variant={pending.length > 0 ? "warn" : "default"}>{pending.length}</Badge>
        </h3>
        {pending.length === 0 ? (
          <p className="mt-2 text-sm text-muted">No agent is waiting on your eyes. The policy engine is auto-deciding.</p>
        ) : (
          <ul className="mt-3 space-y-2">
            {pending.map((a) => (
              <li key={a.id} className="flex items-start gap-3 rounded-md border border-warn/30 bg-warn/5 px-3 py-2">
                <ShieldAlert className="mt-0.5 size-4 shrink-0 text-warn" />
                <div className="min-w-0 flex-1">
                  <div className="font-medium text-foreground/90">{approvalLabel(a)}</div>
                  {a.tool_name && a.capability && a.tool_name !== a.capability && (
                    <div className="mt-0.5 text-[11px] text-muted">tool: <span className="font-mono">{a.tool_name}</span></div>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-1.5">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => a.id && decide(a.id, "deny")}
                    disabled={!a.id || busy === a.id}
                    aria-label="Deny"
                  >
                    <X className="size-3.5" /> Deny
                  </Button>
                  <Button
                    size="sm"
                    variant="accent"
                    onClick={() => a.id && decide(a.id, "grant")}
                    disabled={!a.id || busy === a.id}
                    aria-label="Grant"
                  >
                    <Check className="size-3.5" /> Grant
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="glass rounded-2xl p-4">
        <div className="flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted">
            <History className="size-3.5" />
            History
          </h3>
          <div className="inline-flex rounded-md border border-border bg-card p-0.5" role="group" aria-label="Filter">
            {(["pending", "granted", "denied"] as Filter[]).map((f) => (
              <button
                key={f}
                type="button"
                aria-pressed={filter === f}
                onClick={() => setFilter(f)}
                className={cn(
                  "h-7 rounded px-2 text-[11px] font-medium transition-colors",
                  filter === f ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
                )}
              >
                {f}
              </button>
            ))}
          </div>
        </div>

        {filter === "pending" ? (
          <p className="mt-3 text-sm text-muted">Switch to <em>granted</em> or <em>denied</em> to see past decisions. Pending is the live queue above.</p>
        ) : historyFiltered.length === 0 ? (
          <p className="mt-3 text-sm text-muted">No {filter} decisions on record yet.</p>
        ) : (
          <ul className="mt-3 space-y-1.5">
            {historyFiltered.map((h) => (
              <li key={h.id} className="flex items-center gap-2 rounded-md border border-border/40 bg-card/30 px-3 py-1.5 text-sm">
                {h.decision === "granted" ? (
                  <ShieldCheck className="size-3.5 shrink-0 text-good" />
                ) : (
                  <X className="size-3.5 shrink-0 text-bad" />
                )}
                <span className="min-w-0 flex-1 truncate text-foreground/90">
                  {h.summary || h.capability || h.tool_name || "decision"}
                </span>
                <span className="font-mono text-[10px] uppercase tracking-wide text-muted">{h.decision}</span>
                <ArrowRight className="size-3 text-muted" />
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
