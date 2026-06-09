import type { ReactNode } from "react";
import { RefreshCw } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { SkeletonList } from "@/components/ui/skeleton";
import { ErrorText } from "@/components/JsonView";

// Panel is the shared read-view shell: it fetches a /api route, shows a refresh
// button + loading/error states, and hands the data (plus a reload fn, for
// action buttons) to a render prop. Keeps every bespoke view boilerplate-free.
export function Panel<T = any>({
  title,
  path,
  params,
  headerExtra,
  children,
}: {
  title: string;
  path: string;
  params?: Record<string, string>;
  headerExtra?: ReactNode;
  children: (data: T, reload: () => void) => ReactNode;
}) {
  const { data, error, loading, reload } = usePanel<T>(path, params);
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {headerExtra}
        <Button variant="ghost" size="icon" className="ml-auto" onClick={reload} title="Refresh">
          <RefreshCw className={loading ? "animate-spin" : ""} />
        </Button>
      </CardHeader>
      <CardBody className="space-y-3">
        {error ? (
          <ErrorText>{error}</ErrorText>
        ) : data != null ? (
          children(data, reload)
        ) : (
          <SkeletonList count={3} lines={2} />
        )}
      </CardBody>
    </Card>
  );
}

// Stats renders a compact label/value metric grid.
export function Stats({ pairs }: { pairs: [string, ReactNode][] }) {
  return (
    <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm">
      {pairs.map(([k, v], i) => (
        <div key={i} className="flex flex-col">
          <span className="text-xs uppercase tracking-wider text-muted">{k}</span>
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
