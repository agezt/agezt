import { useEffect, useMemo, useRef, useState } from "react";
import { Search, CornerDownLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { filterCommands, type CommandItem } from "@/lib/commands";

// CommandPalette is the ⌘K launcher: fuzzy-search every view, action and recent
// run, navigate with the keyboard, Enter to run. A magnificent way to reach
// everything instantly.
export function CommandPalette({ open, onClose, items }: { open: boolean; onClose: () => void; items: CommandItem[] }) {
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const results = useMemo(() => filterCommands(items, q), [items, q]);

  // Reset + focus on open.
  useEffect(() => {
    if (open) {
      setQ("");
      setSel(0);
      // focus after paint
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Clamp selection as results change.
  useEffect(() => {
    setSel((s) => Math.min(s, Math.max(0, results.length - 1)));
  }, [results.length]);

  // Keep the selected row in view.
  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-i="${sel}"]`)?.scrollIntoView({ block: "nearest" });
  }, [sel]);

  if (!open) return null;

  function choose(i: number) {
    const item = results[i];
    if (!item) return;
    onClose();
    item.run();
  }

  function onKey(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSel((s) => Math.min(s + 1, results.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSel((s) => Math.max(s - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      choose(sel);
    } else if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    }
  }

  // Group results for display while keeping a flat index for keyboard nav.
  let flat = -1;
  const groups = new Map<string, { item: CommandItem; idx: number }[]>();
  results.forEach((item) => {
    flat++;
    const arr = groups.get(item.group) || [];
    arr.push({ item, idx: flat });
    groups.set(item.group, arr);
  });

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-4 pt-[12vh]" onClick={onClose}>
      <div
        className="w-full max-w-xl overflow-hidden rounded-xl bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 px-3">
          <Search className="size-4 shrink-0 text-muted" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onKey}
            placeholder="Jump to a view, run an action, open a run…"
            className="h-11 flex-1 bg-transparent text-sm outline-none placeholder:text-muted focus-glow"
          />
          <kbd className="rounded border border-border px-1.5 py-0.5 text-xs text-muted">esc</kbd>
        </div>
        <div className="gradient-rule" />
        <ul ref={listRef} className="max-h-[55vh] overflow-auto py-1">
          {results.length === 0 ? (
            <li className="px-3 py-6 text-center text-sm text-muted">no matches</li>
          ) : (
            [...groups.entries()].map(([group, rows]) => (
              <li key={group}>
                <div className="px-3 pb-0.5 pt-2 text-xs font-semibold uppercase tracking-normal text-muted">
                  {group}
                </div>
                <ul>
                  {rows.map(({ item, idx }) => (
                    <li
                      key={item.id}
                      data-i={idx}
                      onMouseEnter={() => setSel(idx)}
                      onClick={() => choose(idx)}
                      className={cn(
                        "mx-1 flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-sm",
                        idx === sel ? "bg-accent/15 text-accent" : "hover:bg-panel",
                      )}
                    >
                      <span className="truncate">{item.label}</span>
                      {item.hint && <span className="ml-auto shrink-0 text-xs text-muted">{item.hint}</span>}
                      {idx === sel && <CornerDownLeft className="size-3.5 shrink-0 text-muted" />}
                    </li>
                  ))}
                </ul>
              </li>
            ))
          )}
        </ul>
      </div>
    </div>
  );
}
