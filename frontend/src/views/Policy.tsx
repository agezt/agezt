import { useCallback, useEffect, useState } from "react";
import { RefreshCw, ShieldCheck, Trash2, Plus } from "lucide-react";
import { Panel, Stats, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
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

// LEVEL_BAR colours the trust-level distribution from locked-down (L0, red) to
// fully autonomous (L4, green), so the security posture reads at a glance.
const LEVEL_BAR: Record<string, { bar: string; text: string }> = {
  L0: { bar: "bg-bad", text: "text-bad" },
  L1: { bar: "bg-amber-500", text: "text-amber-500" },
  L2: { bar: "bg-amber-500/70", text: "text-amber-500" },
  L3: { bar: "bg-accent", text: "text-accent" },
  L4: { bar: "bg-good", text: "text-good" },
};
const LEVEL_FLOW = ["L0", "L1", "L2", "L3", "L4"] as const;

// LevelSummary shows how the governed capabilities spread across the trust ladder
// — a stacked bar from deny (L0) to allow (L4) plus a count chip per level — so the
// operator can see the overall security posture without reading every row.
function LevelSummary({ levels }: { levels: [string, string][] }) {
  const counts: Record<string, number> = {};
  for (const [, lvl] of levels) counts[lvl] = (counts[lvl] || 0) + 1;
  const present = LEVEL_FLOW.filter((l) => counts[l] > 0);
  const total = levels.length || 1;
  if (present.length === 0) return null;
  return (
    <div className="mb-3 rounded-md border border-border/70 bg-panel/40 p-2.5">
      <div className="flex h-2 overflow-hidden rounded-full bg-panel">
        {present.map((l) => (
          <div
            key={l}
            className={cn("h-full transition-[width] duration-500", LEVEL_BAR[l].bar)}
            style={{ width: `${(counts[l] / total) * 100}%` }}
            title={`${counts[l]} at ${l}`}
          />
        ))}
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]">
        {present.map((l) => (
          <span key={l} className="inline-flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", LEVEL_BAR[l].bar)} />
            <span className={cn("font-semibold tabular-nums", LEVEL_BAR[l].text)}>{counts[l]}</span>
            <span className="text-muted">{l}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

// Policy is the capability control center: it SHOWS each governed capability's
// current trust level and lets the operator GRANT or restrict it at runtime
// (M610) — the "approve" knob. Changing a level posts /api/edict/set_level; the
// engine-wide ask mode and the hard-deny list are editable too. The decision
// stats + log stay below for context.
export function Policy() {
  const ui = useUI();
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

  async function act(
    key: string,
    path: string,
    params: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(key);
    try {
      await postAction(path, params);
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
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
                onChange={(e) => act("mode", "/api/edict/set_mode", { mode: e.target.value }, { success: `Ask mode → ${e.target.value}` })}
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
          <>
          <LevelSummary levels={levels} />
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
                  onChange={(e) => act(cap, "/api/edict/set_level", { capability: cap, level: e.target.value }, { success: `${cap} → ${e.target.value}` })}
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
          </>
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
                      onClick={() =>
                        act(r.name, "/api/edict/deny_rm", { name: r.name }, {
                          confirm: {
                            title: "Remove this deny rule?",
                            message: `“${r.name}” will be deleted — this loosens the security policy.`,
                            confirmLabel: "Remove",
                            danger: true,
                          },
                          success: "Deny rule removed",
                        })
                      }
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
          <DenyAddForm
            capabilities={levels.map(([cap]) => cap)}
            onAdded={(rule) => {
              ui.toast(`Deny rule added — “${rule}” is now blocked`, "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
          />
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

// DenyAddForm adds a runtime hard-deny rule from the UI (M717) — the safety floor
// that blocks any tool call whose input contains a substring, optionally scoped to
// one capability. Adding a rule only tightens policy. It builds the deny-rule spec
// ("substring" or "<capability>:substring") and posts it to edict_deny_add.
export function DenyAddForm({
  capabilities,
  onAdded,
  onError,
}: {
  capabilities: string[];
  onAdded: (rule: string) => void;
  onError: (msg: string) => void;
}) {
  const [substring, setSubstring] = useState("");
  const [scope, setScope] = useState(""); // "" = all capabilities
  const [submitting, setSubmitting] = useState(false);

  const valid = substring.trim() !== "";

  async function add() {
    if (!valid) return;
    const s = substring.trim();
    const rule = scope ? `${scope}:${s}` : s;
    setSubmitting(true);
    try {
      await postAction("/api/edict/deny_add", { rule });
      setSubstring("");
      setScope("");
      onAdded(rule);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5 border-t border-border pt-2">
      <span className="text-[11px] text-muted">add deny rule</span>
      <select
        value={scope}
        onChange={(e) => setScope(e.target.value)}
        aria-label="Deny rule capability scope"
        className="h-7 rounded-md border border-border bg-panel px-1.5 text-xs outline-none focus:border-accent"
      >
        <option value="">all capabilities</option>
        {capabilities.map((c) => (
          <option key={c} value={c}>
            {c}
          </option>
        ))}
      </select>
      <input
        value={substring}
        onChange={(e) => setSubstring(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") add();
        }}
        placeholder="substring to block (e.g. rm -rf)"
        aria-label="Deny rule substring"
        className="h-7 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 font-mono text-xs outline-none focus:border-accent"
      />
      <Button size="sm" onClick={add} disabled={!valid || submitting} title="Add deny rule">
        {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add
      </Button>
    </div>
  );
}
