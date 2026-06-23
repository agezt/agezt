import * as React from "react";
import type { ReactNode } from "react";
import { ChevronDown, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

// CollapsibleSection — a widget-level accordion: icon badge, bold title,
// optional badge/count, an actions slot, and an animated body. Collapses to
// just the header so users can hide widgets they don't need. Uses the CSS
// grid 0fr→1fr height trick (no JS measurement), is reduced-motion aware,
// and keyboard accessible (aria-expanded/controls).
export function CollapsibleSection({
  icon: Icon,
  title,
  description,
  children,
  actions,
  count,
  defaultOpen = true,
  open: controlledOpen,
  onOpenChange,
  className,
  headerClassName,
  tone = "accent",
}: {
  icon?: LucideIcon;
  /** Short widget title — shown as the collapse trigger. */
  title: string;
  /** Optional one-line context under the title (hidden when collapsed). */
  description?: string;
  children: ReactNode;
  /** Right-aligned action buttons shown in the header. */
  actions?: ReactNode;
  /** Optional count badge next to the title. */
  count?: number | string;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
  headerClassName?: string;
  /** Accent tone for the icon badge — matches section hues in App.tsx */
  tone?: "accent" | "good" | "warn" | "bad" | "muted";
}) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultOpen);
  const isOpen = controlledOpen ?? uncontrolled;
  const regionId = React.useId();

  function toggle() {
    const next = !isOpen;
    if (controlledOpen === undefined) setUncontrolled(next);
    onOpenChange?.(next);
  }

  const toneCls = {
    accent: "from-accent/20 to-accent2/10 ring-accent/30 text-accent",
    good: "from-good/20 to-good/10 ring-good/30 text-good",
    warn: "from-warn/20 to-warn/10 ring-warn/30 text-warn",
    bad: "from-bad/20 to-bad/10 ring-bad/30 text-bad",
    muted: "from-panel to-panel ring-border text-muted",
  }[tone];

  return (
    <div className={cn("rounded-xl border border-border bg-card shadow-e1", className)}>
      {/* Header / collapse trigger */}
      <button
        type="button"
        onClick={toggle}
        aria-expanded={isOpen}
        aria-controls={regionId}
        className={cn(
          "flex w-full items-center gap-2.5 rounded-xl px-3 py-2.5 text-left",
          "hover:bg-panel/60 transition-colors",
          headerClassName,
        )}
      >
        {/* Icon badge */}
        {Icon ? (
          <span
            className={cn(
              "grid size-8 shrink-0 place-items-center rounded-lg bg-gradient-to-br ring-1 ring-inset",
              toneCls,
            )}
          >
            <Icon className="size-4" />
          </span>
        ) : null}

        {/* Title + optional description */}
        <span className="min-w-0 flex-1">
          <span className="text-sm font-semibold text-foreground">{title}</span>
          {description && isOpen ? (
            <span className="ml-2 text-xs text-muted">{description}</span>
          ) : null}
        </span>

        {/* Count badge */}
        {count !== undefined ? (
          <span className="inline-flex min-w-5 items-center justify-center rounded-full bg-panel px-1.5 py-0.5 text-xs font-semibold tabular-nums text-muted">
            {count}
          </span>
        ) : null}

        {/* Actions slot — stop propagation so clicking buttons doesn't collapse */}
        {actions ? (
          <span
            className="flex items-center gap-1"
            onClick={(e) => { e.stopPropagation(); }}
            onKeyDown={(e) => { e.stopPropagation(); }}
          >
            {actions}
          </span>
        ) : null}

        {/* Collapse chevron */}
        <ChevronDown
          className={cn(
            "size-4 shrink-0 text-muted transition-transform duration-200",
            "motion-reduce:transition-none",
            isOpen ? "rotate-0" : "-rotate-90",
          )}
          aria-hidden
        />
      </button>

      {/* Animated body */}
      <div
        className={cn(
          "grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none",
          isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        <div id={regionId} className="overflow-hidden" aria-hidden={!isOpen}>
          <div className="border-t border-border/60 px-3 py-3">{children}</div>
        </div>
      </div>
    </div>
  );
}
