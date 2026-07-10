import type { ReactNode } from "react";
import { Activity, Bot, ShieldCheck, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { agentIdentityKind, type AgentProfile } from "./shared";
import type { AgentCommandStripItem, AgentLifecycleRailStep } from "./passports";

export function RosterSignalPanel({
  icon: Icon,
  title,
  status,
  tone,
  children,
}: {
  icon: LucideIcon;
  title: string;
  status: string;
  tone: "warn" | "good" | "muted";
  children: ReactNode;
}) {
  const toneCls = {
    warn: "border-warn/35 bg-warn/5 text-warn",
    good: "border-good/35 bg-good/5 text-good",
    muted: "border-border bg-panel text-muted",
  }[tone];
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg border", toneCls)}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}

export function ImpactList({
  label,
  count,
  items,
  note,
}: {
  label: string;
  count: number;
  items: string[];
  note?: string;
}) {
  return (
    <div className="min-w-0 rounded-lg border border-border bg-card/65 p-2 text-xs">
      <div className="flex items-center gap-2">
        <span className="font-medium text-foreground">{label}</span>
        <span className="ml-auto rounded-md bg-panel px-1.5 py-0.5 font-mono text-xs text-muted">{count}</span>
      </div>
      {note && <div className="mt-1 text-[11px] text-muted">{note}</div>}
      {items.length > 0 && (
        <ul className="mt-1 max-h-20 space-y-0.5 overflow-auto rounded-md bg-panel/60 px-2 py-1 text-[11px] text-muted">
          {items.slice(0, 8).map((item) => (
            <li key={item} className="truncate" title={item}>
              {item}
            </li>
          ))}
          {items.length > 8 && <li>{items.length - 8} more</li>}
        </ul>
      )}
    </div>
  );
}

export function CascadeOption({
  label,
  count,
  checked,
  onChange,
  items,
  note,
}: {
  label: string;
  count: number;
  checked: boolean;
  onChange: (checked: boolean) => void;
  items: string[];
  note?: string;
}) {
  return (
    <label className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-panel/35 p-2 text-xs">
      <span className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={checked}
          disabled={count === 0}
          onChange={(e) => onChange(e.target.checked)}
          className="size-3.5"
        />
        <span className="font-medium text-foreground">{label}</span>
        <span className="ml-auto rounded-md bg-card px-1.5 py-0.5 font-mono text-xs text-muted">{count}</span>
      </span>
      {note && <span className="text-[11px] text-muted">{note}</span>}
      {items.length > 0 && (
        <ul className="max-h-20 space-y-0.5 overflow-auto rounded-md bg-card/60 px-2 py-1 text-[11px] text-muted">
          {items.slice(0, 8).map((item) => (
            <li key={item} className="truncate" title={item}>
              {item}
            </li>
          ))}
          {items.length > 8 && <li>{items.length - 8} more</li>}
        </ul>
      )}
    </label>
  );
}

export function AgentKindBadge({ profile }: { profile: AgentProfile }) {
  const kind = agentIdentityKind(profile);
  if (kind === "system") {
    return (
      <IdentityPill className="bg-accent/15 text-accent" title="Shipped internal guardian — protected from removal">
        <ShieldCheck className="h-2.5 w-2.5" /> guardian
      </IdentityPill>
    );
  }
  if (kind === "subagent") {
    return (
      <IdentityPill className="bg-good/10 text-good" title="Managed sub-agent — woken by its parent/owner agent">
        <Bot className="h-2.5 w-2.5" /> managed sub-agent
      </IdentityPill>
    );
  }
  return (
    <IdentityPill title="User-created custom agent identity">
      <Bot className="h-2.5 w-2.5" /> custom
    </IdentityPill>
  );
}

export function IdentityPill({ children, className, title }: { children: ReactNode; className?: string; title?: string }) {
  return (
    <span
      title={title}
      className={cn("inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 text-xs font-medium text-muted", className)}
    >
      {children}
    </span>
  );
}

export function RosterCommandStrip({ items, slug }: { items: AgentCommandStripItem[]; slug: string }) {
  return (
    <div className="mt-2 grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3" aria-label={`${slug} command strip`}>
      {items.map((item) => (
        <div
          key={item.label}
          title={item.detail || item.value}
          className={cn(
            "min-w-0 rounded-md border border-border/60 bg-card/60 px-2 py-1.5",
            item.tone === "good" && "border-good/25 bg-good/5",
            item.tone === "bad" && "border-bad/30 bg-bad/5",
            item.tone === "warn" && "border-warn/35 bg-warn/10",
            item.tone === "accent" && "border-accent/30 bg-accent/10",
            item.tone === "muted" && "bg-panel/45",
          )}
        >
          <div className="flex min-w-0 items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full bg-muted/60",
                item.tone === "good" && "bg-good",
                item.tone === "bad" && "bg-bad",
                item.tone === "warn" && "bg-warn",
                item.tone === "accent" && "bg-accent",
              )}
            />
            <span className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{item.label}</span>
          </div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
              item.tone === "good" && "text-good",
              item.tone === "bad" && "text-bad",
              item.tone === "warn" && "text-warn",
              item.tone === "accent" && "text-accent",
              item.tone === "muted" && "text-muted",
            )}
          >
            {item.value}
          </div>
        </div>
      ))}
    </div>
  );
}

export function RosterPassportSection({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="min-w-0 rounded-lg border border-border/60 bg-card/35 p-2">
      <div className="mb-1.5 text-[9px] font-semibold uppercase tracking-normal text-muted/75">{label}</div>
      <div className="grid gap-1.5 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">{children}</div>
    </section>
  );
}

export function RosterPassportCell({
  label,
  value,
  title,
  tone = "muted",
}: {
  label: string;
  value: string;
  title?: string;
  tone?: "good" | "bad" | "warn" | "accent" | "muted";
}) {
  return (
    <div
      title={title || value}
      className={cn(
        "min-w-0 rounded-md border border-border/60 bg-panel/45 px-2 py-1.5",
        tone === "good" && "border-good/25 bg-good/5",
        tone === "bad" && "border-bad/30 bg-bad/5",
        tone === "warn" && "border-warn/35 bg-warn/10",
        tone === "accent" && "border-accent/30 bg-accent/10",
      )}
    >
      <div className="text-[9px] font-semibold uppercase tracking-normal text-muted/80">{label}</div>
      <div
        className={cn(
          "mt-0.5 truncate text-[11px] text-foreground/90",
          tone === "good" && "text-good",
          tone === "bad" && "text-bad",
          tone === "warn" && "text-warn",
          tone === "accent" && "text-accent",
        )}
      >
        {value}
      </div>
    </div>
  );
}

export function RosterLifecycleRail({ steps }: { steps: AgentLifecycleRailStep[] }) {
  return (
    <div className="mt-2 grid gap-1.5 sm:grid-cols-4" aria-label="Agent lifecycle rail">
      {steps.map((step) => (
        <div
          key={step.id}
          title={[step.value, step.detail].filter(Boolean).join(" · ")}
          className={cn(
            "min-w-0 rounded-md border border-border/60 bg-panel/45 px-2 py-1.5",
            step.tone === "good" && "border-good/25 bg-good/5",
            step.tone === "bad" && "border-bad/30 bg-bad/5",
            step.tone === "warn" && "border-warn/35 bg-warn/10",
            step.tone === "accent" && "border-accent/30 bg-accent/10",
          )}
        >
          <div className="flex items-center gap-1.5">
            <span
              className={cn(
                "size-1.5 shrink-0 rounded-full bg-muted/60",
                step.tone === "good" && "bg-good",
                step.tone === "bad" && "bg-bad",
                step.tone === "warn" && "bg-warn",
                step.tone === "accent" && "bg-accent",
              )}
            />
            <span className="truncate text-[9px] font-semibold uppercase tracking-normal text-muted/80">{step.label}</span>
          </div>
          <div
            className={cn(
              "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
              step.tone === "good" && "text-good",
              step.tone === "bad" && "text-bad",
              step.tone === "warn" && "text-warn",
              step.tone === "accent" && "text-accent",
            )}
          >
            {step.value}
          </div>
        </div>
      ))}
    </div>
  );
}

export function RosterNowBand({
  phase,
  detail,
  since,
  last,
}: {
  phase: string;
  detail: string;
  since?: number;
  last?: number;
}) {
  const timing = [
    since ? `since ${new Date(since).toLocaleString()}` : "",
    last ? `last ${new Date(last).toLocaleString()}` : "",
  ].filter(Boolean).join(" · ");
  return (
    <div
      title={[detail, timing].filter(Boolean).join(" · ")}
      className="mt-2 grid min-h-[48px] grid-cols-[auto_1fr] gap-2 rounded-md border border-accent/35 bg-accent/10 px-2 py-1.5"
    >
      <div className="grid size-8 place-items-center rounded-md bg-accent/15 text-accent">
        <Activity className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-[9px] font-semibold uppercase tracking-normal text-accent">Now</span>
          <span className="truncate text-[11px] font-medium text-foreground">{phase}</span>
        </div>
        <div className="mt-0.5 truncate text-[11px] text-muted">{detail}</div>
      </div>
    </div>
  );
}
