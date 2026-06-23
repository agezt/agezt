import { useEffect, useState } from "react";
import { BarChart3, RefreshCw, Wallet, ListTree, Activity, Timer, Repeat } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money, pct } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/ui/page-header";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { CollapsibleSection } from "@/components/ui/collapsible-section";
import { SpendArea, BarList, OutcomeBar } from "@/components/Charts";
import { computeInsights, type RunRow as InsightsRunRow } from "@/lib/insights";

type RunRow = InsightsRunRow & { intent?: string };

function dur(ms: number): string {
  if (ms <= 0) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

// Insights is the analytics cockpit: spend over time, per-model breakdown, run
// outcomes, and throughput — all derived client-side from the runs list, so it
// reflects the journal without any new backend. Refreshes on a timer and is
// nudged when a run finishes.
export function Insights() {
  const { events } = useEvents();
  const [runs, setRuns] = useState<RunRow[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ runs?: RunRow[] }>("/api/runs");
      setRuns(d.runs || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 8000);
    return () => clearInterval(id);
  }, []);

  // Nudge on terminal events.
  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "task.completed" || head === "task.failed") reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const ins = runs ? computeInsights(runs) : null;

  return (
    <div className="space-y-4">
      <PageHeader
        icon={BarChart3}
        title="Insights"
        actions={
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
          </Button>
        }
      />

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !ins ? (
        <SkeletonList count={3} lines={2} />
      ) : ins.total === 0 ? (
        <Muted>no runs yet — start one from Chat or the CLI</Muted>
      ) : (
        <>
          <MetricGrid cols="grid-cols-2 sm:grid-cols-3 lg:grid-cols-5">
            <MetricWidget icon={ListTree} label="Runs" value={ins.total} tone="muted" />
            <MetricWidget icon={Wallet} label="Total spend" value={money(ins.totalSpentMc)} tone="warn" />
            <MetricWidget icon={Activity} label="Success" value={pct(ins.successRate, ins.completed + ins.failed)} tone="good" />
            <MetricWidget icon={Timer} label="Avg duration" value={dur(ins.avgDurationMs)} tone="muted" />
            <MetricWidget icon={Repeat} label="Avg iters" value={ins.avgIters ? ins.avgIters.toFixed(1) : "—"} tone="muted" />
          </MetricGrid>

          <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
            <CollapsibleSection icon={Wallet} title="Cumulative spend" tone="muted">
              <SpendArea values={ins.spend.map((p) => p.cum)} />
              <div className="mt-1 flex justify-between text-xs text-muted">
                <span>{ins.spend.length} runs</span>
                <span className="tabular-nums">peak {money(ins.totalSpentMc)}</span>
              </div>
            </CollapsibleSection>

            <CollapsibleSection icon={Activity} title="Run outcomes" tone="muted">
              <OutcomeBar completed={ins.completed} failed={ins.failed} running={ins.running} />
            </CollapsibleSection>
          </div>

          <CollapsibleSection icon={BarChart3} title="Spend by model" tone="muted">
            <BarList
              rows={ins.byModel.map((m) => ({
                label: m.model,
                value: m.spentMc,
                sub: `${money(m.spentMc)} · ${m.runs} run${m.runs === 1 ? "" : "s"}`,
              }))}
            />
            {ins.byModel.length > 0 && (
              <div className="mt-3 grid grid-cols-2 gap-1.5 text-[11px] text-muted sm:grid-cols-3">
                {ins.byModel.slice(0, 6).map((m) => (
                  <div key={m.model} className="rounded-md border border-border/60 bg-card/40 px-2 py-1.5">
                    <div className="truncate font-medium text-foreground/80">{m.model}</div>
                    <div>avg {money(m.avgSpentMc)}/run</div>
                    <div>{m.avgIters ? `${m.avgIters.toFixed(1)} iters/run` : "no iters"}</div>
                  </div>
                ))}
              </div>
            )}
          </CollapsibleSection>

          <CollapsibleSection icon={ListTree} title="Recent runs" tone="muted">
            <ul className="divide-y divide-border/60">
              {(runs || []).slice(0, 8).map((r) => (
                <li key={r.correlation_id} className="flex items-center gap-2 py-1.5 text-xs">
                  <span className={cn("shrink-0 font-medium", r.status === "completed" ? "text-good" : r.status === "failed" ? "text-bad" : r.status === "running" ? "text-accent" : "text-muted")}>
                    {r.status || "—"}
                  </span>
                  <span className="min-w-0 flex-1 truncate" title={r.intent || r.correlation_id}>{r.intent || r.correlation_id}</span>
                  {r.model && <span className="shrink-0 truncate font-mono text-xs text-muted" title={r.model}>{r.model}</span>}
                  <span className="shrink-0 tabular-nums text-muted">{money(r.spent_mc ?? 0)}</span>
                </li>
              ))}
            </ul>
          </CollapsibleSection>
        </>
      )}
    </div>
  );
}
