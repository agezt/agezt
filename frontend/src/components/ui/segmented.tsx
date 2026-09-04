import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Segmented — an exclusive choice between a few modes, as a pill row.
 *
 * Views had been hand-rolling this: `rounded-full border px-2.5 py-0.5
 * text-[11px]` with an inline conditional accent class, copied ten times with
 * drifting padding and hover states. It is a real control with real semantics
 * (a radiogroup), so it gets a real component — including the keyboard and
 * screen-reader behaviour every hand-rolled copy was missing.
 *
 * Not the same thing as `ui/tab-nav`: TabNav switches PANELS within a page and
 * owns their content. Segmented switches how the SAME content is shown.
 */
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  ariaLabel,
  className,
}: {
  value: T;
  onChange: (value: T) => void;
  options: { value: T; label: string; icon?: LucideIcon; title?: string; count?: number | string }[];
  ariaLabel: string;
  className?: string;
}) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className={cn("flex items-center gap-1", className)}>
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            role="radio"
            aria-checked={on}
            onClick={() => onChange(o.value)}
            title={o.title || o.label}
            className={cn(
              "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
              "outline-none focus-visible:ring-2 focus-visible:ring-accent/50",
              on ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:text-foreground",
            )}
          >
            {o.icon && <o.icon className="size-3" aria-hidden />}
            {o.label}
            {o.count !== undefined && (
              <span className={cn("rounded-full px-1 text-xs tabular-nums", on ? "bg-accent/20" : "bg-panel")}>
                {o.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}

/**
 * ToggleChip — one on/off filter ("Show run outputs (12)"). Same pill language
 * as Segmented so a page's controls read as one row, but it is a switch, not a
 * choice between alternatives.
 */
export function ToggleChip({
  on,
  onToggle,
  children,
  title,
  className,
}: {
  on: boolean;
  onToggle: () => void;
  children: React.ReactNode;
  title?: string;
  className?: string;
}) {
  return (
    <button
      role="switch"
      aria-checked={on}
      onClick={onToggle}
      title={title}
      className={cn(
        "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
        "outline-none focus-visible:ring-2 focus-visible:ring-accent/50",
        on ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:text-foreground",
        className,
      )}
    >
      {children}
    </button>
  );
}

/**
 * FilterToken — a filter that is currently ON and can be cleared by clicking
 * it ("correlation: 8f3c1a… ×"). The third member of the pill family: not a
 * choice between alternatives (Segmented) and not an on/off switch the
 * operator will flip back (ToggleChip), but a narrowing they applied and now
 * want out of the way.
 */
export function FilterToken({
  children,
  onClear,
  title,
  className,
}: {
  children: React.ReactNode;
  onClear: () => void;
  title?: string;
  className?: string;
}) {
  return (
    <button
      onClick={onClear}
      title={title || "Clear this filter"}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border border-accent px-2.5 py-0.5 text-[11px] text-accent",
        "outline-none transition-colors hover:bg-accent/10 focus-visible:ring-2 focus-visible:ring-accent/50",
        className,
      )}
    >
      {children}
    </button>
  );
}
