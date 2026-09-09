import { useEffect, useRef, useState, type ReactNode } from "react";
import { HeartPulse, RefreshCw, Clock, ShieldAlert, Brain, ListTree, Pause, CheckSquare, Stethoscope, CalendarClock, CheckCircle2, XOctagon, Activity, Cpu, Server, Database, Network, Sparkles, Wrench, KeyRound } from "lucide-react";
import { cn, fmtWhen } from "@/app/utils";
import { getJSON } from "@/app/api";
import { Button } from "@/components/ui/button";
import { Page } from "@/components/ui/page";
import { SectionPanel } from "@/components/ui/section-panel";
import { Sparkline, BarRow } from "@/components/Widgets";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { Badge } from "@/components/ui/badge";
import { Advanced, Calm } from "@/components/ui/advanced";

interface Status {
  uptime_seconds?: number;
  halted?: boolean;
  model?: string;
  active_runs?: number;
  journal_head?: number;
  pending_approvals?: number;
  memory_records?: number;
  world_entities?: number;
  active_skills?: number;
  schedules?: { total?: number; enabled?: number; running?: number; resident?: boolean };
  provider_fallbacks?: { count?: number; last_reason?: string; last_ms?: number };
  model_fallbacks?: { count?: number; last_reason?: string };
  // Absorbed from the former "System" view (2026-09 merge): the same /api/status
  // payload, read by a second page that duplicated five of this page's tiles.
  daemon?: string;
  protocol?: number;
  tools?: number;
  delegation?: { enabled?: boolean; max_depth?: number; max_fanout?: number; max_spend_microcents?: number };
  http_servers?: { name?: string; addr?: string; loopback?: boolean }[];
  cred_chain?: string;
}
interface Stats {
  total?: number;
  completed?: number;
  failed?: number;
  success_rate?: number;
}

// Diagnostics (M921): the webui "doctor" — active checks over the daemon's live
// state, each with a remediation hint + a deep-link to the view that fixes it. The
// CLI has `agt doctor`; this brings the same "what's wrong and how to fix it" to
// the console. Pure + unit-tested; journalOk is null while the verify is in flight.
export type DiagLevel = "ok" | "info" | "warn" | "fail";

export interface Diagnostic {
  id: string;
  level: DiagLevel;
  title: string;
  detail: string;
  fixHash?: string;
  fixLabel?: string;
}

const DIAG_RANK: Record<DiagLevel, number> = { ok: 0, info: 1, warn: 2, fail: 3 };

// worstLevel returns the most severe level across the checks (ok when empty).
export function worstLevel(diags: Diagnostic[]): DiagLevel {
  return diags.reduce<DiagLevel>((w, d) => (DIAG_RANK[d.level] > DIAG_RANK[w] ? d.level : w), "ok");
}

// runDiagnostics evaluates the daemon's state into a list of issues worth the
// operator's attention — only the not-ok ones (an empty list means "all healthy").
// Pure + unit-tested.
export function runDiagnostics(
  st: Status | null,
  stats: Stats | null,
  journalOk: boolean | null,
): Diagnostic[] {
  const out: Diagnostic[] = [];
  if (!st) {
    out.push({ id: "daemon", level: "fail", title: "Daemon unreachable", detail: "The console can't read the daemon's status.", fixHash: "status", fixLabel: "Status" });
    return out;
  }
  if (st.halted) {
    out.push({ id: "halted", level: "fail", title: "Daemon is halted", detail: "Runs are paused until you resume.", fixHash: "policy", fixLabel: "Resume" });
  }
  if (journalOk === false) {
    out.push({ id: "journal", level: "fail", title: "Journal verification failed", detail: "The hash-chained journal didn't verify — possible corruption or tampering.", fixHash: "search", fixLabel: "Inspect" });
  }
  const pf = st.provider_fallbacks?.count ?? 0;
  if (pf > 0) {
    const why = (st.provider_fallbacks?.last_reason || "").trim();
    // The fallback count folds the WHOLE journal, so yesterday's failover storm
    // would otherwise read as a live incident forever. Anchor the message to
    // WHEN the last one happened, and only warn while it's recent (24h).
    // Unknown timestamp (older daemon) counts as recent: we can't prove the
    // failover is stale, so the honest default is to keep warning.
    const lastMs = st.provider_fallbacks?.last_ms ?? 0;
    const recent = lastMs === 0 || Date.now() - lastMs < 24 * 60 * 60 * 1000;
    const when = lastMs > 0 ? ` (${fmtWhen(lastMs)})` : "";
    out.push({
      id: "provider",
      level: recent ? "warn" : "info",
      title: recent ? "A provider is failing over" : "Past provider failovers",
      detail: why
        ? `${pf} fallback(s); last${when}: ${why}`
        : `${pf} provider fallback(s)${when} — a primary provider ${recent ? "is" : "was"} erroring.`,
      fixHash: "providers",
      fixLabel: "Providers",
    });
  }
  if (!st.model || !st.model.trim()) {
    out.push({ id: "model", level: "warn", title: "No default model set", detail: "Runs have no model to route to until you pick one.", fixHash: "models", fixLabel: "Models" });
  }
  const total = stats?.total ?? 0;
  const failed = stats?.failed ?? 0;
  if (total >= 5 && failed / total > 0.2) {
    out.push({ id: "failrate", level: "warn", title: "Elevated failure rate", detail: `${failed} of ${total} runs failed (${Math.round((failed / total) * 100)}%).`, fixHash: "runs", fixLabel: "Runs" });
  }
  const pending = st.pending_approvals ?? 0;
  if (pending > 0) {
    out.push({ id: "approvals", level: "info", title: `${pending} request${pending === 1 ? "" : "s"} awaiting approval`, detail: "The agent is blocked on your decision.", fixHash: "approvals", fixLabel: "Approvals" });
  }
  const schedulesRunning = st.schedules?.running ?? 0;
  const schedulesEnabled = st.schedules?.enabled ?? 0;
  if (schedulesEnabled > 0 && st.schedules?.resident === false) {
    out.push({
      id: "schedule-resident",
      level: "warn",
      title: "Schedule resident is offline",
      detail: `${schedulesEnabled} enabled schedule${schedulesEnabled === 1 ? "" : "s"} cannot wake until the cadence resident is attached.`,
      fixHash: "status",
      fixLabel: "Status",
    });
  }
  if (schedulesRunning > 0) {
    out.push({
      id: "schedule-running",
      level: "info",
      title: `${schedulesRunning} schedule${schedulesRunning === 1 ? "" : "s"} running`,
      detail: "Cadence has active autonomous work in flight.",
      fixHash: "schedules",
      fixLabel: "Schedules",
    });
  }
  return out;
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
  const [journalOk, setJournalOk] = useState<boolean | null>(null);
  const [series, setSeries] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const lastHead = useRef<number | null>(null);

  async function refresh() {
    setLoading(true);
    const [s, t, p, j] = await Promise.allSettled([
      getJSON<Status>("/api/status"),
      getJSON<Stats>("/api/stats"),
      getJSON<Providers>("/api/providers"),
      getJSON("/api/journal/verify"),
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
    // The verify endpoint resolves on a clean chain and rejects when it fails to
    // verify — so a rejection IS the "journal broken" signal (M921).
    setJournalOk(j.status === "fulfilled");
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
  // Schedule health reads three ways: "offline" when schedules are enabled but
  // the cadence resident is not running (they will never fire), "N live" while
  // firing, else enabled/total. Ported from the System view — a bare running
  // count hid the offline case entirely.
  const scheduleRunning = st?.schedules?.running ?? 0;
  const scheduleEnabled = st?.schedules?.enabled ?? 0;
  const scheduleTotal = st?.schedules?.total ?? 0;
  const scheduleOffline = scheduleEnabled > 0 && st?.schedules?.resident === false;
  const scheduleValue = scheduleOffline
    ? "offline"
    : scheduleRunning > 0
      ? `${scheduleRunning} live`
      : `${scheduleEnabled}/${scheduleTotal}`;
  const fallbacks = prov?.fallbacks_by_primary ? Object.entries(prov.fallbacks_by_primary) : [];
  const maxFb = Math.max(1, ...fallbacks.map(([, c]) => c));

  return (
    <Page
      icon={HeartPulse}
      title="Health"
      width="wide"
      actions={
        <>
          {st?.halted && (
            <Badge variant="bad">
              <Pause className="size-3" /> Halted
            </Badge>
          )}
          <Button variant="ghost" size="sm" onClick={refresh} disabled={loading}>
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
          </Button>
        </>
      }
    >

      {/* Doctor — active diagnostics with remedies (M921) */}
      <DoctorCard diags={runDiagnostics(st, stats, journalOk)} loaded={st !== null} />

      {/* Key metrics */}
      <MetricGrid>
        <MetricWidget
          icon={CheckCircle2}
          label="Success rate"
          value={total ? `${successPct}%` : "—"}
          subvalue={total ? `all time · ${total} runs` : undefined}
          tone={successPct >= 90 ? "good" : successPct >= 70 ? "warn" : "bad"}
          trend={[]}
        />
        <MetricWidget
          icon={XOctagon}
          label="Error rate"
          value={total ? `${errorPct}%` : "—"}
          subvalue={total ? "all time" : undefined}
          tone={errorPct === 0 ? "good" : errorPct < 10 ? "warn" : "bad"}
        />
        <MetricWidget
          icon={ShieldAlert}
          label="Fallbacks"
          value={`${fbRatePct}%`}
          subvalue={`${prov?.fallbacks ?? 0} providers`}
          tone={fbRatePct < 5 ? "good" : fbRatePct < 20 ? "warn" : "bad"}
        />
        <MetricWidget
          icon={Clock}
          label="Uptime"
          value={humanizeUptime(st?.uptime_seconds ?? 0)}
          tone="muted"
        />
        <MetricWidget
          icon={ListTree}
          label="Active runs"
          value={st?.active_runs ?? 0}
          tone={(st?.active_runs ?? 0) > 0 ? "accent" : "muted"}
          pulse={(st?.active_runs ?? 0) > 0}
        />
        <MetricWidget
          icon={CalendarClock}
          label="Schedules"
          value={scheduleValue}
          subvalue={`${scheduleEnabled} enabled`}
          tone={scheduleOffline ? "bad" : scheduleRunning > 0 ? "accent" : "muted"}
          pulse={scheduleRunning > 0}
        />
        <MetricWidget
          icon={CheckSquare}
          label="Approvals"
          value={st?.pending_approvals ?? 0}
          tone={(st?.pending_approvals ?? 0) > 0 ? "warn" : "muted"}
        />
        <MetricWidget
          icon={Brain}
          label="Memory"
          value={st?.memory_records ?? 0}
          tone="muted"
        />
      </MetricGrid>

      {/* System counters — the former "System" page. It read the same
          /api/status and repeated Uptime / Active runs / Schedules / Approvals /
          Memory; only its own tiles survive here (docs/CONSOLE-IA.md 2.4). */}
      <MetricGrid>
        <MetricWidget
          icon={Activity}
          label={st?.halted ? "halted" : "operational"}
          value={st ? (st.halted ? "HALTED" : "OK") : "—"}
          tone={st?.halted ? "bad" : "good"}
        />
        <MetricWidget icon={Cpu} label="model" value={st?.model || "—"} tone="muted" />
        <MetricWidget icon={Server} label="daemon" value={st?.daemon || "—"} tone="muted" />
        <MetricWidget icon={Database} label="journal head" value={st?.journal_head ?? "—"} tone="muted" />
        <MetricWidget icon={Network} label="world entities" value={st?.world_entities ?? "—"} tone="muted" />
        <MetricWidget icon={Sparkles} label="active skills" value={st?.active_skills ?? "—"} tone="muted" />
        <MetricWidget icon={Wrench} label="tools" value={st?.tools ?? "—"} tone="muted" />
      </MetricGrid>

      {/* Delegation limits, HTTP surface, credentials and routing detail — the
          System view's Advanced tab, folded here per the declutter law. */}
      <Calm>
        <p className="text-xs text-muted">Advanced mode for delegation, HTTP, credentials, and routing detail.</p>
      </Calm>
      <Advanced>
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <SectionPanel
            icon={Network}
            title="Delegation"
            status={st?.delegation?.enabled ? "enabled" : "off"}
            tone={st?.delegation?.enabled ? "accent" : "muted"}
          >
            {st?.delegation ? (
              <ul className="space-y-1 text-xs">
                <Row k="enabled" v={st.delegation.enabled ? "yes" : "no"} />
                <Row k="max depth" v={st.delegation.max_depth ?? "—"} />
                <Row k="max fan-out" v={st.delegation.max_fanout ? st.delegation.max_fanout : "unbounded"} />
                <Row
                  k="max spend"
                  v={st.delegation.max_spend_microcents ? `${(st.delegation.max_spend_microcents / 1e9).toFixed(4)}` : "uncapped"}
                />
              </ul>
            ) : (
              <div className="text-xs text-muted">—</div>
            )}
          </SectionPanel>

          <SectionPanel
            icon={Server}
            title="HTTP surface"
            status={`${st?.http_servers?.length ?? 0} server${(st?.http_servers?.length ?? 0) === 1 ? "" : "s"}`}
            tone={(st?.http_servers?.length ?? 0) > 0 ? "accent" : "muted"}
          >
            {st?.http_servers?.length ? (
              <ul className="space-y-1 text-xs">
                {st.http_servers.map((sv, i) => (
                  <li key={i} className="flex items-center gap-2">
                    <span className="font-mono">{sv.addr}</span>
                    <span className="text-muted">{sv.name}</span>
                    {sv.loopback && <Badge variant="good" className="ml-auto">loopback</Badge>}
                  </li>
                ))}
              </ul>
            ) : (
              <div className="text-xs text-muted">none</div>
            )}
          </SectionPanel>

          <SectionPanel
            icon={KeyRound}
            title="Credentials"
            status={st?.cred_chain ? "chain loaded" : "not reported"}
            tone={st?.cred_chain ? "good" : "muted"}
          >
            <div className="break-words font-mono text-xs text-muted">{st?.cred_chain || "—"}</div>
          </SectionPanel>

          <SectionPanel
            icon={ShieldAlert}
            title="Provider routing"
            status={`${st?.provider_fallbacks?.count ?? 0} fallback${(st?.provider_fallbacks?.count ?? 0) === 1 ? "" : "s"}`}
            tone={(st?.provider_fallbacks?.count ?? 0) > 0 ? "warn" : "good"}
          >
            <div className="text-xs">
              <Row k="fallbacks" v={st?.provider_fallbacks?.count ?? 0} />
              {st?.provider_fallbacks?.last_reason && (
                <div className="mt-1.5 rounded-md border border-warn/40 bg-warn/5 p-2 text-[11px] text-muted">
                  <span className="text-warn">last fallback: </span>
                  {st.provider_fallbacks.last_reason.slice(0, 200)}
                </div>
              )}
            </div>
          </SectionPanel>
        </div>
      </Advanced>

      <SectionPanel
        icon={HeartPulse}
        title="Activity pulse"
        status={series.length >= 2 ? `${series[series.length - 1]} events/5s` : "collecting..."}
        tone="accent"
      >
        <Sparkline data={series} tone="accent" height={64} />
      </SectionPanel>

      {/* Provider fallback breakdown */}
      {fallbacks.length > 0 && (
        <SectionPanel
          icon={ShieldAlert}
          title="Provider fallbacks"
          status={`${prov?.fallbacks ?? 0} routed away`}
          tone="warn"
        >
          <div className="space-y-1.5">
            {fallbacks
              .sort((a, b) => b[1] - a[1])
              .map(([name, count]) => (
                <BarRow key={name} label={name} value={count} max={maxFb} display={String(count)} tone="warn" />
              ))}
          </div>
          {st?.provider_fallbacks?.last_reason && (
            <div className="mt-2 break-words text-xs text-muted">
              last{st.provider_fallbacks.last_ms ? ` (${fmtWhen(st.provider_fallbacks.last_ms)})` : ""}:{" "}
              {st.provider_fallbacks.last_reason}
            </div>
          )}
        </SectionPanel>
      )}
    </Page>
  );
}


// Row is one key/value line inside an advanced panel.
function Row({ k, v }: { k: string; v: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 border-b border-border/40 py-0.5 last:border-0">
      <span className="text-muted">{k}</span>
      <span className="tabular-nums">{v}</span>
    </div>
  );
}

function DoctorCard({ diags, loaded }: { diags: Diagnostic[]; loaded: boolean }) {
  // Which diagnostic's detail is expanded — the inline row truncates, and a
  // failover reason is exactly the kind of text you need to read to the end.
  const [openId, setOpenId] = useState<string | null>(null);
  const worst = worstLevel(diags);
  const healthy = loaded && diags.length === 0;
  const tone =
    worst === "fail" ? "border-bad/40 bg-bad/5" : worst === "warn" ? "border-warn/40 bg-warn/5" : "border-good/40 bg-good/5";
  const badgeVariant: Record<DiagLevel, "good" | "accent" | "warn" | "bad"> = {
    ok: "good", info: "accent", warn: "warn", fail: "bad",
  };
  return (
    <div className={cn("rounded-lg border p-3", healthy ? "border-good/40 bg-good/5" : tone)}>
      <div className="mb-1.5 flex items-center gap-2">
        <Stethoscope className={cn("size-3.5", healthy ? "text-good" : "text-bad")} />
        <span className={cn("text-xs font-semibold", healthy ? "text-good" : "text-bad")}>Diagnostics</span>
        <span className="text-xs text-muted">
          {!loaded ? "checking…" : healthy ? "all clear" : `${diags.length} issue${diags.length === 1 ? "" : "s"}`}
        </span>
      </div>
      {diags.length > 0 && (
        <ul className="space-y-1">
          {diags.map((d) => (
            <li key={d.id} className="text-xs">
              <div className="flex items-center gap-2">
                <Badge variant={badgeVariant[d.level]}>{d.level}</Badge>
                <span className="shrink-0 font-medium text-foreground">{d.title}</span>
                <button
                  onClick={() => setOpenId((v) => (v === d.id ? null : d.id))}
                  className="min-w-0 flex-1 truncate text-left text-muted transition-colors hover:text-foreground"
                  title={openId === d.id ? "Collapse" : "Show the full message"}
                >
                  {d.detail}
                </button>
                {d.fixHash && (
                  <button
                    onClick={() => (location.hash = d.fixHash!)}
                    className="ml-auto shrink-0 rounded border border-border px-1.5 py-0.5 text-xs text-accent transition-colors hover:border-accent"
                  >
                    {d.fixLabel || "Fix"} →
                  </button>
                )}
              </div>
              {openId === d.id && (
                <div className="mt-1 whitespace-pre-wrap break-words rounded-md border border-border bg-panel/60 p-2 text-muted">
                  {d.detail}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
