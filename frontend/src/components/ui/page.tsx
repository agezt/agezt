import * as React from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/ui/page-header";

// Page — the single scroll-safe, responsive scaffold every view sits on. It
// replaces the ~28 ad-hoc `flex h-full min-h-0` fixed shells that collapsed
// their inner panes to 0px (and killed page scrolling) on short screens.
//
// Two modes:
//   • scroll (default) — document flow. Root is `min-h-full`, so the view fills
//     at least the viewport and grows past it when content is tall; the app's
//     single <main overflow-auto> then scrolls the whole page. This is what
//     almost every view wants.
//   • fill — a bounded fixed shell for genuine full-height apps (chat stream,
//     graph canvas, kanban board) whose own inner regions scroll. Root is
//     `h-full min-h-0`.
//
// Smart width keeps long form/config/detail pages in a readable column while
// letting tables, boards, and canvases run full-bleed:
//   • readable (default) — centered, capped at max-w-5xl
//   • wide — centered, capped very wide (dense dashboards)
//   • full — edge to edge (tables, boards, canvases)
//
// The `.view-enter` entrance animation lives on App's keyed wrapper, so Page
// does not re-declare it.
export type PageWidth = "readable" | "wide" | "full";
export type PageMode = "scroll" | "fill";

const WIDTH_CLASS: Record<PageWidth, string> = {
  readable: "mx-auto w-full max-w-5xl",
  wide: "mx-auto w-full max-w-[110rem]",
  full: "w-full",
};

export function Page({
  icon,
  title,
  description,
  actions,
  width = "readable",
  mode = "scroll",
  className,
  headerClassName,
  children,
}: {
  icon?: LucideIcon;
  title?: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  width?: PageWidth;
  mode?: PageMode;
  className?: string;
  headerClassName?: string;
  /** When false, omit the PageHeader entirely (view renders its own top). */
  children?: React.ReactNode;
}) {
  const root =
    mode === "fill"
      ? "flex h-full min-h-0 flex-col gap-3"
      : "flex min-h-full flex-col gap-3";
  const hasHeader =
    title !== undefined || icon !== undefined || actions !== undefined || description !== undefined;
  return (
    <div className={cn(root, WIDTH_CLASS[width], className)}>
      {hasHeader && (
        <PageHeader
          icon={icon}
          title={title}
          description={description}
          actions={actions}
          className={headerClassName}
        />
      )}
      {mode === "fill" ? (
        // In fill mode the body owns the remaining height so inner panes can
        // scroll; callers give their content `min-h-0 flex-1 overflow-auto`.
        <div className="flex min-h-0 flex-1 flex-col gap-3">{children}</div>
      ) : (
        children
      )}
    </div>
  );
}
