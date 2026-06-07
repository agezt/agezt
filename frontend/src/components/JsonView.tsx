import { cn } from "@/lib/utils";

// A compact, theme-aware pretty-printed JSON block — the fallback renderer for
// panels that don't yet have a bespoke view (their data still surfaces fully).
export function JsonView({ value, className }: { value: unknown; className?: string }) {
  return (
    <pre
      className={cn(
        "max-h-[60vh] overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-panel p-3 text-xs",
        className,
      )}
    >
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

export function KeyValue({ pairs }: { pairs: [string, React.ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
      {pairs.map(([k, v], i) => (
        <div key={i} className="contents">
          <dt className="text-muted">{k}</dt>
          <dd className="break-words">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function Muted({ children }: { children: React.ReactNode }) {
  return <span className="text-muted">{children}</span>;
}

export function ErrorText({ children }: { children: React.ReactNode }) {
  return <span className="text-bad">{children}</span>;
}
