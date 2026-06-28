import { Waypoints } from "lucide-react";
import { cn } from "@/lib/utils";
import { isChainRef, chainName } from "@/lib/chains";
import { modelHealth, type ModelCatalog, type ModelHealth } from "@/lib/models";

// ModelChip renders a single model slot consistently wherever chains can appear
// (Routing rows, agent Model tab, …): a plain model id shows with an optional
// health dot; a "@name" chain reference (M963) shows as ⛓ name with its model
// count and the expanded models in the tooltip. One place for the dot + chain
// rendering so the surfaces don't drift.
export function ModelChip({
  id,
  chains,
  cat,
  className,
}: {
  id: string;
  chains?: Record<string, string[]>; // chain registry, for resolving "@name"
  cat?: ModelCatalog | null; // catalog, for the per-model health dot
  className?: string;
}) {
  if (isChainRef(id)) {
    const name = chainName(id);
    const models = chains?.[name];
    const known = models !== undefined;
    return (
      <span
        className={cn(
          "inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-xs",
          known ? "bg-accent/15 text-accent" : "bg-bad/15 text-bad",
          className,
        )}
        title={known ? `Fallback chain → ${models!.join(" → ") || "empty"}` : `@${name} — no such chain (falls through to default)`}
      >
        <Waypoints className="size-2.5" />
        {name}
        {known && <span className="opacity-70">· {models!.length}</span>}
      </span>
    );
  }
  return (
    <span className={cn("inline-flex items-center gap-1 rounded bg-card px-1.5 py-0.5 font-mono text-xs", className)}>
      {cat && <HealthDot status={modelHealth(cat, id)} />}
      {id}
    </span>
  );
}

// HealthDot: green = a keyed provider serves the model, amber = needs an API
// key, red = unknown to the catalog (typo / removed). Exported for reuse.
export function HealthDot({ status }: { status: ModelHealth }) {
  const meta: Record<ModelHealth, { cls: string; title: string }> = {
    ok: { cls: "bg-good", title: "A keyed provider can run this model" },
    nokey: { cls: "bg-warn", title: "No keyed provider — add an API key under Models" },
    unknown: { cls: "bg-bad", title: "Not in the catalog — check the model id" },
  };
  const m = meta[status];
  return <span className={cn("size-1.5 shrink-0 rounded-full", m.cls)} title={m.title} aria-label={`model health: ${status}`} />;
}
