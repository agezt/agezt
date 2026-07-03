import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

// Workspace — a 3-pane shell for the wide management surfaces (Files, Artifacts,
// Agents). Three independent scroll regions, persistent headers, configurable
// widths. The body owns the remaining height in fill-mode pages, so inner panes
// can scroll without killing the page.
//
// Width tokens are deliberately small (240/320/420) so the design stays
// scannable on a laptop. Splitter-resizable columns are a follow-up once we
// have a real reason to tune them per-operator.

const WIDTH_CLASS = {
  left: "w-60 shrink-0",
  center: "w-[28rem] shrink-0",
  right: "flex-1 min-w-0",
} as const;

export function Workspace({
  left,
  center,
  right,
  className,
}: {
  left?: ReactNode;
  center?: ReactNode;
  right: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex h-full min-h-0 flex-1 gap-3", className)}>
      {left && (
        <aside className={cn(WIDTH_CLASS.left, "flex min-h-0 flex-col gap-2")}>
          <Pane>{left}</Pane>
        </aside>
      )}
      {center !== undefined && (
        <section className={cn(WIDTH_CLASS.center, "flex min-h-0 flex-col gap-2")}>
          <Pane>{center}</Pane>
        </section>
      )}
      <section className={cn(WIDTH_CLASS.right, "flex min-h-0 flex-col gap-2")}>
        <Pane>{right}</Pane>
      </section>
    </div>
  );
}

// Pane is the rounded card that hosts one workspace column. It is intentionally
// borderless here (the visual is owned by the column's own content); the only
// shared responsibility is "scrolling only inside this region".
function Pane({ children }: { children: ReactNode }) {
  return <div className="flex min-h-0 flex-1 flex-col overflow-hidden">{children}</div>;
}

// WorkspaceColumn is the standard "title row + scrollable body" used inside each
// pane. Keeps the height maths honest: header is fixed, body fills the rest.
export function WorkspaceColumn({
  title,
  actions,
  children,
  className,
  bodyClassName,
}: {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
}) {
  return (
    <div className={cn("flex min-h-0 flex-1 flex-col rounded-xl border border-border bg-card/30", className)}>
      {(title !== undefined || actions !== undefined) && (
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs font-semibold uppercase tracking-normal text-muted">
          <span className="min-w-0 flex-1 truncate">{title}</span>
          {actions}
        </div>
      )}
      <div className={cn("min-h-0 flex-1 overflow-auto", bodyClassName)}>{children}</div>
    </div>
  );
}
