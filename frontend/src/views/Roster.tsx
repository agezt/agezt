import { useEffect, useRef, useState } from "react";
import { Users, RefreshCw, Pause, Play, Trash2, Plus, X, Pencil, Bot, Archive, ArchiveRestore, Skull, Activity, Sparkles, IdCard, ShieldCheck } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { openAgent } from "@/lib/agentnav";
import { cn } from "@/lib/utils";
import { money } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { AgentAvatar } from "@/components/AgentAvatar";
import { AgentActivity } from "@/components/AgentActivity";
import { ModelPicker } from "@/components/ModelPicker";
import { isChainRef } from "@/lib/chains";

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
  system?: boolean; // shipped internal guardian (M961) — protected from removal
}

// slugOk mirrors the kernel's roster slug rule (lowercase, digit/letter first,
// then letters/digits/dot/dash/underscore, ≤64) so the form can validate before
// the round-trip. Pure + unit-tested.
export function slugOk(s: string): boolean {
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(s);
}

// agentHue maps a slug to a stable hue (0–359) so every agent gets a consistent
// colored identity avatar across the UI. The deterministic hue + monogram now
// live in @/lib/agent (M948) so the avatar can be shared; re-exported here for
// existing importers.
import { agentHue, initials } from "@/lib/agent";
export { agentHue, initials };

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

// profileFields collects the shared New/Edit form fields into the wire shape.
// Exported for unit tests (the model/fallbacks mapping, incl. the @chain
// self-contained rule, is the meaningful logic).
export function profileFields(f: {
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
  // A "@chain" model is self-contained — its fallback ladder lives in the chain,
  // so per-agent fallbacks are ignored (and the form hides the field).
  if (!isChainRef(p.model as string)) {
    const fb = f.fallbacks
      .split(",")
      .map((m) => m.trim())
      .filter(Boolean);
    if (fb.length > 0) p.fallbacks = fb;
  }
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
  const modelIsChain = isChainRef(state.model || "");
  return (
    <>
      <div className="grid gap-2 sm:grid-cols-2">
        {field("Name", "name", "e.g. The Researcher", "Agent name")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Model
          <div className="flex h-[30px] items-center">
            <ModelPicker value={state.model || ""} activeModel="daemon default" onChange={(id) => set("model", id)} />
          </div>
          {modelIsChain && <span className="text-[10px] text-accent">chain is self-contained — fallbacks come from @{state.model.slice(1)}</span>}
        </label>
        {modelIsChain
          ? null
          : field("Fallback models (comma-separated)", "fallbacks", "m1, m2", "Fallback models")}
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
export function Roster() {
  const ui = useUI();
  const [profiles, setProfiles] = useState<AgentProfile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [activityFor, setActivityFor] = useState<string | null>(null);
  // Per-agent private-skill counts (M943): how many skills each agent owns
  // (Skill.Agent == slug), so an operator sees who has learned what before
  // sharing/reassigning (M942) or exporting (`agt skill export --all --agent`).
  const [skillCounts, setSkillCounts] = useState<Record<string, number>>({});
  // Keep the interval handle so a refresh-on-event nudge can coexist with the
  // poll without leaking timers (preserves the original useRef import).
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const [d, sk] = await Promise.all([
        getJSON<{ profiles?: AgentProfile[] }>("/api/agents"),
        getJSON<{ skills?: { agent?: string }[] }>("/api/skills").catch(() => ({ skills: [] })),
      ]);
      setProfiles(d.profiles || []);
      const counts: Record<string, number> = {};
      for (const s of sk.skills || []) {
        if (s.agent) counts[s.agent] = (counts[s.agent] || 0) + 1;
      }
      setSkillCounts(counts);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
    pollRef.current = setInterval(reload, 8000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
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
              "rounded-lg border border-border bg-card p-3 shadow-e1 transition-[box-shadow,border-color] hover:shadow-e2",
              open && "sm:col-span-2 xl:col-span-3",
            )}
          >
            <div className="flex flex-wrap items-center gap-2">
              <button onClick={() => openAgent(p.slug)} title="Open identity page" className="shrink-0">
                <AgentAvatar slug={p.slug} name={p.name} status={p.retired ? "retired" : undefined} />
              </button>
              <button
                onClick={() => openAgent(p.slug)}
                title="Open identity page"
                className={cn("font-mono text-sm hover:underline", p.retired ? "text-muted line-through" : "text-foreground")}
              >
                {p.slug}
              </button>
              {p.name && p.name !== p.slug && <span className="text-xs text-muted">{p.name}</span>}
              {p.system && (
                <span
                  className="inline-flex items-center gap-1 rounded-full bg-accent/15 px-1.5 py-0.5 text-[10px] font-medium text-accent"
                  title="Shipped internal guardian — part of the daemon's self-healing fleet (protected from removal)"
                >
                  <ShieldCheck className="h-2.5 w-2.5" /> guardian
                </span>
              )}
              {p.retired ? (
                <Badge variant="default" className="inline-flex items-center gap-1 text-muted">
                  <Skull className="h-3 w-3" /> graveyard
                </Badge>
              ) : (
                <Badge variant={p.enabled ? "good" : "default"}>{p.enabled ? "enabled" : "paused"}</Badge>
              )}
              {skillCounts[p.slug] > 0 && (
                <span
                  className="inline-flex items-center gap-1 rounded-full bg-accent/10 px-1.5 py-0.5 text-[10px] text-accent"
                  title={`${skillCounts[p.slug]} skill(s) private to this agent`}
                >
                  <Sparkles className="h-2.5 w-2.5" /> {skillCounts[p.slug]}
                </span>
              )}
              <span className="ml-auto flex items-center gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Identity page for ${p.slug}`}
                  title="Open the full identity page (everything about this agent)"
                  onClick={() => openAgent(p.slug)}
                >
                  <IdCard className="h-3.5 w-3.5" />
                </Button>
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
                {!p.system && (
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
                )}
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
    <div className={cn("rounded-lg border bg-card p-2.5 shadow-e1", accent ? "border-accent/50" : "border-border")}>
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}
