// Fleet.tsx (M952) — the visual layer for the unified agent census (see
// lib/fleet.ts). FleetCard is one entity at a glance; FleetDetail is the drill-in
// that answers, loudly, "how does this thing actually run?". Every kind of agent
// — roster identity, standing order, schedule, workflow, system engine — wears
// the same card shape so the page reads as one army roster, not five disjoint
// lists. Trigger info is the hero on both surfaces.
import {
  Users,
  Anchor,
  CalendarClock,
  GitFork,
  Cpu,
  Zap,
  Timer,
  Webhook,
  Infinity as InfinityIcon,
  MousePointerClick,
  Share2,
  Clock,
  Coins,
  X,
  ArrowUpRight,
  Activity,
  Flame,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";
import type { FleetEntity, FleetKind, FleetState, TriggerMode } from "@/lib/fleet";
import { AgentAvatar } from "@/components/AgentAvatar";
import { Button } from "@/components/ui/button";
import { cn, clip, fmtDateTime, fmtAgo } from "@/lib/utils";
import { money } from "@/lib/format";

// Per-archetype identity: a label, an icon, and a hue so each kind has its own
// colour (the owner wants the console colourful, not a grid of grey).
export const KIND_META: Record<FleetKind, { label: string; icon: LucideIcon; hue: number | null }> = {
  roster: { label: "Roster agent", icon: Users, hue: 210 },
  standing: { label: "Standing order", icon: Anchor, hue: 150 },
  schedule: { label: "Schedule", icon: CalendarClock, hue: 35 },
  workflow: { label: "Workflow", icon: GitFork, hue: 280 },
  system: { label: "System engine", icon: Cpu, hue: null }, // neutral
};

const TRIGGER_ICON: Record<TriggerMode, LucideIcon> = {
  cron: CalendarClock,
  event: Zap,
  webhook: Webhook,
  cadence: Timer,
  continuous: InfinityIcon,
  manual: MousePointerClick,
  delegation: Share2,
};

const STATE_LABEL: Record<FleetState, string> = {
  running: "running",
  armed: "armed",
  manual: "manual",
  paused: "paused",
  retired: "retired",
};

function kindColor(kind: FleetKind): string | undefined {
  const hue = KIND_META[kind].hue;
  return hue == null ? undefined : `hsl(${hue} 60% 55%)`;
}

function StatePill({ state }: { state: FleetState }) {
  const cls =
    state === "running"
      ? "text-accent"
      : state === "armed"
        ? "text-good"
        : state === "paused" || state === "retired"
          ? "text-muted"
          : "text-foreground/70";
  return (
    <span className={cn("inline-flex items-center gap-1 text-[10px] uppercase tracking-wider", cls)}>
      {state === "running" && <span className="size-1.5 animate-pulse rounded-full bg-accent" />}
      {STATE_LABEL[state]}
    </span>
  );
}

// TriggerChip — the answer to "what makes this run", compact for cards.
export function TriggerChip({ mode, label }: { mode: TriggerMode; label: string }) {
  const Icon = TRIGGER_ICON[mode];
  return (
    <span
      title={`${mode} · ${label}`}
      className="inline-flex max-w-full items-center gap-1 rounded-md border border-border bg-panel/50 px-1.5 py-0.5 text-[10px] text-foreground/80"
    >
      <Icon className="size-2.5 shrink-0 text-muted" />
      <span className="truncate font-mono">{label}</span>
    </span>
  );
}

// FleetCard — one agent/automation, whatever its kind, at a glance: identity,
// type, state, and how it gets triggered.
export function FleetCard({ e, onOpen }: { e: FleetEntity; onOpen: () => void }) {
  const Kind = KIND_META[e.kind].icon;
  const color = kindColor(e.kind);
  return (
    <button
      onClick={onOpen}
      className={cn(
        "group flex flex-col gap-2 rounded-lg border bg-card p-3 text-left transition-colors hover:border-accent",
        e.running ? "border-accent/60" : "border-border",
        (e.state === "paused" || e.state === "retired") && "opacity-60",
      )}
    >
      <div className="flex items-center gap-2">
        <AgentAvatar
          slug={e.slug}
          name={e.name}
          size={26}
          status={e.running ? "running" : e.retired ? "retired" : undefined}
        />
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-semibold" title={e.name}>
            {e.name}
          </div>
          <div className="flex items-center gap-1 text-[10px] text-muted" style={color ? { color } : undefined}>
            <Kind className="size-2.5" /> {KIND_META[e.kind].label}
            {e.system && (
              <span className="ml-1 inline-flex items-center gap-0.5 rounded-full bg-accent/15 px-1.5 text-[9px] font-medium text-accent" title="Shipped internal guardian — part of the daemon's self-healing fleet">
                <ShieldCheck className="size-2.5" /> guardian
              </span>
            )}
          </div>
        </div>
        <StatePill state={e.state} />
      </div>

      {e.description && (
        <div className="line-clamp-2 text-[11px] text-muted" title={e.description}>
          {clip(e.description, 140)}
        </div>
      )}

      <div className="flex flex-wrap gap-1">
        {e.triggers.map((t, i) => (
          <TriggerChip key={`${t.mode}-${i}`} mode={t.mode} label={t.label} />
        ))}
      </div>

      <div className="flex items-center gap-2 text-[10px] text-muted">
        {e.model && (
          <span className="truncate font-mono" title={e.model}>
            {e.model}
          </span>
        )}
        {typeof e.fires === "number" && e.fires > 0 && (
          <span className="inline-flex items-center gap-1">
            <Flame className="size-2.5" /> {e.fires}
          </span>
        )}
        <span className="ml-auto inline-flex shrink-0 items-center gap-1">
          {e.nextRunMs ? (
            <>
              <CalendarClock className="size-2.5" /> {fmtDateTime(e.nextRunMs)}
            </>
          ) : e.lastRunMs ? (
            <>
              <Clock className="size-2.5" /> {fmtAgo(e.lastRunMs)}
            </>
          ) : null}
        </span>
      </div>
    </button>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex gap-2 text-[11px]">
      <span className="w-24 shrink-0 text-muted">{label}</span>
      <span className="min-w-0 flex-1 break-words">{value}</span>
    </div>
  );
}

// FleetDetail — the drill-in. Leads with a plain-language "How does this run?"
// block (the whole point of the page), then kind-specific config, then a deep
// link to the page that manages this kind.
export function FleetDetail({
  e,
  onClose,
  onManage,
  onLive,
}: {
  e: FleetEntity;
  onClose: () => void;
  onManage: (view: string) => void;
  onLive?: () => void;
}) {
  const Kind = KIND_META[e.kind].icon;
  const color = kindColor(e.kind);
  const raw = e.raw as Record<string, unknown> | null;
  const str = (k: string) => (raw && typeof raw[k] === "string" ? (raw[k] as string) : undefined);
  const num = (k: string) => (raw && typeof raw[k] === "number" ? (raw[k] as number) : undefined);

  return (
    <aside className="flex min-h-0 flex-col gap-3 overflow-auto rounded-lg border border-border bg-card p-3 lg:w-96 lg:shrink-0">
      <div className="flex items-start gap-2">
        <AgentAvatar slug={e.slug} name={e.name} size={36} status={e.running ? "running" : e.retired ? "retired" : undefined} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold" title={e.name}>
            {e.name}
          </div>
          <div className="flex items-center gap-1.5 text-[10px]" style={color ? { color } : undefined}>
            <Kind className="size-3" /> {KIND_META[e.kind].label}
            <StatePill state={e.state} />
          </div>
        </div>
        <button
          onClick={onClose}
          className="shrink-0 rounded-md border border-border p-1 text-muted hover:border-accent hover:text-foreground"
          title="Close"
        >
          <X className="size-3.5" />
        </button>
      </div>

      {e.description && <p className="text-[11px] text-muted">{e.description}</p>}

      {/* The hero: how this comes alive. */}
      <div className="rounded-lg border border-accent/40 bg-accent/5 p-2.5">
        <div className="mb-1.5 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-accent">
          <Activity className="size-3" /> How does this run?
        </div>
        <div className="space-y-2">
          {e.triggers.map((t, i) => {
            const Icon = TRIGGER_ICON[t.mode];
            return (
              <div key={`${t.mode}-${i}`} className="flex gap-2">
                <Icon className="mt-0.5 size-3.5 shrink-0 text-accent" />
                <div className="min-w-0">
                  <div className="text-[11px] font-medium">
                    {t.mode}
                    {t.label && t.label !== t.mode ? <span className="ml-1 font-mono text-muted">· {t.label}</span> : null}
                    {t.via ? <span className="ml-1 text-muted">via {t.via}</span> : null}
                  </div>
                  <div className="text-[11px] text-muted">{t.needs}</div>
                </div>
              </div>
            );
          })}
          {e.nextRunMs && (
            <div className="text-[11px] text-muted">
              Next fire: <span className="font-mono text-foreground/80">{fmtDateTime(e.nextRunMs)}</span>
            </div>
          )}
          {e.kind === "workflow" && (e.raw as Record<string, unknown>)?.["trigger_kind"] === "webhook" && (
            <div className="text-[11px] text-muted">
              Endpoint: <span className="font-mono text-foreground/80">POST /api/workflows/webhook</span> — exact URL &amp; secret
              live in Flow Studio.
            </div>
          )}
        </div>
      </div>

      {/* Kind-specific config. */}
      <div className="space-y-1.5 rounded-lg border border-border bg-panel/30 p-2.5">
        {e.kind === "roster" && (
          <>
            <Row label="model" value={e.model && <span className="font-mono">{e.model}</span>} />
            <Row label="memory" value={str("memory_scope")} />
            <Row label="workdir" value={str("workdir") && <span className="font-mono">{str("workdir")}</span>} />
            <Row label="max / run" value={num("max_cost_mc") ? money(num("max_cost_mc")) : undefined} />
            <Row label="last run" value={e.lastRunMs ? fmtAgo(e.lastRunMs) : "never"} />
            {str("soul") && (
              <div>
                <div className="mb-1 text-[10px] uppercase tracking-wider text-muted">soul</div>
                <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded-md bg-card p-2 font-mono text-[10px] text-foreground/80">
                  {str("soul")}
                </pre>
              </div>
            )}
          </>
        )}
        {e.kind === "standing" && (
          <>
            <Row label="runs as" value={str("agent") && <span className="font-mono">{str("agent")}</span>} />
            <Row
              label="initiative"
              value={
                raw && raw["initiative"] && typeof (raw["initiative"] as Record<string, unknown>)["mode"] === "string"
                  ? ((raw["initiative"] as Record<string, unknown>)["mode"] as string)
                  : undefined
              }
            />
            <Row label="plan" value={str("plan")} />
          </>
        )}
        {e.kind === "schedule" && (
          <>
            <Row label="cadence" value={str("cadence")} />
            <Row label="mode" value={str("mode") || "interval"} />
            <Row label="source" value={str("source")} />
            <Row label="next fire" value={e.nextRunMs ? fmtDateTime(e.nextRunMs) : "—"} />
            <Row label="fires" value={typeof e.fires === "number" ? String(e.fires) : undefined} />
            <Row label="last" value={str("last_status")} />
            <Row label="intent" value={str("intent")} />
          </>
        )}
        {e.kind === "workflow" && (
          <>
            <Row label="trigger" value={`${str("trigger_kind") || "manual"}${str("trigger_detail") ? ` (${str("trigger_detail")})` : ""}`} />
            <Row label="nodes" value={typeof num("node_count") === "number" ? String(num("node_count")) : undefined} />
          </>
        )}
        {e.kind === "system" && (
          <Row label="status" value={e.running ? "running" : e.state} />
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {e.running && onLive && (
          <Button variant="ghost" size="sm" onClick={onLive}>
            <Activity className="size-3.5" /> View live
          </Button>
        )}
        {e.manageView && (
          <Button variant="ghost" size="sm" onClick={() => onManage(e.manageView)} className="ml-auto">
            Manage <ArrowUpRight className="size-3.5" />
          </Button>
        )}
      </div>
    </aside>
  );
}
