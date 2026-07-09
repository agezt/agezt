import { useEffect, useState } from "react";
import { Anchor, Clock, ArrowUpRight, Play, Pause, Flame, CalendarClock, Zap, Mail, LifeBuoy, Megaphone, Trash2 } from "lucide-react";
import { getJSON } from "@/lib/api";
import { fmtTime, fmtDateTime, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { type ConfirmOptions } from "@/components/ui/feedback";
import { TriggerChip } from "@/components/Fleet";
import { openAgent } from "@/lib/agentnav";
import { type AgentProfile } from "@/views/Roster";
import { type FleetTrigger, type ApiOrder, type ApiSchedule } from "@/lib/fleet";
import { AgentWakeAccess } from "@/components/agentdetail/shared";
import { agentMailboxSubjects, mailboxSubjectBinding, mailboxWakeArmIssue } from "@/components/agentdetail/comms";
import { agentManagedSubagent } from "@/components/agentdetail/capability";
import { agentScheduleBindingTitle } from "@/components/agentdetail/lifecycle";

export function TriggersTab({
  slug,
  profile,
  wakeAccess,
  orders,
  schedules,
  triggers,
  busy,
  onAction,
  onCreateMailboxWake,
  onManage,
}: {
  slug: string;
  profile: AgentProfile;
  wakeAccess?: AgentWakeAccess;
  orders: ApiOrder[];
  schedules: ApiSchedule[];
  triggers: FleetTrigger[];
  busy: boolean;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
    opts?: { confirm?: ConfirmOptions },
  ) => void;
  onCreateMailboxWake: (kind: "dm" | "help" | "broadcast", subject: string) => void;
  onManage: (view: string) => void;
}) {
  const [why, setWhy] = useState<string | null>(null);
  const mailboxArmIssue = mailboxWakeArmIssue(profile, wakeAccess);
  const mailboxArmOwner =
    wakeAccess?.manager ||
    (agentManagedSubagent(profile)
      ? profile.parent_agent || profile.owner_agent || ""
      : "");
  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-1.5 text-xs uppercase tracking-normal text-muted">
          how this agent is triggered
        </div>
        <div className="flex flex-wrap gap-2">
          {triggers.length === 0 ? (
            <span className="text-[11px] text-muted">
              No automatic triggers — runs manually or via delegation.
            </span>
          ) : (
            triggers.map((t, i) => (
              <TriggerChip key={i} mode={t.mode} label={t.label} />
            ))
          )}
        </div>
      </div>

      <MailboxWakeSubjects
        slug={slug}
        orders={orders}
        busy={busy}
        armIssue={mailboxArmIssue}
        armOwner={mailboxArmOwner}
        onCreate={onCreateMailboxWake}
      />

      {/* Upcoming fires — what this agent WILL do next, from each binding schedule. */}
      {schedules.length > 0 && (
        <div>
          <div className="mb-1 flex items-center gap-1.5 text-xs uppercase tracking-normal text-muted">
            <CalendarClock className="size-3" /> upcoming runs
          </div>
          <ul className="space-y-2">
            {schedules.map((s) => (
              <li
                key={s.id}
                className="rounded-lg border border-border bg-panel/30 p-2.5"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={s.enabled ? "good" : "default"}>
                    {s.enabled ? "armed" : "paused"}
                  </Badge>
                  <span className="text-xs font-medium">
                    {s.cadence || s.mode || s.id}
                  </span>
                  <span className="inline-flex min-w-0 items-center gap-1 rounded-md bg-card px-1.5 py-0.5 text-xs text-muted" title={agentScheduleBindingTitle(s, slug)}>
                    <span className="truncate">{agentScheduleBindingTitle(s, slug)}</span>
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    className="ml-auto"
                    aria-label={`Run schedule ${s.id}`}
                    title="Run now"
                    onClick={() =>
                      onAction("/api/schedule/run", { id: s.id }, `ran ${s.id}`)
                    }
                  >
                    <Flame className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`${s.enabled ? "Pause" : "Resume"} schedule ${s.id}`}
                    title={s.enabled ? "Pause" : "Resume"}
                    onClick={() =>
                      onAction(
                        "/api/schedule/enable",
                        { id: s.id, enabled: s.enabled ? "false" : "true" },
                        s.enabled ? "paused" : "resumed",
                      )
                    }
                  >
                    {s.enabled ? (
                      <Pause className="size-3.5" />
                    ) : (
                      <Play className="size-3.5" />
                    )}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`Remove schedule ${s.id}`}
                    title="Remove this schedule binding"
                    onClick={() =>
                      onAction(
                        "/api/schedule/remove",
                        { id: s.id },
                        `removed ${s.id}`,
                        {
                          confirm: {
                            title: "Remove this schedule binding?",
                            message: `Schedule ${s.id} will stop: ${agentScheduleBindingTitle(s, slug)}.`,
                            confirmLabel: "Remove",
                            danger: true,
                          },
                        },
                      )
                    }
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
                <ScheduleForecast id={s.id} fallbackNext={s.next_run_unix} />
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="text-xs uppercase tracking-normal text-muted">
        standing orders firing this agent
      </div>
      {orders.length === 0 ? (
        <EmptyState
          icon={Anchor}
          title="No standing orders bind this agent"
          hint="Create a standing order in the Standing page and set it to run as this agent — cron or event triggered."
        />
      ) : (
        <ul className="space-y-2">
          {orders.map((o) => (
            <li
              key={o.id}
              className="rounded-lg border border-border bg-panel/30 p-2.5"
            >
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={o.enabled ? "good" : "default"}>
                  {o.enabled ? "armed" : "paused"}
                </Badge>
                <span className="text-xs font-medium">{o.name || o.id}</span>
                {o.initiative?.mode && (
                  <span className="text-xs text-muted">
                    · {o.initiative.mode}
                  </span>
                )}
                <span className="ml-auto flex items-center gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    title="Fire now"
                    onClick={() =>
                      onAction(
                        "/api/standing/fire",
                        { id: o.id },
                        `fired ${o.name || o.id}`,
                      )
                    }
                  >
                    <Flame className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    title={o.enabled ? "Pause" : "Resume"}
                    onClick={() =>
                      onAction(
                        "/api/standing/enable",
                        { id: o.id, enabled: o.enabled ? "false" : "true" },
                        o.enabled ? "paused" : "resumed",
                      )
                    }
                  >
                    {o.enabled ? (
                      <Pause className="size-3.5" />
                    ) : (
                      <Play className="size-3.5" />
                    )}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    aria-label={`Remove standing order ${o.id}`}
                    title="Remove this standing order binding"
                    onClick={() =>
                      onAction(
                        "/api/standing/remove",
                        { id: o.id },
                        `removed ${o.name || o.id}`,
                        {
                          confirm: {
                            title: "Remove this standing order binding?",
                            message: `${o.name || o.id} will stop waking ${slug}.`,
                            confirmLabel: "Remove",
                            danger: true,
                          },
                        },
                      )
                    }
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </span>
              </div>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {(o.triggers || []).map((t, i) => (
                  <span
                    key={i}
                    className="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-xs text-foreground/80"
                  >
                    {t.type === "event" ? (
                      <Zap className="size-2.5 text-muted" />
                    ) : (
                      <CalendarClock className="size-2.5 text-muted" />
                    )}
                    <span className="font-mono">
                      {t.type === "event" ? t.subject : t.schedule}
                    </span>
                  </span>
                ))}
              </div>
              {o.plan && (
                <div className="mt-1.5 text-[11px] text-muted">
                  {clip(o.plan, 200)}
                </div>
              )}
              <button
                onClick={() => setWhy(why === o.id ? null : o.id)}
                className="mt-1.5 text-xs text-accent hover:underline"
              >
                {why === o.id ? "hide history" : "firing history"}
              </button>
              {why === o.id && <WhyHistory id={o.id} />}
            </li>
          ))}
        </ul>
      )}
      <Button variant="ghost" size="sm" onClick={() => onManage("standing")}>
        Manage standing orders <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}


function MailboxWakeSubjects({
  slug,
  orders,
  busy,
  armIssue,
  armOwner,
  onCreate,
}: {
  slug: string;
  orders: ApiOrder[];
  busy: boolean;
  armIssue: string;
  armOwner: string;
  onCreate: (kind: "dm" | "help" | "broadcast", subject: string) => void;
}) {
  const subjects = agentMailboxSubjects(slug);
  if (subjects.length === 0) return null;
  const icon = { dm: Mail, help: LifeBuoy, broadcast: Megaphone };
  return (
    <div className="rounded-lg border border-border bg-panel/30 p-2.5">
      <div className="mb-1.5 flex items-center gap-1.5 text-xs uppercase tracking-normal text-muted">
        <Mail className="size-3" /> mailbox wake subjects
      </div>
      <div className="grid gap-1.5 md:grid-cols-3">
        {subjects.map((row) => {
          const bound = mailboxSubjectBinding(orders, row.subject);
          const Icon = icon[row.kind];
          return (
            <div
              key={row.subject}
              className="min-w-0 rounded-md border border-border bg-card px-2 py-1.5"
            >
              <div className="flex items-center gap-1.5">
                <Icon className="size-3.5 text-muted" />
                <span className="text-xs font-medium">{row.label}</span>
                <Badge variant={bound?.enabled ? "good" : "default"}>
                  {bound?.enabled ? "armed" : "idle"}
                </Badge>
              </div>
              <div className="mt-1 truncate font-mono text-xs text-muted">
                {row.subject}
              </div>
              {bound?.name && (
                <div className="mt-1 truncate text-xs text-foreground/75">
                  {bound.name}
                </div>
              )}
              {!bound && (
                <div className="mt-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy || !!armIssue}
                    className="h-6 px-1.5 text-xs"
                    aria-label={`Arm ${row.label} mailbox wake for ${slug}`}
                    title={armIssue || `Create a standing order that wakes ${slug} on ${row.subject}`}
                    onClick={() => onCreate(row.kind, row.subject)}
                  >
                    <Zap className="size-3" /> Arm wake
                  </Button>
                  {armIssue && (
                    <div className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted">
                      <span>{armIssue}</span>
                      {armOwner && (
                        <button
                          type="button"
                          className="font-mono text-accent hover:underline"
                          aria-label={`Open parent agent ${armOwner}`}
                          onClick={() => openAgent(armOwner)}
                        >
                          Open {armOwner}
                        </button>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

interface WhyEvent {
  seq?: number;
  kind?: string;
  ts_unix_ms?: number;
}
function WhyHistory({ id }: { id: string }) {
  const [events, setEvents] = useState<WhyEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ events?: WhyEvent[] }>("/api/standing/why", { id })
      .then((d) => alive && setEvents(d.events || []))
      .catch((e) => alive && setErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, [id]);
  if (err) return <ErrorText>{err}</ErrorText>;
  if (!events) return <SkeletonList count={2} lines={1} />;
  if (events.length === 0)
    return <div className="mt-1 text-[11px] text-muted">no history yet</div>;
  return (
    <ul className="mt-1 space-y-1">
      {events.map((e, i) => (
        <li key={e.seq ?? i} className="flex items-center gap-2 text-[11px]">
          <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-accent">
            {(e.kind || "").replace(/^standing\./, "")}
          </span>
          <span className="ml-auto font-mono text-xs text-muted">
            {fmtTime(e.ts_unix_ms)}
          </span>
        </li>
      ))}
    </ul>
  );
}

// ScheduleForecast shows the next few fire times of a schedule (the agent's
// near future), via the read-only /api/schedule/test dry-run.
function ScheduleForecast({
  id,
  fallbackNext,
}: {
  id: string;
  fallbackNext?: number;
}) {
  const [fires, setFires] = useState<number[] | null>(null);
  useEffect(() => {
    let alive = true;
    getJSON<{ forecasts?: { unix?: number }[] }>("/api/schedule/test", {
      id,
      count: "4",
    })
      .then(
        (d) =>
          alive &&
          setFires((d.forecasts || []).map((f) => f.unix || 0).filter(Boolean)),
      )
      .catch(() => alive && setFires([]));
    return () => {
      alive = false;
    };
  }, [id]);
  const list =
    fires && fires.length ? fires : fallbackNext ? [fallbackNext] : [];
  if (fires === null)
    return <div className="mt-1.5 text-xs text-muted">forecasting…</div>;
  if (list.length === 0)
    return (
      <div className="mt-1.5 text-xs text-muted">no upcoming runs</div>
    );
  return (
    <div className="mt-1.5 flex flex-wrap gap-1.5">
      {list.map((u, i) => (
        <span
          key={i}
          className="inline-flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-0.5 text-xs text-foreground/80"
        >
          <Clock className="size-2.5 text-muted" />
          {fmtDateTime(u * 1000)}
        </span>
      ))}
    </div>
  );
}

