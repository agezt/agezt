import { useEffect, useMemo, useRef, useState } from "react";
import { Clapperboard, RefreshCw, Radio } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import { mergeEvents } from "@/lib/rundetail";
import { buildReplay } from "@/lib/replay";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/ui/page-header";
import { FlightRecorder } from "@/components/FlightRecorder";

interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  started_unix_ms?: number;
}

// Replay is the flight recorder: pick any run and scrub/play through exactly
// what the agent did, step by step — LLM rounds, tool calls and results, policy
// decisions, operator steering, spend — with the cumulative state at each point.
// The newest run is selected by default and folds live events, so you can watch
// a run record itself in real time and then rewind it.
export function Replay() {
  const { subscribe } = useEvents();
  const [runs, setRuns] = useState<Run[]>([]);
  const [sel, setSel] = useState<string>("");
  const [arc, setArc] = useState<AgentEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const fetchedFor = useRef<string>("");

  // Load the run list (newest first); default-select the newest.
  async function loadRuns() {
    try {
      const d = await getJSON<{ runs?: Run[] }>("/api/runs");
      const list = d.runs || [];
      setRuns(list);
      setErr(null);
      if (!sel && list[0]?.correlation_id) setSel(list[0].correlation_id);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  useEffect(() => {
    loadRuns();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Load the selected run's journaled arc.
  useEffect(() => {
    if (!sel || fetchedFor.current === sel) return;
    fetchedFor.current = sel;
    setLoading(true);
    setArc([]);
    getJSON<{ events?: AgentEvent[] }>("/api/journal", { correlation_id: sel, limit: "1000" })
      .then((d) => setArc(d.events || []))
      .catch((e) => setErr((e as Error).message))
      .finally(() => setLoading(false));
  }, [sel]);

  // Fold live events for the selected run so an in-flight run records itself.
  useEffect(() => {
    if (!sel) return;
    return subscribe((e: AgentEvent) => {
      if (e.correlation_id !== sel) return;
      setArc((prev) => mergeEvents(prev, [e]));
    });
  }, [sel, subscribe]);

  const steps = useMemo(() => buildReplay(arc), [arc]);
  const selRun = runs.find((r) => r.correlation_id === sel);
  const isLive = selRun?.status === "running";

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={Clapperboard}
        title="Flight recorder"
        description="Pick any run and scrub through exactly what the agent did, step by step."
        actions={
          <Button variant="ghost" size="sm" onClick={loadRuns} title="Reload run list">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        }
      />

      {runs.length > 0 ? (
        <div className="flex gap-1.5 overflow-x-auto pb-1" role="group" aria-label="Replay run">
          {runs.map((r) => {
            const id = r.correlation_id || "";
            const selected = sel === id;
            return (
              <button
                key={id}
                type="button"
                aria-pressed={selected}
                onClick={() => setSel(id)}
                className={cn(
                  "inline-flex h-9 max-w-[18rem] shrink-0 items-center gap-1.5 rounded-md border px-2 text-left text-xs transition-colors",
                  selected
                    ? "border-accent bg-accent/15 text-accent"
                    : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
                )}
              >
                {r.status === "running" ? <Radio className="size-3.5 animate-pulse" /> : <Clapperboard className="size-3.5" />}
                <span className="truncate">{r.intent || id || "run"}</span>
              </button>
            );
          })}
        </div>
      ) : (
        <div className="rounded-lg border border-border bg-panel/60 px-3 py-2 text-xs text-muted">no runs yet</div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading ? (
        <SkeletonList count={3} lines={2} />
      ) : (
        <div className="min-h-0 flex-1">
          <FlightRecorder steps={steps} live={isLive} />
        </div>
      )}
    </div>
  );
}
