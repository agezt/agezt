import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

// EmptyState is the friendly "there's nothing here yet" panel: a dashed card with
// a soft icon, a title, and an optional hint (and optional action), used instead
// of a bare "no X yet" line so an empty view still feels designed and tells the
// operator how to fill it.
export function EmptyState({
  icon: Icon,
  title,
  hint,
  action,
}: {
  icon: LucideIcon;
  title: string;
  hint?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div
      className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-16 text-center"
      role="status"
    >
      {/* Same gradient-badge family as PageHeader, so an empty view still
          speaks the console's visual language instead of going grey. */}
      <span className="grid size-12 place-items-center rounded-xl bg-gradient-to-br from-accent/15 to-accent2/10 text-accent/70 ring-1 ring-inset ring-accent/15">
        <Icon className="size-6" />
      </span>
      <div className="max-w-sm px-4">
        <p className="font-display text-sm font-semibold">{title}</p>
        {hint && <p className="mt-1 text-xs leading-relaxed text-muted">{hint}</p>}
      </div>
      {action}
    </div>
  );
}
