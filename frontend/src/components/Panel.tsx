import type { ReactNode } from "react";
import { RefreshCw, type LucideIcon } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { ErrorText } from "@/components/JsonView";
import { Page, type PageWidth } from "@/components/ui/page";

// Panel is the shared read-view shell: it fetches a /api route, shows a refresh
// button + loading/error states, and hands the data (plus a reload fn, for
// action buttons) to a render prop. Keeps every bespoke view boilerplate-free.
//
// Two header modes (M989): pass `icon` to render the standard scroll-safe `Page`
// scaffold (gradient header + glass card) — this is for Panel-backed TOP-LEVEL
// views so they match every other screen and scroll on short viewports. Without
// `icon` it keeps the compact card header, the right shape for a Panel used as a
// sub-section inside a larger view (e.g. the "Decisions" panel on Policy, which
// has its own page header already).
export function Panel<T = any>({
  title,
  icon,
  description,
  width,
  path,
  params,
  headerExtra,
  children,
}: {
  title: string;
  icon?: LucideIcon;
  description?: ReactNode;
  /** Content width when in page mode (icon set). Defaults to readable. */
  width?: PageWidth;
  path: string;
  params?: Record<string, string>;
  headerExtra?: ReactNode;
  children: (data: T, reload: () => void) => ReactNode;
}) {
  const { data, error, loading, reload } = usePanel<T>(path, params);
  const body = (
    <CardBody className="space-y-3">
      {error ? (
        <ErrorText>{error}</ErrorText>
      ) : data != null ? (
        children(data, reload)
      ) : (
        <SkeletonList count={3} lines={2} />
      )}
    </CardBody>
  );
  const refresh = (
    <Button variant="ghost" size="icon" onClick={reload} title="Refresh">
      <RefreshCw className={loading ? "animate-spin" : ""} />
    </Button>
  );

  // Page mode: gradient PageHeader above a glass card body. Uses the scroll-safe
  // Page scaffold so Panel-backed top-level views scroll on short screens instead
  // of clipping (the fixed-shell root here used to collapse the card to 0 height).
  if (icon) {
    return (
      <Page
        icon={icon}
        title={title}
        description={description}
        width={width}
        actions={
          <>
            {headerExtra}
            {refresh}
          </>
        }
      >
        <Card glass>{body}</Card>
      </Page>
    );
  }

  // Sub-panel mode: compact card header (unchanged).
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {headerExtra}
        <span className="ml-auto">{refresh}</span>
      </CardHeader>
      {body}
    </Card>
  );
}

// Stats renders a compact label/value metric grid.
export function Stats({ pairs }: { pairs: [string, ReactNode][] }) {
  return (
    <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm">
      {pairs.map(([k, v], i) => (
        <div key={i} className="flex flex-col">
          <span className="text-xs uppercase tracking-normal text-muted">{k}</span>
          <span className="tabular-nums">{v}</span>
        </div>
      ))}
    </div>
  );
}

// Row is a single list line: a leading chip/badge, a label, and trailing meta.
export function Row({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2 border-b border-border/50 py-1.5">{children}</div>;
}

export function Count({ children }: { children: ReactNode }) {
  return <div className="text-xs text-muted">{children}</div>;
}
