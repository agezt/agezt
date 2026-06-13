import { useEffect, useRef, useState } from "react";
import { Users, RefreshCw, Pause, Play, Trash2, Plus, X, Pencil, Bot, Archive, ArchiveRestore, Skull, Activity, ListTree, ScrollText } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { useEvents } from "@/lib/events";
import { RunDetailLoader } from "@/components/RunDetail";

export interface AgentProfile {
  id: string;
  slug: string;
  name?: string;
  soul?: string;
  model?: string;
  fallbacks?: string[];
  task_type?: string;
  max_cost_mc?: number;
  max_daily_mc?: number;
  memory_scope?: string;
  workdir?: string;
  description?: string;
  enabled?: boolean;
  retired?: boolean;
}

// slugOk mirrors the kernel's roster slug rule (lowercase, digit/letter first,
// then letters/digits/dot/dash/underscore, ≤64) so the form can validate before
// the round-trip. Pure + unit-tested.
export function slugOk(s: string): boolean {
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(s);
}

// agentHue maps a slug to a stable hue (0–359) so every agent gets a consistent
// colored identity avatar across the UI. A tiny deterministic string hash —
// pure + unit-tested.
export function agentHue(slug: string): number {
  let h = 0;
  for (let i = 0; i < slug.length; i++) h = (h * 31 + slug.charCodeAt(i)) % 360;
  return h;
}

// initials derives a 1–2 char monogram for the avatar: from the name's words if
// present, else the first two slug characters. Pure + unit-tested.
export function initials(name: string | undefined, slug: string): string {
  const src = (name || "").trim();
  if (src) {
    const words = src.split(/\s+/).filter(Boolean);
    if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
    return src.slice(0, 2).toUpperCase();
  }
  return slug.slice(0, 2).toUpperCase();
}

// usdToMc converts a dollar string ("0.50", "$0.50") to USD-microcents
// ($1 = 1e9, the kernel's budget unit). Returns null for blank, NaN, or
// negative input. Pure + unit-tested.
export function usdToMc(s: string): number | null {
  const t = s.trim().replace(/^\$/, "");
  if (t === "") return 0;
  const v = Number(t);
  if (!Number.isFinite(v) || v < 0) return null;
  return Math.round(v * 1e9);
}

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent";

// AgentAvatar is the agent's colored identity monogram — a deterministic hue
// from the slug, dimmed when retired so the graveyard reads at a glance.
function AgentAvatar({ slug, name, retired }: { slug: string; name?: string; retired?: boolean }) {
  const hue = agentHue(slug);
  return (
    <span
      aria-hidden
      className={cn(
        "flex size-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold text-white",
        retired && "opacity-40 grayscale",
      )}
      style={{ backgroundColor: `hsl(${hue} 55% 42%)` }}
    >
      {initials(name, slug)}
    </span>
  );
}

// profileFields collects the shared New/Edit form fields into the wire shape.
function profileFields(f: {
  name: string;
  soul: string;
  model: string;
  fallbacks: string;
  taskType: string;
  maxCost: string;
  maxDaily: string;
  memoryScope: string;
  workdir: string;
  description: string;
}): Record<string, unknown> | string {
  const mc = usdToMc(f.maxCost);
  if (mc === null) return "max cost must be a dollar amount like 0.50";
  const dailyMc = usdToMc(f.maxDaily);
  if (dailyMc === null) return "max daily must be a dollar amount like 5.00";
  const p: Record<string, unknown> = {
    name: f.name.trim(),
    soul: f.soul.trim(),
    model: f.model.trim(),
    task_type: f.taskType.trim(),
    max_cost_mc: mc,
    max_daily_mc: dailyMc,
    memory_scope: f.memoryScope.trim(),
    workdir: f.workdir.trim(),
    description: f.description.trim(),
  };
  const fb = f.fallbacks
    .split(",")
    .map((m) => m.trim())
    .filter(Boolean);
  if (fb.length > 0) p.fallbacks = fb;
  return p;
}

// AgentFormFields renders the shared editable fields for New/Edit.
function AgentFormFields(props: {
  state: Record<string, string>;
  set: (key: string, value: string) => void;
}) {
  const { state, set } = props;
  const field = (label: string, key: string, placeholder: string, aria: string) => (
    <label className="flex flex-col gap-1 text-[11px] text-muted">
      {label}
      <input
        value={state[key] || ""}
        onChange={(e) => set(key, e.target.value)}
        placeholder={placeholder}
        aria-label={aria}
        className={inputCls}
      />
    </label>
  );
  return (
    <>
      <div className="grid gap-2 sm:grid-cols-2">
        {field("Name", "name", "e.g. The Researcher", "Agent name")}
        {field("Model", "model", "blank = daemon default", "Agent model")}
        {field("Fallback models (comma-separated)", "fallbacks", "m1, m2", "Fallback models")}
        {field("Task type", "taskType", "e.g. research, code", "Task type")}
        {field("Max cost per run (USD)", "maxCost", "e.g. 0.50 — blank = no cap", "Max cost per run")}
        {field("Max cost per day (USD)", "maxDaily", "e.g. 5.00 — blank = no cap", "Max cost per day")}
        {field("Memory scope", "memoryScope", "blank = the slug", "Memory scope")}
        {field("Workdir (workspace-relative)", "workdir", "e.g. research", "Workdir")}
        {field("Description", "description", "what this agent is for", "Description")}
      </div>
      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Soul — who this agent IS (its system prompt)
        <textarea
          value={state.soul || ""}
          onChange={(e) => set("soul", e.target.value)}
          placeholder="You are Researcher. You dig deep and cite sources."
          aria-label="Agent soul"
          rows={3}
          className={cn(inputCls, "resize-y")}
        />
      </label>
    </>
  );
}

// NewAgentForm creates a roster profile (M785). Exported for tests and reuse
// (the M714 "creatable from UI" recipe).
export function NewAgentForm({
  onCreated,
  onError,
}: {
  onCreated: (slug: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));
  const slug = (state.slug || "").trim();
  const valid = slugOk(slug);

  async function create() {
    if (!valid) return;
    const fields = profileFields({
      name: state.name || "",
      soul: state.soul || "",
      model: state.model || "",
      fallbacks: state.fallbacks || "",
      taskType: state.taskType || "",
      maxCost: state.maxCost || "",
      maxDaily: state.maxDaily || "",
      memoryScope: state.memoryScope || "",
      workdir: state.workdir || "",
      description: state.description || "",
    });
    if (typeof fields === "string") {
      onError(fields);
      return;
    }
    setSubmitting(true);
    try {
      await postJSON("/api/agents/add", { profile: { slug, ...fields } });
      onCreated(slug);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Slug — the agent's permanent handle (lowercase; cannot be changed later)
        <input
          value={state.slug || ""}
          onChange={(e) => set("slug", e.target.value)}
          placeholder="e.g. researcher"
          aria-label="Agent slug"
          className={cn(inputCls, slug !== "" && !valid && "border-bad")}
        />
      </label>
      <div className="mt-2">
        <AgentFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          <Plus className="h-3.5 w-3.5" /> Create agent
        </Button>
      </div>
    </div>
  );
}

// EditAgentForm edits a profile's mutable fields (the slug is the agent's
// address — immutable, shown but not editable).
export function EditAgentForm({
  profile,
  onSaved,
  onError,
}: {
  profile: AgentProfile;
  onSaved: (slug: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({
    name: profile.name || "",
    soul: profile.soul || "",
    model: profile.model || "",
    fallbacks: (profile.fallbacks || []).join(", "),
    taskType: profile.task_type || "",
    maxCost: profile.max_cost_mc ? String(profile.max_cost_mc / 1e9) : "",
    maxDaily: profile.max_daily_mc ? String(profile.max_daily_mc / 1e9) : "",
    memoryScope: profile.memory_scope || "",
    workdir: profile.workdir || "",
    description: profile.description || "",
  });
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));

  async function save() {
    const fields = profileFields({
      name: state.name,
      soul: state.soul,
      model: state.model,
      fallbacks: state.fallbacks,
      taskType: state.taskType,
      maxCost: state.maxCost,
      maxDaily: state.maxDaily,
      memoryScope: state.memoryScope,
      workdir: state.workdir,
      description: state.description,
    });
    if (typeof fields === "string") {
      onError(fields);
      return;
    }
    setSubmitting(true);
    try {
      await postJSON("/api/agents/edit", { ref: profile.slug, profile: fields });
      onSaved(profile.slug);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="text-[11px] text-muted">
        Editing <span className="font-mono text-foreground">{profile.slug}</span> (slug is permanent)
      </div>
      <div className="mt-2">
        <AgentFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={save} disabled={submitting}>
          <Pencil className="h-3.5 w-3.5" /> Save
        </Button>
      </div>
    </div>
  );
}

// Roster is the agent-identity console (M785): the durable, named agents
// (M783) — each with its own soul, model, cost ceiling, and memory scope —
// with create/edit/pause/resume/remove governance. Run one from chat or the
// CLI with `agt run --agent <slug>`; the lead delegates to them by name.
interface ActivityItem {
  seq: number;
  kind: string;
  ts_unix_ms?: number;
  correlation_id?: string;
  summary: string;
}

// RunLite is the subset of /api/runs we need to list an agent's own runs and
// derive its spend/last-active summary. Mirrors ApiRun in Agents.tsx.
interface RunLite {
  correlation_id?: string;
  status?: string;
  model?: string;
  spent_mc?: number;
  iters?: number;
  intent?: string;
  agent?: string;
  started_unix_ms?: number;
}

type DrillTab = "activity" | "runs" | "logs";

// AgentActivity is the per-agent drill-in (M941): a tabbed, live view of one
// agent — its journal-derived activity timeline (M854), its own runs (with
// inline run detail + steer), and a live raw-log tail filtered from the SSE
// stream. Re-fetches on any event attributable to the agent so it tracks a
// running agent in real time. Reuses RunDetailLoader, the Skeleton kit, and
// the live events context.
function AgentActivity({ slug }: { slug: string }) {
  const { events, subscribe } = useEvents();
  const [tab, setTab] = useState<DrillTab>("activity");
  const [items, setItems] = useState<ActivityItem[] | null>(null);
  const [runs, setRuns] = useState<RunLite[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [openRun, setOpenRun] = useState<string | null>(null);
  const [bump, setBump] = useState(0);

  // Re-load the digested timeline + run list whenever the agent changes or a
  // fresh event for this agent lands (debounced via the bump counter below).
  useEffect(() => {
    let alive = true;
    Promise.all([
      getJSON<{ activity?: ActivityItem[] }>("/api/agents/activity", { ref: slug, limit: "60" }),
      getJSON<{ runs?: RunLite[] }>("/api/runs"),
    ])
      .then(([a, r]) => {
        if (!alive) return;
        setItems(a.activity || []);
        setRuns((r.runs || []).filter((x) => x.agent === slug));
      })
      .catch((e) => alive && setErr((e as Error).message));
    return () => {
      alive = false;
    };
  }, [slug, bump]);

  // Live: any event whose actor is this agent triggers a debounced refetch so
  // the timeline/runs track a running agent without a manual reload.
  useEffect(() => {
    let t: ReturnType<typeof setTimeout> | undefined;
    const off = subscribe((e) => {
      if (e.actor !== slug) return;
      if (t) clearTimeout(t);
      t = setTimeout(() => setBump((b) => b + 1), 700);
    });
    return () => {
      if (t) clearTimeout(t);
      off();
    };
  }, [slug, subscribe]);

  // Live raw-log tail: the rolling SSE buffer filtered to this agent. No fetch —
  // it updates as events arrive.
  const logs = events.filter((e) => e.actor === slug).slice(0, 60);

  const runCount = runs?.length ?? 0;
  const totalSpent = (runs || []).reduce((s, r) => s + (r.spent_mc || 0), 0);
  const lastActive = items && items.length > 0 ? items[0].ts_unix_ms : undefined;

  const tabBtn = (id: DrillTab, label: string, count: number | undefined, Icon: typeof Activity) => (
    <button
      onClick={() => setTab(id)}
      className={cn(
        "flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium transition-colors",
        tab === id ? "bg-card text-foreground" : "text-muted hover:text-foreground",
      )}
    >
      <Icon className="h-3 w-3" />
      {label}
      {count !== undefined && count > 0 && (
        <span className="ml-0.5 rounded bg-panel px-1 font-mono text-[9px] text-muted">{count}</span>
      )}
    </button>
  );

  return (
    <div className="mt-2 rounded-md border border-border bg-panel/60 p-2">
      {/* summary band */}
      <div className="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-muted">
        <span><span className="font-mono text-foreground">{runCount}</span> runs</span>
        <span><span className="font-mono text-foreground">{money(totalSpent)}</span> spent</span>
        <span>last active <span className="font-mono text-foreground/85">{lastActive ? fmtTime(lastActive) : "—"}</span></span>
      </div>

      {/* tabs */}
      <div className="mb-2 flex items-center gap-1 border-b border-border pb-1">
        {tabBtn("activity", "Activity", items?.length, Activity)}
        {tabBtn("runs", "Runs", runCount, ListTree)}
        {tabBtn("logs", "Logs", logs.length, ScrollText)}
      </div>

      {err && <ErrorText>{err}</ErrorText>}

      {tab === "activity" && (
        !items ? (
          <SkeletonList count={4} lines={1} />
        ) : items.length === 0 ? (
          <div className="text-xs text-muted">no recorded activity yet</div>
        ) : (
          <ul className="space-y-1">
            {items.map((a) => {
              const open = !!a.correlation_id && openRun === a.correlation_id;
              return (
                <li key={a.seq} className="text-xs">
                  <div
                    className={cn("flex items-start gap-2", a.correlation_id && "cursor-pointer")}
                    onClick={() => a.correlation_id && setOpenRun(open ? null : a.correlation_id!)}
                  >
                    <span className="shrink-0 rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{a.kind}</span>
                    <span className="text-foreground/85">{a.summary}</span>
                    <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(a.ts_unix_ms)}</span>
                  </div>
                  {open && (
                    <div className="mt-1 pl-2">
                      <RunDetailLoader correlationId={a.correlation_id} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )
      )}

      {tab === "runs" && (
        !runs ? (
          <SkeletonList count={3} lines={1} />
        ) : runs.length === 0 ? (
          <div className="text-xs text-muted">this agent has no runs yet</div>
        ) : (
          <ul className="space-y-1">
            {runs.map((r) => {
              const open = openRun === r.correlation_id;
              return (
                <li key={r.correlation_id} className="text-xs">
                  <div
                    className="flex cursor-pointer items-center gap-2"
                    onClick={() => setOpenRun(open ? null : r.correlation_id || null)}
                  >
                    <Badge variant={statusVariant(r.status)}>{r.status || "?"}</Badge>
                    <span className="truncate text-foreground/85">{r.intent || r.correlation_id || "run"}</span>
                    <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">
                      {r.spent_mc ? money(r.spent_mc) + " · " : ""}{fmtTime(r.started_unix_ms)}
                    </span>
                  </div>
                  {open && (
                    <div className="mt-1 pl-2">
                      <RunDetailLoader correlationId={r.correlation_id} status={r.status} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )
      )}

      {tab === "logs" && (
        logs.length === 0 ? (
          <div className="text-xs text-muted">no live events from this agent yet</div>
        ) : (
          <ul className="space-y-1">
            {logs.map((e, i) => (
              <li key={e.id || e.seq || i} className="flex items-start gap-2 text-xs">
                <span className="shrink-0 rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{e.kind}</span>
                <span className="truncate text-foreground/85">{e.subject || ""}</span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(e.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        )
      )}
    </div>
  );
}

export function Roster() {
  const ui = useUI();
  const [profiles, setProfiles] = useState<AgentProfile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [activityFor, setActivityFor] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ profiles?: AgentProfile[] }>("/api/agents");
      setProfiles(d.profiles || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    const t = setInterval(reload, 8000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(
    ref: string,
    path: string,
    params?: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(ref);
    try {
      await postAction(path, { ref, ...params });
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  // retire moves an agent to the graveyard (M846): fetch the impact first (which
  // standing orders fire it) and show it in the confirm, so the effects are
  // explicit before the agent is retired. Recoverable via Revive.
  async function retire(slug: string) {
    let impactLine = "";
    try {
      const imp = await getJSON<{ standing_orders?: string[]; standing_count?: number }>("/api/agents/impact", { ref: slug });
      if (imp.standing_count && imp.standing_count > 0) {
        impactLine = `\n\n⚠ ${imp.standing_count} standing order(s) fire this agent and will stop running it:\n• ${(imp.standing_orders || []).join("\n• ")}`;
      }
    } catch {
      // Impact is advisory; proceed without it if the lookup fails.
    }
    await act(slug, "/api/agents/retire", undefined, {
      confirm: {
        title: `Retire agent ${slug} to the graveyard?`,
        message: `It is kept and recoverable (Revive), but paused and excluded from delegation.${impactLine}`,
        confirmLabel: "Retire",
        danger: true,
      },
      success: `${slug} retired to the graveyard`,
    });
  }

  const list = profiles || [];
  const enabled = list.filter((p) => p.enabled && !p.retired).length;
  const paused = list.filter((p) => !p.enabled && !p.retired).length;
  const graveyard = list.filter((p) => p.retired).length;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Users className="h-4 w-4 text-accent" />
          <h2 className="text-sm font-semibold">Agent roster</h2>
          {profiles && (
            <span className="text-xs text-muted">
              {profiles.length} agent(s) · {enabled} enabled
            </span>
          )}
        </div>
        <div className="flex items-center gap-1.5">
          <Button size="sm" variant="ghost" onClick={reload} disabled={loading} aria-label="Refresh">
            <RefreshCw className={cn("h-3.5 w-3.5", loading && "animate-spin")} />
          </Button>
          <Button size="sm" onClick={() => setShowForm((v) => !v)}>
            {showForm ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
            {showForm ? "Close" : "New agent"}
          </Button>
        </div>
      </div>

      {showForm && (
        <NewAgentForm
          onCreated={(slug) => {
            setShowForm(false);
            ui.toast(`agent ${slug} created`, "success");
            reload();
          }}
          onError={(msg) => ui.toast(msg, "error")}
        />
      )}

      {/* Summary band — the roster at a glance. */}
      {profiles && profiles.length > 0 && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <RosterStat label="agents" value={list.length} />
          <RosterStat label="enabled" value={enabled} accent={enabled > 0} />
          <RosterStat label="paused" value={paused} />
          <RosterStat label="graveyard" value={graveyard} />
        </div>
      )}

      {err && <ErrorText>{err}</ErrorText>}
      {!profiles && !err && <SkeletonList count={3} />}
      {profiles && profiles.length === 0 && !showForm && (
        <EmptyState
          icon={Bot}
          title="No agents yet"
          hint='Create a named agent — its soul, model, and budget — then run it with "agt run --agent <slug>" or delegate to it by name.'
        />
      )}

      <ul className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
        {(profiles || []).map((p) => {
          const open = editing === p.slug || activityFor === p.slug;
          return (
          <li
            key={p.id}
            className={cn(
              "rounded-lg border border-border bg-card p-3",
              open && "sm:col-span-2 xl:col-span-3",
            )}
          >
            <div className="flex flex-wrap items-center gap-2">
              <AgentAvatar slug={p.slug} name={p.name} retired={p.retired} />
              <span className={cn("font-mono text-sm", p.retired ? "text-muted line-through" : "text-foreground")}>{p.slug}</span>
              {p.name && p.name !== p.slug && <span className="text-xs text-muted">{p.name}</span>}
              {p.retired ? (
                <Badge variant="default" className="inline-flex items-center gap-1 text-muted">
                  <Skull className="h-3 w-3" /> graveyard
                </Badge>
              ) : (
                <Badge variant={p.enabled ? "good" : "default"}>{p.enabled ? "enabled" : "paused"}</Badge>
              )}
              <span className="ml-auto flex items-center gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Activity for ${p.slug}`}
                  title="What this agent did (runs, consults, memory, messages)"
                  onClick={() => setActivityFor(activityFor === p.slug ? null : p.slug)}
                >
                  <Activity className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Edit ${p.slug}`}
                  onClick={() => setEditing(editing === p.slug ? null : p.slug)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                {!p.retired && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={p.enabled ? `Pause ${p.slug}` : `Resume ${p.slug}`}
                    onClick={() =>
                      act(p.slug, "/api/agents/enable", { enabled: p.enabled ? "false" : "true" }, {
                        success: p.enabled ? `${p.slug} paused` : `${p.slug} resumed`,
                      })
                    }
                  >
                    {p.enabled ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
                  </Button>
                )}
                {p.retired ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Revive ${p.slug}`}
                    title="Revive from the graveyard"
                    onClick={() => act(p.slug, "/api/agents/revive", undefined, { success: `${p.slug} revived (paused)` })}
                  >
                    <ArchiveRestore className="h-3.5 w-3.5" />
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy === p.slug}
                    aria-label={`Retire ${p.slug}`}
                    title="Retire to the graveyard"
                    onClick={() => retire(p.slug)}
                  >
                    <Archive className="h-3.5 w-3.5" />
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy === p.slug}
                  aria-label={`Remove ${p.slug}`}
                  onClick={() =>
                    act(p.slug, "/api/agents/remove", undefined, {
                      confirm: {
                        title: `Remove agent ${p.slug}?`,
                        message: "Its profile (soul, model, budget) is deleted. Past runs stay in the journal.",
                        confirmLabel: "Remove",
                        danger: true,
                      },
                      success: `${p.slug} removed`,
                    })
                  }
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </span>
            </div>
            <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted">
              <span>model: {p.model || "(default)"}</span>
              {p.task_type && <span>task: {p.task_type}</span>}
              {(p.max_cost_mc || 0) > 0 && <span>max/run: {money(p.max_cost_mc)}</span>}
              {(p.max_daily_mc || 0) > 0 && <span>max/day: {money(p.max_daily_mc)}</span>}
              {p.memory_scope && <span>memory: {p.memory_scope}</span>}
              {p.workdir && <span>workdir: {p.workdir}</span>}
              {(p.fallbacks || []).length > 0 && <span>fallbacks: {(p.fallbacks || []).join(" → ")}</span>}
            </div>
            {p.description && <div className="mt-1 text-xs text-muted">{p.description}</div>}
            {p.soul && (
              <div className="mt-1.5 rounded-md bg-panel px-2 py-1.5 text-xs text-muted whitespace-pre-wrap">
                {p.soul}
              </div>
            )}
            {editing === p.slug && (
              <div className="mt-2">
                <EditAgentForm
                  profile={p}
                  onSaved={(slug) => {
                    setEditing(null);
                    ui.toast(`agent ${slug} updated`, "success");
                    reload();
                  }}
                  onError={(msg) => ui.toast(msg, "error")}
                />
              </div>
            )}
            {activityFor === p.slug && <AgentActivity slug={p.slug} />}
          </li>
          );
        })}
      </ul>
    </div>
  );
}

function RosterStat({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  return (
    <div className={cn("rounded-lg border bg-card p-2.5", accent ? "border-accent/50" : "border-border")}>
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}
