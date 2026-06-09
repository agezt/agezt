import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Search, Check, Cpu, Wrench, Brain, KeyRound, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { getJSON } from "@/lib/api";
import {
  flattenModels,
  filterModels,
  groupByProvider,
  fmtContext,
  type ModelCatalog,
  type ModelOption,
} from "@/lib/models";

// ModelPicker replaces the raw model-override text box with a searchable,
// capability-aware picker. The trigger shows the current selection (or the
// daemon default); clicking opens a modal listing every provider→model from
// /api/catalog with tool/reasoning/context/cost badges, so you pick by what the
// model can actually do — not by remembering its id.
export function ModelPicker({
  value,
  onChange,
  activeModel,
}: {
  value: string; // "" = use the daemon default
  onChange: (id: string) => void;
  activeModel?: string; // the daemon's default, shown as a hint
}) {
  const [open, setOpen] = useState(false);
  const label = value || activeModel || "default";
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        title="Choose model"
        className="inline-flex h-6 max-w-[16rem] items-center gap-1 rounded border border-border bg-panel px-2 text-xs outline-none transition-colors hover:border-accent focus-visible:border-accent"
      >
        <Cpu className="size-3 shrink-0 text-muted" />
        <span className="truncate">{label}</span>
        <ChevronDown className="size-3 shrink-0 text-muted" />
      </button>
      {open && (
        <ModelModal
          value={value}
          activeModel={activeModel}
          onClose={() => setOpen(false)}
          onPick={(id) => {
            onChange(id);
            setOpen(false);
          }}
        />
      )}
    </>
  );
}

function ModelModal({
  value,
  activeModel,
  onClose,
  onPick,
}: {
  value: string;
  activeModel?: string;
  onClose: () => void;
  onPick: (id: string) => void;
}) {
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  const [err, setErr] = useState<string | null>(null);
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

  useEffect(() => {
    getJSON<ModelCatalog>("/api/catalog")
      .then(setCat)
      .catch((e) => setErr((e as Error).message));
  }, []);

  const groups = useMemo(() => {
    const all = flattenModels(cat);
    return groupByProvider(filterModels(all, q));
  }, [cat, q]);

  const total = useMemo(() => flattenModels(cat).length, [cat]);

  return (
    <div
      className="modal-overlay fixed inset-0 z-[110] flex items-start justify-center bg-black/50 p-4 pt-[10vh]"
      onClick={onClose}
    >
      <div
        className="modal-in flex max-h-[70vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        {/* Search header */}
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          <Search className="size-4 shrink-0 text-muted" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={`Search ${total || ""} models…`}
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
          <button onClick={onClose} className="shrink-0 text-muted hover:text-foreground" title="Close">
            <X className="size-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-auto py-1">
          {/* The "default" option always leads — clears the override. */}
          <Option
            selected={value === ""}
            onClick={() => onPick("")}
            title="Daemon default"
            subtitle={activeModel ? `currently ${activeModel}` : "the daemon's configured model"}
          />

          {err ? (
            <div className="px-3 py-6 text-center text-xs text-bad">{err}</div>
          ) : !cat ? (
            <div className="px-3 py-6 text-center text-xs text-muted">loading catalog…</div>
          ) : groups.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-muted">no models match “{q}”</div>
          ) : (
            groups.map((g) => (
              <div key={g.providerId} className="mt-1">
                <div className="flex items-center gap-1.5 px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-muted">
                  {g.providerName}
                  {g.credentialed && <KeyRound className="size-3 text-good" />}
                  <span className="text-muted/60">· {g.options.length}</span>
                </div>
                {g.options.map((m) => (
                  <ModelRow key={`${g.providerId}/${m.id}`} m={m} selected={m.id === value} onClick={() => onPick(m.id)} />
                ))}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

function Option({
  selected,
  onClick,
  title,
  subtitle,
}: {
  selected: boolean;
  onClick: () => void;
  title: string;
  subtitle: string;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-panel",
        selected && "bg-accent/10",
      )}
    >
      <Check className={cn("size-4 shrink-0", selected ? "text-accent" : "text-transparent")} />
      <div className="min-w-0">
        <div className="truncate font-medium">{title}</div>
        <div className="truncate text-xs text-muted">{subtitle}</div>
      </div>
    </button>
  );
}

function ModelRow({ m, selected, onClick }: { m: ModelOption; selected: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-panel",
        selected && "bg-accent/10",
      )}
    >
      <Check className={cn("size-4 shrink-0", selected ? "text-accent" : "text-transparent")} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{m.name}</div>
        <div className="truncate font-mono text-[10px] text-muted">{m.id}</div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5 text-[10px] text-muted">
        {m.toolCall && (
          <span title="supports tool calls" className="inline-flex items-center gap-0.5 text-accent">
            <Wrench className="size-3" />
          </span>
        )}
        {m.reasoning && (
          <span title="reasoning model" className="inline-flex items-center gap-0.5 text-accent">
            <Brain className="size-3" />
          </span>
        )}
        {m.context > 0 && <span title="context window">{fmtContext(m.context)}</span>}
        {m.costInput > 0 && <span title="input $/Mtok">${m.costInput}</span>}
      </div>
    </button>
  );
}
