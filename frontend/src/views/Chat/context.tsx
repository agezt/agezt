import { useEffect, useState } from "react";
import { Gauge, Scissors, Sparkles, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { fmtCount } from "@/lib/format";
import { findModelContext, fmtContext, type ModelCatalog } from "@/lib/models";
import { getJSON } from "@/lib/api";
import { CHARS_PER_TOKEN, contextTokensUsed, type ChatTurn, type TurnCompaction } from "@/lib/chat";

// loadCatalog caches the /api/catalog fetch for the context chips: every turn
// needs the same model→window lookup, so one fetch serves the whole thread.
let catalogPromise: Promise<ModelCatalog> | null = null;
function loadCatalog(): Promise<ModelCatalog> {
  if (!catalogPromise) {
    catalogPromise = getJSON<ModelCatalog>("/api/catalog").catch(() => {
      catalogPromise = null; // transient failure — let a later chip retry
      return {} as ModelCatalog;
    });
  }
  return catalogPromise;
}

// barTone maps window usage to the traffic-light color the bar fills with:
// comfortable (<50%), getting full (<90%), near the limit (≥90%).
export function barTone(pctOfWindow: number): "good" | "warn" | "bad" {
  if (pctOfWindow >= 90) return "bad";
  if (pctOfWindow >= 50) return "warn";
  return "good";
}

// ContextChip is the per-turn context-window gauge (M925): a mini fill bar with
// the usage percentage (when the model's window is known from the catalog, else
// the absolute token count), plus a scissors marker when the loop had to compact.
// Clicking opens the full breakdown modal.
export function ContextChip({ turn }: { turn: ChatTurn }) {
  const [open, setOpen] = useState(false);
  const [windowTokens, setWindowTokens] = useState(0);
  const model = turn.model;
  useEffect(() => {
    if (!model) return;
    let live = true;
    loadCatalog().then((cat) => {
      if (live) setWindowTokens(findModelContext(cat, model));
    });
    return () => {
      live = false;
    };
  }, [model]);
  const c = turn.context;
  if (!c) return null;
  const used = contextTokensUsed(c);
  if (used <= 0) return null;
  const pctOfWindow = windowTokens > 0 ? Math.min(100, (used / windowTokens) * 100) : 0;
  const tone = barTone(pctOfWindow);
  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title="Context window usage — click for the breakdown"
        className="inline-flex items-center gap-1.5 text-xs text-muted transition-colors hover:text-foreground"
      >
        <Gauge className="size-3" />
        {windowTokens > 0 ? (
          <>
            <span className="h-1.5 w-12 overflow-hidden rounded-full bg-panel">
              <span
                className={cn(
                  "block h-full rounded-full",
                  tone === "bad" ? "bg-bad" : tone === "warn" ? "bg-warn" : "bg-good",
                )}
                style={{ width: `${Math.max(2, pctOfWindow)}%` }}
              />
            </span>
            <span className="tabular-nums">{Math.round(pctOfWindow)}%</span>
          </>
        ) : (
          <span className="tabular-nums">{fmtCount(used)} tok</span>
        )}
        {c.compactions.length > 0 && <Scissors className="size-3 text-warn" aria-label="context was compacted" />}
      </button>
      {open && <ContextModal turn={turn} windowTokens={windowTokens} onClose={() => setOpen(false)} />}
    </>
  );
}

// The role split renders in a fixed, meaningful order (what the model reads
// first → last), each with its own shade of the accent so the stacked bar and
// the legend match without leaning on the semantic good/warn/bad colors.
const ROLE_ORDER = ["system", "user", "assistant", "tool"];
const ROLE_FILL: Record<string, string> = {
  system: "bg-accent",
  user: "bg-accent/70",
  assistant: "bg-accent/45",
  tool: "bg-accent/25",
};

function rescuedSkillSummary(count: number, chars: number): string {
  return `${count} skill resource${count === 1 ? "" : "s"} kept · ${fmtCount(chars)} chars`;
}

// ContextModal is the context breakdown (M925): how full the window got, where
// the context came from (role split from the last llm.request), what the
// provider actually billed (token usage incl. cache hits), and what compaction
// elided. Everything it shows is already folded into the turn — no extra fetch.
export function ContextModal({
  turn,
  windowTokens,
  onClose,
}: {
  turn: ChatTurn;
  windowTokens: number;
  onClose: () => void;
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const c = turn.context;
  if (!c) return null;
  const used = contextTokensUsed(c);
  const pctOfWindow = windowTokens > 0 ? Math.min(100, (used / windowTokens) * 100) : 0;
  const tone = barTone(pctOfWindow);
  const byRole = c.byRole || {};
  // Known roles first (reading order), then anything unexpected the loop reported.
  const roles = [
    ...ROLE_ORDER.filter((r) => (byRole[r] || 0) > 0),
    ...Object.keys(byRole).filter((r) => !ROLE_ORDER.includes(r) && byRole[r] > 0),
  ];
  const totalChars = roles.reduce((a, r) => a + byRole[r], 0) || c.chars;
  const section = "px-3 pt-2.5 pb-0.5 text-xs font-semibold uppercase tracking-normal text-muted";

  return (
    <div
      className="modal-overlay fixed inset-0 z-[110] flex items-start justify-center bg-black/50 p-4 pt-[10vh]"
      onClick={onClose}
    >
      <div
        className="modal-in flex max-h-[75vh] w-full max-w-md flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Context window breakdown"
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          <Gauge className="size-4 shrink-0 text-accent" />
          <span className="text-sm font-semibold">Context window</span>
          {turn.model && <span className="truncate font-mono text-xs text-muted">{turn.model}</span>}
          <button onClick={onClose} className="ml-auto shrink-0 text-muted hover:text-foreground" title="Close">
            <X className="size-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-auto pb-3">
          {/* Headline: how full the window got on the last call. */}
          <div className="px-3 pt-3">
            <div className="flex items-baseline justify-between text-sm">
              <span className="font-medium tabular-nums">
                {fmtCount(used)} tokens
                {windowTokens > 0 && <span className="text-muted"> of {fmtContext(windowTokens)} window</span>}
              </span>
              {windowTokens > 0 && (
                <span
                  className={cn(
                    "tabular-nums text-xs font-semibold",
                    tone === "bad" ? "text-bad" : tone === "warn" ? "text-warn" : "text-good",
                  )}
                >
                  {Math.round(pctOfWindow)}%
                </span>
              )}
            </div>
            {windowTokens > 0 ? (
              <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-panel">
                <div
                  className={cn(
                    "h-full rounded-full",
                    tone === "bad" ? "bg-bad" : tone === "warn" ? "bg-warn" : "bg-good",
                  )}
                  style={{ width: `${Math.max(2, pctOfWindow)}%` }}
                />
              </div>
            ) : (
              <p className="mt-1 text-xs text-muted">window size unknown — model not in the catalog</p>
            )}
          </div>

          {/* Composition: where the last request's context came from. */}
          {roles.length > 0 && (
            <>
              <div className={section}>Composition</div>
              <div className="px-3">
                <div className="flex h-2 overflow-hidden rounded-full bg-panel">
                  {roles.map((r) => (
                    <div
                      key={r}
                      className={ROLE_FILL[r] || "bg-accent/25"}
                      style={{ width: `${(byRole[r] / totalChars) * 100}%` }}
                      title={r}
                    />
                  ))}
                </div>
                <div className="mt-1.5 space-y-1">
                  {roles.map((r) => (
                    <div key={r} className="flex items-center gap-2 text-xs">
                      <span className={cn("size-2 shrink-0 rounded-full", ROLE_FILL[r] || "bg-accent/25")} />
                      <span className="w-20 capitalize">{r}</span>
                      <span className="tabular-nums text-muted">
                        ≈{fmtCount(byRole[r] / CHARS_PER_TOKEN)} tok · {fmtCount(byRole[r])} chars
                      </span>
                      <span className="ml-auto tabular-nums text-muted">{Math.round((byRole[r] / totalChars) * 100)}%</span>
                    </div>
                  ))}
                </div>
              </div>
            </>
          )}

          {/* Provider-billed tokens, across every iteration of the run. */}
          {(c.inputTokens > 0 || c.outputTokens > 0) && (
            <>
              <div className={section}>
                Tokens billed{turn.iters > 1 ? ` · ${turn.iters} iterations` : ""}
              </div>
              <div className="space-y-1 px-3 text-xs">
                <div className="flex justify-between">
                  <span>Input</span>
                  <span className="tabular-nums text-muted">
                    {fmtCount(c.inputTokens)}
                    {c.cachedTokens > 0 && <span className="text-good"> · {fmtCount(c.cachedTokens)} cached</span>}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span>Output</span>
                  <span className="tabular-nums text-muted">{fmtCount(c.outputTokens)}</span>
                </div>
                {c.cacheWriteTokens > 0 && (
                  <div className="flex justify-between">
                    <span>Cache write</span>
                    <span className="tabular-nums text-muted">{fmtCount(c.cacheWriteTokens)}</span>
                  </div>
                )}
              </div>
            </>
          )}

          {/* Compaction: what the loop elided to stay inside the budget. */}
          <div className={section}>Compaction</div>
          <div className="px-3 text-xs">
            {c.compactions.length === 0 ? (
              <p className="text-muted">None needed — the context fit the budget.</p>
            ) : (
              <div className="space-y-1">
                {c.compactions.map((e, i) => {
                  const rescuedCount = e.skillRescuedCount || 0;
                  const rescuedChars = e.skillRescuedChars || 0;
                  return (
                    <div key={i} className="space-y-0.5">
                      <div className="flex items-center gap-1.5">
                        <Scissors className="size-3 shrink-0 text-warn" />
                        <span>
                          {e.elided} tool output{e.elided === 1 ? "" : "s"} elided · reclaimed {fmtCount(e.reclaimedChars)}{" "}
                          chars
                          {e.beforeChars > 0 && (
                            <span className="text-muted">
                              {" "}
                              ({fmtCount(e.beforeChars)} → {fmtCount(e.afterChars)})
                            </span>
                          )}
                        </span>
                      </div>
                      {rescuedCount > 0 && (
                        <div className="ml-4 flex items-center gap-1.5 text-good">
                          <Sparkles className="size-3 shrink-0" />
                          <span>{rescuedSkillSummary(rescuedCount, rescuedChars)}</span>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          <p className="px-3 pt-3 text-xs leading-snug text-muted/70">
            Composition is measured in characters (≈{CHARS_PER_TOKEN} chars/token estimate) and covers the system
            prompt + messages — tool definitions add to the billed input on top. Token totals are provider-reported;
            the gauge uses the last call's real prompt tokens when available.
          </p>
        </div>
      </div>
    </div>
  );
}


// CompactionNote surfaces that the loop trimmed its own context mid-run (M925) —
// the same visibility rule as FallbackNote: when the run quietly did something
// that changes what the model saw, say so right in the thread.
export function CompactionNote({ events }: { events: TurnCompaction[] }) {
  const elided = events.reduce((a, e) => a + e.elided, 0);
  const reclaimed = events.reduce((a, e) => a + e.reclaimedChars, 0);
  const rescuedCount = events.reduce((a, e) => a + (e.skillRescuedCount || 0), 0);
  const rescuedChars = events.reduce((a, e) => a + (e.skillRescuedChars || 0), 0);
  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-md bg-panel/40 px-2 py-1 text-xs text-muted">
      <Scissors className="size-3.5 shrink-0" />
      <span className="font-medium text-foreground/80">context compacted</span>
      <span>
        {elided} old tool output{elided === 1 ? "" : "s"} elided · {fmtCount(reclaimed)} chars reclaimed
      </span>
      {rescuedCount > 0 && <span className="text-good">{rescuedSkillSummary(rescuedCount, rescuedChars)}</span>}
    </div>
  );
}
