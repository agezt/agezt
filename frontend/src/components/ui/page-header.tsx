import * as React from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

// PageHeader (M978) — the consistent top of every view: a gradient-ringed icon
// badge, a strong title (gradient on the dark command-center theme), a one-line
// description of what the page is for, and a right-aligned actions slot. Gives
// all ~30 screens the same modern, self-explaining header instead of each
// rolling its own ad-hoc title row.
export function PageHeader({
  icon: Icon,
  title,
  description,
  actions,
  className,
}: {
  icon?: LucideIcon;
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-wrap items-center gap-3", className)}>
      {Icon && (
        <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-accent/25 to-accent2/20 text-accent ring-1 ring-inset ring-accent/30">
          <Icon className="size-5" />
        </span>
      )}
      <div className="min-w-0">
        <h2 className="text-gradient text-base font-bold leading-tight tracking-tight">{title}</h2>
        {description && <p className="mt-0.5 text-xs text-muted">{description}</p>}
      </div>
      {actions && <div className="ml-auto flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}
