import { useEffect, useMemo, useRef, useState } from "react";
import { Search, X, Sparkles, Brain, ListTree, Check } from "lucide-react";
import { cn } from "@/lib/utils";
import { getJSON } from "@/lib/api";
import { skillToRef, memoryToRef, runToRef, type AttachRef, type RefKind } from "@/lib/attach";

// AttachPicker is the modal behind the composer's paperclip: pick an existing
// skill, memory, or past run to hand the agent as context for your next message.
// It loads the same list APIs the dedicated views use; selecting an item resolves
// it to an AttachRef (label + content) the caller stacks above the textarea.
const TABS: { kind: RefKind; label: string; icon: typeof Sparkles; path: string }[] = [
  { kind: "skill", label: "Skills", icon: Sparkles, path: "/api/skills" },
  { kind: "memory", label: "Memory", icon: Brain, path: "/api/memory" },
  { kind: "run", label: "Runs", icon: ListTree, path: "/api/runs" },
];

export function AttachPicker({
  selectedIds,
  onPick,
  onClose,
}: {
  selectedIds: Set<string>;
  onPick: (ref: AttachRef) => void;
  onClose: () => void;
}) {
  const [tab, setTab] = useState<RefKind>("skill");
  const [data, setData] = useState<Record<RefKind, AttachRef[]>>({ skill: [], memory: [], run: [] });
  const [loading, setLoading] = useState<Record<RefKind, boolean>>({ skill: false, memory: false, run: false });
  const [q, setQ] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Lazily load each tab's list the first time it's shown.
  useEffect(() => {
    if (data[tab].length || loading[tab]) return;
    setLoading((l) => ({ ...l, [tab]: true }));
    const t = TABS.find((x) => x.kind === tab)!;
    getJSON<any>(t.path)
      .then((d) => {
        const refs =
          tab === "skill"
            ? (d.skills || []).map(skillToRef)
            : tab === "memory"
              ? (d.records || []).map(memoryToRef)
              : (d.runs || []).map(runToRef);
        setData((prev) => ({ ...prev, [tab]: refs.filter(Boolean) as AttachRef[] }));
      })
      .catch(() => {
        /* leave empty; the tab just shows "nothing to attach" */
      })
      .finally(() => setLoading((l) => ({ ...l, [tab]: false })));
  }, [tab, data, loading]);

  const shown = useMemo(() => {
    const f = q.trim().toLowerCase();
    const list = data[tab];
    if (!f) return list;
    return list.filter((r) => `${r.label} ${r.content}`.toLowerCase().includes(f));
  }, [data, tab, q]);

  return (
    <div className="modal-overlay fixed inset-0 z-[110] flex items-start justify-center bg-black/50 p-4 pt-[10vh]" onClick={onClose}>
      <div
        className="modal-in flex max-h-[70vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        {/* Tabs */}
        <div className="flex items-center gap-1 border-b border-border px-2 py-2">
          {TABS.map((t) => (
            <button
              key={t.kind}
              onClick={() => setTab(t.kind)}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs transition-colors",
                tab === t.kind ? "bg-accent/15 text-accent" : "text-muted hover:bg-panel",
              )}
            >
              <t.icon className="size-3.5" /> {t.label}
            </button>
          ))}
          <button onClick={onClose} className="ml-auto rounded p-1 text-muted hover:text-foreground" title="Close">
            <X className="size-4" />
          </button>
        </div>

        {/* Search */}
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <Search className="size-4 shrink-0 text-muted" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={`Search ${tab}…`}
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
        </div>

        <div className="min-h-0 flex-1 overflow-auto py-1">
          {loading[tab] ? (
            <div className="px-3 py-6 text-center text-xs text-muted">loading…</div>
          ) : shown.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-muted">nothing to attach</div>
          ) : (
            shown.map((r) => {
              const picked = selectedIds.has(r.id);
              return (
                <button
                  key={r.id}
                  onClick={() => onPick(r)}
                  disabled={picked}
                  className={cn(
                    "flex w-full items-start gap-2 px-3 py-2 text-left transition-colors hover:bg-panel disabled:opacity-50",
                    picked && "bg-accent/5",
                  )}
                >
                  <Check className={cn("mt-0.5 size-4 shrink-0", picked ? "text-accent" : "text-transparent")} />
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{r.label}</div>
                    <div className="line-clamp-2 text-xs text-muted">{r.content}</div>
                  </div>
                </button>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}
