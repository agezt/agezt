import { useEffect, useMemo, useState } from "react";
import {
  Activity,
  Bot,
  CalendarClock,
  ChevronLeft,
  Clock,
  Cpu,
  ExternalLink,
  Fingerprint,
  ListChecks,
  Play,
  Route,
  ShieldCheck,
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Disclosure } from "@/components/ui/disclosure";
import { ErrorText } from "@/components/JsonView";
import { AgentAvatar } from "@/components/AgentAvatar";
import type { AgentProfile } from "@/views/Roster";
import {
  buildFleet,
  scheduleAgentSlug,
  type ApiProfile,
  type ApiOrder,
  type ApiSchedule,
  type ApiWorkflow,
  type ApiPulse,
  type FleetState,
  type FleetTrigger,
} from "@/lib/fleet";
import { summarizeAgentRuntimeStatus, type RunLite } from "@/lib/agentdetail";
import { fmtAgo, fmtDue } from "@/lib/utils";

// AgentPage is the plain-language identity page for one agent. The richer
// command center still exists in Fleet; this route answers the first questions:
// who is it, is it awake, what wakes it, and where do I manage it?
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
          title={`No agent “${slug}”`}
          hint="This agent may have been removed or never existed. Open one from the Fleet."
        />
      </div>
    );
  }

  return (
    <SimpleAgentIdentityPage
      back={back}
      entityState={entity.state}
      profile={profile}
      runs={runs}
      schedules={schedules}
      triggers={entity.triggers}
      onNavigate={onNavigate}
    />
  );
}

function SimpleAgentIdentityPage({
  back,
  entityState,
  profile,
  runs,
  schedules,
  triggers,
  onNavigate,
}: {
  back: React.ReactNode;
  entityState: FleetState;
  profile: AgentProfile;
  runs: RunLite[];
  schedules: ApiSchedule[];
  triggers: FleetTrigger[];
  onNavigate: (view: string) => void;
}) {
  const runtime = summarizeAgentRuntimeStatus(profile.status);
  const running = runtime.activeRunCount > 0 || entityState === "running";
  const avatarStatus = profile.retired
    ? "retired"
    : running
      ? "running"
      : runtime.operationalState === "paused" || !profile.enabled
        ? "paused"
        : "sleeping";
  const myRuns = runs.filter((run) => run.agent === profile.slug);
  const lastRun = myRuns.reduce<RunLite | undefined>((latest, run) => {
    if (!latest) return run;
    return (run.started_unix_ms || 0) > (latest.started_unix_ms || 0) ? run : latest;
  }, undefined);
  const activeRun =
    myRuns.find((run) => run.status === "running" || run.status === "active") ||
    (running ? myRuns[0] : undefined);
  const mySchedules = schedules.filter(
    (schedule) =>
      (schedule.agent || "") === profile.slug ||
      scheduleAgentSlug(schedule.intent) === profile.slug,
  );
  const enabledSchedules = mySchedules.filter((schedule) => schedule.enabled !== false);
  const enabledOrders = triggers.filter(
    (trigger) =>
      trigger.mode === "cron" ||
      trigger.mode === "event" ||
      trigger.mode === "cadence",
  );
  const purpose = profile.description || firstSentence(profile.soul) || "No purpose description yet.";
  const statusText = profile.retired
    ? "Retired"
    : running
      ? "Running now"
      : !profile.enabled
        ? "Paused"
        : entityState === "armed"
          ? "Ready and automatic"
          : "Ready when called";
  const statusTone = profile.retired || !profile.enabled
    ? "default"
    : running
      ? "good"
      : entityState === "armed"
        ? "accent"
        : "default";
  const nextWake = runtime.nextWakeMs
    ? fmtDue(runtime.nextWakeMs)
    : enabledSchedules.length > 0 || enabledOrders.length > 0
      ? "automatic wake configured"
      : "manual or delegated";

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        {back}
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => onNavigate("roster")}>
            Edit in Roster <ExternalLink className="size-3.5" />
          </Button>
          <Button variant="ghost" size="sm" onClick={() => onNavigate("agents")}>
            Open Fleet details
          </Button>
        </div>
      </div>

      <Card className="p-4">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
          <AgentAvatar
            slug={profile.slug}
            name={profile.name}
            size={64}
            status={avatarStatus}
          />
          <div className="min-w-0 flex-1 space-y-3">
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="font-mono text-xl font-semibold text-foreground">
                  {profile.slug}
                </h1>
                {profile.name && profile.name !== profile.slug && (
                  <span className="text-sm text-muted">{profile.name}</span>
                )}
                <Badge variant={statusTone}>{statusText}</Badge>
                {profile.system && <Badge variant="default">system</Badge>}
                {profile.kind === "subagent" && <Badge variant="default">subagent</Badge>}
              </div>
              <p className="max-w-3xl text-sm leading-6 text-foreground/80">{purpose}</p>
            </div>

            <div className="grid gap-2 sm:grid-cols-3">
              <SimpleFact
                icon={Activity}
                label="Now"
                value={activeRun?.status || runtime.operationalText || (running ? "running" : "idle")}
                detail={runtime.liveDetail || runtime.lastActivitySummary || undefined}
              />
              <SimpleFact
                icon={CalendarClock}
                label="Next wake"
                value={nextWake}
                detail={runtime.wakeDetail || undefined}
              />
              <SimpleFact
                icon={Cpu}
                label="Model"
                value={profile.model || "default model"}
                detail={profile.task_type || undefined}
              />
            </div>
          </div>
        </div>
      </Card>

      <div className="grid gap-3 lg:grid-cols-[1fr_0.9fr]">
        <Card>
          <CardHeader>
            <Route className="size-4 text-accent" />
            <CardTitle>How this agent starts</CardTitle>
          </CardHeader>
          <CardBody className="space-y-2">
            {triggers.length > 0 ? (
              triggers.slice(0, 5).map((trigger, index) => (
                <div
                  key={`${trigger.mode}-${trigger.label}-${index}`}
                  className="rounded-md border border-border bg-panel/40 p-2"
                >
                  <div className="flex flex-wrap items-center gap-2 text-sm text-foreground">
                    <Badge variant="default">{trigger.mode}</Badge>
                    <span>{trigger.label}</span>
                  </div>
                  <p className="mt-1 text-xs text-muted">
                    {trigger.needs}{trigger.via ? ` · ${trigger.via}` : ""}
                  </p>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted">It starts only when an operator or another agent calls it.</p>
            )}
            {triggers.length > 5 && (
              <p className="text-xs text-muted">
                +{triggers.length - 5} more triggers in Fleet details.
              </p>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader>
            <ListChecks className="size-4 text-accent" />
            <CardTitle>What to know</CardTitle>
          </CardHeader>
          <CardBody className="space-y-2 text-sm">
            <PlainRow
              icon={Clock}
              label="Last run"
              value={lastRun?.started_unix_ms ? fmtAgo(lastRun.started_unix_ms) : "No recent runs"}
            />
            <PlainRow icon={Play} label="Runs" value={`${myRuns.length} recorded`} />
            <PlainRow
              icon={CalendarClock}
              label="Schedules"
              value={`${enabledSchedules.length} active`}
            />
            <PlainRow
              icon={ShieldCheck}
              label="Trust"
              value={profile.trust_ceiling || "default"}
            />
            <PlainRow
              icon={Fingerprint}
              label="Memory"
              value={profile.memory_scope || `agent/${profile.slug}`}
            />
            {profile.parent_agent || profile.owner_agent ? (
              <PlainRow
                icon={Route}
                label="Managed by"
                value={profile.parent_agent || profile.owner_agent || "—"}
              />
            ) : null}
          </CardBody>
        </Card>
      </div>

      <Disclosure
        className="rounded-lg border border-border bg-card px-1.5 py-0.5"
        summary={<span className="text-xs font-medium text-foreground/90">More identity details</span>}
      >
        <div className="space-y-3 p-2">
          {profile.soul ? (
            <div>
              <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted">Identity prompt</div>
              <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-md bg-panel p-3 font-mono text-xs text-foreground/85">{profile.soul}</pre>
            </div>
          ) : null}
          {(profile.instructions || []).length > 0 ? (
            <div>
              <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted">Standing instructions</div>
              <ul className="space-y-1 rounded-md bg-panel p-3 text-sm text-foreground/85">
                {(profile.instructions || []).map((instruction, index) => (
                  <li key={`${index}-${instruction}`}>{instruction}</li>
                ))}
              </ul>
            </div>
          ) : null}
          <div className="grid gap-2 sm:grid-cols-2">
            <MiniDetail label="Kind" value={profile.system ? "system" : profile.kind || "custom"} />
            <MiniDetail label="Workdir" value={profile.workdir || "not set"} />
            <MiniDetail label="Daily budget" value={profile.max_daily_mc ? `${profile.max_daily_mc.toLocaleString()} mc` : "default"} />
            <MiniDetail label="Run budget" value={profile.max_cost_mc ? `${profile.max_cost_mc.toLocaleString()} mc` : "default"} />
          </div>
        </div>
      </Disclosure>
    </div>
  );
}

function SimpleFact({ icon: Icon, label, value, detail }: { icon: typeof Activity; label: string; value: string; detail?: string }) {
  return (
    <div className="rounded-lg border border-border bg-panel/40 p-3">
      <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        <Icon className="size-3" /> {label}
      </div>
      <div className="mt-1 truncate text-sm font-medium text-foreground">{value}</div>
      {detail ? <div className="mt-0.5 line-clamp-2 text-xs text-muted">{detail}</div> : null}
    </div>
  );
}

function PlainRow({ icon: Icon, label, value }: { icon: typeof Activity; label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md bg-panel/35 px-2.5 py-2">
      <span className="flex min-w-0 items-center gap-2 text-muted">
        <Icon className="size-3.5 shrink-0" />
        <span className="truncate">{label}</span>
      </span>
      <span className="min-w-0 truncate text-right font-medium text-foreground">{value}</span>
    </div>
  );
}

function MiniDetail({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border bg-panel/30 p-2">
      <div className="text-[10px] uppercase tracking-wider text-muted">{label}</div>
      <div className="mt-0.5 break-words text-sm text-foreground">{value}</div>
    </div>
  );
}

function firstSentence(text?: string): string {
  const clean = (text || "").trim().replace(/\s+/g, " ");
  if (!clean) return "";
  const match = /^(.{1,180}?[.!?])\s/.exec(clean);
  return match?.[1] || clean.slice(0, 180);
}
