import { useEffect, useState, type ReactNode } from "react";
import { Sparkles, RefreshCw, ChevronRight, ChevronDown, Check, ShieldX, Undo2, Plus, Pencil, Search, Bot, Share2, AlertTriangle, X, FileText, Tags, History, type LucideIcon } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime, fmtAgo } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { PageHeader } from "@/components/ui/page-header";
import { ErrorText } from "@/components/JsonView";
import { Badge } from "@/components/ui/badge";
import { Disclosure } from "@/components/ui/disclosure";

interface Skill {
  id?: string;
  name?: string;
  description?: string;
  body?: string;
  status?: string;
  version?: number;
  // Owning roster agent (M932): a skill the agent learned itself, retrieved
  // only when IT acts. Empty/absent = shared pool.
  agent?: string;
  triggers?: string[];
  tools_required?: string[];
  created_ms?: number;
  metrics?: {
    shadow_wins?: number;
    shadow_evals?: number;
    uses?: number;
    wins?: number;
    successes?: number;
    failures?: number;
    last_used_ms?: number;
  };
  // Present on the hygiene (idle) list (M858).
  uses?: number;
  last_used_ms?: number;
}

const statusVariant: Record<string, "good" | "accent" | "default" | "bad"> = {
  active: "good",
  shadow: "accent",
  draft: "default",
  quarantined: "bad",
};

// skillMatches tests a skill against a lowercased query over its name, description,
// status, owning agent, triggers, and required tools — so a growing library stays
// findable by what a skill does, when it fires, what it needs, its lifecycle state
// (M778), or whose private skill it is (M932).
export function skillMatches(s: Skill, q: string): boolean {
  if (!q) return true;
  const hay = [s.name, s.description, s.status, s.agent, ...(s.triggers || []), ...(s.tools_required || [])]
    .filter((x): x is string => typeof x === "string")
    .join(" ")
    .toLowerCase();
  return hay.includes(q);
}

// Skills is the learned-procedure library: each skill is a card with its status,
// description, triggers, required tools, shadow/usage metrics and the full
// procedure body (expandable), plus its promote / quarantine / revert controls.
export function Skills() {
  const ui = useUI();
  const [skills, setSkills] = useState<Skill[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [authoring, setAuthoring] = useState<Skill | "new" | null>(null);
  const [q, setQ] = useState("");
  const [idle, setIdle] = useState<Skill[]>([]);

  async function reload() {
    setLoading(true);
    try {
      const [d, h] = await Promise.all([
        getJSON<{ skills?: Skill[] }>("/api/skills"),
        getJSON<{ idle?: Skill[] }>("/api/skills/hygiene").catch(() => ({ idle: [] }) as { idle?: Skill[] }),
      ]);
      setSkills(d.skills || []);
      setIdle(h.idle || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
  }, []);

  async function act(id: string, path: string, opts?: { confirm?: ConfirmOptions; success?: string }) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(id);
    try {
      await postAction(path, { id });
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={Sparkles}
        title="Skills"
        actions={
          <>
            <Button
              size="sm"
              onClick={() => {
                setAuthoring("new");
              }}
              title="Author a skill"
            >
              <Plus className="size-3.5" /> Author skill
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {idle.length > 0 && (
        <SkillOpsPanel
          icon={AlertTriangle}
          title="Idle skills"
          status={`${idle.length} stale`}
          tone="warn"
        >
          <ul className="space-y-1">
            {idle.map((s) => (
              <li key={s.id} className="flex items-center gap-2 text-xs">
                <span className="min-w-0 flex-1 truncate font-medium text-foreground/85">{s.name}</span>
                <span className="shrink-0 text-xs text-muted">{(s.uses ?? 0) === 0 ? "never used" : `${s.uses} uses`}</span>
                <button
                  onClick={() =>
                    act(s.id!, "/api/skill/quarantine", {
                      confirm: {
                        title: `Retire idle skill "${s.name}"?`,
                        message: "It's pulled from the retrieval pool (quarantined). Reversible — promote it again to restore.",
                        confirmLabel: "Retire",
                        danger: true,
                      },
                      success: "Idle skill retired",
                    })
                  }
                  disabled={busy === s.id}
                  className="shrink-0 rounded px-1.5 py-0.5 text-xs text-bad hover:bg-bad/10"
                >
                  retire
                </button>
              </li>
            ))}
          </ul>
        </SkillOpsPanel>
      )}

      {authoring && (
        <SkillModal
          title={authoring === "new" ? "Author skill" : `Revise ${authoring.name || "skill"}`}
          icon={authoring === "new" ? Plus : Pencil}
          onClose={() => setAuthoring(null)}
        >
          <AuthorSkillForm
            key={authoring === "new" ? "new" : authoring.id || authoring.name || "edit"}
            initial={authoring === "new" ? undefined : authoring}
            onCreated={(name, status) => {
              const editing = authoring !== "new";
              setAuthoring(null);
              ui.toast(editing ? `Saved new version of "${name}" (${status})` : `Skill "${name}" created (${status})`, "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
          />
        </SkillModal>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !skills ? (
        <SkeletonList count={4} lines={2} />
      ) : skills.length === 0 ? (
        <EmptyState
          icon={Sparkles}
          title="No skills learned yet"
          hint={
            <>
              Skills are reusable procedures the agent distills from successful runs — it learns them with the{" "}
              <code className="rounded bg-panel px-1 py-0.5">skill</code> tool, then proves them in shadow before they go active.
            </>
          }
        />
      ) : (
        <div className="min-h-0 flex-1 space-y-2 overflow-auto">
          <StatusSummary skills={skills} />
          {/* Find a skill by name, what it does, when it fires, or its status (M778). */}
          {skills.length > 4 && (
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
          placeholder="filter skills..."
                aria-label="Filter skills"
                className="h-8 w-full rounded-md border border-border bg-panel pl-7 pr-12 text-xs text-foreground outline-none focus-visible:border-accent"
              />
              {q.trim() && (
                <span className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted">
                  {skills.filter((s) => skillMatches(s, q.trim().toLowerCase())).length}/{skills.length}
                </span>
              )}
            </div>
          )}
          {(() => {
            const query = q.trim().toLowerCase();
            const shown = query ? skills.filter((s) => skillMatches(s, query)) : skills;
            if (shown.length === 0) return <p className="px-1 py-2 text-xs text-muted">no skills match "{q.trim()}"</p>;
            return shown.map((s, i) => {
            const m = s.metrics || {};
            const isOpen = open === (s.id || String(i));
            return (
              <div key={s.id || i} className="glass rounded-xl p-3">
                <div className="flex items-center gap-2">
                  <Badge variant={statusVariant[s.status || ""] || "default"}>
                    {s.status || "?"}
                  </Badge>
                  <span className="truncate text-sm font-semibold">{s.name || "—"}</span>
                  {s.version != null && <span className="text-xs text-muted">v{s.version}</span>}
                  {s.agent && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-accent/10 px-1.5 py-0.5 text-xs text-accent"
                      title={`private skill — only the "${s.agent}" agent retrieves it`}
                    >
                      <Bot className="size-2.5" /> {s.agent}
                    </span>
                  )}
                  <div className="ml-auto flex shrink-0 gap-1">
                    {s.id && s.body && (
                      <IconBtn
                        label="revise"
                        tone="muted"
                        icon={Pencil}
                        onClick={() => setAuthoring(s)}
                      />
                    )}
                    {s.agent && s.id && (
                      <IconBtn
                        label="share"
                        tone="good"
                        icon={Share2}
                        busy={busy === s.id}
                        onClick={() =>
                          act(s.id!, "/api/skill/share", {
                            confirm: {
                              title: "Share this skill with every agent?",
                              message: s.name
                                ? `"${s.name}" is private to "${s.agent}". Sharing moves it into the pool every agent retrieves.`
                                : `This skill is private to "${s.agent}". Sharing moves it into the pool every agent retrieves.`,
                              confirmLabel: "Share",
                            },
                            success: "Skill shared with every agent",
                          })
                        }
                      />
                    )}
                    {(s.status === "draft" || s.status === "shadow") && s.id && (
                      <IconBtn
                        label="promote"
                        tone="good"
                        icon={Check}
                        busy={busy === s.id}
                        onClick={() => act(s.id!, "/api/skill/promote", { success: "Skill promoted to active" })}
                      />
                    )}
                    {s.status === "active" && s.id && (
                      <IconBtn
                        label="quarantine"
                        tone="bad"
                        icon={ShieldX}
                        busy={busy === s.id}
                        onClick={() =>
                          act(s.id!, "/api/skill/quarantine", {
                            confirm: {
                              title: "Quarantine this skill?",
                              message: s.name
                                ? `"${s.name}" will stop being used until you reinstate it.`
                                : "This skill will stop being used until you reinstate it.",
                              confirmLabel: "Quarantine",
                              danger: true,
                            },
                            success: "Skill quarantined",
                          })
                        }
                      />
                    )}
                    {s.id && (
                      <IconBtn
                        label="revert"
                        tone="muted"
                        icon={Undo2}
                        busy={busy === s.id}
                        onClick={() =>
                          act(s.id!, "/api/skill/revert", {
                            confirm: {
                              title: "Revert this skill?",
                              message: "The most recent change to this skill will be rolled back.",
                              confirmLabel: "Revert",
                              danger: true,
                            },
                            success: "Skill reverted",
                          })
                        }
                      />
                    )}
                  </div>
                </div>

                {s.description && <p className="mt-1.5 text-xs text-foreground/85">{s.description}</p>}

                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
                  {s.created_ms ? <span>{fmtTime(s.created_ms)}</span> : null}
                  {(m.shadow_evals || 0) > 0 && (
                    <span className="text-accent">
                      shadow {m.shadow_wins || 0}/{m.shadow_evals}
                    </span>
                  )}
                  {(m.uses || 0) > 0 ? (
                    <span>
                      used {m.uses}×{` `}
                      {(m.successes ?? m.wins) != null ? ` · ${m.successes ?? m.wins} ok` : ""}
                      {m.last_used_ms ? ` · last ${fmtAgo(m.last_used_ms)}` : ""}
                    </span>
                  ) : (
                    s.status === "active" && <span className="text-amber-500/80">idle · never used</span>
                  )}
                  {s.triggers?.length ? <span>triggers: {s.triggers.slice(0, 3).join(", ")}</span> : null}
                  {s.tools_required?.length ? <span>tools: {s.tools_required.join(", ")}</span> : null}
                </div>

                {s.body && (
                  <>
                    <button
                      onClick={() => setOpen(isOpen ? null : s.id || String(i))}
                      className="mt-2 flex items-center gap-1 text-[11px] text-muted hover:text-foreground"
                    >
                      {isOpen ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />} procedure
                    </button>
                    {isOpen && (
                      <pre className="mt-1 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-panel p-2 text-[11px] text-foreground/85">
                        {s.body}
                      </pre>
                    )}
                  </>
                )}
              </div>
            );
            });
          })()}
        </div>
      )}
    </div>
  );
}

function SkillModal({
  title,
  icon: Icon,
  children,
  onClose,
}: {
  title: string;
  icon: LucideIcon;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Icon className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Draft now, promote after it proves itself.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close skill modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}

// STATUS_ORDER + STATUS_BAR drive the lifecycle breakdown: the order skills flow
// through (draft → shadow → active, with quarantined/archived off to the side) and
// the bar/text colour for each.
const STATUS_ORDER = ["active", "shadow", "draft", "quarantined", "archived"] as const;
const STATUS_BAR: Record<string, { bar: string; text: string }> = {
  active: { bar: "bg-good", text: "text-good" },
  shadow: { bar: "bg-accent", text: "text-accent" },
  draft: { bar: "bg-muted", text: "text-foreground" },
  quarantined: { bar: "bg-bad", text: "text-bad" },
  archived: { bar: "bg-panel", text: "text-muted" },
};

function SkillOpsPanel({
  icon: Icon,
  title,
  status,
  tone,
  children,
}: {
  icon: LucideIcon;
  title: string;
  status: string;
  tone: "warn" | "accent" | "bad" | "muted";
  children: ReactNode;
}) {
  const toneCls = {
    warn: "border-warn/35 bg-warn/5 text-warn",
    accent: "border-accent/35 bg-accent/5 text-accent",
    bad: "border-bad/35 bg-bad/5 text-bad",
    muted: "border-border bg-panel text-muted",
  }[tone];
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg border", toneCls)}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}

// StatusSummary shows the skill library's lifecycle health at a glance: a stacked
// proportion bar of the statuses plus a count chip per status, so you can see how
// many skills are live vs still being proven vs pulled — instead of scanning the
// per-card badges.
function StatusSummary({ skills }: { skills: Skill[] }) {
  const counts: Record<string, number> = {};
  for (const s of skills) counts[s.status || "draft"] = (counts[s.status || "draft"] || 0) + 1;
  const present = STATUS_ORDER.filter((s) => counts[s] > 0);
  const total = skills.length;
  return (
    <div className="glass rounded-xl p-3">
      <div className="flex h-2.5 overflow-hidden rounded-full bg-panel">
        {present.map((s) => (
          <div
            key={s}
            className={cn("h-full transition-[width] duration-500", STATUS_BAR[s].bar)}
            style={{ width: `${(counts[s] / total) * 100}%` }}
            title={`${counts[s]} ${s}`}
          />
        ))}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
        {present.map((s) => (
          <span key={s} className="inline-flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", STATUS_BAR[s].bar)} />
            <span className={cn("font-semibold tabular-nums", STATUS_BAR[s].text)}>{counts[s]}</span>
            <span className="text-muted">{s}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

function IconBtn({
  label,
  tone,
  icon: Icon,
  busy,
  onClick,
}: {
  label: string;
  tone: "good" | "bad" | "muted";
  icon: typeof Check;
  busy?: boolean;
  onClick: () => void;
}) {
  const c = { good: "border-good text-good hover:bg-good", bad: "border-bad text-bad hover:bg-bad", muted: "border-border text-muted hover:border-accent" }[tone];
  return (
    <button
      onClick={onClick}
      disabled={busy}
      title={label}
      className={cn("inline-flex h-6 items-center gap-1 rounded border px-1.5 text-xs transition-colors hover:text-white disabled:opacity-50", c)}
    >
      <Icon className="size-3" /> {label}
    </button>
  );
}

function SkillFormBlock({
  icon: Icon,
  title,
  meta,
  children,
  defaultOpen = false,
}: {
  icon: LucideIcon;
  title: string;
  meta: string;
  children: ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <Disclosure
      defaultOpen={defaultOpen}
      className="rounded-lg border border-border bg-panel/45"
      summaryClassName="px-2.5 py-2"
      contentClassName="px-2.5 pb-2"
      summary={
        <span className="flex min-w-0 items-center gap-2">
          <span className="grid size-7 shrink-0 place-items-center rounded-md border border-border bg-background/70 text-accent">
            <Icon className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">{title}</span>
            <span className="block truncate text-[11px] font-normal text-muted">{meta}</span>
          </span>
        </span>
      }
    >
      {children}
    </Disclosure>
  );
}

// AuthorSkillForm lets the owner hand-author a skill from the UI (M736) — define a
// reusable procedure for the agent instead of waiting for it to distill one. Name +
// body are required; triggers (the phrases that surface it in recall) and tools are
// comma-separated. It posts to skill_import, which lands the skill as a DRAFT (auto-
// staged to shadow if well-formed) — never auto-active, so it goes through the normal
// promote lifecycle like any learned skill.
export function AuthorSkillForm({
  onCreated,
  onError,
  initial,
}: {
  onCreated: (name: string, status: string) => void;
  onError: (msg: string) => void;
  // When provided (revise/clone a card, M737), the form prefills from this skill.
  // Re-authoring the same name with a changed body creates a NEW version (lineage
  // tracked); an unchanged body dedupes (a no-op), so revising is always safe.
  initial?: { name?: string; description?: string; body?: string; triggers?: string[]; tools_required?: string[]; agent?: string };
}) {
  const editing = !!initial;
  const [name, setName] = useState(initial?.name ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [triggers, setTriggers] = useState((initial?.triggers ?? []).join(", "));
  const [tools, setTools] = useState((initial?.tools_required ?? []).join(", "));
  const [agent, setAgent] = useState(initial?.agent ?? "");
  const [body, setBody] = useState(initial?.body ?? "");
  const [submitting, setSubmitting] = useState(false);
  const matchingConfigured = Boolean(triggers.trim() || tools.trim() || agent.trim());

  const valid = name.trim() !== "" && body.trim() !== "";
  const splitList = (s: string) =>
    s
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);

  async function create() {
    if (!valid) return;
    const args: Record<string, unknown> = { name: name.trim(), body: body.trim() };
    if (description.trim()) args.description = description.trim();
    const trig = splitList(triggers);
    if (trig.length) args.triggers = trig;
    const tl = splitList(tools);
    if (tl.length) args.tools_required = tl;
    if (agent.trim()) args.agent = agent.trim();
    setSubmitting(true);
    try {
      const r = await postJSON<{ name?: string; status?: string }>("/api/skill/import", args);
      onCreated(r.name || name.trim(), r.status || "draft");
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-xl border border-border/70 bg-panel/70 p-3">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <div className="grid size-8 place-items-center rounded-lg border border-accent/25 bg-accent/10 text-accent">
            {editing ? <Pencil className="size-4" /> : <Sparkles className="size-4" />}
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-foreground">{name.trim() || (editing ? "Skill revision" : "New skill")}</div>
            <div className="text-[11px] text-muted">{editing ? "drafts a replacement version" : "lands as draft or shadow"}</div>
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Badge variant={valid ? "good" : "warn"}>{valid ? "ready" : "needs name/body"}</Badge>
          {matchingConfigured ? <Badge variant="accent">matched</Badge> : <Badge variant="default">manual</Badge>}
          {agent.trim() ? <Badge variant="accent">agent {agent.trim()}</Badge> : null}
        </div>
      </div>

      <div className="space-y-2">
        <SkillFormBlock icon={Bot} title="Identity" meta={description.trim() || name.trim() || "name and recall line"} defaultOpen>
          <div className="grid gap-2 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[11px] text-muted">
              Name
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. deploy-release"
                aria-label="Skill name"
                className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
            </label>
            <label className="flex flex-col gap-1 text-[11px] text-muted">
              Description
              <input
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="one line, used for recall"
                aria-label="Skill description"
                className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
            </label>
          </div>
        </SkillFormBlock>

        <SkillFormBlock icon={FileText} title="Procedure" meta={body.trim() ? `${body.trim().slice(0, 54)}${body.trim().length > 54 ? "..." : ""}` : "reusable steps required"} defaultOpen>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Body (the procedure)
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder="Concrete, reusable steps the agent should follow when this skill applies..."
              aria-label="Skill body"
              className="h-28 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
            />
          </label>
        </SkillFormBlock>

        <SkillFormBlock
          icon={Tags}
          title="Matching and ownership"
          meta={matchingConfigured ? "configured" : "shared manual skill"}
          defaultOpen={matchingConfigured}
        >
          <div className="grid gap-2 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Triggers
            <input
              value={triggers}
              onChange={(e) => setTriggers(e.target.value)}
              placeholder="deploy, ship, release"
              aria-label="Skill triggers"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Tools required
            <input
              value={tools}
              onChange={(e) => setTools(e.target.value)}
              placeholder="shell, file"
              aria-label="Skill tools required"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted sm:col-span-2">
            Private to agent
            <input
              value={agent}
              onChange={(e) => setAgent(e.target.value)}
              placeholder="roster slug — empty = shared with every agent"
              aria-label="Skill owning agent"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          </label>
          </div>
        </SkillFormBlock>

        <SkillFormBlock icon={History} title="Lifecycle" meta={editing ? "new version, old one untouched" : "draft first, promote later"}>
          <div className="text-xs text-muted">
            {editing
              ? "Changed procedures create a new version and leave the current active skill untouched until promotion."
              : "New skills enter the library as draft or shadow, then move to active only after promotion."}
          </div>
        </SkillFormBlock>
      </div>
      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />}{" "}
          {editing ? "Save as new version" : "Create skill"}
        </Button>
      </div>
    </div>
  );
}
