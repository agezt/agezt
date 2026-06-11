import { useEffect, useMemo, useState } from "react";
import {
  Eye,
  RefreshCw,
  Activity as ActivityIcon,
  Users,
  LifeBuoy,
  ArrowRight,
  CircleDot,
  Megaphone,
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

// Shapes mirror the read routes this view aggregates — kept loose (all optional)
// so a field the backend drops never crashes the dashboard.
interface Run {
  correlation_id?: string;
  status?: string;
  intent?: string;
  model?: string;
  iters?: number;
  duration_ms?: number;
  started_unix_ms?: number;
  parent_correlation?: string;
}
interface AgentProfile {
  slug: string;
  name?: string;
  model?: string;
  task_type?: string;
  enabled?: boolean;
  retired?: boolean;
}
interface HelpMsg {
  id?: string;
  topic?: string;
  from?: string;
  to?: string;
  text?: string;
  ts_unix_ms?: number;
  help?: boolean;
}

interface OverseerData {
  runs: Run[];
  agents: AgentProfile[];
  help: HelpMsg[];
}

const EMPTY: OverseerData = { runs: [], agents: [], help: [] };

// Overseer is the supervisory dashboard — the "brain that watches" surface
// (M850/M862). It folds three existing read routes into one screen so the
// operator (or an overseeing agent) sees, at a glance: what is running right
// now, who is on the roster, and which agents have raised an unanswered call
// for help. Read-only and conflict-free — it mutates nothing, it only watches.
export function Overseer() {
  const [data, setData] = useState<OverseerData>(EMPTY);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [firstLoad, setFirstLoad] = useState(true);

  async function reload() {
    setLoading(true);
    try {
      const [runsRes, agentsRes, helpRes] = await Promise.all([
        getJSON<{ runs?: Run[] }>("/api/runs", { limit: "200" }).catch(() => ({ runs: [] })),
        getJSON<{ profiles?: AgentProfile[] }>("/api/agents").catch(() => ({ profiles: [] })),
        getJSON<{ open_help?: HelpMsg[] }>("/api/board/help").catch(() => ({ open_help: [] })),
      ]);
      setData({
        runs: runsRes.runs || [],
        agents: agentsRes.profiles || [],
        help: helpRes.open_help || [],
      });
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
      setFirstLoad(false);
    }
  }

  useEffect(() => {
    reload();
    const id = setInterval(reload, 5000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const active = useMemo(
    () => data.runs.filter((r) => (r.status || "running") === "running"),
    [data.runs],
  );
  const live = useMemo(() => {
    const roster = data.agents.filter((a) => !a.retired);
    return { total: roster.length, enabled: roster.filter((a) => a.enabled !== false).length };
  }, [data.agents]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Eye className="size-4 text-accent" /> Overseer
        </h2>
        <span className="text-xs text-muted">live supervisory view · refreshes every 5s</span>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          onClick={reload}
          disabled={loading}
          title="Refresh now"
        >
          <RefreshCw className={cn("size-4", loading && "animate-spin")} />
        </Button>
      </div>

      {err && <ErrorText>{err}</ErrorText>}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Stat
          icon={<ActivityIcon className="size-4" />}
          label="Active runs"
          value={active.length}
          tone={active.length > 0 ? "accent" : "muted"}
        />
        <Stat
          icon={<Users className="size-4" />}
          label="Agents (enabled / total)"
          value={`${live.enabled} / ${live.total}`}
          tone="muted"
        />
        <Stat
          icon={<LifeBuoy className="size-4" />}
          label="Open help requests"
          value={data.help.length}
          tone={data.help.length > 0 ? "warn" : "muted"}
        />
      </div>

      {firstLoad ? (
        <SkeletonList count={4} />
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-2">
          <Panel title="Active runs" icon={<ActivityIcon className="size-4 text-accent" />}>
            {active.length === 0 ? (
              <span className="text-xs text-muted">Nothing running right now.</span>
            ) : (
              <ul className="flex flex-col gap-1.5">
                {active.map((r) => (
                  <li
                    key={r.correlation_id}
                    className="rounded-md border border-border bg-surface/40 px-2.5 py-1.5"
                  >
                    <div className="flex items-center gap-2">
                      <CircleDot className="size-3.5 shrink-0 animate-pulse text-accent" />
                      <span className="truncate text-xs font-medium" title={r.intent}>
                        {r.intent || r.correlation_id}
                      </span>
                      {r.parent_correlation && (
                        <span className="shrink-0 rounded bg-surface px-1 text-[10px] text-muted">
                          sub-agent
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 flex items-center gap-2 text-[10px] text-muted">
                      {r.model && <span className="truncate">{r.model}</span>}
                      {typeof r.iters === "number" && <span>· {r.iters} iters</span>}
                      {r.started_unix_ms ? <span>· started {fmtTime(r.started_unix_ms)}</span> : null}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Needs attention" icon={<LifeBuoy className="size-4 text-amber-400" />}>
            {data.help.length === 0 ? (
              <span className="text-xs text-muted">No open help requests.</span>
            ) : (
              <ul className="flex flex-col gap-1.5">
                {data.help.map((m, i) => (
                  <li
                    key={m.id || i}
                    className="rounded-md border border-amber-500/30 bg-amber-500/5 px-2.5 py-1.5"
                  >
                    <div className="flex items-center gap-1.5 text-[10px] text-muted">
                      {m.to === "*" ? (
                        <Megaphone className="size-3 text-amber-400" />
                      ) : (
                        <LifeBuoy className="size-3 text-amber-400" />
                      )}
                      <span className="font-medium text-foreground">{m.from || "agent"}</span>
                      {m.to && m.to !== "*" && (
                        <>
                          <ArrowRight className="size-3" />
                          <span>{m.to}</span>
                        </>
                      )}
                      {m.topic && <span className="ml-auto">#{m.topic}</span>}
                    </div>
                    <p className="mt-0.5 line-clamp-2 text-xs">{m.text}</p>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>
      )}
    </div>
  );
}

function Stat({
  icon,
  label,
  value,
  tone,
}: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  tone: "accent" | "warn" | "muted";
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-lg border border-border bg-surface/40 px-3 py-2.5",
        tone === "accent" && "border-accent/40",
        tone === "warn" && "border-amber-500/40",
      )}
    >
      <span
        className={cn(
          "grid size-8 place-items-center rounded-md bg-surface",
          tone === "accent" && "text-accent",
          tone === "warn" && "text-amber-400",
          tone === "muted" && "text-muted",
        )}
      >
        {icon}
      </span>
      <div className="min-w-0">
        <div className="text-lg font-semibold leading-none">{value}</div>
        <div className="mt-1 truncate text-[10px] uppercase tracking-wide text-muted">{label}</div>
      </div>
    </div>
  );
}

function Panel({
  title,
  icon,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-col gap-2 overflow-hidden rounded-lg border border-border bg-surface/20 p-3">
      <h3 className="flex items-center gap-2 text-xs font-semibold">
        {icon} {title}
      </h3>
      <div className="min-h-0 flex-1 overflow-auto">{children}</div>
    </div>
  );
}
