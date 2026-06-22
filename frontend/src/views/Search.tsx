import { useState } from "react";
import { Search as SearchIcon, Loader2, GitBranch, ShieldCheck, ShieldAlert, ShieldQuestion, Download } from "lucide-react";
import { getJSON } from "@/lib/api";
import { downloadText } from "@/lib/export";
import type { AgentEvent } from "@/lib/events";
import { categoryOf, isErrorKind } from "@/lib/eventmeta";
import { cn, fmtTime } from "@/lib/utils";
import { DataView } from "@/components/DataView";
import { Muted, ErrorText } from "@/components/JsonView";
import { PageHeader } from "@/components/ui/page-header";
import { IncidentBadges } from "@/components/IncidentBadges";
import {
  incidentBadgeItem,
  incidentEventSummary,
  isIncidentFamilyEvent,
} from "@/lib/incidentevents";

// Search queries the FULL journal server-side (CmdJournalGrep) — the historical
// counterpart to the live stream. Filter by free-text pattern plus
// kind/actor/correlation; results are colour-coded and payload-expandable, so
// you can find and inspect any past event across the daemon's whole history.
export function Search() {
  const [pattern, setPattern] = useState("");
  const [kind, setKind] = useState("");
  const [actor, setActor] = useState("");
  const [corr, setCorr] = useState("");
  const [results, setResults] = useState<AgentEvent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  async function run() {
    setLoading(true);
    setErr(null);
    try {
      const params: Record<string, string> = { limit: "200" };
      if (pattern.trim()) params.pattern = pattern.trim();
      if (kind.trim()) params.kind = kind.trim();
      if (actor.trim()) params.actor = actor.trim();
      if (corr.trim()) params.correlation_id = corr.trim();
      const d = await getJSON<{ events?: AgentEvent[] }>("/api/journal_search", params);
      setResults(d.events || []);
    } catch (e) {
      setErr((e as Error).message);
      setResults(null);
    } finally {
      setLoading(false);
    }
  }

  function onKey(e: React.KeyboardEvent) {
    if (e.key === "Enter") run();
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={SearchIcon}
        title="Journal search"
        description="find and inspect any past event across the daemon's whole history"
        actions={
          <>
            <JournalIntegrity />
            <JournalExport />
          </>
        }
      />

      {/* Filters */}
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="text" value={pattern} onChange={setPattern} onKey={onKey} placeholder="match anywhere…" autoFocus />
        <Field label="kind" value={kind} onChange={setKind} onKey={onKey} placeholder="e.g. tool.result" />
        <Field label="actor" value={actor} onChange={setActor} onKey={onKey} placeholder="e.g. agent-…" />
        <Field label="correlation" value={corr} onChange={setCorr} onKey={onKey} placeholder="run id" />
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={run}
          disabled={loading}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-accent px-3 text-sm text-accent transition-colors hover:bg-accent hover:text-white disabled:opacity-50"
        >
          {loading ? <Loader2 className="size-4 animate-spin" /> : <SearchIcon className="size-4" />} Search
        </button>
        {results && <span className="text-xs text-muted">{results.length} match{results.length === 1 ? "" : "es"}</span>}
        <span className="ml-auto text-[11px] text-muted">searches the full journal (server-side)</span>
      </div>

      {/* Results */}
      <div className="min-h-0 flex-1 overflow-auto glass rounded-xl font-mono text-xs">
        {err ? (
          <div className="p-3">
            <ErrorText>{err}</ErrorText>
          </div>
        ) : !results ? (
          <div className="p-3">
            <Muted>enter a filter and search the daemon's whole history</Muted>
          </div>
        ) : results.length === 0 ? (
          <div className="p-3">
            <Muted>no events match</Muted>
          </div>
        ) : (
          <ul className="divide-y divide-border/40">
            {results.map((e, i) => {
              const cat = categoryOf(e.kind);
              const err2 = isErrorKind(e.kind);
              const id = e.id || `${e.seq}-${i}`;
              const isOpen = open === id;
              return (
                <li key={id} className={cn(err2 && "bg-bad/5")}>
                  <div
                    onClick={() => setOpen(isOpen ? null : id)}
                    className="flex cursor-pointer items-center gap-2 px-2.5 py-1 hover:bg-panel/60"
                  >
                    <span className="w-14 shrink-0 tabular-nums text-muted">{fmtTime(e.ts_unix_ms)}</span>
                    <span className="size-2 shrink-0 rounded-full" style={{ background: cat.color }} />
                    <span className="w-40 shrink-0 truncate font-medium" style={{ color: err2 ? undefined : cat.color }}>
                      <span className={cn(err2 && "text-bad")}>{e.kind}</span>
                    </span>
                    {isIncidentFamilyEvent(e) && <IncidentBadges item={incidentBadgeItem(e)} mono />}
                    <span className="min-w-0 flex-1 truncate text-foreground/80">
                      {incidentEventSummary(e) || e.subject}
                    </span>
                    {e.correlation_id && (
                      <span className="shrink-0 text-xs text-muted">{e.correlation_id.slice(-6)}</span>
                    )}
                  </div>
                  {isOpen && (
                    <div className="border-t border-border/40 bg-panel/40 px-3 py-2">
                      <div className="mb-1 flex gap-3 text-xs text-muted">
                        <span>seq {e.seq ?? "—"}</span>
                        <span>actor {e.actor || "—"}</span>
                        <span>{e.correlation_id || "—"}</span>
                      </div>
                      {e.payload != null ? <DataView data={e.payload} /> : <span className="text-[11px] text-muted">no payload</span>}
                      {e.id && <CausationTrace eventId={e.id} />}
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

// JournalIntegrity verifies the journal's tamper-evident hash chain on demand (M759).
// The journal is the daemon's append-only source of truth (SPEC-08 §4.2); each entry
// is hash-linked to the previous, so any edit/deletion breaks the chain. One click
// walks it server-side (CmdJournalVerify) and reports intact (✓) or, if a link is
// broken, the failure — the audit guarantee made visible and checkable.
export function JournalIntegrity() {
  const [state, setState] = useState<"idle" | "checking" | "ok" | "bad">("idle");
  const [msg, setMsg] = useState("");

  async function verify() {
    setState("checking");
    setMsg("");
    try {
      await getJSON("/api/journal/verify");
      setState("ok");
    } catch (e) {
      setState("bad");
      setMsg((e as Error).message);
    }
  }

  const cls =
    state === "ok"
      ? "border-good/40 text-good"
      : state === "bad"
        ? "border-bad/40 text-bad"
        : "border-border text-muted hover:text-foreground";
  return (
    <button
      onClick={verify}
      disabled={state === "checking"}
      title={state === "bad" ? msg : "Verify the journal's tamper-evident hash chain"}
      className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] transition-colors disabled:opacity-60 ${cls}`}
    >
      {state === "checking" ? (
        <Loader2 className="size-3 animate-spin" />
      ) : state === "ok" ? (
        <ShieldCheck className="size-3" />
      ) : state === "bad" ? (
        <ShieldAlert className="size-3" />
      ) : (
        <ShieldQuestion className="size-3" />
      )}
      {state === "ok" ? "chain intact" : state === "bad" ? "chain broken" : state === "checking" ? "verifying…" : "verify integrity"}
    </button>
  );
}

interface JournalExportResult {
  events?: unknown[];
  count?: number;
  first_seq?: number;
  last_seq?: number;
  head_seq?: number;
  head_hash?: string;
  truncated?: boolean;
}

// journalExportBundle wraps the daemon's export payload into a self-describing,
// offline-verifiable file (M772): the chain head travels with the events so the bundle
// can be re-checked with `agt journal verify --bundle <file>`.
export function journalExportBundle(data: JournalExportResult): string {
  return JSON.stringify(
    {
      version: 1,
      kind: "agezt-journal-export",
      head_seq: data.head_seq ?? -1,
      head_hash: data.head_hash ?? "",
      first_seq: data.first_seq ?? -1,
      last_seq: data.last_seq ?? -1,
      count: data.count ?? (data.events?.length ?? 0),
      truncated: !!data.truncated,
      events: data.events ?? [],
    },
    null,
    2,
  );
}

// JournalExport downloads an integrity-attested journal bundle (M772) — the audit log
// itself, the one thing the per-domain exports (chat/standing/schedules/memory/world)
// couldn't yet give you. Every event ships with its hash and the chain head, so the file
// re-verifies offline. Pairs with JournalIntegrity (M759): verify in place, or take a
// signed copy with you.
export function JournalExport() {
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");

  async function run() {
    setBusy(true);
    setNote("");
    try {
      const data = await getJSON<JournalExportResult>("/api/journal/export");
      downloadText("agezt-journal.json", journalExportBundle(data), "application/json");
      setNote(`${data.count ?? data.events?.length ?? 0} events${data.truncated ? " (truncated)" : ""}`);
    } catch (e) {
      setNote((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <button
      onClick={run}
      disabled={busy}
      title={note || "Download an integrity-attested journal bundle (re-verifiable offline)"}
      className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-0.5 text-[11px] text-muted transition-colors hover:text-foreground disabled:opacity-60"
    >
      {busy ? <Loader2 className="size-3 animate-spin" /> : <Download className="size-3" />}
      {busy ? "exporting…" : "export journal"}
    </button>
  );
}

interface WhyResult {
  events?: AgentEvent[];
  correlation?: string;
  parent_correlation?: string;
  causation_chain?: AgentEvent[];
}

// CausationTrace answers "why did this happen?" for a single journal event (M755).
// It calls /api/why, which walks the causation_id links from the ROOT CAUSE down to
// this event — crossing correlation boundaries the journal-search filters can't (e.g.
// a heartbeat tick → the initiative it spawned → the run that acted). The chain is
// shown oldest-first (root → this); a sub-agent's parent run is surfaced too. Loaded
// on demand (one click) so browsing results stays cheap.
export function CausationTrace({ eventId }: { eventId: string }) {
  const [data, setData] = useState<WhyResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [shown, setShown] = useState(false);

  async function toggle() {
    if (shown) {
      setShown(false);
      return;
    }
    setShown(true);
    if (data || loading) return; // already loaded
    setLoading(true);
    try {
      setData(await getJSON<WhyResult>("/api/why", { event_id: eventId }));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  const chain = data?.causation_chain ?? [];

  return (
    <div className="mt-2 border-t border-border/40 pt-1.5">
      <button
        onClick={toggle}
        className="inline-flex items-center gap-1 text-[11px] text-accent/80 transition-colors hover:text-accent"
        title="Trace what caused this event"
      >
        <GitBranch className="size-3" /> {shown ? "hide cause" : "trace cause"}
        {loading && <Loader2 className="size-3 animate-spin" />}
      </button>
      {shown && !loading && (
        <div className="mt-1.5">
          {err ? (
            <span className="text-[11px] text-bad">{err}</span>
          ) : chain.length > 0 ? (
            <ol className="space-y-0.5">
              {chain.map((c, i) => {
                const cat = categoryOf(c.kind);
                const isLast = i === chain.length - 1;
                return (
                  <li key={c.id || i} className="flex items-center gap-2 text-[11px]">
                    <span className="w-4 shrink-0 text-right tabular-nums text-muted">{i === 0 ? "root" : i + 1}</span>
                    <span className="size-1.5 shrink-0 rounded-full" style={{ background: cat.color }} />
                    <span className="shrink-0 font-medium" style={{ color: cat.color }}>
                      {c.kind}
                    </span>
                    {isIncidentFamilyEvent(c) && <IncidentBadges item={incidentBadgeItem(c)} mono />}
                    <span className="min-w-0 flex-1 truncate text-foreground/70">
                      {incidentEventSummary(c) || c.subject}
                    </span>
                    {isLast && <span className="shrink-0 text-[9px] uppercase tracking-wider text-accent">this</span>}
                    <span className="shrink-0 tabular-nums text-muted">{fmtTime(c.ts_unix_ms)}</span>
                  </li>
                );
              })}
            </ol>
          ) : (
            <span className="text-[11px] text-muted">no upstream cause recorded — this event is a root cause</span>
          )}
          {data?.parent_correlation && (
            <div className="mt-1 text-xs text-muted">
              part of a sub-agent run · parent <span className="font-mono text-foreground/70">{data.parent_correlation.slice(-8)}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  onKey,
  placeholder,
  autoFocus,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  onKey: (e: React.KeyboardEvent) => void;
  placeholder?: string;
  autoFocus?: boolean;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs uppercase tracking-wider text-muted">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKey}
        placeholder={placeholder}
        autoFocus={autoFocus}
        className="h-8 rounded-md border border-border bg-panel px-2 text-xs outline-none focus:border-accent"
      />
    </label>
  );
}
