import { useState } from "react";
import { RefreshCw, Wallet, Check, Infinity as InfinityIcon } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { postAction } from "@/lib/api";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { Ring, BarRow } from "@/components/Widgets";

interface BudgetData {
  utc_date?: string;
  spent_mc?: number;
  ceiling_mc?: number;
  strict_pricing?: boolean;
  per_task?: { task_type: string; spent_mc: number; ceiling_mc: number }[];
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
    <Card className="h-full">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Wallet className="size-4 text-accent" /> Budget
        </CardTitle>
        <Button variant="ghost" size="icon" className="ml-auto" onClick={reload} title="Refresh">
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </CardHeader>
      <CardBody className="space-y-5">
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

            {/* Runtime ceiling control — the "ayarla" knob */}
            <div className="rounded-lg border border-border bg-card p-3">
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted">
                Adjust daily ceiling
              </div>
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
                {/* Quick presets */}
                <div className="ml-auto flex items-center gap-1">
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
              </div>
              {note && <div className={cn("mt-2 text-xs", noteBad ? "text-bad" : "text-good")}>{note}</div>}
            </div>

            {/* Per-task-type caps */}
            {perTask.length > 0 && (
              <div>
                <div className="mb-2 text-xs uppercase tracking-wider text-muted">Per task type</div>
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
          <Muted>loading…</Muted>
        )}
      </CardBody>
    </Card>
  );
}
