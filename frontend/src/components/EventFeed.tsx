import { useEffect, useMemo, useRef, useState } from "react";
import { Pause, Play, Search, X } from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { categoryOf, isErrorKind, CATEGORIES } from "@/lib/eventmeta";
import { cn, fmtTime } from "@/lib/utils";
import { DataView } from "@/components/DataView";
import { IncidentBadges } from "@/components/IncidentBadges";
import {
  incidentBadgeItem,
  incidentEventSummary,
  isIncidentFamilyEvent,
} from "@/lib/incidentevents";

// EventFeed is the live stream console: the daemon's whole journal firehose,
// colour-coded by category, filterable by category + free text + correlation,
// pausable, with a click-to-expand payload. The nervous system, observable.
export function EventFeed() {
  const { events, connected } = useEvents();
  const [text, setText] = useState("");
  const [off, setOff] = useState<Set<string>>(new Set()); // categories toggled OFF
  const [corr, setCorr] = useState<string | null>(null);
  const [paused, setPaused] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  // Pause freezes the rendered snapshot so reading isn't disrupted by new rows.
  const frozen = useRef<AgentEvent[]>([]);
  useEffect(() => {
    if (!paused) frozen.current = events;
  }, [paused, events]);
  const source = paused ? frozen.current : events;

  // Per-category live counts (over the unfiltered stream).
  const counts = useMemo(() => {
    const m: Record<string, number> = {};
    for (const e of events) {
      const k = categoryOf(e.kind).key;
      m[k] = (m[k] || 0) + 1;
    }
    return m;
  }, [events]);

  const f = text.trim().toLowerCase();
  const shown = useMemo(
    () =>
      source.filter((e) => {
        if (off.has(categoryOf(e.kind).key)) return false;
        if (corr && e.correlation_id !== corr) return false;
        if (f) {
          const hay = `${e.kind} ${e.subject} ${e.actor} ${e.correlation_id}`.toLowerCase();
          if (!hay.includes(f)) return false;
        }
        return true;
      }),
    [source, off, corr, f],
  );

  function toggle(key: string) {
    setOff((prev) => {
      const n = new Set(prev);
      if (n.has(key)) n.delete(key);
      else n.add(key);
      return n;
    });
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-2">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold">Live stream</h2>
        <span className={cn("inline-flex items-center gap-1 text-xs", connected ? "text-good" : "text-bad")}>
          ● {connected ? "live" : "off"}
        </span>
        <span className="text-xs text-muted">
          {shown.length}/{events.length}
        </span>
        <div className="relative ml-auto">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="search kind / subject / actor / id"
            className="h-7 w-56 rounded-md border border-border bg-panel pl-7 pr-2 text-xs outline-none focus:border-accent"
          />
        </div>
        <button
          onClick={() => setPaused((p) => !p)}
          className={cn(
            "inline-flex h-7 items-center gap-1 rounded-md border px-2 text-xs transition-colors",
            paused ? "border-accent text-accent" : "border-border hover:border-accent",
          )}
          title={paused ? "Resume" : "Pause"}
        >
          {paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
          {paused ? "paused" : "live"}
        </button>
      </div>

      {/* Category chips */}
      <div className="flex flex-wrap gap-1.5">
        {CATEGORIES.map((c) => {
          const isOff = off.has(c.key);
          const count = counts[c.key] || 0;
          return (
            <button
              key={c.key}
              onClick={() => toggle(c.key)}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-[11px] transition-opacity",
                isOff && "opacity-40",
              )}
            >
              <span className="size-2 rounded-full" style={{ background: c.color }} />
              {c.label}
              {count > 0 && <span className="tabular-nums text-muted">{count}</span>}
            </button>
          );
        })}
        {corr && (
          <button
            onClick={() => setCorr(null)}
            className="inline-flex items-center gap-1 rounded-full border border-accent px-2 py-0.5 text-[11px] text-accent"
          >
            <X className="size-3" /> {corr.slice(0, 14)}…
          </button>
        )}
      </div>

      {/* Stream */}
      <div className="min-h-0 flex-1 overflow-auto rounded-lg border border-border bg-card font-mono text-xs">
        {shown.length === 0 ? (
          <div className="p-3 text-muted">no events match</div>
        ) : (
          <ul className="divide-y divide-border/40">
            {shown.map((e, i) => {
              const cat = categoryOf(e.kind);
              const err = isErrorKind(e.kind);
              const id = e.id || `${e.seq}-${i}`;
              const isOpen = open === id;
              return (
                <li key={id} className={cn(err && "bg-bad/5")}>
                  <div
                    onClick={() => setOpen(isOpen ? null : id)}
                    className="flex cursor-pointer items-center gap-2 px-2.5 py-1 hover:bg-panel/60"
                  >
                    <span className="w-14 shrink-0 tabular-nums text-muted">{fmtTime(e.ts_unix_ms)}</span>
                    <span className="size-2 shrink-0 rounded-full" style={{ background: cat.color }} />
                    <span className="w-40 shrink-0 truncate font-medium" style={{ color: err ? undefined : cat.color }}>
                      <span className={cn(err && "text-bad")}>{e.kind}</span>
                    </span>
                    {isIncidentFamilyEvent(e) && <IncidentBadges item={incidentBadgeItem(e)} mono />}
                    <span className="min-w-0 flex-1 truncate text-foreground/80">
                      {incidentEventSummary(e) || e.subject}
                    </span>
                    {e.correlation_id && (
                      <button
                        onClick={(ev) => {
                          ev.stopPropagation();
                          setCorr(e.correlation_id || null);
                        }}
                        className="shrink-0 truncate text-[10px] text-muted hover:text-accent"
                        title="filter to this run"
                      >
                        {e.correlation_id.slice(-6)}
                      </button>
                    )}
                  </div>
                  {isOpen && (
                    <div className="border-t border-border/40 bg-panel/40 px-3 py-2">
                      <div className="mb-1 flex gap-3 text-[10px] text-muted">
                        <span>seq {e.seq ?? "—"}</span>
                        <span>actor {e.actor || "—"}</span>
                        <span>{cat.label}</span>
                      </div>
                      {e.payload != null ? (
                        <DataView data={e.payload} />
                      ) : (
                        <span className="text-[11px] text-muted">no payload</span>
                      )}
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
