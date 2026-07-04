import { useEffect, useRef, useState } from "react";
import {
  Wrench,
  ShieldCheck,
  ShieldX,
  ChevronRight,
  ChevronDown,
  Loader2,
  User,
  Sparkles,
  AlertTriangle,
  Copy,
  Check,
  Brain,
  RotateCcw,
  ArrowRight,
  X,
  Volume2,
  VolumeX,
  Pencil,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
import { ToolOutput } from "@/components/DataView";
import { AgentAvatar } from "@/components/AgentAvatar";
import { speak, stopSpeaking, speechSupported } from "@/lib/speech";
import {
  turnText,
  type ChatTurn,
  type ChatTool,
  type TimelineItem,
} from "@/lib/chat";
import { useChat } from "@/lib/chatStore";
import { type Msg } from "@/lib/conversations";
import { ContextChip, CompactionNote } from "./context";
import { ConversationPersona, ExecutionProfilePicker, FallbackNote, PromptLauncher, SteerNote, SummaryDivider, TurnMeta } from "./pickers";
export { ContextChip, ContextModal, CompactionNote, barTone } from "./context";
export { ConversationPersona, ExecutionProfilePicker, FallbackNote, PromptLauncher, SummaryDivider } from "./pickers";

export function MessageRow({
  msg,
  agent,
  onRetry,
  onContinue,
  onRegenerate,
  onEdit,
}: {
  msg: Msg;
  agent?: string;
  onRetry?: () => void;
  onContinue?: () => void;
  onRegenerate?: () => void;
  onEdit?: (text: string) => void;
}) {
  if (msg.role === "user") {
    return <UserBubble text={msg.text} onEdit={onEdit} />;
  }
  return <AssistantBubble turn={msg.turn} agent={agent} onRetry={onRetry} onContinue={onContinue} onRegenerate={onRegenerate} />;
}

// UserBubble renders one user message, with an inline edit affordance: a pencil
// (shown on hover) switches the bubble to a textarea so you can refine the ask
// and re-run from that point — Enter / Save submits, Esc / Cancel restores. The
// edit handler is absent while a run is in flight, so the pencil simply hides.
export function UserBubble({ text, onEdit }: { text: string; onEdit?: (text: string) => void }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(text);
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (editing) {
      const el = ref.current;
      if (el) {
        el.focus();
        el.setSelectionRange(el.value.length, el.value.length);
        el.style.height = "auto";
        el.style.height = `${el.scrollHeight}px`;
      }
    }
  }, [editing]);

  function begin() {
    setDraft(text);
    setEditing(true);
  }
  function cancel() {
    setEditing(false);
    setDraft(text);
  }
  function save() {
    const t = draft.trim();
    if (!t || !onEdit) return cancel();
    setEditing(false);
    if (t !== text) onEdit(t);
  }
  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      save();
    } else if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  }

  if (editing) {
    return (
      <div className="flex justify-end gap-2">
        <div className="w-full max-w-[85%] rounded-xl rounded-br-sm border border-accent/40 bg-accent/10 p-2">
          <textarea
            ref={ref}
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              e.target.style.height = "auto";
              e.target.style.height = `${e.target.scrollHeight}px`;
            }}
            onKeyDown={onKeyDown}
            aria-label="Edit message"
            className="max-h-60 w-full resize-none bg-transparent text-sm text-foreground outline-none"
          />
          <div className="mt-1.5 flex items-center justify-end gap-1.5">
            <button
              onClick={cancel}
              className="rounded-md px-2 py-1 text-xs text-muted transition-colors hover:text-foreground"
            >
              Cancel
            </button>
            <button
              onClick={save}
              className="rounded-md bg-accent px-2 py-1 text-xs font-medium text-white transition-opacity hover:opacity-90"
            >
              Save & run
            </button>
          </div>
        </div>
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-panel text-muted">
          <User className="size-4" />
        </div>
      </div>
    );
  }

  return (
    <div className="group flex items-start justify-end gap-2">
      {onEdit && (
        <button
          onClick={begin}
          title="Edit & re-run"
          className="mt-1 shrink-0 self-center text-muted opacity-0 transition-opacity hover:text-foreground group-hover:opacity-100"
        >
          <Pencil className="size-3.5" />
        </button>
      )}
      <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-accent/15 px-4 py-2.5 text-sm text-foreground whitespace-pre-wrap break-words shadow-sm">
        {text}
      </div>
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-accent/20 to-accent/5 text-accent shadow-sm">
        <User className="size-4" />
      </div>
    </div>
  );
}

export function AssistantBubble({
  turn,
  agent,
  onRetry,
  onContinue,
  onRegenerate,
}: {
  turn: ChatTurn;
  agent?: string;
  onRetry?: () => void;
  onContinue?: () => void;
  onRegenerate?: () => void;
}) {
  const text = turnText(turn);
  const streaming = turn.status === "streaming";
  // Turns restored from older storage have no `timeline` — fall back to the prior
  // shape (tools first, then the final text) so old conversations still render.
  const timeline: TimelineItem[] =
    turn.timeline && turn.timeline.length > 0
      ? turn.timeline
      : [
          ...turn.tools.map((c) => ({ kind: "tool" as const, callId: c.callId })),
          ...(text ? [{ kind: "text" as const, text }] : []),
        ];
  // The trailing timeline item: if it's a tool that hasn't produced output yet,
  // that's the call in flight — drive the live "working" pulse from it.
  const lastItem = timeline[timeline.length - 1];
  const lastTool = lastItem?.kind === "tool" ? turn.tools.find((c) => c.callId === lastItem.callId) : undefined;
  const runningTool = lastTool && !lastTool.output && lastTool.allow !== false ? lastTool : undefined;
  // Show the live pulse at the very start (nothing streamed yet, and no reasoning
  // header to stand in for it) or while the trailing tool runs. Once text streams,
  // the text + cursor convey liveliness instead.
  const working = streaming && (timeline.length === 0 ? !turn.reasoning : !!runningTool);

  return (
    <div className="flex gap-2">
      {/* Who's answering: a specific roster agent shows its gradient monogram
          (breathing while it streams) so the thread reflects the agent at work;
          the default house assistant keeps the spark mark. */}
      {agent ? (
        <AgentAvatar slug={agent} size={28} status={streaming ? "running" : undefined} className="mt-0.5" />
      ) : (
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-accent/25 to-accent2/10 text-accent shadow-sm ring-1 ring-inset ring-accent/20">
          <Sparkles className="size-4" />
        </div>
      )}
      <div className="min-w-0 flex-1 space-y-2">
        {turn.reasoning && <ReasoningBlock text={turn.reasoning} live={streaming && timeline.length === 0} />}

        {/* Chronological timeline: each text run and tool call in the order it
            happened, so the conversation reads as "said this → ran that → said
            this" and the final answer is simply the last text run. While
            streaming, text is plain (an unclosed code fence would otherwise
            swallow the rest mid-stream); once final, it renders as Markdown. */}
        {timeline.map((item, i) => {
          if (item.kind === "tool") {
            const c = turn.tools.find((x) => x.callId === item.callId);
            return c ? <ToolChip key={`tl${i}`} c={c} /> : null;
          }
          const isLast = i === timeline.length - 1;
          return streaming ? (
            <div key={`tl${i}`} className="text-sm leading-relaxed text-foreground whitespace-pre-wrap break-words">
              {item.text}
              {isLast && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-accent align-text-bottom" />}
            </div>
          ) : (
            <Markdown key={`tl${i}`} source={item.text} className="text-sm text-foreground" />
          );
        })}

        {working && <WorkingIndicator running={runningTool?.tool} />}

        {turn.fallbacks && turn.fallbacks.length > 0 && <FallbackNote hops={turn.fallbacks} />}

        {turn.steers && turn.steers.length > 0 && <SteerNote steers={turn.steers} />}

        {turn.context && turn.context.compactions.length > 0 && <CompactionNote events={turn.context.compactions} />}

        {turn.status === "error" && (
          <div className="space-y-2">
            <div className="flex items-start gap-2 rounded-lg bg-bad/10 px-3 py-2 text-sm text-bad">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span className="break-words">{turn.error || "the run failed"}</span>
            </div>
            <div className="flex items-center gap-2">
              {onContinue && (
                <button
                  onClick={onContinue}
                  title="Resume from where it stopped, keeping the work so far (use this if it hit the iteration limit)"
                  className="inline-flex items-center gap-1.5 rounded-md border border-accent/50 px-2.5 py-1 text-xs text-accent transition-colors hover:bg-accent/10"
                >
                  <ArrowRight className="size-3.5" /> Continue
                </button>
              )}
              {onRetry && (
                <button
                  onClick={onRetry}
                  title="Start this task over from scratch"
                  className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-accent"
                >
                  <RotateCcw className="size-3.5" /> Retry
                </button>
              )}
            </div>
          </div>
        )}

        {turn.status === "done" && (
          <div className="flex items-center gap-3">
            <TurnMeta turn={turn} />
            <ContextChip turn={turn} />
            {text && <CopyAnswer text={text} />}
            {text && <SpeakAnswer text={text} />}
            {onRegenerate && (
              <button
                onClick={onRegenerate}
                title="Regenerate this answer"
                className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-foreground"
              >
                <RotateCcw className="size-3" /> Regenerate
              </button>
            )}
          </div>
        )}

        {turn.status === "done" && turn.correlationId && <LearnedChips corr={turn.correlationId} />}
      </div>
    </div>
  );
}

// WorkingIndicator is the live "the agent is working" pulse: a soft pulsing dot,
// what it's doing right now (running a specific tool, or thinking), and a ticking
// elapsed timer — so a run feels alive and you can see the agent at work rather
// than staring at a static spinner.
function WorkingIndicator({ running }: { running?: string }) {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const start = Date.now();
    const id = window.setInterval(() => setElapsed((Date.now() - start) / 1000), 100);
    return () => window.clearInterval(id);
  }, []);
  const label = running ? `running ${running}…` : "thinking…";
  return (
    <div className="flex items-center gap-2 text-sm text-muted">
      <span className="work-pulse size-2.5 rounded-full bg-accent" />
      <span>{label}</span>
      <span className="tabular-nums text-xs text-muted/70">{elapsed.toFixed(1)}s</span>
    </div>
  );
}

// ReasoningBlock shows a reasoning model's chain of thought: expanded and live
// while the model is still thinking (before any answer text), collapsible once
// the answer arrives — so it's visible but never in the way.
function ReasoningBlock({ text, live }: { text: string; live: boolean }) {
  const [open, setOpen] = useState(false);
  const show = live || open;
  return (
    <div className="rounded-lg border border-border bg-panel/40 text-xs">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left text-muted hover:text-foreground"
      >
        {live ? (
          <Loader2 className="size-3.5 shrink-0 animate-spin" />
        ) : show ? (
          <ChevronDown className="size-3.5 shrink-0" />
        ) : (
          <ChevronRight className="size-3.5 shrink-0" />
        )}
        <Brain className="size-3.5 shrink-0" />
        <span>{live ? "Thinking…" : "Reasoning"}</span>
      </button>
      {show && (
        <div className="max-h-48 overflow-auto whitespace-pre-wrap break-words border-t border-border px-2.5 py-1.5 font-mono text-[11px] leading-snug text-muted">
          {text}
        </div>
      )}
    </div>
  );
}

// CopyAnswer copies the agent's full reply — the whole answer is often what you
// want to paste elsewhere, not just a code block within it.
function CopyAnswer({ text }: { text: string }) {
  const ui = useUI();
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      ui.toast("Couldn't copy — clipboard unavailable", "error");
    }
  }
  return (
    <button
      onClick={copy}
      title={copied ? "Copied" : "Copy answer"}
      className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-foreground"
    >
      {copied ? <Check className="size-3 text-good" /> : <Copy className="size-3" />}
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

// SpeakAnswer reads one answer aloud via the browser's speech synthesis — the
// per-message companion to the global auto-speak toggle. Renders nothing when the
// browser can't do TTS. Toggles between speak and stop, and stops on unmount.
function SpeakAnswer({ text }: { text: string }) {
  const [speaking, setSpeaking] = useState(false);
  useEffect(() => () => stopSpeaking(), []);
  if (!speechSupported()) return null;
  function toggle() {
    if (speaking) {
      stopSpeaking();
      setSpeaking(false);
    } else {
      setSpeaking(true);
      speak(text, () => setSpeaking(false));
    }
  }
  return (
    <button
      onClick={toggle}
      title={speaking ? "Stop" : "Read aloud"}
      className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-foreground"
    >
      {speaking ? <VolumeX className="size-3" /> : <Volume2 className="size-3" />}
      {speaking ? "Stop" : "Speak"}
    </button>
  );
}

// LearnedChips shows the memories the daemon recorded during this turn — what the
// agent took away from the exchange — each one forgettable in a click if it's not
// worth keeping. Renders nothing until something is actually learned.
function LearnedChips({ corr }: { corr: string }) {
  const { learnedFor, forgetLearned } = useChat();
  const ui = useUI();
  const [busy, setBusy] = useState<string | null>(null);
  const items = learnedFor(corr);
  if (items.length === 0) return null;

  async function forget(id: string, subject: string) {
    const ok = await ui.confirm({
      title: "Forget this memory?",
      message: subject ? `“${subject}” will be permanently removed.` : "This memory will be permanently removed.",
      confirmLabel: "Forget",
      danger: true,
    });
    if (!ok) return;
    setBusy(id);
    try {
      await forgetLearned(corr, id);
      ui.toast("Memory forgotten", "success");
    } catch (e) {
      ui.toast(`forget failed: ${(e as Error).message}`, "error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
      <span className="inline-flex items-center gap-1 text-xs text-muted">
        <Brain className="size-3 text-accent" /> learned
      </span>
      {items.map((m) => (
        <span
          key={m.id}
          className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-xs"
          title={`${m.action} · ${m.type}`}
        >
          <span className="font-semibold uppercase tracking-normal text-accent">{m.type}</span>
          <span className="max-w-[16rem] truncate">{m.subject || "(memory)"}</span>
          <button
            onClick={() => forget(m.id, m.subject)}
            disabled={busy === m.id}
            title="Forget this memory"
            className="text-muted transition-colors hover:text-bad disabled:opacity-50"
          >
            <X className="size-3" />
          </button>
        </span>
      ))}
    </div>
  );
}

function ToolChip({ c }: { c: ChatTool }) {
  const [open, setOpen] = useState(false);
  const denied = c.allow === false;
  // The chip is expandable whenever there's a trace to show — the arguments it
  // was called with and/or the result it returned.
  const hasDetail = !!c.input || !!c.output;
  return (
    <div className="rounded-lg border border-border bg-panel/60 text-xs">
      <button
        onClick={() => hasDetail && setOpen((o) => !o)}
        className={cn("flex w-full items-center gap-2 px-2.5 py-1.5 text-left", !hasDetail && "cursor-default")}
      >
        {hasDetail ? (
          open ? <ChevronDown className="size-3.5 text-muted" /> : <ChevronRight className="size-3.5 text-muted" />
        ) : (
          <Wrench className="size-3.5 text-muted" />
        )}
        <span className="font-medium">{c.tool || "tool"}</span>
        {c.capability && <Badge variant="accent">{c.capability}</Badge>}
        {denied ? (
          <Badge variant="bad">
            <ShieldX className="mr-1 size-3" />
            {c.hardDenied ? "hard-denied" : "denied"}
          </Badge>
        ) : (
          <Badge variant="good">
            <ShieldCheck className="mr-1 size-3" />
            allowed
          </Badge>
        )}
        {c.error && <Badge variant="bad">error</Badge>}
      </button>
      {open && hasDetail && (
        <div className="border-t border-border">
          {c.input && (
            <div>
              <div className="px-2.5 pt-1.5 text-xs font-semibold uppercase tracking-normal text-muted">Arguments</div>
              <ToolOutput text={c.input} />
            </div>
          )}
          {c.output && (
            <div>
              <div className="px-2.5 pt-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
                {c.error ? "Error" : "Result"}
              </div>
              <ToolOutput text={c.output} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
