import { cn } from "@/lib/utils";

// Skeleton is a content-shaped loading placeholder: a muted block with a soft
// shimmer sweep (reduced-motion aware via the .skeleton class in index.css), so
// a view that's fetching looks alive and hints at the shape of what's coming
// instead of a bare "loading…" line.
export function Skeleton({ className }: { className?: string }) {
  return <div className={cn("skeleton rounded-md", className)} aria-hidden="true" />;
}

// SkeletonCard mimics one list/grid card: a header row (badge + title) and a
// couple of text lines. Used to fill list views while they load.
export function SkeletonCard({ lines = 2 }: { lines?: number }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="flex items-center gap-2">
        <Skeleton className="h-4 w-12" />
        <Skeleton className="h-4 w-32" />
        <Skeleton className="ml-auto size-4 rounded-full" />
      </div>
      <div className="mt-2.5 space-y-1.5">
        {Array.from({ length: lines }).map((_, i) => (
          <Skeleton key={i} className={cn("h-3", i === lines - 1 ? "w-2/3" : "w-full")} />
        ))}
      </div>
    </div>
  );
}

// SkeletonList stacks N SkeletonCards — the standard placeholder for the
// list-style views (Standing, Schedules, Skills, …).
export function SkeletonList({ count = 4, lines = 2 }: { count?: number; lines?: number }) {
  return (
    <div className="space-y-2" role="status" aria-label="loading">
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} lines={lines} />
      ))}
    </div>
  );
}

// SkeletonGrid lays SkeletonCards in a responsive grid — the placeholder for the
// card-grid views (Memory, World, …).
export function SkeletonGrid({ count = 6, lines = 2 }: { count?: number; lines?: number }) {
  return (
    <div className="grid grid-cols-1 gap-2 lg:grid-cols-2" role="status" aria-label="loading">
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} lines={lines} />
      ))}
    </div>
  );
}
