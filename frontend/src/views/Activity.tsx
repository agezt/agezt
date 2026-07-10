import { useEffect, useMemo, useState } from "react";
import {
  Activity as ActivityIcon,
  RefreshCw,
  Cpu,
  CheckCircle2,
  XCircle,
  CornerDownRight,
  Ban,
  ChevronRight,
  ChevronDown,
  LifeBuoy,
} from "lucide-react";
import { cn, fmtTime } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { useUI } from "@/components/ui/feedback";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Page } from "@/components/ui/page";
import { EmptyState } from "@/components/ui/empty";
import { SkeletonList } from "@/components/ui/skeleton";
import { Disclosure } from "@/components/ui/disclosure";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { RunDetailLoader } from "@/components/RunDetail";
import { DoctorIncidentTrees } from "@/components/DoctorIncidentTrees";
import { IncidentBadges } from "@/components/IncidentBadges";
import { openIncident } from "@/lib/incidentnav";
import {
  seedFromRuns,
  foldActivityEvent,
  summarize,
  buildTree,
  type ActiveRun,
  type ActivityState,
} from "@/lib/activity";
import {
  autonomyEventMatches,
  doctorIncidentTrees,
  filterDoctorAutonomy,
  type AutonomyItem,
} from "@/lib/autonomy";

// Activity is the live fleet monitor: "is anything running right now, and what
// is it doing?". It seeds the in-flight runs from /api/runs, then folds the
// event firehose so each run's current activity, iteration, spend and any
// delegated sub-agents update in real time — the operator stays in control.
export function Activity() {
  const { subscribe, connected } = useEvents();
  const ui = useUI();
  const [state, setState] = useState<ActivityState>({});
  const [doctorFeed, setDoctorFeed] = useState<AutonomyItem[]>([]);
  const [now, setNow] = useState(() => Date.now());
  const [seeding, setSeeding] = useState(true);
  const [cancelling, setCancelling] = useState<Record<string, boolean>>({});

  // Cancel one in-flight run (the targeted alternative to the global Halt). The
  // daemon emits task.failed(reason=canceled), which the firehose fold turns
  // into a failed row — so we just fire the request and mark the button busy.
  async function cancelRun(corr: string) {
    setCancelling((c) => ({ ...c, [corr]: true }));
    try {
      await postAction("/api/cancel_run", { correlation: corr });
    } catch (e) {
      ui.toast(`cancel failed: ${(e as Error).message}`, "error");
    } finally {
      setCancelling((c) => {
        const next = { ...c };
        delete next[corr];
        return next;
      });
    }
  }

  async function seed() {
    setSeeding(true);
    try {
      const [res, auto] = await Promise.all([
        getJSON<{ runs?: ActiveRun[] }>("/api/runs"),
        getJSON<{ items?: AutonomyItem[] }>("/api/autonomy", { limit: "20" }),
      ]);
      // Merge the seed over live state so an in-flight run already being folded
      // isn't clobbered (live activity lines win; seed only fills gaps).
      setState((live) => {
        const seeded = seedFromRuns(res.runs || []);
        return { ...seeded, ...live };
      });
      setDoctorFeed(filterDoctorAutonomy(auto.items, 6));
    } catch {
      /* daemon momentarily unreachable — the firehose will refill */
    } finally {
      setSeeding(false);
    }
  }

  useEffect(() => {
    seed();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fold every firehose event into the run map.
  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      setState((s) => foldActivityEvent(s, e));
      if (!autonomyEventMatches(e)) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => {
        getJSON<{ items?: AutonomyItem[] }>("/api/autonomy", { limit: "20" })
          .then((auto) => setDoctorFeed(filterDoctorAutonomy(auto.items, 6)))
          .catch(() => {});
      }, 700);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [subscribe]);

  // Tick once a second so elapsed timers advance while anything is running.
  const tree = buildTree(state);
  const summary = summarize(state);
  const running = summary.running;
  const hasDoctorFeed = doctorFeed.length > 0;
  const doctorRows = useMemo(() => doctorFeed.slice(0, 6), [doctorFeed]);
  const doctorIncidents = useMemo(
    () => doctorIncidentTrees(doctorFeed, 4),
    [doctorFeed],
  );
  useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [running]);

  return (
    <Page
      icon={ActivityIcon}
      title="Live activity"
      description={
        <span
          className={cn(
            "inline-flex items-center gap-1 text-xs font-medium",
            connected ? "text-good" : "text-bad",
          )}
        >
          ● {connected ? "live" : "disconnected"}
        </span>
      }
      actions={
        <Button variant="ghost" size="sm" onClick={seed} disabled={seeding}>
          <RefreshCw
            className={cn("size-3.5", seeding && "animate-spin")}
          />{" "}
          Refresh
        </Button>
      }
    >

      <MetricGrid cols="grid-cols-3 sm:grid-cols-4">
        <MetricWidget
          icon={ActivityIcon}
          label="Running"
          value={summary.running}
          tone="accent"
          pulse={summary.running > 0}
        />
        <MetricWidget
          icon={CheckCircle2}
          label="Completed"
          value={summary.completed}
          tone="good"
        />
        <MetricWidget
          icon={XCircle}
          label="Failed"
          value={summary.failed}
          tone={summary.failed ? "bad" : "muted"}
        />
        <MetricWidget
          icon={Cpu}
          label="Spent"
          value={money(summary.spentMc)}
          tone="muted"
        />
      </MetricGrid>

      {hasDoctorFeed && (
        <div className="glass rounded-xl px-3 py-2.5">
          <div className="mb-2 flex items-center gap-2">
            <LifeBuoy className="size-4 text-warn" />
            <span className="text-sm font-medium">Autonomous doctor</span>
            <Badge variant="warn">{doctorRows.length}</Badge>
          </div>
          <ul className="space-y-1.5">
            {doctorRows.map((row) => {
              return (
                <li key={row.seq} className="flex items-start gap-2 text-xs">
                  <div className="mt-0.5 flex shrink-0 items-center gap-1">
                    <IncidentBadges item={row} mono />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-foreground/90">{row.title}</div>
                    {row.detail && (
                      <div className="truncate text-muted">{row.detail}</div>
                    )}
                  </div>
                  <span className="shrink-0 font-mono text-xs text-muted opacity-70">
                    {fmtTime(row.ts_unix_ms)}
                  </span>
                </li>
              );
            })}
          </ul>
          {doctorIncidents.length > 0 && (
            <div className="mt-3 border-t border-border pt-2">
              <Disclosure
                summary={
                  <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                    <LifeBuoy className="size-3.5" /> Incident trees ({doctorIncidents.length})
                  </span>
                }
              >
                <DoctorIncidentTrees
                  trees={doctorIncidents}
                  compact
                  onOpenIncident={openIncident}
                />
              </Disclosure>
            </div>
          )}
        </div>
      )}

      {tree.length === 0 ? (
        seeding ? (
          <SkeletonList count={3} lines={2} />
        ) : (
          <EmptyState
            icon={Cpu}
            title="Nothing running"
            hint="In-flight runs and their sub-agents appear here the moment work starts."
          />
        )
      ) : (
        <div className="space-y-2">
          {tree.map((node) => (
            <div key={node.run.corr} className="space-y-1.5">
              <RunRow
                run={node.run}
                now={now}
                onCancel={cancelRun}
                cancelling={!!cancelling[node.run.corr]}
              />
              {node.children.length > 0 && (
                <div className="ml-4 space-y-1.5 border-l border-border pl-3">
                  {node.children.map((c) => (
                    <RunRow
                      key={c.corr}
                      run={c}
                      now={now}
                      onCancel={cancelRun}
                      cancelling={!!cancelling[c.corr]}
                      child
                    />
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </Page>
  );
}

function RunRow({
  run,
  now,
  child,
  onCancel,
  cancelling,
}: {
  run: ActiveRun;
  now: number;
  child?: boolean;
  onCancel?: (corr: string) => void;
  cancelling?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const live = run.status === "running";
  const elapsed = (live ? now : run.endedMs || now) - (run.startedMs || now);
  return (
    <div
      className={cn(
        "rounded-lg border bg-card",
        live ? "border-accent/40" : "border-border",
      )}
    >
      <div className="flex items-start gap-2.5 px-3 py-2">
        <button
          onClick={() => setOpen((v) => !v)}
          className="flex min-w-0 flex-1 items-start gap-2.5 text-left"
          title={open ? "Hide detail" : "Show tool calls & answer"}
        >
          <span className="mt-0.5 shrink-0">
            {run.status === "running" ? (
              <span className="block size-2.5 animate-pulse rounded-full bg-accent" />
            ) : run.status === "completed" ? (
              <CheckCircle2 className="size-4 text-good" />
            ) : (
              <XCircle className="size-4 text-bad" />
            )}
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1.5">
              {open ? (
                <ChevronDown className="size-3.5 shrink-0 text-muted" />
              ) : (
                <ChevronRight className="size-3.5 shrink-0 text-muted" />
              )}
              {child && (
                <CornerDownRight className="size-3.5 shrink-0 text-muted" />
              )}
              <span className="truncate text-sm font-medium">
                {run.intent || <span className="text-muted">(no intent)</span>}
              </span>
            </span>
            <span className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted">
              <span className={cn(live && "text-accent")}>{run.activity}</span>
              <span>·</span>
              <span className="tabular-nums">{fmtElapsed(elapsed)}</span>
              {run.iters > 0 && (
                <>
                  <span>·</span>
                  <span>
                    {run.iters} iter{run.iters === 1 ? "" : "s"}
                  </span>
                </>
              )}
              {run.spentMc > 0 && (
                <>
                  <span>·</span>
                  <span>{money(run.spentMc)}</span>
                </>
              )}
              {child && (
                <>
                  <span>·</span>
                  <span className="inline-flex items-center gap-0.5 text-accent/80">
                    <CornerDownRight className="size-3" /> sub
                  </span>
                </>
              )}
            </span>
          </span>
        </button>
        {live && onCancel && (
          <button
            onClick={() => onCancel(run.corr)}
            disabled={cancelling}
            title="Cancel this run"
            className="mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-md border border-border px-2 py-1 text-xs text-muted transition-colors hover:border-bad hover:text-bad disabled:opacity-50"
          >
            <Ban className="size-3.5" /> {cancelling ? "…" : "Cancel"}
          </button>
        )}
      </div>
      {open && (
        <div className="border-t border-border px-3 py-2 text-sm">
          <RunDetailLoader correlationId={run.corr} status={run.status} />
        </div>
      )}
    </div>
  );
}

// fmtElapsed renders a live duration compactly: 8s, 1m04s, 1h02m.
function fmtElapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rs = s % 60;
  if (m < 60) return `${m}m${String(rs).padStart(2, "0")}s`;
  const h = Math.floor(m / 60);
  const rm = m % 60;
  return `${h}h${String(rm).padStart(2, "0")}m`;
}
