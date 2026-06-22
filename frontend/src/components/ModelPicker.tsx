import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Search, Check, Cpu, Wrench, Brain, KeyRound, X, Route, Waypoints } from "lucide-react";
import { cn } from "@/lib/utils";
import { getJSON } from "@/lib/api";
import {
  flattenModels,
  filterModels,
  groupByProvider,
  pinnedOptions,
  fmtContext,
  type ModelCatalog,
  type ModelOption,
} from "@/lib/models";
import { isChainRef, chainName, chainRef, type ChainsState } from "@/lib/chains";

// PinnedModels is an ordered id list shown as the picker's FIRST group (M931) —
// e.g. the chat task's routing chain, so the models the run will actually fall
// back through lead the list instead of being buried under provider groups.
export interface PinnedModels {
  label: string;
  ids: string[];
}

// ModelPicker replaces the raw model-override text box with a searchable,
// capability-aware picker. The trigger shows the current selection (or the
// daemon default); clicking opens a modal listing every provider→model from
// /api/catalog with tool/reasoning/context/cost badges, so you pick by what the
// model can actually do — not by remembering its id.
export function ModelPicker({
  value,
  onChange,
  activeModel,
  pinned,
  allowChains = true,
  triggerClassName,
}: {
  value: string; // "" = use the daemon default
  onChange: (id: string) => void;
  activeModel?: string; // the daemon's default, shown as a hint
  pinned?: PinnedModels; // optional ordered group shown first (e.g. the chat routing chain)
  allowChains?: boolean; // show named fallback chains as selectable options (M963); off inside the chain editor itself (no chain-of-chains)
  triggerClassName?: string; // override the trigger button's size/width (twMerge wins); for prominent full-width pickers
}) {
  const [open, setOpen] = useState(false);
  const [chainCount, setChainCount] = useState<number | null>(null);
  // When the current value is a "@chain" reference, the trigger shows it as a
  // chain (⛓ name) with its model count — fetched lazily so the button label is
  // accurate even before the modal is opened.
  const isRef = isChainRef(value);
  useEffect(() => {
    if (!isRef) return;
    getJSON<ChainsState>("/api/chains")
      .then((c) => setChainCount(c.chains?.[chainName(value)]?.length ?? null))
      .catch(() => setChainCount(null));
  }, [isRef, value]);

  const label = isRef
    ? `⛓ ${chainName(value)}${chainCount != null ? ` (${chainCount})` : ""}`
    : value || activeModel || "default";
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        title="Choose model"
        className={cn(
          "inline-flex h-6 max-w-[16rem] items-center gap-1 rounded border bg-panel px-2 text-xs outline-none transition-colors hover:border-accent focus-visible:border-accent",
          isRef ? "border-accent/60 text-accent" : "border-border",
          triggerClassName,
        )}
      >
        {isRef ? <Waypoints className="size-3 shrink-0" /> : <Cpu className="size-3 shrink-0 text-muted" />}
        <span className="flex-1 truncate text-left">{label}</span>
        <ChevronDown className="size-3 shrink-0 text-muted" />
      </button>
      {open && (
        <ModelModal
          value={value}
          activeModel={activeModel}
          pinned={pinned}
          allowChains={allowChains}
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
  pinned,
  allowChains,
  onClose,
  onPick,
}: {
  value: string;
  activeModel?: string;
  pinned?: PinnedModels;
  allowChains: boolean;
  onClose: () => void;
  onPick: (id: string) => void;
}) {
  const [cat, setCat] = useState<ModelCatalog | null>(null);
  const [chains, setChains] = useState<Record<string, string[]>>({});
  const [defaultChain, setDefaultChain] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [q, setQ] = useState("");
  // Default to only providers that have an API key — you can only run those — with
  // an opt-in to reveal the rest (e.g. to pick a model before adding its key).
  const [keyedOnly, setKeyedOnly] = useState(true);
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

  useEffect(() => {
    if (!allowChains) return;
    getJSON<ChainsState>("/api/chains")
      .then((c) => {
        setChains(c.chains || {});
        setDefaultChain(c.default || "");
      })
      .catch(() => {
        /* chains are optional — a daemon without any is fine */
      });
  }, [allowChains]);

  // Named fallback chains lead the list (M963), narrowed by the same search.
  const chainEntries = useMemo(() => {
    const ql = q.trim().toLowerCase();
    return Object.entries(chains)
      .filter(([name, models]) => !ql || name.toLowerCase().includes(ql) || models.some((m) => m.toLowerCase().includes(ql)))
      .sort(([a], [b]) => a.localeCompare(b));
  }, [chains, q]);

  const allGroups = useMemo(() => groupByProvider(filterModels(flattenModels(cat), q)), [cat, q]);
  const keyedGroups = useMemo(() => allGroups.filter((g) => g.credentialed), [allGroups]);
  const groups = keyedOnly ? keyedGroups : allGroups;
  const hiddenCount = allGroups.length - keyedGroups.length;

  // The pinned group (e.g. the chat routing chain) leads the list, in its own
  // order, narrowed by the same search query as the provider groups.
  const pinnedOpts = useMemo(
    () => (pinned?.ids?.length ? pinnedOptions(filterModels(flattenModels(cat), q), pinned.ids) : []),
    [cat, q, pinned],
  );

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

          {allowChains && chainEntries.length > 0 && (
            <div className="mt-1">
              <div className="flex items-center gap-1.5 px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-accent">
                <Waypoints className="size-3" />
                Fallback chains
                <span className="text-muted/60">· {chainEntries.length}</span>
              </div>
              {chainEntries.map(([name, models]) => (
                <ChainRowOption
                  key={`chain/${name}`}
                  name={name}
                  models={models}
                  isDefault={name === defaultChain}
                  selected={isChainRef(value) && chainName(value) === name}
                  onClick={() => onPick(chainRef(name))}
                />
              ))}
            </div>
          )}

          {pinnedOpts.length > 0 && (
            <div className="mt-1">
              <div className="flex items-center gap-1.5 px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-accent">
                <Route className="size-3" />
                {pinned!.label}
                <span className="text-muted/60">· {pinnedOpts.length}</span>
              </div>
              {pinnedOpts.map((m, i) => (
                <ModelRow key={`pinned/${m.id}-${i}`} m={m} selected={m.id === value} onClick={() => onPick(m.id)} />
              ))}
            </div>
          )}

          {err ? (
            <div className="px-3 py-6 text-center text-xs text-bad">{err}</div>
          ) : !cat ? (
            <div className="px-3 py-6 text-center text-xs text-muted">loading catalog…</div>
          ) : groups.length === 0 ? (
            keyedOnly && hiddenCount > 0 ? (
              <div className="px-3 py-6 text-center text-xs text-muted">
                No providers have an API key yet.
                <br />
                Add one in <span className="text-foreground/80">Models</span>, or{" "}
                <button onClick={() => setKeyedOnly(false)} className="text-accent underline-offset-2 hover:underline">
                  show all {hiddenCount} providers
                </button>
                .
              </div>
            ) : (
              <div className="px-3 py-6 text-center text-xs text-muted">no models match “{q}”</div>
            )
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

        {/* Footer: keyed-only toggle. Only providers with a key can actually run,
            so the picker defaults to those; reveal the rest on demand. */}
        {cat && (keyedOnly ? hiddenCount > 0 : true) && (
          <div className="flex items-center justify-between border-t border-border px-3 py-1.5 text-[10px] text-muted">
            <span className="inline-flex items-center gap-1">
              <KeyRound className="size-3 text-good" />
              {keyedOnly ? "Keyed providers only" : "All providers"}
            </span>
            <button
              onClick={() => setKeyedOnly((v) => !v)}
              className="text-accent transition-colors hover:text-accent/80"
            >
              {keyedOnly ? `Show all (${hiddenCount} more)` : "Show keyed only"}
            </button>
          </div>
        )}
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

function ChainRowOption({
  name,
  models,
  isDefault,
  selected,
  onClick,
}: {
  name: string;
  models: string[];
  isDefault: boolean;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-panel",
        selected && "bg-accent/10",
      )}
    >
      <Check className={cn("size-4 shrink-0", selected ? "text-accent" : "text-transparent")} />
      <Waypoints className="size-3.5 shrink-0 text-accent" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-sm font-medium">{name}</span>
          {isDefault && <span className="rounded bg-accent/15 px-1 text-[9px] font-medium uppercase text-accent">default</span>}
        </div>
        <div className="truncate font-mono text-[10px] text-muted">{models.join(" → ") || "empty"}</div>
      </div>
      <span className="shrink-0 text-[10px] text-muted">
        {models.length} model{models.length === 1 ? "" : "s"}
      </span>
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
