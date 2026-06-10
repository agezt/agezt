import { useEffect, useState } from "react";
import { Sparkles, RefreshCw, ChevronRight, ChevronDown, Check, ShieldX, Undo2, Plus, X, Pencil, Search } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";

interface Skill {
  id?: string;
  name?: string;
  description?: string;
  body?: string;
  status?: string;
  version?: number;
  triggers?: string[];
  tools_required?: string[];
  created_ms?: number;
  metrics?: { shadow_wins?: number; shadow_evals?: number; uses?: number; wins?: number };
}

const statusTone: Record<string, string> = {
  active: "bg-good/15 text-good",
  shadow: "bg-accent/15 text-accent",
  draft: "bg-panel text-muted",
  quarantined: "bg-bad/15 text-bad",
};

// skillMatches tests a skill against a lowercased query over its name, description,
// status, triggers, and required tools — so a growing library stays findable by what a
// skill does, when it fires, what it needs, or its lifecycle state (M778).
export function skillMatches(s: Skill, q: string): boolean {
  if (!q) return true;
  const hay = [s.name, s.description, s.status, ...(s.triggers || []), ...(s.tools_required || [])]
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
  const [active, setActive] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editSkill, setEditSkill] = useState<Skill | null>(null);
  const [q, setQ] = useState("");

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ skills?: Skill[]; active?: number }>("/api/skills");
      setSkills(d.skills || []);
      setActive(d.active ?? null);
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
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Sparkles className="size-4 text-accent" /> Skills
        </h2>
        {skills && (
          <span className="text-xs text-muted">
            {active != null ? `${active} active · ` : ""}
            {skills.length} total
          </span>
        )}
        <Button
          size="sm"
          className="ml-auto"
          onClick={() => {
            setEditSkill(null);
            setShowForm((v) => !(v && !editSkill));
          }}
          title="Author a skill"
        >
          {showForm && !editSkill ? <X className="size-3.5" /> : <Plus className="size-3.5" />} Author skill
        </Button>
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {showForm && (
        <AuthorSkillForm
          key={editSkill?.id || "new"}
          initial={editSkill ?? undefined}
          onCreated={(name, status) => {
            setShowForm(false);
            setEditSkill(null);
            ui.toast(editSkill ? `Saved new version of “${name}” (${status})` : `Skill “${name}” created (${status})`, "success");
            void reload();
          }}
          onError={(m) => ui.toast(m, "error")}
        />
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
                placeholder="filter skills…"
                aria-label="Filter skills"
                className="h-8 w-full rounded-md border border-border bg-panel pl-7 pr-12 text-xs text-foreground outline-none focus-visible:border-accent"
              />
              {q.trim() && (
                <span className="absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-muted">
                  {skills.filter((s) => skillMatches(s, q.trim().toLowerCase())).length}/{skills.length}
                </span>
              )}
            </div>
          )}
          {(() => {
            const query = q.trim().toLowerCase();
            const shown = query ? skills.filter((s) => skillMatches(s, query)) : skills;
            if (shown.length === 0) return <p className="px-1 py-2 text-xs text-muted">no skills match “{q.trim()}”</p>;
            return shown.map((s, i) => {
            const m = s.metrics || {};
            const isOpen = open === (s.id || String(i));
            return (
              <div key={s.id || i} className="rounded-lg border border-border bg-card p-3">
                <div className="flex items-center gap-2">
                  <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider", statusTone[s.status || ""] || "bg-panel text-muted")}>
                    {s.status || "?"}
                  </span>
                  <span className="truncate text-sm font-semibold">{s.name || "—"}</span>
                  {s.version != null && <span className="text-[10px] text-muted">v{s.version}</span>}
                  <div className="ml-auto flex shrink-0 gap-1">
                    {s.id && s.body && (
                      <IconBtn
                        label="revise"
                        tone="muted"
                        icon={Pencil}
                        onClick={() => {
                          setEditSkill(s);
                          setShowForm(true);
                        }}
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
                                ? `“${s.name}” will stop being used until you reinstate it.`
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

                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted">
                  {s.created_ms ? <span>{fmtTime(s.created_ms)}</span> : null}
                  {(m.shadow_evals || 0) > 0 && (
                    <span className="text-accent">
                      shadow {m.shadow_wins || 0}/{m.shadow_evals}
                    </span>
                  )}
                  {(m.uses || 0) > 0 && (
                    <span>
                      used {m.uses}× {m.wins != null ? `· ${m.wins} ok` : ""}
                    </span>
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
    <div className="rounded-lg border border-border bg-card p-3">
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
      className={cn("inline-flex h-6 items-center gap-1 rounded border px-1.5 text-[10px] transition-colors hover:text-white disabled:opacity-50", c)}
    >
      <Icon className="size-3" /> {label}
    </button>
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
  initial?: { name?: string; description?: string; body?: string; triggers?: string[]; tools_required?: string[] };
}) {
  const editing = !!initial;
  const [name, setName] = useState(initial?.name ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [triggers, setTriggers] = useState((initial?.triggers ?? []).join(", "));
  const [tools, setTools] = useState((initial?.tools_required ?? []).join(", "));
  const [body, setBody] = useState(initial?.body ?? "");
  const [submitting, setSubmitting] = useState(false);

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
    <div className="rounded-lg border border-accent/30 bg-card p-3">
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

      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Body (the procedure)
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Concrete, reusable steps the agent should follow when this skill applies…"
          aria-label="Skill body"
          className="h-28 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
        />
      </label>

      <div className="mt-2 grid gap-2 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Triggers (comma-separated)
          <input
            value={triggers}
            onChange={(e) => setTriggers(e.target.value)}
            placeholder="deploy, ship, release"
            aria-label="Skill triggers"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Tools required (comma-separated)
          <input
            value={tools}
            onChange={(e) => setTools(e.target.value)}
            placeholder="shell, file"
            aria-label="Skill tools required"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
      </div>

      <p className="mt-2 text-[10px] text-muted">
        {editing ? (
          <>
            Saving with a changed body creates a new <span className="text-foreground/80">version</span> of this skill — it
            lands as a draft and goes through promotion again; the current version is untouched until you promote.
          </>
        ) : (
          <>
            New skills land as a <span className="text-foreground/80">draft</span> (auto-staged to shadow if well-formed) —
            promote it to active from its card once you trust it.
          </>
        )}
      </p>
      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />}{" "}
          {editing ? "Save as new version" : "Create skill"}
        </Button>
      </div>
    </div>
  );
}
