import { useCallback, useEffect, useState, type ReactNode } from "react";
import { RefreshCw, ShieldCheck, Trash2, Plus, FlaskConical, EyeOff, ShieldAlert, X, SlidersHorizontal, CheckCircle2, XCircle, type LucideIcon } from "lucide-react";
import { Panel, Row, Count } from "@/components/Panel";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import { LogDetail } from "@/components/LogDetail";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { byDescValue, pct } from "@/lib/format";
import { cn, fmtTime } from "@/lib/utils";

interface EdictShow {
  ask_policy?: string;
  levels?: Record<string, string>;
  hard_deny?: { name: string; substring?: string; applies_to?: string[] | null }[];
}

// The trust ladder (edict L0..L4). Lower = more restrictive.
const LEVELS: { value: string; label: string }[] = [
  { value: "L0", label: "L0 · deny" },
  { value: "L1", label: "L1 · ask" },
  { value: "L2", label: "L2 · ask-first" },
  { value: "L3", label: "L3 · ask-scoped" },
  { value: "L4", label: "L4 · allow" },
];
const MODES = ["allow", "prompt", "deny"];

function levelTone(level: string): string {
  if (level === "L4") return "text-good border-good/40";
  if (level === "L0") return "text-bad border-bad/40";
  return "text-accent border-accent/40";
}

// LEVEL_BAR colours the trust-level distribution from locked-down (L0, red) to
// fully autonomous (L4, green), so the security posture reads at a glance.
const LEVEL_BAR: Record<string, { bar: string; text: string }> = {
  L0: { bar: "bg-bad", text: "text-bad" },
  L1: { bar: "bg-amber-500", text: "text-amber-500" },
  L2: { bar: "bg-amber-500/70", text: "text-amber-500" },
  L3: { bar: "bg-accent", text: "text-accent" },
  L4: { bar: "bg-good", text: "text-good" },
};
const LEVEL_FLOW = ["L0", "L1", "L2", "L3", "L4"] as const;

// LevelSummary shows how the governed capabilities spread across the trust ladder
// — a stacked bar from deny (L0) to allow (L4) plus a count chip per level — so the
// operator can see the overall security posture without reading every row.
function LevelSummary({ levels }: { levels: [string, string][] }) {
  const counts: Record<string, number> = {};
  for (const [, lvl] of levels) counts[lvl] = (counts[lvl] || 0) + 1;
  const present = LEVEL_FLOW.filter((l) => counts[l] > 0);
  const total = levels.length || 1;
  if (present.length === 0) return null;
  return (
    <div className="mb-3 rounded-md border border-border/70 bg-panel/40 p-2.5">
      <div className="flex h-2 overflow-hidden rounded-full bg-panel">
        {present.map((l) => (
          <div
            key={l}
            className={cn("h-full transition-[width] duration-500", LEVEL_BAR[l].bar)}
            style={{ width: `${(counts[l] / total) * 100}%` }}
            title={`${counts[l]} at ${l}`}
          />
        ))}
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]">
        {present.map((l) => (
          <span key={l} className="inline-flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", LEVEL_BAR[l].bar)} />
            <span className={cn("font-semibold tabular-nums", LEVEL_BAR[l].text)}>{counts[l]}</span>
            <span className="text-muted">{l}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

// Policy is the capability control center: it SHOWS each governed capability's
// current trust level and lets the operator GRANT or restrict it at runtime
// (M610) — the "approve" knob. Changing a level posts /api/edict/set_level; the
// engine-wide ask mode and the hard-deny list are editable too. The decision
// stats + log stay below for context.
export function Policy() {
  const ui = useUI();
  const [show, setShow] = useState<EdictShow | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [modeOpen, setModeOpen] = useState(false);
  const [editingCap, setEditingCap] = useState<{ capability: string; level: string } | null>(null);
  const [denyOpen, setDenyOpen] = useState(false);
  const [testOpen, setTestOpen] = useState(false);
  const [redactOpen, setRedactOpen] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      setShow(await getJSON<EdictShow>("/api/edict_show"));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  async function act(
    key: string,
    path: string,
    params: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(key);
    try {
      await postAction(path, params);
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const levels = show?.levels ? Object.entries(show.levels).sort((a, b) => a[0].localeCompare(b[0])) : [];
  const denies = show?.hard_deny ?? [];

  return (
    <div className="space-y-4">
      {/* Control center */}
      <div className="glass rounded-xl p-3">
        <PageHeader
          className="mb-3"
          icon={ShieldCheck}
          title="Capability policy"
          actions={
            <>
              <label className="flex items-center gap-1.5 text-xs text-muted">
                ask mode <Badge variant={show?.ask_policy === "deny" ? "bad" : show?.ask_policy === "prompt" ? "warn" : "good"}>{show?.ask_policy || "allow"}</Badge>
              </label>
              <Button variant="ghost" size="sm" onClick={() => setModeOpen(true)} disabled={!show || busy === "mode"}>
                <SlidersHorizontal className="size-3.5" /> Policy mode
              </Button>
              <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
                <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
              </Button>
            </>
          }
        />

        {err && <div className="mb-2 text-xs text-bad">{err}</div>}

        {levels.length === 0 ? (
          <div className="text-xs text-muted">{loading ? "loading…" : "no governed capabilities"}</div>
        ) : (
          <PolicyPanel icon={ShieldCheck} title="Capabilities" status={`${levels.length} governed`} tone="accent">
            <LevelSummary levels={levels} />
            <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
              {levels.map(([cap, lvl]) => (
                <div
                  key={cap}
                  className="flex items-center gap-2 rounded-md border border-border/70 bg-panel/40 px-2.5 py-1.5"
                >
                  <span className="truncate font-mono text-xs">{cap}</span>
                  <button
                    className={cn("ml-auto rounded-md border bg-card px-2 py-1 text-xs tabular-nums transition-colors hover:border-accent disabled:opacity-50", levelTone(lvl))}
                    disabled={busy === cap}
                    onClick={() => setEditingCap({ capability: cap, level: lvl })}
                    title={`Edit ${cap} trust level`}
                  >
                    {lvl}
                  </button>
                </div>
              ))}
            </div>
          </PolicyPanel>
        )}

        {/* Hard-deny rules */}
        <PolicyPanel icon={ShieldAlert} title="Hard-deny rules" status={`${denies.length} active`} tone="bad">
          {denies.length === 0 ? (
            <div className="text-xs text-muted">none</div>
          ) : (
            <ul className="space-y-1">
              {denies.map((r) => (
                <li key={r.name} className="flex items-center gap-2 text-xs">
                  <Badge variant="bad">deny</Badge>
                  <span className="font-mono">{r.name}</span>
                  {r.substring && <span className="text-muted">“{r.substring}”</span>}
                  {r.applies_to && r.applies_to.length > 0 && (
                    <span className="text-muted">→ {r.applies_to.join(", ")}</span>
                  )}
                  {/* Only runtime-added rules (named runtime[N]) are removable. */}
                  {r.name.startsWith("runtime") && (
                    <button
                      onClick={() =>
                        act(r.name, "/api/edict/deny_rm", { name: r.name }, {
                          confirm: {
                            title: "Remove this deny rule?",
                            message: `“${r.name}” will be deleted — this loosens the security policy.`,
                            confirmLabel: "Remove",
                            danger: true,
                          },
                          success: "Deny rule removed",
                        })
                      }
                      disabled={busy === r.name}
                      title="Remove this runtime deny rule"
                      className="ml-auto text-muted transition-colors hover:text-bad disabled:opacity-50"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )}
          <div className="mt-2 border-t border-border pt-2">
            <Button size="sm" variant="ghost" onClick={() => setDenyOpen(true)}>
              <Plus className="size-3.5" /> Add deny rule
            </Button>
          </div>
        </PolicyPanel>

        {/* Dry-run a decision (M753) */}
        {levels.length > 0 && (
          <div className="mt-3 border-t border-border pt-2">
            <Button size="sm" variant="ghost" onClick={() => setTestOpen(true)}>
              <FlaskConical className="size-3.5" /> Test decision
            </Button>
          </div>
        )}
      </div>

      {/* Secret redaction check (M754) */}
      <div className="glass rounded-xl p-3">
        <PageHeader
          className="mb-2"
          icon={EyeOff}
          title="Secret redaction"
          description="check what the scrubber catches before it leaves the daemon"
          actions={
            <Button size="sm" variant="ghost" onClick={() => setRedactOpen(true)}>
              <EyeOff className="size-3.5" /> Probe
            </Button>
          }
        />
        <div className="text-xs text-muted">
          Paste-and-probe runs in a focused modal; this surface only shows the redaction control.
        </div>
      </div>

      {/* Decision stats + log (existing read-only view) */}
      <Panel<Record<string, any>> title="Decisions" path="/api/policy">
        {(d) => {
          const byCap: Record<string, number> = d.denied_by_capability || {};
          const names = byDescValue(byCap);
          return (
            <>
              <MetricGrid cols="repeat(3, minmax(0, 1fr))">
                <MetricWidget icon={CheckCircle2} label="allowed" tone="good" value={d.allowed || 0} />
                <MetricWidget
                  icon={XCircle}
                  label="denied"
                  tone="bad"
                  value={d.denied || 0}
                  subvalue={d.hard_denied ? `${d.hard_denied} hard` : undefined}
                />
                <MetricWidget icon={ShieldAlert} label="denial rate" tone="warn" value={pct(d.denial_rate, d.total)} />
              </MetricGrid>
              {names.length > 0 && (
                <div>
                  <Count>denied by capability</Count>
                  {names.map((n) => (
                    <Row key={n}>
                      <Badge variant="bad">{byCap[n]}</Badge>
                      <span>{n}</span>
                    </Row>
                  ))}
                </div>
              )}
              <LogDetail
                label="decision log"
                path="/api/policy_log"
                params={{ limit: "40" }}
                extract={(x) => x.decisions || []}
                render={(ev: PolicyLogEntry, i: number) => (
                  <div key={i} className="flex items-center gap-2 border-b border-border/40 py-0.5">
                    {ev.allow ? (
                      <CheckCircle2 className="size-3.5 shrink-0 text-good" aria-hidden />
                    ) : (
                      <XCircle className="size-3.5 shrink-0 text-bad" aria-hidden />
                    )}
                    <span className="text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                    <span className={ev.allow ? "text-good" : "text-bad"}>
                      {ev.allow ? "allow" : ev.hard_denied ? "DENY(hard)" : "DENY"} {ev.capability || ""}{" "}
                      {ev.tool || ""}
                    </span>
                    {ev.reason ? <span className="truncate text-muted">{ev.reason}</span> : null}
                  </div>
                )}
              />
            </>
          );
        }}
      </Panel>

      {modeOpen && (
        <PolicyModal title="Ask mode" icon={ShieldCheck} onClose={() => setModeOpen(false)}>
          <div className="grid gap-2">
            {MODES.map((mode) => (
              <button
                key={mode}
                className={cn(
                  "flex items-center justify-between rounded-lg border border-border bg-panel/45 px-3 py-2 text-left transition-colors hover:border-accent",
                  show?.ask_policy === mode && "border-accent bg-accent/10",
                )}
                onClick={async () => {
                  await act("mode", "/api/edict/set_mode", { mode }, { success: `Ask mode → ${mode}` });
                  setModeOpen(false);
                }}
                disabled={busy === "mode"}
              >
                <span className="text-sm font-medium text-foreground">{mode}</span>
                <Badge variant={mode === "deny" ? "bad" : mode === "prompt" ? "warn" : "good"}>{mode === "allow" ? "autonomous" : mode}</Badge>
              </button>
            ))}
          </div>
        </PolicyModal>
      )}

      {editingCap && (
        <PolicyModal title={editingCap.capability} icon={ShieldCheck} onClose={() => setEditingCap(null)}>
          <div className="grid gap-2">
            {LEVELS.map((level) => (
              <button
                key={level.value}
                className={cn(
                  "flex items-center justify-between rounded-lg border border-border bg-panel/45 px-3 py-2 text-left transition-colors hover:border-accent",
                  editingCap.level === level.value && "border-accent bg-accent/10",
                )}
                onClick={async () => {
                  await act(editingCap.capability, "/api/edict/set_level", { capability: editingCap.capability, level: level.value }, { success: `${editingCap.capability} → ${level.value}` });
                  setEditingCap(null);
                }}
                disabled={busy === editingCap.capability}
              >
                <span className={cn("rounded border px-2 py-1 text-xs font-semibold tabular-nums", levelTone(level.value))}>{level.value}</span>
                <span className="text-xs text-muted">{level.label.replace(/^L\d · /, "")}</span>
              </button>
            ))}
          </div>
        </PolicyModal>
      )}

      {denyOpen && (
        <PolicyModal title="Add hard-deny rule" icon={ShieldAlert} onClose={() => setDenyOpen(false)}>
          <DenyAddForm
            capabilities={levels.map(([cap]) => cap)}
            onAdded={(rule) => {
              setDenyOpen(false);
              ui.toast(`Deny rule added — “${rule}” is now blocked`, "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
          />
        </PolicyModal>
      )}

      {testOpen && (
        <PolicyModal title="Test policy decision" icon={FlaskConical} onClose={() => setTestOpen(false)}>
          <PolicyTestForm capabilities={levels.map(([cap]) => cap)} />
        </PolicyModal>
      )}

      {redactOpen && (
        <PolicyModal title="Secret redaction probe" icon={EyeOff} onClose={() => setRedactOpen(false)}>
          <RedactionCheckForm compact />
        </PolicyModal>
      )}
    </div>
  );
}

function PolicyPanel({
  icon: Icon,
  title,
  status,
  tone,
  children,
}: {
  icon: LucideIcon;
  title: string;
  status: string;
  tone: "accent" | "bad" | "muted";
  children: ReactNode;
}) {
  const toneCls: Record<typeof tone, string> = {
    accent: "border-accent/35 bg-accent/5 text-accent",
    bad: "border-bad/35 bg-bad/5 text-bad",
    muted: "border-border bg-panel text-muted",
  };
  return (
    <section className="mb-3 rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg border", toneCls[tone])}>
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

function PolicyModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: LucideIcon;
  onClose: () => void;
  children: React.ReactNode;
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="modal-overlay fixed inset-0 z-[160] flex items-start justify-center overflow-y-auto bg-black/55 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="modal-in mt-10 w-full max-w-xl rounded-lg border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="mb-3 flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
            <Icon className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close policy modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// DenyAddForm adds a runtime hard-deny rule from the UI (M717) — the safety floor
// that blocks any tool call whose input contains a substring, optionally scoped to
// one capability. Adding a rule only tightens policy. It builds the deny-rule spec
// ("substring" or "<capability>:substring") and posts it to edict_deny_add.
export function DenyAddForm({
  capabilities,
  onAdded,
  onError,
}: {
  capabilities: string[];
  onAdded: (rule: string) => void;
  onError: (msg: string) => void;
}) {
  const [substring, setSubstring] = useState("");
  const [scope, setScope] = useState(""); // "" = all capabilities
  const [submitting, setSubmitting] = useState(false);

  const valid = substring.trim() !== "";

  async function add() {
    if (!valid) return;
    const s = substring.trim();
    const rule = scope ? `${scope}:${s}` : s;
    setSubmitting(true);
    try {
      await postAction("/api/edict/deny_add", { rule });
      setSubstring("");
      setScope("");
      onAdded(rule);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-3">
      <div>
        <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">Scope</div>
        <div className="flex max-h-28 flex-wrap gap-1.5 overflow-auto rounded-lg border border-border/70 bg-panel/50 p-2" aria-label="Deny rule capability scope">
          <button
            type="button"
            onClick={() => setScope("")}
            className={cn(
              "rounded-md border px-2 py-1 text-xs transition-colors",
              scope === "" ? "border-accent bg-accent/15 text-accent" : "border-border bg-card text-muted hover:text-foreground",
            )}
            aria-pressed={scope === ""}
          >
            all capabilities
          </button>
          {capabilities.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => setScope(c)}
              className={cn(
                "rounded-md border px-2 py-1 font-mono text-xs transition-colors",
                scope === c ? "border-accent bg-accent/15 text-accent" : "border-border bg-card text-muted hover:text-foreground",
              )}
              aria-pressed={scope === c}
            >
              {c}
            </button>
          ))}
        </div>
      </div>
      <label className="grid gap-1 text-[11px] text-muted">
        Blocked substring
      <input
        value={substring}
        onChange={(e) => setSubstring(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") add();
        }}
        placeholder="substring to block (e.g. rm -rf)"
        aria-label="Deny rule substring"
        className="h-9 min-w-0 rounded-md border border-border bg-panel px-2 font-mono text-xs outline-none focus:border-accent"
      />
      </label>
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-xs text-muted">{scope || "all"} blocks “{substring.trim() || "substring"}”</span>
      <Button size="sm" onClick={add} disabled={!valid || submitting} title="Add deny rule">
        {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add
      </Button>
      </div>
    </div>
  );
}

interface EdictDecision {
  decision?: string; // "allow" | "deny"
  capability?: string;
  level?: string; // "L0".."L4"
  reason?: string;
  hard_denied?: boolean;
  hard_deny_rule?: string;
  would_ask?: boolean;
  requires_approval?: boolean;
}

interface PolicyLogEntry {
  ts_unix_ms?: number;
  allow?: boolean;
  hard_denied?: boolean;
  capability?: string;
  tool?: string;
  reason?: string;
}

// effectiveOutcome folds the raw decision into the operator-facing verdict: a hard
// deny, a plain deny, an ask (the call is allowed but would pause for approval), or a
// clean allow — each with a tone for the badge.
function effectiveOutcome(d: EdictDecision): { label: string; tone: "bad" | "accent" | "good" } {
  if (d.hard_denied) return { label: "DENY · hard", tone: "bad" };
  if (d.decision === "deny" && !d.requires_approval) return { label: "DENY", tone: "bad" };
  if (d.requires_approval || d.would_ask) return { label: "ASK", tone: "accent" };
  return { label: "ALLOW", tone: "good" };
}

// PolicyTestForm dry-runs a policy decision (M753): pick a capability and an optional
// input, and the edict engine reports whether the agent would be allowed, asked, or
// denied — and via which hard-deny rule. Read-only (eng.Decide mutates nothing), so
// it's the safe way to understand "why is this blocked?" or to check a deny rule's
// effect before/after adding it. Pairs with DenyAddForm above.
export function PolicyTestForm({ capabilities }: { capabilities: string[] }) {
  const [capability, setCapability] = useState(capabilities[0] ?? "");
  const [input, setInput] = useState("");
  const [result, setResult] = useState<EdictDecision | null>(null);
  const [running, setRunning] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Keep the picker valid if the capability list arrives/changes after mount.
  useEffect(() => {
    if (!capability && capabilities[0]) setCapability(capabilities[0]);
  }, [capabilities, capability]);

  async function run() {
    if (!capability) return;
    setRunning(true);
    try {
      const d = await getJSON<EdictDecision>("/api/edict/test", { capability, input });
      setResult(d);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
      setResult(null);
    } finally {
      setRunning(false);
    }
  }

  const outcome = result ? effectiveOutcome(result) : null;

  return (
    <div className="space-y-3">
      <div>
        <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
          <FlaskConical className="size-3 text-accent" /> Capability
        </div>
        <div className="flex max-h-32 flex-wrap gap-1.5 overflow-auto rounded-lg border border-border/70 bg-panel/50 p-2" aria-label="Test capability">
          {capabilities.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => setCapability(c)}
              className={cn(
                "rounded-md border px-2 py-1 font-mono text-xs transition-colors",
                capability === c ? "border-accent bg-accent/15 text-accent" : "border-border bg-card text-muted hover:text-foreground",
              )}
              aria-pressed={capability === c}
            >
              {c}
            </button>
          ))}
        </div>
      </div>
      <div className="grid gap-1 text-[11px] text-muted">
        Probe input
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") run();
          }}
          placeholder="input to probe (e.g. rm -rf /)"
          aria-label="Test input"
          className="h-9 min-w-0 rounded-md border border-border bg-panel px-2 font-mono text-xs outline-none focus:border-accent"
        />
      </div>
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-xs text-muted">{capability || "capability"} · dry-run only</span>
        <Button size="sm" variant="ghost" onClick={run} disabled={!capability || running} title="Dry-run this decision">
          {running ? <RefreshCw className="size-3.5 animate-spin" /> : <FlaskConical className="size-3.5" />} Test
        </Button>
      </div>

      {err && <div className="text-xs text-bad">{err}</div>}
      {outcome && result && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-border/70 bg-panel/40 px-2.5 py-1.5 text-xs">
          <Badge variant={outcome.tone === "bad" ? "bad" : outcome.tone === "good" ? "good" : "default"}>
            {outcome.label}
          </Badge>
          {result.level && <span className={cn("rounded border px-1.5 py-0.5 text-xs font-semibold tabular-nums", levelTone(result.level))}>{result.level}</span>}
          {result.hard_denied && result.hard_deny_rule && (
            <span className="text-muted">
              rule <span className="font-mono text-foreground/80">{result.hard_deny_rule}</span>
            </span>
          )}
          {result.reason && <span className="truncate text-muted">{result.reason}</span>}
        </div>
      )}
    </div>
  );
}

interface RedactResult {
  enabled?: boolean;
  would_redact?: boolean;
  redacted?: string;
  categories?: string[];
  literal_hit?: boolean;
}

// RedactionCheckForm dry-runs the LIVE secret redactor (M754): paste text and see
// whether the scrubber that guards outbound content (logs, channel messages, prompts)
// would redact it — and into which categories (api_key, jwt, …) or as a configured
// secret literal. Read-only, and the probe text rides the POST body (not a URL) so the
// secret never lands in an access log; the response returns only the REDACTED form and
// category names, never the matched secret. Lets an operator confirm "my key won't leak."
export function RedactionCheckForm({ compact = false }: { compact?: boolean } = {}) {
  const [text, setText] = useState("");
  const [result, setResult] = useState<RedactResult | null>(null);
  const [running, setRunning] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function run() {
    if (!text.trim()) return;
    setRunning(true);
    try {
      setResult(await postJSON<RedactResult>("/api/redact/test", { text }));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
      setResult(null);
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className={compact ? "space-y-2" : "glass rounded-xl p-3"}>
      {!compact && (
        <PageHeader
          className="mb-2"
          icon={EyeOff}
          title="Secret redaction"
          description="check what the scrubber catches before it leaves the daemon"
        />
      )}
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Paste text to test — e.g. a log line or message that might carry a key…"
        aria-label="Redaction test text"
        className="h-20 w-full resize-y rounded-md border border-border bg-panel p-2 font-mono text-xs outline-none placeholder:text-muted/60 focus-visible:border-accent"
      />
      <div className="mt-2 flex items-center justify-between gap-2">
        <span className="text-xs text-muted">The probe text is sent in the request body (never a URL) and the response shows only the redacted form.</span>
        <Button size="sm" variant="ghost" onClick={run} disabled={!text.trim() || running} title="Test redaction">
          {running ? <RefreshCw className="size-3.5 animate-spin" /> : <EyeOff className="size-3.5" />} Check
        </Button>
      </div>

      {err && <div className="mt-1.5 text-xs text-bad">{err}</div>}
      {result && (
        <div className="mt-2 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2 text-xs">
            {!result.enabled ? (
              <Badge variant="bad">redactor OFF</Badge>
            ) : result.would_redact ? (
              <Badge variant="good">would redact</Badge>
            ) : (
              <Badge>no match</Badge>
            )}
            {result.categories?.map((c) => (
              <span key={c} className="rounded bg-panel px-1.5 py-0.5 font-mono text-xs text-accent">
                {c}
              </span>
            ))}
            {result.literal_hit && <span className="text-xs text-muted">matched a configured secret literal</span>}
          </div>
          {result.would_redact && (
            <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded-md border border-border/70 bg-panel/40 p-2 font-mono text-[11px] text-foreground/85">
              {result.redacted}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}
