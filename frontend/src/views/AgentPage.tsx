import { useEffect, useMemo, useState } from "react";
import { ChevronLeft, Bot } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { AgentDetail } from "@/components/AgentDetail";
import type { AgentProfile } from "@/views/Roster";
import {
  buildFleet,
  type ApiProfile,
  type ApiOrder,
  type ApiSchedule,
  type ApiWorkflow,
  type ApiPulse,
  type FleetState,
  type FleetTrigger,
} from "@/lib/fleet";
import type { RunLite } from "@/lib/agentdetail";

// AgentPage is the full-page identity route for one agent (#agent/<slug>).
// It renders the same AgentDetail Command Center used inside the Fleet tab —
// all tabs (Overview, Activity, Triggers, Comms, Soul, Model, Memory, Skills,
// Repair, Diagnostics, Files) are available here in page mode.
export function AgentPage({ slug, onNavigate }: { slug: string; onNavigate: (view: string) => void }) {
  const { events } = useEvents();
  const [profiles, setProfiles] = useState<AgentProfile[] | null>(null);
  const [runs, setRuns] = useState<RunLite[]>([]);
  const [orders, setOrders] = useState<ApiOrder[]>([]);
  const [schedules, setSchedules] = useState<ApiSchedule[]>([]);
  const [workflows, setWorkflows] = useState<ApiWorkflow[]>([]);
  const [pulse, setPulse] = useState<ApiPulse | undefined>(undefined);
  const [err, setErr] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  async function loadRuns() {
    try {
      const d = await getJSON<{ runs?: RunLite[] }>("/api/runs");
      setRuns(d.runs || []);
    } catch {
      /* keep prior runs on a transient failure */
    }
  }

  async function loadCatalog() {
    const [a, o, s, w, p] = await Promise.allSettled([
      getJSON<{ profiles?: AgentProfile[] }>("/api/agents"),
      getJSON<{ orders?: ApiOrder[] }>("/api/standing"),
      getJSON<{ schedules?: ApiSchedule[] }>("/api/schedules"),
      getJSON<{ workflows?: ApiWorkflow[] }>("/api/workflows"),
      getJSON<ApiPulse>("/api/pulse"),
    ]);
    if (a.status === "fulfilled") setProfiles(a.value.profiles || []);
    else setErr((a.reason as Error)?.message || "failed to load agents");
    if (o.status === "fulfilled") setOrders(o.value.orders || []);
    if (s.status === "fulfilled") setSchedules(s.value.schedules || []);
    if (w.status === "fulfilled") setWorkflows(w.value.workflows || []);
    if (p.status === "fulfilled") setPulse(p.value);
    setLoaded(true);
  }

  useEffect(() => {
    setLoaded(false);
    loadRuns();
    loadCatalog();
    const id = setInterval(loadRuns, 6000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [slug]);

  // Nudge runs on lifecycle events for this agent (live-ish without polling hard).
  const head = events[0]?.kind;
  useEffect(() => {
    if (head === "task.completed" || head === "task.failed" || head === "task.received" || head === "subagent.spawned") loadRuns();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const fleet = useMemo(
    () => buildFleet((profiles || []) as unknown as ApiProfile[], orders, schedules, workflows, runs, pulse),
    [profiles, orders, schedules, workflows, runs, pulse],
  );
  const entity = useMemo(() => fleet.find((e) => e.kind === "roster" && e.slug === slug), [fleet, slug]);
  const profile = useMemo(() => (profiles || []).find((p) => p.slug === slug), [profiles, slug]);

  const back = (
    <Button variant="ghost" size="sm" onClick={() => onNavigate("agents")}>
      <ChevronLeft className="size-3.5" /> Fleet
    </Button>
  );

  if (!loaded && !profiles) {
    return (
      <div className="space-y-3">
        {back}
        <SkeletonList count={5} lines={2} />
      </div>
    );
  }

  if (err && !profiles) {
    return (
      <div className="space-y-3">
        {back}
        <ErrorText>{err}</ErrorText>
      </div>
    );
  }

  if (!profile || !entity) {
    return (
      <div className="space-y-3">
        {back}
        <EmptyState
          icon={Bot}
          title={`No agent "${slug}"`}
          hint="This agent may have been removed or never existed. Open one from the Fleet."
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-col gap-3">
      <div>
        {back}
      </div>
      <AgentDetail
        slug={slug}
        profile={profile}
        runs={runs}
        orders={orders}
        triggers={entity.triggers as FleetTrigger[]}
        state={entity.state as FleetState}
        schedules={schedules}
        page
        onClose={() => onNavigate("agents")}
        onManage={onNavigate}
      />
    </div>
  );
}
