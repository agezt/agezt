// Reflect.tsx — Day 26 bring-back.
//
// "Reflect" is the third Thinking Partner. Unlike Research (which calls the
// daemon's planner) or Analyst (which reads the journal), Reflect surfaces
// the agent's own self-talk: memory writes/supersedes, decisions, and
// recurring themes — i.e. the kinds of entries where the daemon was
// reasoning about itself. We pull /api/memory/audit and /api/journal_search
// in parallel, then group by subject and highlight the entries that look
// like self-corrections or lessons learned.
import { useEffect, useMemo, useState } from "react";
import { Brain, ChevronRight, Lightbulb, RefreshCw, Repeat } from "lucide-react";
import { getJSON } from "@/app/api";
import { Button } from "@/components/ui/button";
import { cn, fmtAgo } from "@/app/utils/utils";

interface MemoryEvent {
  id?: string;
  ts?: number;
  subject?: string;
  type?: string;
  content?: string;
  confidence?: number;
  evidence?: string;
  superseded_by?: string;
  supersedes?: string;
}

interface JournalEvent {
  ts?: number;
  kind?: string;
  actor?: string;
  subject?: string;
  note?: string;
}

const REFLECTIVE_KINDS = new Set([
  "memory.write",
  "memory.supersede",
  "policy.decision",
  "run.fail",
  "approval.request",
  "approval.decide",
]);

const LESSON_RE = /(learned|lesson|next time|in future|remember|keep in mind|don't forget)/i;
const CORRECTION_RE = /(mistake|wrong|reverted|rolled back|fixed|fixed-up)/i;

export function Reflect() {
  const [memories, setMemories] = useState<MemoryEvent[]>([]);
  const [journal, setJournal] = useState<JournalEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    setLoading(true);
    setError(null);
    try {
      const [mem, jour] = await Promise.all([
        getJSON<{ audit?: MemoryEvent[]; memories?: MemoryEvent[] }>(
          `/api/memory/audit?limit=120`,
        ).catch(() => ({ audit: [], memories: [] }) as any),
        getJSON<{ events?: JournalEvent[] }>(`/api/journal?limit=80`).catch(
          () => ({ events: [] }) as any,
        ),
      ]);
      const memList = mem.audit || mem.memories || [];
      const jourList = jour.events || [];
      setMemories(memList);
      setJournal(jourList);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  const lessons = useMemo(
    () =>
      memories.filter((m) => m.content && LESSON_RE.test(m.content)),
    [memories],
  );

  const corrections = useMemo(
    () => [
      ...memories.filter((m) => m.content && CORRECTION_RE.test(m.content)),
      ...journal.filter((j) => j.note && CORRECTION_RE.test(j.note)),
    ],
    [memories, journal],
  );

  const supersedes = useMemo(
    () => memories.filter((m) => m.superseded_by || m.supersedes),
    [memories],
  );

  const bySubject = useMemo(() => {
    const counts = new Map<string, number>();
    [...memories, ...journal].forEach((e) => {
      const k = (e as any).subject;
      if (!k) return;
      counts.set(k, (counts.get(k) || 0) + 1);
    });
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 10);
  }, [memories, journal]);

  const reflectiveJournal = useMemo(
    () =>
      journal
        .filter((j) => j.kind && REFLECTIVE_KINDS.has(j.kind))
        .slice(0, 30),
    [journal],
  );

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      <header className="rounded-lg border border-border bg-card p-4">
        <div className="flex items-center gap-2">
          <Brain className="size-5 text-accent" />
          <h2 className="text-lg font-semibold text-fg">Reflect</h2>
          <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-accent">
            Thinking partner
          </span>
          <span className="ml-auto">
            <Button onClick={refresh} disabled={loading} variant="ghost">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
          </span>
        </div>
        <p className="mt-1 text-sm text-muted">
          Read what the daemon has been telling itself: lessons written, corrections made,
          memories superseded, and decisions logged.
        </p>
      </header>

      {error && (
        <div className="rounded-lg border border-danger/40 bg-danger/5 p-3 text-sm text-danger">
          {error}
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Lessons written" icon={Lightbulb} empty="No explicit lessons captured yet.">
          <ul className="space-y-2">
            {lessons.map((m, i) => (
              <li
                key={m.id || i}
                className="rounded-md border border-success/30 bg-success/5 px-3 py-2 text-xs text-fg"
              >
                <div className="flex items-center justify-between gap-2 text-[10px] uppercase tracking-wide text-muted">
                  <span>{m.subject || "—"}</span>
                  <span>{fmtAgo(m.ts)}</span>
                </div>
                <p className="mt-1 leading-relaxed">{m.content}</p>
              </li>
            ))}
          </ul>
        </Card>

        <Card title="Corrections" icon={Repeat} empty="No corrections recorded yet.">
          <ul className="space-y-2">
            {corrections.map((m: any, i) => (
              <li
                key={m.id || `${m.kind || "j"}-${i}`}
                className="rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-fg"
              >
                <div className="flex items-center justify-between gap-2 text-[10px] uppercase tracking-wide text-muted">
                  <span>{m.subject || m.kind || "—"}</span>
                  <span>{fmtAgo(m.ts)}</span>
                </div>
                <p className="mt-1 leading-relaxed">{m.content || m.note}</p>
              </li>
            ))}
          </ul>
        </Card>

        <Card title="Superseded memories" icon={RefreshCw} empty="No supersede activity.">
          <ul className="space-y-2">
            {supersedes.map((m, i) => (
              <li key={m.id || i} className="rounded-md border border-border bg-bg/40 px-3 py-2 text-xs">
                <div className="flex items-center justify-between gap-2 text-[10px] uppercase tracking-wide text-muted">
                  <span>{m.subject || "—"}</span>
                  <span>{fmtAgo(m.ts)}</span>
                </div>
                <p className="mt-1 leading-relaxed text-fg/90">{m.content}</p>
                <p className="mt-1 text-[10px] text-muted">
                  {m.superseded_by
                    ? `→ replaced by ${m.superseded_by}`
                    : `← replaces ${m.supersedes}`}
                </p>
              </li>
            ))}
          </ul>
        </Card>

        <Card title="Top subjects" icon={ChevronRight} empty="No subjects yet.">
          <ul className="space-y-1.5">
            {bySubject.map(([label, count]) => (
              <li key={label} className="flex items-center gap-2 text-xs">
                <span className="w-32 shrink-0 truncate text-fg/90" title={label}>
                  {label}
                </span>
                <span className="h-2 flex-1 overflow-hidden rounded-full bg-bg">
                  <span
                    className="block h-full rounded-full bg-accent"
                    style={{ width: `${Math.max(8, (count / bySubject[0][1]) * 100)}%` }}
                  />
                </span>
                <span className="w-8 shrink-0 text-right text-[10px] text-muted">{count}</span>
              </li>
            ))}
          </ul>
        </Card>
      </div>

      <section className="rounded-lg border border-border bg-card p-4">
        <h3 className="text-sm font-semibold text-fg">Reflective journal entries</h3>
        {reflectiveJournal.length === 0 ? (
          <p className="mt-2 text-xs text-muted">
            The journal hasn't recorded any reflective events yet. Run a few policy decisions or
            memory writes to populate this.
          </p>
        ) : (
          <ul className="mt-3 divide-y divide-border">
            {reflectiveJournal.map((j, i) => (
              <li key={i} className="flex items-start gap-3 py-2 text-xs">
                <span className="w-20 shrink-0 text-muted">{fmtAgo(j.ts)}</span>
                <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-accent">
                  {j.kind}
                </span>
                <span className="w-28 shrink-0 truncate text-fg/80" title={j.actor}>
                  {j.actor || "—"}
                </span>
                <span className="flex-1 text-fg/90">{j.note || j.subject || "—"}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function Card({
  title,
  icon: Icon,
  empty,
  children,
}: {
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  empty: string;
  children: React.ReactNode;
}) {
  const isEmpty =
    !children ||
    (Array.isArray((children as any).props?.children)
      ? (children as any).props.children.every((c: any) => !c)
      : true);
  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-fg">
        <Icon className="size-4 text-accent" />
        {title}
      </h3>
      <div className="mt-2">
        {isEmpty ? <p className="text-xs text-muted">{empty}</p> : children}
      </div>
    </section>
  );
}
