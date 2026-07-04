import { useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowUp, ListPlus, Pencil, Pin, Send, Sparkles, Trash2, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { getJSON } from "@/lib/api";
import { type Msg } from "@/lib/conversations";
import type { QueuedMsg } from "@/lib/queue";
import { type Suggestion } from "@/components/SuggestionsBar";

// ConversationItem is one row in the thread sidebar: click to switch, hover to
// reveal rename (pencil → inline input) and delete. Renaming a thread (M720) lets
// you organise the sidebar instead of living with auto-derived titles.
export function ConversationItem({
  title,
  active,
  pinned,
  onSelect,
  onRemove,
  onRename,
  onTogglePin,
}: {
  title: string;
  active: boolean;
  pinned?: boolean;
  onSelect: () => void;
  onRemove: () => void;
  onRename: (title: string) => void;
  onTogglePin?: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(title);
  const ref = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) {
      ref.current?.focus();
      ref.current?.select();
    }
  }, [editing]);

  function begin() {
    setDraft(title);
    setEditing(true);
  }
  function commit() {
    setEditing(false);
    if (draft !== title) onRename(draft);
  }

  return (
    <div
      className={cn(
        "group relative flex items-center gap-1 rounded-md px-2 py-1.5 text-xs transition-colors",
        active
          ? "bg-gradient-to-r from-accent/20 to-transparent font-medium text-accent before:absolute before:left-0 before:top-1/2 before:h-4 before:w-[3px] before:-translate-y-1/2 before:rounded-r-full before:bg-accent before:content-['']"
          : "text-muted hover:bg-panel",
      )}
    >
      {editing ? (
        <input
          ref={ref}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
            else if (e.key === "Escape") {
              setDraft(title);
              setEditing(false);
            }
          }}
          aria-label="Conversation title"
          className="min-w-0 flex-1 rounded bg-panel px-1 text-xs outline-none ring-1 ring-accent/40"
        />
      ) : (
        <>
          {onTogglePin && (
            <button
              onClick={onTogglePin}
              title={pinned ? "Unpin conversation" : "Pin conversation"}
              aria-label={pinned ? "Unpin conversation" : "Pin conversation"}
              className={cn(
                "shrink-0 transition-opacity hover:text-foreground",
                pinned ? "text-accent opacity-100" : "opacity-0 group-hover:opacity-100",
              )}
            >
              <Pin className={cn("size-3", pinned && "fill-current")} />
            </button>
          )}
          <button onClick={onSelect} onDoubleClick={begin} className="min-w-0 flex-1 truncate text-left">
            {title}
          </button>
          <button
            onClick={begin}
            title="Rename conversation"
            aria-label="Rename conversation"
            className="shrink-0 opacity-0 transition-opacity hover:text-foreground group-hover:opacity-100"
          >
            <Pencil className="size-3" />
          </button>
          <button
            onClick={onRemove}
            title="Delete conversation"
            aria-label="Delete conversation"
            className="shrink-0 opacity-0 transition-opacity hover:text-bad group-hover:opacity-100"
          >
            <Trash2 className="size-3.5" />
          </button>
        </>
      )}
    </div>
  );
}

const DEFAULT_EXAMPLES = [
  "What can you do?",
  "Summarize my recent runs",
  "Turn off the living room light",
  "What's the weather in Istanbul?",
];

// lastAssistantTools returns the tool names the most recent assistant turn used,
// so the suggestions bar can tailor next-step prompts to what just happened.
export function lastAssistantTools(messages: Msg[]): string[] {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.role === "assistant") {
      return m.turn.tools.map((t) => t.tool).filter(Boolean);
    }
  }
  return [];
}

export function EmptyState({ onPick }: { onPick: (s: string) => void }) {
  // Saved prompts (M713) replace the generic examples once the owner defines any —
  // their own reusable workflows, one click to launch. When there are none, fall
  // back to memory-derived starter prompts (built from the agent's own memory),
  // and only then to the generic examples.
  const [prompts, setPrompts] = useState<{ title: string; text: string }[]>([]);
  const [memChips, setMemChips] = useState<{ title: string; text: string }[]>([]);
  useEffect(() => {
    let live = true;
    getJSON<{ prompts?: { title: string; text: string }[] }>("/api/prompts")
      .then((r) => {
        if (!live) return;
        const saved = r.prompts || [];
        setPrompts(saved);
        // Only reach for memory starters when the owner hasn't defined prompts.
        if (saved.length === 0) {
          getJSON<{ suggestions?: Suggestion[] }>("/api/suggestions")
            .then((d) => {
              if (!live) return;
              setMemChips(
                (d.suggestions || [])
                  .filter((s) => s.category === "memory")
                  .map((s) => ({ title: s.label, text: s.prompt })),
              );
            })
            .catch(() => {
              /* suggestions are optional — fall back to the examples */
            });
        }
      })
      .catch(() => {
        /* prompts are optional — fall back to the examples */
      });
    return () => {
      live = false;
    };
  }, []);
  const hasSaved = prompts.length > 0;
  const chips = hasSaved
    ? prompts
    : memChips.length > 0
      ? memChips
      : DEFAULT_EXAMPLES.map((text) => ({ title: text, text }));
  const fromMemory = !hasSaved && memChips.length > 0;

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 text-center">
      <div className="flex size-12 items-center justify-center rounded-xl bg-gradient-to-br from-accent/30 to-accent/5 text-accent shadow-e2 ring-1 ring-inset ring-accent/20">
        <Sparkles className="size-6" />
      </div>
      <div>
        <h2 className="text-lg font-semibold">Talk to your agent</h2>
        <p className="mt-1 text-sm text-muted">
          Type an intent and watch it run — streaming answer, live tool calls, real cost.
        </p>
      </div>
      <div className="flex max-w-2xl flex-wrap justify-center gap-2">
        {chips.map((p, i) => (
          <button
            key={`${p.title}-${i}`}
            onClick={() => onPick(p.text)}
            title={hasSaved || fromMemory ? p.text : undefined}
            className="rounded-full bg-panel/60 px-3 py-1 text-xs text-muted shadow-sm transition-all hover:-translate-y-0.5 hover:bg-accent/10 hover:text-accent hover:shadow-md"
          >
            {p.title}
          </button>
        ))}
      </div>
      {hasSaved && <p className="text-xs text-muted/70">your saved prompts · manage in System → Prompts</p>}
      {fromMemory && <p className="text-xs text-muted/70">drawn from your agent's memory</p>}
    </div>
  );
}

// QueuePanel (M962) shows the pending follow-up messages lined up while a run
// streams. The next one auto-sends when the run finishes; here the operator can
// reorder (up/down), delete one, clear all, or — when idle — send the front now.
export function QueuePanel({
  queue,
  busy,
  onUp,
  onDown,
  onRemove,
  onClear,
  onSendNow,
}: {
  queue: QueuedMsg[];
  busy: boolean;
  onUp: (id: string) => void;
  onDown: (id: string) => void;
  onRemove: (id: string) => void;
  onClear: () => void;
  onSendNow: () => void;
}) {
  return (
    <div className="mb-2 rounded-lg bg-panel/40 p-2">
      <div className="mb-1.5 flex items-center gap-2 px-0.5 text-xs text-muted">
        <ListPlus className="size-3.5" />
        <span className="font-medium text-foreground/80">Queue</span>
        <span className="rounded-full bg-card px-1.5 tabular-nums">{queue.length}</span>
        <span className="text-xs">{busy ? "next sends when this run finishes" : "idle — send the next now"}</span>
        <button onClick={onClear} className="ml-auto inline-flex items-center gap-1 text-xs hover:text-bad" title="Clear the whole queue">
          <Trash2 className="size-3" /> Clear
        </button>
      </div>
      <ol className="space-y-1">
        {queue.map((m, i) => (
          <li key={m.id} className="flex items-center gap-1.5 rounded-md bg-card px-2 py-1 text-xs">
            <span className="w-4 shrink-0 text-center font-mono text-xs text-muted">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate" title={m.text}>{m.text}</span>
            {!busy && i === 0 && (
              <button onClick={onSendNow} className="shrink-0 text-accent hover:underline" title="Send this now">
                <Send className="size-3.5" />
              </button>
            )}
            <button onClick={() => onUp(m.id)} disabled={i === 0} className="shrink-0 text-muted enabled:hover:text-foreground disabled:opacity-30" title="Move up">
              <ArrowUp className="size-3.5" />
            </button>
            <button onClick={() => onDown(m.id)} disabled={i === queue.length - 1} className="shrink-0 text-muted enabled:hover:text-foreground disabled:opacity-30" title="Move down">
              <ArrowDown className="size-3.5" />
            </button>
            <button onClick={() => onRemove(m.id)} className="shrink-0 text-muted hover:text-bad" title="Remove">
              <X className="size-3.5" />
            </button>
          </li>
        ))}
      </ol>
    </div>
  );
}
