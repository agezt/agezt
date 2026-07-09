import { useEffect, useState, type ReactNode } from "react";
import { BarChart3, RefreshCw, Wallet, ListTree, Activity, Timer, Repeat } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money, pct } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Disclosure } from "@/components/ui/disclosure";
import { Page } from "@/components/ui/page";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { SpendArea, BarList, OutcomeBar } from "@/components/Charts";
import { computeInsights, type RunRow as InsightsRunRow } from "@/lib/insights";

type RunRow = InsightsRunRow & { intent?: string };

// INSIGHTS_RUN_LIMIT bounds the /api/runs fetch — analytics cover the most
// recent window of runs rather than the entire journal.
const INSIGHTS_RUN_LIMIT = 300;

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
      // Bounded fetch: analytics derive from the most recent window of runs so
      // the page stays fast on daemons with a long run history.
      const d = await getJSON<{ runs?: RunRow[] }>("/api/runs", { limit: String(INSIGHTS_RUN_LIMIT) });
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
    <Page
      icon={BarChart3}
      title="Insights"
      description={`derived from the last ${INSIGHTS_RUN_LIMIT} runs`}
      width="wide"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      }
    >
      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !ins ? (
        <SkeletonList count={3} lines={2} />
      ) : ins.total === 0 ? (
        <EmptyState
          icon={BarChart3}
          title="No runs yet"
          hint="Analytics appear once there are runs — start one from Chat or the CLI."
        />
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
            <InsightPanel
              icon={Wallet}
              title="Cumulative spend"
              status={`${ins.spend.length} runs · peak ${money(ins.totalSpentMc)}`}
              tone={ins.totalSpentMc > 0 ? "warn" : "muted"}
            >
              <SpendArea values={ins.spend.map((p) => p.cum)} />
              <div className="mt-1 flex justify-between text-xs text-muted">
                <span>{ins.spend.length} runs</span>
                <span className="tabular-nums">peak {money(ins.totalSpentMc)}</span>
              </div>
            </InsightPanel>

            <InsightPanel
              icon={Activity}
              title="Run outcomes"
              status={`${ins.completed} ok · ${ins.failed} failed · ${ins.running} running`}
              tone={ins.failed > 0 ? "warn" : ins.running > 0 ? "accent" : "good"}
            >
              <OutcomeBar completed={ins.completed} failed={ins.failed} running={ins.running} />
            </InsightPanel>
          </div>

          <InsightPanel
            icon={BarChart3}
            title="Spend by model"
            status={ins.byModel.length ? `${ins.byModel.length} models` : "no model spend"}
            tone={ins.byModel.length ? "accent" : "muted"}
          >
            <BarList
              rows={ins.byModel.map((m) => ({
                label: m.model,
                value: m.spentMc,
                sub: `${money(m.spentMc)} · ${m.runs} run${m.runs === 1 ? "" : "s"}`,
              }))}
            />
            {ins.byModel.length > 0 && (
              <Disclosure
                className="mt-2"
                summary={<span className="text-xs text-muted">Per-model averages</span>}
              >
                <div className="grid grid-cols-2 gap-1.5 text-[11px] text-muted sm:grid-cols-3">
                  {ins.byModel.slice(0, 6).map((m) => (
                    <div key={m.model} className="rounded-md border border-border/60 bg-card/40 px-2 py-1.5">
                      <div className="truncate font-medium text-foreground/80">{m.model}</div>
                      <div>avg {money(m.avgSpentMc)}/run</div>
                      <div>{m.avgIters ? `${m.avgIters.toFixed(1)} iters/run` : "no iters"}</div>
                    </div>
                  ))}
                </div>
              </Disclosure>
            )}
          </InsightPanel>

          <InsightPanel
            icon={ListTree}
            title="Recent runs"
            status={`${Math.min((runs || []).length, 8)} shown`}
            tone={(runs || []).some((r) => r.status === "running") ? "accent" : "muted"}
          >
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
          </InsightPanel>
        </>
      )}
    </Page>
  );
}

function InsightPanel({
  icon: Icon,
  title,
  status,
  tone,
  children,
}: {
  icon: typeof BarChart3;
  title: string;
  status: string;
  tone: "accent" | "warn" | "bad" | "good" | "muted";
  children: ReactNode;
}) {
  const toneCls: Record<typeof tone, string> = {
    accent: "border-accent/35 bg-accent/5 text-accent",
    warn: "border-warn/35 bg-warn/5 text-warn",
    bad: "border-bad/35 bg-bad/5 text-bad",
    good: "border-good/35 bg-good/5 text-good",
    muted: "border-border bg-panel text-muted",
  };
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className={cn("grid size-8 place-items-center rounded-lg border", toneCls[tone])}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}
