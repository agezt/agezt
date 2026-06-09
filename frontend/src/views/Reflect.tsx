import { useEffect, useState } from "react";
import { Brain, RefreshCw, Lightbulb, Play } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

interface Observations {
  window_events?: number;
  tasks_started?: number;
  tasks_completed?: number;
  tasks_failed?: number;
  briefs_sent?: number;
  skills_activated?: number;
  approvals_granted?: number;
  approvals_denied?: number;
  entities_total?: number;
}
interface Proposal {
  area?: string;
  observation?: string;
  suggestion?: string;
}
interface Report {
  generated_ms?: number;
  observations?: Observations;
  entities_decayed?: number;
  proposals?: Proposal[];
}

// Reflect is the self-reflection view: the daemon folds its own journal into
// observations (tasks done/failed, briefs, approvals, world size), applies the
// one safe auto-adjustment (world-model decay), and derives advisory PROPOSALS —
// recalibrations it suggests but never auto-applies. The system reasoning about
// its own behaviour; "Run now" triggers a fresh pass.
export function Reflect() {
  const [report, setReport] = useState<Report | null>(null);
  const [found, setFound] = useState<boolean | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [running, setRunning] = useState(false);

  async function load() {
    try {
      const d = await getJSON<{ found?: boolean; report?: Report }>("/api/reflect");
      setFound(!!d.found);
      setReport(d.report || null);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function runNow() {
    setRunning(true);
    try {
      const r = await postAction<{ report?: Report } & Report>("/api/reflect/run");
      // reflect_run returns the report fields at the top level (+ correlation_id).
      setReport((r.report as Report) || (r as Report));
      setFound(true);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  const o = report?.observations || {};
  const props = report?.proposals || [];

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Brain className="size-4 text-accent" /> Reflection
        </h2>
        {report?.generated_ms ? (
          <span className="text-xs text-muted">last {fmtDateTime(report.generated_ms)}</span>
        ) : null}
        <Button variant="ghost" size="sm" className="ml-auto gap-1.5" onClick={runNow} disabled={running}>
          {running ? <RefreshCw className="size-3.5 animate-spin" /> : <Play className="size-3.5" />}
          Run now
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : found === false && !report ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted">
          <Brain className="size-8 opacity-40" />
          <span className="text-sm">no reflection yet — run one to fold the journal into insights</span>
        </div>
      ) : !report ? (
        <SkeletonList count={3} lines={2} />
      ) : (
        <div className="min-h-0 flex-1 space-y-4 overflow-auto">
          {/* Observation tiles */}
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
            <Tile label="events folded" value={o.window_events} />
            <Tile label="tasks done" value={o.tasks_completed} />
            <Tile label="tasks failed" value={o.tasks_failed} tone={o.tasks_failed ? "text-bad" : undefined} />
            <Tile label="briefs sent" value={o.briefs_sent} />
            <Tile label="approvals ✓" value={o.approvals_granted} />
            <Tile label="approvals ✗" value={o.approvals_denied} />
            <Tile label="skills used" value={o.skills_activated} />
            <Tile label="world entities" value={o.entities_total} />
          </div>

          {report.entities_decayed != null && report.entities_decayed > 0 && (
            <div className="text-xs text-muted">
              applied salience decay to <span className="text-foreground">{report.entities_decayed}</span> world entit
              {report.entities_decayed === 1 ? "y" : "ies"} (the one safe auto-adjustment)
            </div>
          )}

          {/* Advisory proposals */}
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
              <Lightbulb className="size-3.5" /> proposals ({props.length})
            </div>
            {props.length === 0 ? (
              <Muted>nothing to recalibrate — the system looks balanced</Muted>
            ) : (
              <ul className="space-y-2">
                {props.map((p, i) => (
                  <li key={i} className="rounded-lg border border-border bg-card p-3">
                    <span className="rounded bg-accent/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-accent">
                      {p.area || "general"}
                    </span>
                    {p.observation && <p className="mt-1.5 text-xs text-foreground/85">{p.observation}</p>}
                    {p.suggestion && (
                      <p className="mt-1 text-xs text-muted">
                        <span className="font-semibold text-foreground/70">consider:</span> {p.suggestion}
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function Tile({ label, value, tone }: { label: string; value?: number; tone?: string }) {
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2">
      <div className="text-[10px] uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-xl font-semibold tabular-nums", tone)}>{value ?? 0}</div>
    </div>
  );
}
