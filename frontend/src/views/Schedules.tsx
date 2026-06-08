import { useEffect, useState } from "react";
import { CalendarClock, RefreshCw, Play, Pause, Trash2, Bot, Heart, Infinity as InfinityIcon, ShieldCheck } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge, statusVariant } from "@/components/ui/badge";
import { Muted, ErrorText } from "@/components/JsonView";

interface Sched {
  id: string;
  intent?: string;
  cadence?: string;
  mode?: string;
  source?: string;
  enabled?: boolean;
  next_run_unix?: number;
  last_status?: string;
  fires?: number;
  assure?: number;
}

// sourceTone colours the origin badge: an agent-scheduled run (the agent used
// the `schedule` tool to arrange its own future work) is the notable one, so it
// gets the accent; operator/env are muted.
function sourceTone(src?: string): string {
  if (src === "agent") return "bg-accent/15 text-accent";
  return "bg-panel text-muted";
}

// Schedules is the autonomy cockpit: every scheduled intent — whether an
// operator added it, an AGEZT_SCHEDULE env job, or the AGENT scheduled it itself
// — with its cadence, next fire, last outcome and origin, plus run-now /
// pause-resume / remove controls so you can manage what fires unattended.
export function Schedules() {
  const [items, setItems] = useState<Sched[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ schedules?: Sched[] }>("/api/schedules");
      setItems(d.schedules || []);
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(id: string, path: string, params?: Record<string, string>) {
    setBusy(id);
    try {
      await postAction(path, { id, ...params });
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  const agentCount = items?.filter((s) => s.source === "agent").length ?? 0;

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <CalendarClock className="size-4 text-accent" /> Schedules
        </h2>
        <span className="text-xs text-muted">
          {items ? `${items.length} total` : ""}
          {agentCount > 0 && <span className="text-accent"> · {agentCount} agent-scheduled</span>}
        </span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !items ? (
        <Muted>loading…</Muted>
      ) : items.length === 0 ? (
        <Muted>no schedules — the agent can add one with the schedule tool, or use `agt schedule add`</Muted>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-2">
            {items.map((s) => (
              <li key={s.id} className="rounded-lg border border-border bg-card p-3">
                <div className="flex items-center gap-2">
                  <Badge>
                    {s.mode === "continuous" && <InfinityIcon className="mr-1 inline size-3 align-[-1px]" />}
                    {s.cadence || s.mode || "?"}
                  </Badge>
                  <span
                    className={cn(
                      "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
                      sourceTone(s.source),
                    )}
                    title={`source: ${s.source || "?"}`}
                  >
                    {s.source === "agent" && <Bot className="size-3" />}
                    {s.source || "?"}
                  </span>
                  {s.mode === "continuous" && s.enabled !== false && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-bad/10 px-1.5 py-0.5 text-[10px] font-semibold text-bad"
                      title={`alive — ${s.fires ?? 0} cycle${s.fires === 1 ? "" : "s"} completed`}
                    >
                      <Heart className="size-3 animate-pulse fill-current" />
                      {s.fires ?? 0}
                    </span>
                  )}
                  {(s.assure ?? 0) > 0 && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-good/10 px-1.5 py-0.5 text-[10px] font-semibold text-good"
                      title={`do-it-for-sure: each firing verifies completion and retries up to ${s.assure}×`}
                    >
                      <ShieldCheck className="size-3" />
                      assured {s.assure}×
                    </span>
                  )}
                  {s.enabled === false && <span className="text-[10px] text-muted">(paused)</span>}
                  {s.last_status && <Badge variant={statusVariant(s.last_status)}>{s.last_status}</Badge>}
                  <div className="ml-auto flex items-center gap-1.5">
                    <button
                      onClick={() => act(s.id, "/api/schedule/run")}
                      disabled={busy === s.id}
                      title="Run now"
                      className="text-muted transition-colors hover:text-accent disabled:opacity-50"
                    >
                      <Play className="size-3.5" />
                    </button>
                    <button
                      onClick={() => act(s.id, "/api/schedule/enable", { enabled: s.enabled === false ? "true" : "false" })}
                      disabled={busy === s.id}
                      title={s.enabled === false ? "Resume" : "Pause"}
                      className="text-muted transition-colors hover:text-foreground disabled:opacity-50"
                    >
                      {s.enabled === false ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
                    </button>
                    <button
                      onClick={() => act(s.id, "/api/schedule/remove")}
                      disabled={busy === s.id}
                      title="Remove"
                      className="text-muted transition-colors hover:text-bad disabled:opacity-50"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                </div>
                <div className="mt-1.5 text-sm">{s.intent || s.id}</div>
                <div className="mt-1 flex flex-wrap items-center gap-x-3 text-[10px] text-muted">
                  {s.enabled !== false && s.next_run_unix ? (
                    <span>next {fmtDateTime(s.next_run_unix * 1000)}</span>
                  ) : null}
                  {s.mode !== "continuous" && (s.fires ?? 0) > 0 && (
                    <span>{s.fires} run{s.fires === 1 ? "" : "s"}</span>
                  )}
                  <span className="font-mono opacity-70">{s.id}</span>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
