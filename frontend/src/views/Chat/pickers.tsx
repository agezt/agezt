import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, Bot, Check, ChevronDown, ChevronRight, CornerDownRight, Forward, Scissors, ShieldCheck, ShieldX, Sparkles, StickyNote, Terminal } from "lucide-react";
import { cn, fmtTime } from "@/lib/utils";
import { money } from "@/lib/format";
import { getJSON } from "@/lib/api";
import { type ChatTurn } from "@/lib/chat";
import { type HistorySummary } from "@/lib/conversations";

interface ExecutionProfileCheck {
  profile_id?: string;
  status?: string;
  title?: string;
  detail?: string;
  next?: string;
}

const EXEC_PROFILE_OPTIONS: { id: string; label: string; hint: string }[] = [
  { id: "", label: "tool defaults", hint: "use each tool's configured execution profile" },
  { id: "local", label: "local", hint: "request direct host execution for shell/code" },
  { id: "warden", label: "warden", hint: "request warden-backed shell/code execution" },
];

const EXEC_PROFILE_HINTS: Record<string, string> = {
  docker: "request container-backed shell/code execution",
  ssh: "request shell-only remote execution over SSH",
  "remote-agezt": "delegate the whole run to a configured AGEZT peer",
};

export function ExecutionProfilePicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (profile: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [checks, setChecks] = useState<Record<string, ExecutionProfileCheck>>({});
  const [routable, setRoutable] = useState<string[]>([]);
  const ref = useRef<HTMLDivElement>(null);
  const options = useMemo(() => {
    const seen = new Set(EXEC_PROFILE_OPTIONS.map((o) => o.id));
    const out = [...EXEC_PROFILE_OPTIONS];
    for (const id of routable) {
      if (!id || seen.has(id)) continue;
      seen.add(id);
      out.push({ id, label: id, hint: EXEC_PROFILE_HINTS[id] || `request ${id} execution profile` });
    }
    if (value && !seen.has(value)) {
      out.push({ id: value, label: value, hint: EXEC_PROFILE_HINTS[value] || `request ${value} execution profile` });
    }
    return out;
  }, [routable, value]);
  const selected = options.find((o) => o.id === value) || options[0];
  const active = selected.id !== "";

  useEffect(() => {
    if (!open) return;
    getJSON<{ checks?: ExecutionProfileCheck[]; routable_run_profiles?: string[] }>("/api/execution_profile_check")
      .then((r) => {
        const next: Record<string, ExecutionProfileCheck> = {};
        for (const c of r.checks || []) {
          if (c.profile_id) next[c.profile_id] = c;
        }
        setChecks(next);
        setRoutable(r.routable_run_profiles || []);
      })
      .catch(() => {
        setChecks({});
        setRoutable([]);
      });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label="Choose execution profile"
        title="Choose execution profile for shell/code runs"
        className={cn(
          "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-xs transition-colors",
          active ? "border-accent/40 text-accent" : "border-border text-muted hover:text-foreground",
        )}
      >
        <Terminal className="size-3.5" />
        <span>{active ? selected.label : "exec"}</span>
        {active && <span className="size-1.5 rounded-full bg-accent" />}
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-30 mb-1.5 w-72 rounded-lg border border-border bg-card p-1 shadow-xl shadow-black/30">
          {options.map((opt) => {
            const c = opt.id ? checks[opt.id] : undefined;
            const status = (c?.status || "").toLowerCase();
            return (
              <button
                key={opt.id || "default"}
                onClick={() => {
                  onChange(opt.id);
                  setOpen(false);
                }}
                className={cn(
                  "flex w-full items-start gap-1.5 rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-panel",
                  selected.id === opt.id ? "text-accent" : "text-foreground",
                )}
              >
                <Check className={cn("mt-0.5 size-3 shrink-0", selected.id === opt.id ? "opacity-100" : "opacity-0")} />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1 font-medium">
                    {opt.label}
                    {status === "ok" && <ShieldCheck className="size-3 text-good" />}
                    {status === "warning" && <AlertTriangle className="size-3 text-warn" />}
                    {status === "fail" && <ShieldX className="size-3 text-bad" />}
                  </span>
                  <span className="block text-muted">{c?.detail || opt.hint}</span>
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ConversationPersona is a legacy-named per-thread identity override (M711): a
// small composer control that, when set, makes runs in THIS conversation use the
// supplied identity text instead of the daemon default identity. It is not a
// durable roster agent.
export function ConversationPersona({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const active = value.trim().length > 0;

  function openEditor() {
    setDraft(value);
    setOpen(true);
  }
  function save() {
    onChange(draft.trim());
    setOpen(false);
  }
  function clear() {
    onChange("");
    setDraft("");
    setOpen(false);
  }

  return (
    <div className="relative">
      <button
        onClick={open ? () => setOpen(false) : openEditor}
        title={active ? "This conversation has a custom identity override" : "Set an identity override for this conversation"}
        className={cn(
          "inline-flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors hover:text-foreground",
          active ? "text-accent" : "text-muted",
        )}
      >
        <Bot className="size-3.5" />
        <span>identity{active ? "" : ": default"}</span>
        {active && <span className="size-1.5 rounded-full bg-accent" />}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-20" onClick={() => setOpen(false)} />
          <div className="absolute bottom-full left-0 z-30 mb-1.5 w-80 rounded-lg border border-border bg-card p-2 shadow-xl shadow-black/30">
            <div className="mb-1.5 text-[11px] text-muted">
              Identity for <span className="text-foreground/80">this conversation</span> — overrides the daemon
              default identity for every run here. Leave empty to use the default identity.
            </div>
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              autoFocus
              spellCheck={false}
              aria-label="Conversation identity"
              placeholder="e.g. You are a senior Go reviewer. Be blunt and specific…"
              className="h-28 w-full resize-none rounded-md border border-border bg-panel p-2 font-mono text-xs text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
            />
            <div className="mt-1.5 flex items-center justify-end gap-1.5">
              <button
                onClick={clear}
                disabled={!active && !draft.trim()}
                className="rounded-md px-2 py-1 text-xs text-muted transition-colors hover:text-foreground disabled:opacity-40"
              >
                Clear
              </button>
              <button
                onClick={save}
                className="rounded-md bg-accent px-2 py-1 text-xs font-medium text-white transition-opacity hover:opacity-90"
              >
                Save
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

// PromptLauncher makes the saved prompt library (M713) reachable anywhere in a
// chat, not just the empty state (M725): a button that opens a menu of your saved
// prompts; picking one drops its text into the composer. Hidden when you have none.
export function PromptLauncher({ onPick }: { onPick: (text: string) => void }) {
  const [prompts, setPrompts] = useState<{ title: string; text: string }[]>([]);
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    getJSON<{ prompts?: { title: string; text: string }[] }>("/api/prompts")
      .then((r) => setPrompts(r.prompts || []))
      .catch(() => {
        /* prompts are optional */
      });
  }, []);

  useEffect(() => {
    if (!open) return;
    function onDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    window.addEventListener("mousedown", onDown);
    return () => window.removeEventListener("mousedown", onDown);
  }, [open]);

  if (prompts.length === 0) return null;

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        title="Insert a saved prompt"
        aria-label="Insert a saved prompt"
        className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-muted transition-colors hover:text-foreground"
      >
        <Sparkles className="size-3.5" />
        <span>prompts</span>
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-30 mb-1.5 max-h-64 w-64 overflow-auto rounded-lg border border-border bg-card p-1 shadow-xl shadow-black/30">
          {prompts.map((p, i) => (
            <button
              key={`${p.title}-${i}`}
              onClick={() => {
                onPick(p.text);
                setOpen(false);
              }}
              title={p.text}
              className="block w-full truncate rounded px-2 py-1.5 text-left text-xs text-foreground/90 transition-colors hover:bg-panel"
            >
              {p.title}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// FallbackNote shows when the per-task model chain (M703) had to fall back: the
// primary model failed and a later model in the chain answered. It makes the
// routing you configured observable right where it matters — so when a different
// model answers, you know why, not just that the model name changed.
export function FallbackNote({ hops }: { hops: { from: string; to: string }[] }) {
  // Collapse consecutive hops into one path: a→b, b→c ⇒ a → b → c.
  const path: string[] = hops.length ? [hops[0].from, ...hops.map((h) => h.to)] : [];
  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-md bg-warn/5 px-2 py-1 text-xs text-warn">
      <CornerDownRight className="size-3.5 shrink-0" />
      <span className="font-medium">{hops.length === 1 ? "fell back" : `fell back ${hops.length}×`}</span>
      <span className="font-mono text-foreground/70">{path.join(" → ")}</span>
    </div>
  );
}

// SteerNote (M962) shows the operator injections this turn received mid-run — a
// forceful steer (re-prioritise) or a soft BTW note — so the human sees their
// guidance landed and how the agent was nudged.
export function SteerNote({ steers }: { steers: { text: string; note: boolean }[] }) {
  return (
    <div className="space-y-1">
      {steers.map((s, i) => (
        <div
          key={i}
          className={cn(
            "flex items-start gap-1.5 rounded-md border px-2 py-1 text-xs",
            s.note ? "border-border bg-panel/50 text-muted" : "border-accent/30 bg-accent/5 text-accent",
          )}
        >
          {s.note ? <StickyNote className="mt-0.5 size-3.5 shrink-0" /> : <Forward className="mt-0.5 size-3.5 shrink-0" />}
          <span className="font-medium">{s.note ? "BTW" : "steered"}</span>
          <span className="min-w-0 flex-1 break-words text-foreground/80">{s.text}</span>
        </div>
      ))}
    </div>
  );
}

export function TurnMeta({ turn }: { turn: ChatTurn }) {
  const parts: string[] = [];
  if (turn.agent) parts.push("as " + turn.agent); // who answered (M789)
  if (turn.model) parts.push(turn.model);
  if (turn.iters) parts.push(`${turn.iters} iter${turn.iters === 1 ? "" : "s"}`);
  if (turn.costMicrocents) parts.push(money(turn.costMicrocents));
  // When this exchange happened (M877) — shown for turns the store stamped; older
  // persisted turns lack `ts` and just omit it.
  if (turn.ts) parts.push(fmtTime(turn.ts));
  if (parts.length === 0) return null;
  return <div className="text-xs text-muted">{parts.join(" · ")}</div>;
}

// SummaryDivider marks the fold point in a long thread (M925): everything above
// it has been condensed into one briefing that rides with each run instead of
// the raw turns (which would otherwise fall off the history window). Click to
// read exactly what the agent now knows about the older conversation.
export function SummaryDivider({ summary }: { summary: HistorySummary }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="mb-4">
      <button
        onClick={() => setOpen((o) => !o)}
        title="Older messages were condensed into a briefing the agent carries — click to read it"
        className="flex w-full items-center gap-2 text-[11px] text-muted transition-colors hover:text-foreground"
      >
        <span className="h-px flex-1 bg-border" />
        <Scissors className="size-3 shrink-0" />
        <span>
          {summary.upto} older message{summary.upto === 1 ? "" : "s"} summarized for the agent
        </span>
        {open ? <ChevronDown className="size-3 shrink-0" /> : <ChevronRight className="size-3 shrink-0" />}
        <span className="h-px flex-1 bg-border" />
      </button>
      {open && (
        <div className="mx-6 mt-2 whitespace-pre-wrap break-words rounded-lg border border-border bg-panel/40 px-3 py-2 text-xs leading-relaxed text-muted">
          {summary.text}
        </div>
      )}
    </div>
  );
}

