import { useCallback, useEffect, useState } from "react";
import { RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { Panel, Stats, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { LogDetail } from "@/components/LogDetail";
import { getJSON, postAction } from "@/lib/api";
import { byDescValue, pct } from "@/lib/format";
import { cn, fmtTime } from "@/lib/utils";

interface EdictShow {
  ask_policy?: string;
  levels?: Record<string, string>;
  hard_deny?: { name: string; substring?: string; applies_to?: string[] | null }[];
}

// The trust ladder (edict L0..L4). Lower = more restrictive.
const LEVELS: { value: string; label: string }[] = [
  { value: "L0", label: "L0 · deny" },
  { value: "L1", label: "L1 · ask" },
  { value: "L2", label: "L2 · ask-first" },
  { value: "L3", label: "L3 · ask-scoped" },
  { value: "L4", label: "L4 · allow" },
];
const MODES = ["allow", "prompt", "deny"];

function levelTone(level: string): string {
  if (level === "L4") return "text-good border-good/40";
  if (level === "L0") return "text-bad border-bad/40";
  return "text-accent border-accent/40";
}

// Policy is the capability control center: it SHOWS each governed capability's
// current trust level and lets the operator GRANT or restrict it at runtime
// (M610) — the "approve" knob. Changing a level posts /api/edict/set_level; the
// engine-wide ask mode and the hard-deny list are editable too. The decision
// stats + log stay below for context.
export function Policy() {
  const [show, setShow] = useState<EdictShow | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      setShow(await getJSON<EdictShow>("/api/edict_show"));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  async function act(key: string, path: string, params: Record<string, string>) {
    setBusy(key);
    setErr(null);
    try {
      await postAction(path, params);
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  const levels = show?.levels ? Object.entries(show.levels).sort((a, b) => a[0].localeCompare(b[0])) : [];
  const denies = show?.hard_deny ?? [];

  return (
    <div className="space-y-4">
      {/* Control center */}
      <div className="rounded-lg border border-border bg-card p-3">
        <div className="mb-3 flex items-center gap-2">
          <ShieldCheck className="size-4 text-accent" />
          <h2 className="text-sm font-semibold">Capability policy</h2>
          <div className="ml-auto flex items-center gap-2">
            <label className="flex items-center gap-1.5 text-xs text-muted">
              ask mode
              <select
                value={show?.ask_policy || "allow"}
                disabled={busy === "mode"}
                onChange={(e) => act("mode", "/api/edict/set_mode", { mode: e.target.value })}
                className="h-7 rounded-md border border-border bg-panel px-1.5 text-xs outline-none focus:border-accent disabled:opacity-50"
              >
                {MODES.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            </label>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </div>
        </div>

        {err && <div className="mb-2 text-xs text-bad">{err}</div>}

        {levels.length === 0 ? (
          <div className="text-xs text-muted">{loading ? "loading…" : "no governed capabilities"}</div>
        ) : (
          <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
            {levels.map(([cap, lvl]) => (
              <div
                key={cap}
                className="flex items-center gap-2 rounded-md border border-border/70 bg-panel/40 px-2.5 py-1.5"
              >
                <span className="truncate font-mono text-xs">{cap}</span>
                <select
                  value={lvl}
                  disabled={busy === cap}
                  onChange={(e) => act(cap, "/api/edict/set_level", { capability: cap, level: e.target.value })}
                  className={cn(
                    "ml-auto h-7 rounded-md border bg-card px-1.5 text-xs tabular-nums outline-none focus:border-accent disabled:opacity-50",
                    levelTone(lvl),
                  )}
                >
                  {LEVELS.map((l) => (
                    <option key={l.value} value={l.value}>
                      {l.label}
                    </option>
                  ))}
                </select>
              </div>
            ))}
          </div>
        )}

        {/* Hard-deny rules */}
        <div className="mt-3">
          <Count>hard-deny rules ({denies.length})</Count>
          {denies.length === 0 ? (
            <div className="text-xs text-muted">none</div>
          ) : (
            <ul className="space-y-1">
              {denies.map((r) => (
                <li key={r.name} className="flex items-center gap-2 text-xs">
                  <Badge variant="bad">deny</Badge>
                  <span className="font-mono">{r.name}</span>
                  {r.substring && <span className="text-muted">“{r.substring}”</span>}
                  {r.applies_to && r.applies_to.length > 0 && (
                    <span className="text-muted">→ {r.applies_to.join(", ")}</span>
                  )}
                  {/* Only runtime-added rules (named runtime[N]) are removable. */}
                  {r.name.startsWith("runtime") && (
                    <button
                      onClick={() => act(r.name, "/api/edict/deny_rm", { name: r.name })}
                      disabled={busy === r.name}
                      title="Remove this runtime deny rule"
                      className="ml-auto text-muted transition-colors hover:text-bad disabled:opacity-50"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {/* Decision stats + log (existing read-only view) */}
      <Panel<Record<string, any>> title="Decisions" path="/api/policy">
        {(d) => {
          const byCap: Record<string, number> = d.denied_by_capability || {};
          const names = byDescValue(byCap);
          return (
            <>
              <Stats
                pairs={[
                  ["allowed", d.allowed || 0],
                  ["denied", `${d.denied || 0}${d.hard_denied ? ` (${d.hard_denied} hard)` : ""}`],
                  ["denial rate", pct(d.denial_rate, d.total)],
                ]}
              />
              {names.length > 0 && (
                <div>
                  <Count>denied by capability</Count>
                  {names.map((n) => (
                    <Row key={n}>
                      <Badge variant="bad">{byCap[n]}</Badge>
                      <span>{n}</span>
                    </Row>
                  ))}
                </div>
              )}
              <LogDetail
                label="decision log"
                path="/api/policy_log"
                params={{ limit: "40" }}
                extract={(x) => x.decisions || []}
                render={(ev: any, i) => (
                  <div key={i} className="flex gap-2 border-b border-border/40 py-0.5">
                    <span className="text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                    <span className={ev.allow ? "text-good" : "text-bad"}>
                      {ev.allow ? "allow" : ev.hard_denied ? "DENY(hard)" : "DENY"} {ev.capability || ""}{" "}
                      {ev.tool || ""}
                    </span>
                    {ev.reason ? <span className="truncate text-muted">{ev.reason}</span> : null}
                  </div>
                )}
              />
            </>
          );
        }}
      </Panel>
    </div>
  );
}
