import { useEffect, useMemo, useRef, useState } from "react";
import { Clapperboard, RefreshCw } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import { mergeEvents } from "@/lib/rundetail";
import { buildReplay } from "@/lib/replay";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
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
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Clapperboard className="size-4 text-accent" /> Flight recorder
        </h2>
        <select
          value={sel}
          onChange={(e) => setSel(e.target.value)}
          className="h-8 max-w-[60%] flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus:border-accent"
        >
          {runs.length === 0 && <option value="">no runs yet</option>}
          {runs.map((r) => (
            <option key={r.correlation_id} value={r.correlation_id}>
              {(r.status === "running" ? "● " : "") + (r.intent ? r.intent.slice(0, 70) : r.correlation_id)}
            </option>
          ))}
        </select>
        <Button variant="ghost" size="sm" onClick={loadRuns} title="Reload run list">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

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
