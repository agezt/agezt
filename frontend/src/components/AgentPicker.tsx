import { useEffect, useRef, useState } from "react";
import { Users, Check } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";

interface AgentOption {
  slug: string;
  name?: string;
  model?: string;
  description?: string;
  enabled?: boolean;
}

// AgentPicker selects the conversation's named agent (M789): the thread runs
// AS the picked roster identity — its soul, model chain, memory scope, and
// budget apply (M783). "" = the daemon's default identity. A compact trigger +
// dropdown (the roster is small; no search needed — the Roster view has one).
export function AgentPicker({
  value,
  onChange,
}: {
  value: string; // "" = default identity; else a roster slug
  onChange: (slug: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [agents, setAgents] = useState<AgentOption[] | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open || agents !== null) return;
    getJSON<{ profiles?: AgentOption[] }>("/api/agents")
      .then((d) => setAgents((d.profiles || []).filter((p) => p.enabled)))
      .catch(() => setAgents([]));
  }, [open, agents]);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label="Pick conversation agent"
        className={cn(
          "inline-flex items-center gap-1 rounded-md border border-border px-1.5 py-0.5 text-xs transition-colors",
          value ? "border-accent/40 text-accent" : "text-muted hover:text-foreground",
        )}
      >
        <Users className="h-3 w-3" />
        {value || "agent"}
      </button>
      {open && (
        <div className="absolute bottom-full left-0 z-20 mb-1 w-64 rounded-lg border border-border bg-card p-1 shadow-lg">
          <Option
            label="default identity"
            hint="the daemon's own persona and model"
            selected={value === ""}
            onPick={() => {
              onChange("");
              setOpen(false);
            }}
          />
          {agents === null && <div className="px-2 py-1.5 text-xs text-muted">loading…</div>}
          {agents !== null && agents.length === 0 && (
            <div className="px-2 py-1.5 text-xs text-muted">
              no enabled agents — create one in the Roster view
            </div>
          )}
          {(agents || []).map((a) => (
            <Option
              key={a.slug}
              label={a.slug}
              hint={[a.name !== a.slug ? a.name : "", a.model || "", a.description || ""]
                .filter(Boolean)
                .join(" · ")}
              selected={value === a.slug}
              onPick={() => {
                onChange(a.slug);
                setOpen(false);
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function Option({
  label,
  hint,
  selected,
  onPick,
}: {
  label: string;
  hint?: string;
  selected: boolean;
  onPick: () => void;
}) {
  return (
    <button
      onClick={onPick}
      aria-label={`Use agent ${label}`}
      className={cn(
        "flex w-full items-start gap-1.5 rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-panel",
        selected ? "text-accent" : "text-foreground",
      )}
    >
      <Check className={cn("mt-0.5 h-3 w-3 shrink-0", selected ? "opacity-100" : "opacity-0")} />
      <span className="min-w-0">
        <span className="block font-medium">{label}</span>
        {hint && <span className="block truncate text-muted">{hint}</span>}
      </span>
    </button>
  );
}
