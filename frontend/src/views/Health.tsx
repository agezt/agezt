import { useEffect, useRef, useState } from "react";
import { HeartPulse, RefreshCw, Clock, ShieldAlert, Brain, Network, Sparkles, ListTree, Pause, CheckSquare, Route } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { getJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Ring, Sparkline, BarRow } from "@/components/Widgets";

interface Status {
  uptime_seconds?: number;
  halted?: boolean;
  active_runs?: number;
  journal_head?: number;
  pending_approvals?: number;
  memory_records?: number;
  world_entities?: number;
  active_skills?: number;
  provider_fallbacks?: { count?: number; last_reason?: string };
  model_fallbacks?: { count?: number; last_reason?: string };
}
interface Stats {
  total?: number;
  completed?: number;
  failed?: number;
  success_rate?: number;
}
interface Providers {
  routed?: number;
  fallbacks?: number;
  fallback_rate?: number;
  fallbacks_by_primary?: Record<string, number>;
}

const MAX_SERIES = 40;

// humanizeUptime renders seconds as a compact "2d 3h 4m" / "5m 12s" string.
function humanizeUptime(s: number): string {
  if (s <= 0) return "just started";
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${sec}s`;
  return `${sec}s`;
}

// Health is the durum-izleme cockpit: the daemon's vital signs as gauges and
// sparklines — success vs error rate, provider resilience, uptime, a live
// activity pulse, the knowledge footprint, and any provider fallbacks — so the
// operator can read system health at a glance and catch trouble early.
export function Health() {
  const [st, setSt] = useState<Status | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [prov, setProv] = useState<Providers | null>(null);
  const [series, setSeries] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const lastHead = useRef<number | null>(null);

  async function refresh() {
    setLoading(true);
    const [s, t, p] = await Promise.allSettled([
      getJSON<Status>("/api/status"),
      getJSON<Stats>("/api/stats"),
      getJSON<Providers>("/api/providers"),
    ]);
    if (s.status === "fulfilled") {
      setSt(s.value);
      const head = Number(s.value.journal_head ?? 0);
      if (lastHead.current !== null) {
        setSeries((prev) => [...prev, Math.max(0, head - lastHead.current!)].slice(-MAX_SERIES));
      }
      lastHead.current = head;
    }
    if (t.status === "fulfilled") setStats(t.value);
    if (p.status === "fulfilled") setProv(p.value);
    setLoading(false);
  }

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, []);

  const total = stats?.total ?? 0;
  const successPct = total ? Math.round((stats?.success_rate ?? 0) * 100) : 0;
  const errorPct = total ? Math.round(((stats?.failed ?? 0) / total) * 100) : 0;
  const fbRatePct = Math.round((prov?.fallback_rate ?? 0) * 100);
  const fallbacks = prov?.fallbacks_by_primary ? Object.entries(prov.fallbacks_by_primary) : [];
  const maxFb = Math.max(1, ...fallbacks.map(([, c]) => c));

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <HeartPulse className="size-4 text-accent" /> Health
        </h2>
        {st?.halted && (
          <span className="inline-flex items-center gap-1 rounded-full bg-bad/15 px-2 py-0.5 text-xs font-semibold text-bad">
            <Pause className="size-3" /> HALTED
          </span>
        )}
        <Button variant="ghost" size="sm" onClick={refresh} disabled={loading} className="ml-auto">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      {/* Vital gauges */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <GaugeCard>
          <Ring
            pct={successPct}
            center={total ? `${successPct}%` : "—"}
            label="success rate"
            tone={successPct >= 90 ? "good" : successPct >= 70 ? "warn" : "bad"}
          />
        </GaugeCard>
        <GaugeCard>
          <Ring
            pct={errorPct}
            center={total ? `${errorPct}%` : "—"}
            label="error rate"
            tone={errorPct === 0 ? "good" : errorPct < 10 ? "warn" : "bad"}
          />
        </GaugeCard>
        <GaugeCard>
          <Ring
            pct={fbRatePct}
            center={`${fbRatePct}%`}
            label="provider fallbacks"
            tone={fbRatePct < 5 ? "good" : fbRatePct < 20 ? "warn" : "bad"}
          />
        </GaugeCard>
        <div className="flex flex-col justify-center rounded-lg border border-border bg-card p-3">
          <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
            <Clock className="size-3.5" /> Uptime
          </div>
          <div className="mt-1 text-xl font-semibold tabular-nums">{humanizeUptime(st?.uptime_seconds ?? 0)}</div>
          <div className="mt-0.5 text-[11px] text-muted">since last start</div>
        </div>
      </div>

      {/* Live activity pulse */}
      <div className="rounded-lg border border-border bg-card p-3">
        <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
          <HeartPulse className="size-3.5" /> Activity pulse
          <span className="ml-auto font-normal normal-case text-muted">
            {series.length >= 2 ? `${series[series.length - 1]} events/5s` : "collecting…"}
          </span>
        </div>
        <Sparkline data={series} tone="accent" height={64} />
      </div>

      {/* Footprint + attention */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-7">
        <Tile icon={ListTree} label="running" value={st?.active_runs ?? 0} pulse={(st?.active_runs ?? 0) > 0} tone="accent" />
        <Tile icon={CheckSquare} label="approvals" value={st?.pending_approvals ?? 0} tone={(st?.pending_approvals ?? 0) > 0 ? "warn" : "muted"} />
        <Tile icon={ShieldAlert} label="provider fb" value={st?.provider_fallbacks?.count ?? 0} tone={(st?.provider_fallbacks?.count ?? 0) > 0 ? "warn" : "muted"} />
        <Tile icon={Route} label="model fb" value={st?.model_fallbacks?.count ?? 0} tone={(st?.model_fallbacks?.count ?? 0) > 0 ? "warn" : "muted"} />
        <Tile icon={Brain} label="memory" value={st?.memory_records ?? 0} tone="muted" />
        <Tile icon={Network} label="entities" value={st?.world_entities ?? 0} tone="muted" />
        <Tile icon={Sparkles} label="skills" value={st?.active_skills ?? 0} tone="muted" />
      </div>

      {/* Provider fallback breakdown */}
      {fallbacks.length > 0 && (
        <div className="rounded-lg border border-border bg-card p-3">
          <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
            <ShieldAlert className="size-3.5" /> Fallbacks by failed provider
          </div>
          <div className="space-y-1.5">
            {fallbacks
              .sort((a, b) => b[1] - a[1])
              .map(([name, count]) => (
                <BarRow key={name} label={name} value={count} max={maxFb} display={String(count)} tone="warn" />
              ))}
          </div>
          {st?.provider_fallbacks?.last_reason && (
            <div className="mt-2 text-[11px] text-muted">last: {st.provider_fallbacks.last_reason}</div>
          )}
        </div>
      )}
    </div>
  );
}

function GaugeCard({ children }: { children: React.ReactNode }) {
  return <div className="flex items-center justify-center rounded-lg border border-border bg-card p-3">{children}</div>;
}

function Tile({
  icon: Icon,
  label,
  value,
  tone,
  pulse,
}: {
  icon: LucideIcon;
  label: string;
  value: number | string;
  tone: "accent" | "good" | "bad" | "warn" | "muted";
  pulse?: boolean;
}) {
  const color = {
    accent: "text-accent",
    good: "text-good",
    bad: "text-bad",
    warn: "text-amber-500",
    muted: "text-foreground",
  }[tone];
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2.5">
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <Icon className="size-3.5" /> {label}
        {pulse && <span className="ml-auto size-2 animate-pulse rounded-full bg-accent" />}
      </div>
      <div className={cn("mt-1 text-2xl font-semibold tabular-nums", color)}>{value}</div>
    </div>
  );
}
