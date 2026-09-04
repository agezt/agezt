import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { tonePlate, type Tone } from "@/lib/tone";

/**
 * SectionPanel — one titled block on a page: an icon plate tinted by state, a
 * title, a one-line status, optional right-aligned actions, and the body.
 *
 * This shape had been re-implemented twenty-one times under twenty-one names —
 * HealthPanel, StatusPanel, StoragePanel, PolicyPanel, InsightPanel,
 * DashboardPanel, ProviderPanel, MemoryPanel, ForgePanel, NodePanel, and so on.
 * They differed only in padding, border opacity and which tones they happened
 * to support, which is precisely the kind of near-miss that makes a console
 * feel like a pile of pages rather than one product.
 *
 * Use it for any "here is a labelled region with a state" block. For a plain
 * container with no status, `ui/card` is still the right primitive.
 */
export function SectionPanel({
  icon: Icon,
  title,
  status,
  tone = "muted",
  actions,
  children,
  className,
  bodyClassName,
  ariaLabel,
  testId,
}: {
  icon?: LucideIcon;
  title: ReactNode;
  /** One line under the title: the panel's current state, in words. */
  status?: ReactNode;
  tone?: Tone;
  /** Right-aligned controls in the header row. */
  actions?: ReactNode;
  children?: ReactNode;
  className?: string;
  bodyClassName?: string;
  /** Overrides the accessible name when the title alone is ambiguous. */
  ariaLabel?: string;
  /** data-testid for tests that need to scope queries to this panel. */
  testId?: string;
}) {
  return (
    <section aria-label={ariaLabel} data-testid={testId} className={cn("rounded-xl border border-border bg-card/70 p-3 shadow-e1", className)}>
      <div className={cn("flex items-center gap-2", children && "mb-2")}>
        {Icon && (
          <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg border", tonePlate[tone])}>
            <Icon className="size-4" />
          </span>
        )}
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-semibold">{title}</h3>
          {status !== undefined && status !== null && status !== "" && (
            <div className="truncate text-xs text-muted">{status}</div>
          )}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
      </div>
      {children && <div className={bodyClassName}>{children}</div>}
    </section>
  );
}
