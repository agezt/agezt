import { useEffect, useState } from "react";
import { BarChart3, RefreshCw, Wallet, ListTree, Activity, Timer, Repeat } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { money, pct } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { SpendArea, BarList, OutcomeBar } from "@/components/Charts";
import { computeInsights, type RunRow } from "@/lib/insights";

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
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <BarChart3 className="size-4 text-accent" /> Insights
        </h2>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !ins ? (
        <SkeletonList count={3} lines={2} />
      ) : ins.total === 0 ? (
        <Muted>no runs yet — start one from Chat or the CLI</Muted>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
            <Tile icon={ListTree} label="runs" value={ins.total} />
            <Tile icon={Wallet} label="total spend" value={money(ins.totalSpentMc)} />
            <Tile icon={Activity} label="success" value={pct(ins.successRate, ins.completed + ins.failed)} />
            <Tile icon={Timer} label="avg duration" value={dur(ins.avgDurationMs)} />
            <Tile icon={Repeat} label="avg iters" value={ins.avgIters ? ins.avgIters.toFixed(1) : "—"} />
          </div>

          <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
            <Card title="Cumulative spend" icon={Wallet}>
              <SpendArea values={ins.spend.map((p) => p.cum)} />
              <div className="mt-1 flex justify-between text-xs text-muted">
                <span>{ins.spend.length} runs</span>
                <span className="tabular-nums">peak {money(ins.totalSpentMc)}</span>
              </div>
            </Card>

            <Card title="Run outcomes" icon={Activity}>
              <OutcomeBar completed={ins.completed} failed={ins.failed} running={ins.running} />
            </Card>
          </div>

          <Card title="Spend by model" icon={BarChart3}>
            <BarList
              rows={ins.byModel.map((m) => ({
                label: m.model,
                value: m.spentMc,
                sub: `${money(m.spentMc)} · ${m.runs} run${m.runs === 1 ? "" : "s"}`,
              }))}
            />
          </Card>
        </>
      )}
    </div>
  );
}

function Tile({ icon: Icon, label, value }: { icon: typeof Activity; label: string; value: number | string }) {
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2.5">
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <Icon className="size-3.5" /> {label}
      </div>
      <div className="mt-1 text-xl font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function Card({ title, icon: Icon, children }: { title: string; icon: typeof Activity; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      {children}
    </div>
  );
}
