import type { ReactNode } from "react";
import { Bot, ShieldCheck, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { agentIdentityKind, type AgentProfile } from "./shared";

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
