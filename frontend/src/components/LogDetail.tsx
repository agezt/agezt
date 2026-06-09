import { useState, type ReactNode } from "react";
import { getJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

// LogDetail is the shared "click for log" drill-down: a toggle that lazily
// fetches a *_log route and renders each entry. Mirrors the old dashboard's
// provider/tool/policy log modals, inline.
export function LogDetail<T = any>({
  label,
  path,
  params,
  extract,
  render,
}: {
  label: string;
  path: string;
  params?: Record<string, string>;
  extract: (d: any) => T[];
  render: (row: T, i: number) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [rows, setRows] = useState<T[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  async function toggle() {
    const next = !open;
    setOpen(next);
    if (next && rows === null) {
      try {
        setRows(extract(await getJSON(path, params)));
      } catch (e) {
        setErr((e as Error).message);
      }
    }
  }

  return (
    <div className="space-y-1">
      <Button variant="ghost" size="sm" onClick={toggle}>
        {open ? "hide log" : label}
      </Button>
      {open &&
        (err ? (
          <ErrorText>{err}</ErrorText>
        ) : rows === null ? (
          <SkeletonList count={2} lines={1} />
        ) : rows.length === 0 ? (
          <Muted>no entries</Muted>
        ) : (
          <div className="max-h-64 overflow-auto rounded-md border border-border bg-panel p-2 text-xs">
            {rows.map((r, i) => render(r, i))}
          </div>
        ))}
    </div>
  );
}
