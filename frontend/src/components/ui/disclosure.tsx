import * as React from "react";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

// Disclosure — the console's progressive-disclosure primitive. Lead with a humane
// summary; fold everything power-user/diagnostic underneath until the operator
// asks for it ("don't show me things I don't need unless I ask for them"). A chevron
// button toggles an animated region (CSS grid 0fr↔1fr height trick, so no JS
// measuring), reduced-motion aware, keyboard-accessible (aria-expanded/-controls).
//
// Children stay MOUNTED when collapsed (hidden via overflow, marked aria-hidden)
// so existing vitest queries still find the content and lazy data isn't dropped.
export function Disclosure({
  summary,
  children,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
  className,
  summaryClassName,
  contentClassName,
}: {
  summary: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
  /** Controlled open state. Omit for self-managed (uncontrolled) behavior. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
  summaryClassName?: string;
  contentClassName?: string;
}) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultOpen);
  const open = controlledOpen ?? uncontrolled;
  const regionId = React.useId();
  const toggle = () => {
    const next = !open;
    if (controlledOpen === undefined) setUncontrolled(next);
    onOpenChange?.(next);
  };
  return (
    <div className={cn("min-w-0", className)}>
      <button
        type="button"
        onClick={toggle}
        aria-expanded={open}
        aria-controls={regionId}
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm font-medium text-foreground/90 transition-colors hover:bg-foreground/5",
          summaryClassName,
        )}
      >
        <ChevronRight
          className={cn(
            "h-3.5 w-3.5 shrink-0 text-muted transition-transform duration-200 motion-reduce:transition-none",
            open && "rotate-90",
          )}
          aria-hidden
        />
        <span className="min-w-0 flex-1">{summary}</span>
      </button>
      <div
        className={cn(
          "grid transition-[grid-template-rows] duration-200 ease-out motion-reduce:transition-none",
          open ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        {/* Children stay mounted + accessible when collapsed (visually clipped by
            the 0fr grid row + overflow). We deliberately do NOT set aria-hidden:
            it would drop the folded controls from the accessibility tree (and from
            role-based test/queries) even though they're one click away. */}
        <div id={regionId} className={cn("overflow-hidden", contentClassName)}>
          <div className="pt-1">{children}</div>
        </div>
      </div>
    </div>
  );
}

// Advanced — a Disclosure preset for "everything I don't need until I ask": power
// settings, diagnostics, contracts. Collapsed by default, labeled, low-key.
export function Advanced({
  children,
  label = "Advanced",
  defaultOpen = false,
  className,
}: {
  children: React.ReactNode;
  label?: string;
  defaultOpen?: boolean;
  className?: string;
}) {
  return (
    <Disclosure
      defaultOpen={defaultOpen}
      className={className}
      summary={<span className="text-xs font-semibold uppercase tracking-wider text-muted">{label}</span>}
    >
      {children}
    </Disclosure>
  );
}
