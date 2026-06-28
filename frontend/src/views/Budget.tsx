import { useState, type ReactNode } from "react";
import { RefreshCw, Wallet, Check, Infinity as InfinityIcon, SlidersHorizontal, X } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { postAction } from "@/lib/api";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/ui/page-header";
import { Ring, BarRow } from "@/components/Widgets";

interface BudgetData {
  utc_date?: string;
  spent_mc?: number;
  ceiling_mc?: number;
  strict_pricing?: boolean;
  per_task?: { task_type: string; spent_mc: number; ceiling_mc: number }[];
}

// projectedDailySpend extrapolates today's spend to UTC end-of-day, "at this
// pace" — spend / fraction-of-the-UTC-day-elapsed. The budget resets at UTC
// midnight (utc_date), and the Unix epoch is UTC-aligned, so nowMs % dayMs is ms
// since midnight. Returns null too early in the day (< ~1h) where the
// extrapolation is meaningless noise. Pure + unit-tested (M920).
export function projectedDailySpend(spentMc: number, nowMs: number): number | null {
  const dayMs = 24 * 60 * 60 * 1000;
  const frac = (nowMs % dayMs) / dayMs;
  if (frac < 0.04) return null; // < ~58 min into the day — too little signal
  return Math.round(spentMc / frac);
}

// Budget is the spend cockpit: it SHOWS today's spend against the daily ceiling
// and lets the operator ADJUST that ceiling at runtime (M607) — the "ayarla"
// knob. Setting a new dollar figure posts /api/budget_set; "Unlimited" clears
// the cap. The panel re-reads the post-set snapshot so the gauge updates live.
export function Budget() {
  const { data, error, loading, reload } = usePanel<BudgetData>("/api/budget");
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const [noteBad, setNoteBad] = useState(false);
  const [adjustOpen, setAdjustOpen] = useState(false);

  const spent = data?.spent_mc ?? 0;
  const ceiling = data?.ceiling_mc ?? 0;
  const pctUsed = ceiling > 0 ? Math.min(100, (spent / ceiling) * 100) : 0;
  const perTask = data?.per_task ?? [];

  async function setCeiling(dollars: number) {
    setBusy(true);
    setNote(null);
    try {
      // ceiling_mc is microcents (1 USD = 1e9). Round to avoid float drift.
      const ceiling_mc = String(Math.max(0, Math.round(dollars * 1e9)));
      await postAction("/api/budget_set", { ceiling_mc });
      setDraft("");
      setAdjustOpen(false);
      setNoteBad(false);
      setNote(dollars > 0 ? `Ceiling set to $${dollars.toFixed(2)}/day` : "Ceiling removed — unlimited");
      reload();
    } catch (e) {
      setNoteBad(true);
      setNote((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  function submitDraft() {
    const v = parseFloat(draft.trim().replace(/^\$/, ""));
    if (!isFinite(v) || v < 0) {
      setNoteBad(true);
      setNote("Enter a dollar amount like 25 or 1.50 (0 = unlimited)");
      return;
    }
    setCeiling(v);
  }

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Wallet}
        title="Budget"
        description="Track today's spend against the daily ceiling and adjust the cap at runtime."
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={() => setAdjustOpen(true)} title="Adjust daily ceiling">
              <SlidersHorizontal className="size-3.5" /> Adjust
            </Button>
            <Button variant="ghost" size="icon" onClick={reload} title="Refresh">
              <RefreshCw className={loading ? "animate-spin" : ""} />
            </Button>
          </>
        }
      />
      <div className="glass rounded-xl p-4 space-y-5">
        {error ? (
          <ErrorText>{error}</ErrorText>
        ) : data ? (
          <>
            {/* Spend gauge */}
            <div className="flex items-center gap-5">
              <Ring
                pct={ceiling > 0 ? pctUsed : 0}
                center={ceiling > 0 ? `${Math.round(pctUsed)}%` : "∞"}
                label={ceiling > 0 ? "of ceiling" : "no ceiling"}
                tone={ceiling === 0 ? "muted" : pctUsed > 85 ? "bad" : pctUsed > 60 ? "warn" : "good"}
              />
              <div className="min-w-0">
                <div className="flex items-baseline gap-2">
                  <span className="text-3xl font-semibold tabular-nums">{money(spent)}</span>
                  <span className="text-sm text-muted">
                    {ceiling > 0 ? `of ${money(ceiling)} daily ceiling` : "spent today · no ceiling"}
                  </span>
                </div>
                <div className="mt-1 text-xs text-muted">
                  as of {data.utc_date} UTC ·{" "}
                  {data.strict_pricing
                    ? "strict pricing (unpriced models refused)"
                    : "lax pricing (unpriced charged $0)"}
                </div>
              </div>
            </div>

            {/* Forecast (M920): where today's spend lands at the current pace. */}
            {(() => {
              const projected = projectedDailySpend(spent, Date.now());
              if (projected == null || spent <= 0) return null;
              const over = ceiling > 0 && projected > ceiling;
              return (
                <div className={cn("rounded-xl p-3", over ? "border border-bad/50 bg-bad/5" : "glass")}>
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="text-xs font-semibold uppercase tracking-normal text-muted">
                      Projected today · at this pace
                    </span>
                    <span className={cn("text-lg font-semibold tabular-nums", over ? "text-bad" : "text-foreground")}>
                      {money(projected)}
                    </span>
                  </div>
                  {ceiling > 0 && (
                    <div className="mt-1 text-xs text-muted">
                      {over
                        ? `On track to exceed the ${money(ceiling)} ceiling — the daily cap halts spend before then.`
                        : `Comfortably within the ${money(ceiling)} daily ceiling at the current rate.`}
                    </div>
                  )}
                </div>
              );
            })()}

            <div className="glass flex flex-wrap items-center gap-2 rounded-xl p-3">
              <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
                <SlidersHorizontal className="size-4" />
              </span>
              <div className="min-w-0 flex-1">
                <div className="text-xs font-semibold uppercase tracking-normal text-muted">Daily ceiling</div>
                <div className="text-sm text-foreground">{ceiling > 0 ? money(ceiling) : "Unlimited"}</div>
              </div>
              <Button size="sm" onClick={() => setAdjustOpen(true)}>
                Adjust
              </Button>
              {note && <div className={cn("basis-full text-xs", noteBad ? "text-bad" : "text-good")}>{note}</div>}
            </div>

            {adjustOpen && (
              <BudgetModal title="Adjust daily ceiling" onClose={() => setAdjustOpen(false)}>
                <div className="flex flex-wrap items-center gap-2">
                  <div className="relative">
                    <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-sm text-muted">
                      $
                    </span>
                    <input
                      type="number"
                      min={0}
                      step="0.5"
                      inputMode="decimal"
                      value={draft}
                      onChange={(e) => setDraft(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && submitDraft()}
                      placeholder={ceiling > 0 ? (ceiling / 1e9).toFixed(2) : "0.00"}
                      disabled={busy}
                      aria-label="Daily ceiling dollars"
                      className="h-9 w-32 rounded-md border border-border bg-panel pl-6 pr-2 text-sm tabular-nums outline-none focus:border-accent disabled:opacity-50"
                    />
                  </div>
                  <Button size="sm" onClick={submitDraft} disabled={busy || draft.trim() === ""}>
                    <Check className="size-4" /> Set ceiling
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setCeiling(0)}
                    disabled={busy}
                    title="Remove the ceiling (unlimited spend)"
                  >
                    <InfinityIcon className="size-4" /> Unlimited
                  </Button>
                </div>
                <div className="flex flex-wrap items-center gap-1">
                  {[5, 20, 50, 100].map((d) => (
                    <button
                      key={d}
                      onClick={() => setCeiling(d)}
                      disabled={busy}
                      className="rounded-md border border-border px-2 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-foreground disabled:opacity-50"
                    >
                      ${d}
                    </button>
                  ))}
                </div>
              </BudgetModal>
            )}

            {/* Per-task-type caps */}
            {perTask.length > 0 && (
              <div>
                <div className="mb-2 text-xs uppercase tracking-normal text-muted">Per task type</div>
                <div className="space-y-2">
                  {perTask.map((r) => {
                    const used = r.ceiling_mc > 0 ? (r.spent_mc / r.ceiling_mc) * 100 : 0;
                    const tone = r.ceiling_mc === 0 ? "muted" : used > 85 ? "bad" : used > 60 ? "warn" : "good";
                    return (
                      <BarRow
                        key={r.task_type}
                        label={r.task_type}
                        value={r.spent_mc}
                        max={r.ceiling_mc > 0 ? r.ceiling_mc : Math.max(r.spent_mc, 1)}
                        display={
                          r.ceiling_mc > 0
                            ? `${money(r.spent_mc)} / ${money(r.ceiling_mc)}`
                            : money(r.spent_mc)
                        }
                        tone={tone}
                      />
                    );
                  })}
                </div>
              </div>
            )}
          </>
        ) : (
          <SkeletonList count={3} lines={2} />
        )}
      </div>
    </div>
  );
}

function BudgetModal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Wallet className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Set a runtime cap; zero means unlimited.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close budget modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="flex min-h-0 flex-col gap-3 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}
