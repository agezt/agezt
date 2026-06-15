import { useEffect, useRef, useState } from "react";
import { CalendarClock, RefreshCw, Play, Pause, Trash2, Bot, Heart, Infinity as InfinityIcon, ShieldCheck, Plus, X, Pencil, Download, Upload } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { downloadText } from "@/lib/export";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { PageHeader } from "@/components/ui/page-header";

interface Sched {
  id: string;
  intent?: string;
  cadence?: string;
  mode?: string;
  source?: string;
  enabled?: boolean;
  next_run_unix?: number;
  last_status?: string;
  fires?: number;
  assure?: number;
}

// sourceTone colours the origin badge: an agent-scheduled run (the agent used
// the `schedule` tool to arrange its own future work) is the notable one, so it
// gets the accent; operator/env are muted.
function sourceTone(src?: string): string {
  if (src === "agent") return "bg-accent/15 text-accent";
  return "bg-panel text-muted";
}

// untilLabel renders a glanceable countdown to the next fire (M917): "now",
// "in 45s", "in 12m", "in 3h", "in 2d", or "overdue" when it's in the past.
// Pure + unit-tested; nowMs is injected so it's deterministic.
export function untilLabel(nextUnixMs: number, nowMs: number): string {
  const d = nextUnixMs - nowMs;
  if (d < -1000) return "overdue";
  if (d < 15_000) return "now";
  const s = Math.round(d / 1000);
  if (s < 90) return `in ${s}s`;
  const m = Math.round(s / 60);
  if (m < 90) return `in ${m}m`;
  const h = Math.round(m / 60);
  if (h < 36) return `in ${h}h`;
  return `in ${Math.round(h / 24)}d`;
}

// DUE_SOON_MS: a schedule firing within this window counts as "due soon" for the
// summary band — the ones worth glancing at.
export const DUE_SOON_MS = 60 * 60 * 1000;

export interface SchedCounts {
  total: number;
  enabled: number;
  paused: number;
  dueSoon: number;
}

// scheduleCounts tallies the summary band: enabled vs paused, and how many enabled
// schedules fire within the due-soon window. Pure + unit-tested.
export function scheduleCounts(items: { enabled?: boolean; next_run_unix?: number }[], nowMs: number): SchedCounts {
  let enabled = 0;
  let dueSoon = 0;
  for (const s of items) {
    const on = s.enabled !== false;
    if (on) enabled++;
    if (on && s.next_run_unix) {
      const d = s.next_run_unix * 1000 - nowMs;
      if (d <= DUE_SOON_MS) dueSoon++;
    }
  }
  return { total: items.length, enabled, paused: items.length - enabled, dueSoon };
}

// parseSchedulesJSON normalises an exported schedules file into a list of
// re-addable `schedule_add` arg objects. Accepts a bare array or a {schedules:[…]}
// wrapper (the list shape). For each entry it rebuilds the cadence args from the
// stored mode — interval (interval_sec), daily (at_minutes+days+tz), window
// (window_start/end+interval_sec+days+tz) or once (once_at_unix) — dropping kernel
// identity/runtime fields (id/source/enabled/fires/…). Continuous schedules are
// agent-managed and have no `schedule_add` path, so they're skipped. Keeps only
// entries with an intent and a valid cadence; throws on bad JSON / nothing valid.
export function parseSchedulesJSON(text: string): Record<string, unknown>[] {
  const data = JSON.parse(text);
  const arr = Array.isArray(data)
    ? data
    : Array.isArray((data as { schedules?: unknown[] })?.schedules)
      ? (data as { schedules: unknown[] }).schedules
      : null;
  if (!arr) throw new Error("expected an array of schedules (or a {schedules:[…]} wrapper)");
  const out: Record<string, unknown>[] = [];
  for (const raw of arr) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;
    const s = raw as Record<string, unknown>;
    const intent = typeof s.intent === "string" ? s.intent.trim() : "";
    if (!intent) continue;
    const num = (k: string) => (typeof s[k] === "number" ? (s[k] as number) : undefined);
    const mode = typeof s.mode === "string" ? s.mode : "";
    const args: Record<string, unknown> = { intent };
    if (typeof s.model === "string" && s.model) args.model = s.model;
    if (mode === "once") {
      const at = num("once_at_unix") ?? num("next_run_unix");
      if (!at) continue; // a one-shot with no fire time can't be re-added
      args.once_at_unix = at;
    } else if (mode === "daily") {
      const at = num("at_minutes");
      if (at === undefined) continue;
      args.at_minutes = at;
      args.days = num("days") ?? 0;
      if (typeof s.tz === "string" && s.tz) args.tz = s.tz;
    } else if (mode === "window") {
      const start = num("at_minutes");
      const end = num("end_minutes");
      const sec = num("interval_sec");
      if (start === undefined || end === undefined || !sec) continue;
      args.window_start = start;
      args.window_end = end;
      args.interval_sec = sec;
      args.days = num("days") ?? 0;
      if (typeof s.tz === "string" && s.tz) args.tz = s.tz;
    } else if (mode === "" || mode === "interval") {
      const sec = num("interval_sec");
      if (!sec || sec < 1) continue;
      args.interval_sec = sec;
    } else {
      continue; // continuous / unknown mode — no schedule_add path
    }
    out.push(args);
  }
  if (out.length === 0) throw new Error("no re-addable schedules (each needs an intent and a valid cadence) found");
  return out;
}

// Schedules is the autonomy cockpit: every scheduled intent — whether an
// operator added it, an AGEZT_SCHEDULE env job, or the AGENT scheduled it itself
// — with its cadence, next fire, last outcome and origin, plus run-now /
// pause-resume / remove controls so you can manage what fires unattended.
export function Schedules() {
  const ui = useUI();
  const [items, setItems] = useState<Sched[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  // Fire-time preview (M744): the schedule id whose next fires are shown + the times.
  const [forecast, setForecast] = useState<{ id: string; times: number[] } | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  // A coarse clock so the "fires in …" countdowns stay live without refetching (M917).
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 5000);
    return () => clearInterval(t);
  }, []);

  function exportSchedules() {
    downloadText("agezt-schedules.json", JSON.stringify({ version: 1, schedules: items ?? [] }, null, 2), "application/json");
  }

  // Restore schedules from a file: re-add each via schedule_add (the daemon mints
  // fresh ids and validates). ADDS — importing onto a daemon that already has them
  // creates duplicates; hence the explicit Import action.
  async function importSchedules(file: File) {
    try {
      const list = parseSchedulesJSON(await file.text());
      let added = 0;
      for (const args of list) {
        try {
          await postJSON("/api/schedule/add", args);
          added++;
        } catch {
          /* skip one the daemon rejects; keep importing the rest */
        }
      }
      ui.toast(`Imported ${added}/${list.length} schedule${list.length === 1 ? "" : "s"}`, added ? "success" : "error");
      void reload();
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  async function previewFires(id: string) {
    if (forecast?.id === id) {
      setForecast(null);
      return;
    }
    try {
      const d = await getJSON<{ forecasts?: { unix: number }[] }>("/api/schedule/test", { id, count: "5" });
      setForecast({ id, times: (d.forecasts || []).map((f) => f.unix) });
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ schedules?: Sched[] }>("/api/schedules");
      setItems(d.schedules || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 8000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(
    id: string,
    path: string,
    params?: Record<string, string>,
    opts?: { confirm?: ConfirmOptions; success?: string },
  ) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(id);
    try {
      await postAction(path, { id, ...params });
      if (opts?.success) ui.toast(opts.success, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const agentCount = items?.filter((s) => s.source === "agent").length ?? 0;

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={CalendarClock}
        title="Schedules"
        description={
          <>
            {items ? `${items.length} total` : "Manage every scheduled intent that fires unattended"}
            {agentCount > 0 && <span className="text-accent"> · {agentCount} agent-scheduled</span>}
          </>
        }
        actions={
          <>
            <Button size="sm" onClick={() => setShowForm((v) => !v)} title="Create a schedule">
              {showForm ? <X className="size-3.5" /> : <Plus className="size-3.5" />} New schedule
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              aria-hidden="true"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) void importSchedules(f);
                e.target.value = "";
              }}
            />
            <Button variant="ghost" size="sm" onClick={() => fileRef.current?.click()} title="Import schedules from a file">
              <Upload className="size-3.5" /> Import
            </Button>
            <Button variant="ghost" size="sm" onClick={exportSchedules} disabled={!items || items.length === 0} title="Export schedules to a file">
              <Download className="size-3.5" /> Export
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {showForm && (
        <NewScheduleForm
          onCreated={() => {
            setShowForm(false);
            ui.toast("Schedule created", "success");
            void reload();
          }}
          onError={(m) => ui.toast(m, "error")}
        />
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !items ? (
        <SkeletonList count={4} lines={2} />
      ) : items.length === 0 ? (
        <EmptyState
          icon={CalendarClock}
          title="No schedules yet"
          hint={
            <>
              Hit <span className="font-medium text-foreground/80">New schedule</span> above to add one — the agent can
              also schedule its own future work with the <code className="rounded bg-panel px-1 py-0.5">schedule</code> tool.
            </>
          }
        />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          {/* Summary band (M917): the schedule fleet at a glance — how many are
              live, paused, and about to fire within the hour. */}
          {(() => {
            const c = scheduleCounts(items, now);
            return (
              <div className="mb-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                <SchedStat label="schedules" value={c.total} />
                <SchedStat label="enabled" value={c.enabled} accent={c.enabled > 0} />
                <SchedStat label="paused" value={c.paused} />
                <SchedStat label="due within 1h" value={c.dueSoon} accent={c.dueSoon > 0} />
              </div>
            );
          })()}
          <ul className="space-y-2">
            {items.map((s) => (
              <li key={s.id} className="glass rounded-xl p-3">
                <div className="flex items-center gap-2">
                  <Badge>
                    {s.mode === "continuous" && <InfinityIcon className="mr-1 inline size-3 align-[-1px]" />}
                    {s.cadence || s.mode || "?"}
                  </Badge>
                  <span
                    className={cn(
                      "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
                      sourceTone(s.source),
                    )}
                    title={`source: ${s.source || "?"}`}
                  >
                    {s.source === "agent" && <Bot className="size-3" />}
                    {s.source || "?"}
                  </span>
                  {s.mode === "continuous" && s.enabled !== false && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-bad/10 px-1.5 py-0.5 text-[10px] font-semibold text-bad"
                      title={`alive — ${s.fires ?? 0} cycle${s.fires === 1 ? "" : "s"} completed`}
                    >
                      <Heart className="size-3 animate-pulse fill-current" />
                      {s.fires ?? 0}
                    </span>
                  )}
                  {(s.assure ?? 0) > 0 && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-good/10 px-1.5 py-0.5 text-[10px] font-semibold text-good"
                      title={`do-it-for-sure: each firing verifies completion and retries up to ${s.assure}×`}
                    >
                      <ShieldCheck className="size-3" />
                      assured {s.assure}×
                    </span>
                  )}
                  {s.enabled === false && <span className="text-[10px] text-muted">(paused)</span>}
                  {s.last_status && <Badge variant={statusVariant(s.last_status)}>{s.last_status}</Badge>}
                  <div className="ml-auto flex items-center gap-1.5">
                    <button
                      onClick={() => act(s.id, "/api/schedule/run", undefined, { success: "Schedule triggered" })}
                      disabled={busy === s.id}
                      title="Run now"
                      className="text-muted transition-colors hover:text-accent disabled:opacity-50"
                    >
                      <Play className="size-3.5" />
                    </button>
                    <button
                      onClick={() =>
                        act(s.id, "/api/schedule/enable", { enabled: s.enabled === false ? "true" : "false" }, {
                          success: s.enabled === false ? "Schedule resumed" : "Schedule paused",
                        })
                      }
                      disabled={busy === s.id}
                      title={s.enabled === false ? "Resume" : "Pause"}
                      className="text-muted transition-colors hover:text-foreground disabled:opacity-50"
                    >
                      {s.enabled === false ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
                    </button>
                    <button
                      onClick={() => setEditingId((cur) => (cur === s.id ? null : s.id))}
                      disabled={busy === s.id}
                      title={editingId === s.id ? "Close editor" : "Edit"}
                      className={cn(
                        "transition-colors disabled:opacity-50",
                        editingId === s.id ? "text-accent" : "text-muted hover:text-accent",
                      )}
                    >
                      <Pencil className="size-3.5" />
                    </button>
                    <button
                      onClick={() =>
                        act(s.id, "/api/schedule/remove", undefined, {
                          confirm: {
                            title: "Remove this schedule?",
                            message: s.intent
                              ? `“${s.intent}” will stop firing and be permanently deleted.`
                              : "This schedule will stop firing and be permanently deleted.",
                            confirmLabel: "Remove",
                            danger: true,
                          },
                          success: "Schedule removed",
                        })
                      }
                      disabled={busy === s.id}
                      title="Remove"
                      className="text-muted transition-colors hover:text-bad disabled:opacity-50"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                </div>
                <div className="mt-1.5 text-sm">{s.intent || s.id}</div>
                <div className="mt-1 flex flex-wrap items-center gap-x-3 text-[10px] text-muted">
                  {s.enabled !== false && s.next_run_unix ? (
                    <span className="inline-flex items-center gap-1">
                      next {fmtDateTime(s.next_run_unix * 1000)}
                      <span
                        className={cn(
                          "rounded px-1 py-0.5 font-semibold tabular-nums",
                          s.next_run_unix * 1000 - now <= DUE_SOON_MS ? "bg-accent/15 text-accent" : "bg-panel",
                        )}
                      >
                        {untilLabel(s.next_run_unix * 1000, now)}
                      </span>
                    </span>
                  ) : null}
                  {s.mode !== "continuous" && (s.fires ?? 0) > 0 && (
                    <span>{s.fires} run{s.fires === 1 ? "" : "s"}</span>
                  )}
                  {s.mode !== "continuous" && (
                    <button onClick={() => previewFires(s.id)} className="text-accent/80 transition-colors hover:text-accent" title="Preview the next fire times">
                      {forecast?.id === s.id ? "hide fires" : "next fires"}
                    </button>
                  )}
                  <span className="font-mono opacity-70">{s.id}</span>
                </div>
                {forecast?.id === s.id && (
                  <ol className="mt-1.5 space-y-0.5 rounded-md border border-border/60 bg-panel/40 p-2 text-[11px]">
                    {forecast.times.length === 0 ? (
                      <li className="text-muted">no upcoming fires (paused, past one-shot, or no matching times)</li>
                    ) : (
                      forecast.times.map((t, i) => (
                        <li key={i} className="flex items-center gap-2">
                          <span className="w-4 text-right tabular-nums text-muted">{i + 1}.</span>
                          <span className="text-foreground/85">{fmtDateTime(t * 1000)}</span>
                        </li>
                      ))
                    )}
                  </ol>
                )}
                {editingId === s.id && (
                  <div className="mt-2">
                    <NewScheduleForm
                      editId={s.id}
                      initialIntent={s.intent}
                      onCreated={() => {
                        setEditingId(null);
                        ui.toast("Schedule updated", "success");
                        void reload();
                      }}
                      onError={(m) => ui.toast(m, "error")}
                    />
                  </div>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function SchedStat({ label, value, accent }: { label: string; value: number; accent?: boolean }) {
  return (
    <div className={cn("rounded-lg border bg-card p-2.5", accent ? "border-accent/50" : "border-border")}>
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}

// NewScheduleForm creates OR edits a scheduled intent from the UI (M715 create;
// M728 edit) — recurring or one-shot — so managing unattended work no longer needs
// the CLI. It captures the common timings (every N, daily at a time, once at a
// moment) and posts to schedule_add / schedule_edit, which pick the cadence branch
// by which timing arg is present. When `editId` is set the form prefills the intent,
// posts an edit (the id rides along), and the button reads "Save changes".
export function NewScheduleForm({
  onCreated,
  onError,
  editId,
  initialIntent,
}: {
  onCreated: () => void;
  onError: (msg: string) => void;
  // When set, the form edits this schedule instead of creating a new one (M728).
  editId?: string;
  initialIntent?: string;
}) {
  type Mode = "interval" | "daily" | "once";
  const editing = !!editId;
  const [intent, setIntent] = useState(initialIntent ?? "");
  const [mode, setMode] = useState<Mode>("interval");
  const [everyN, setEveryN] = useState("30");
  const [everyUnit, setEveryUnit] = useState<"minutes" | "hours">("minutes");
  const [dailyAt, setDailyAt] = useState("09:00");
  const [onceAt, setOnceAt] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const intervalSec = Math.max(1, Number(everyN) || 0) * (everyUnit === "hours" ? 3600 : 60);
  const validTiming =
    (mode === "interval" && Number(everyN) > 0) ||
    (mode === "daily" && /^\d{1,2}:\d{2}$/.test(dailyAt)) ||
    (mode === "once" && onceAt !== "");
  const valid = intent.trim() !== "" && validTiming;

  async function create() {
    if (!valid) return;
    const args: Record<string, unknown> = { intent: intent.trim() };
    if (editing) args.id = editId;
    if (mode === "interval") {
      args.interval_sec = intervalSec;
    } else if (mode === "daily") {
      const [h, m] = dailyAt.split(":").map(Number);
      args.at_minutes = h * 60 + m;
      args.days = 0; // every day
    } else {
      const ms = Date.parse(onceAt);
      if (Number.isNaN(ms)) return onError("Invalid date/time");
      args.once_at_unix = Math.floor(ms / 1000);
    }
    setSubmitting(true);
    try {
      await postJSON(editing ? "/api/schedule/edit" : "/api/schedule/add", args);
      onCreated();
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="glass rounded-xl p-3">
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Intent
        <textarea
          value={intent}
          onChange={(e) => setIntent(e.target.value)}
          placeholder="What the agent should do when this fires…"
          aria-label="Schedule intent"
          className="h-16 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
        />
      </label>

      <div className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        When
        <div className="flex flex-wrap items-center gap-1.5">
          <div className="inline-flex overflow-hidden rounded-md border border-border">
            {(["interval", "daily", "once"] as const).map((m) => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className={cn(
                  "px-2 py-1 text-xs transition-colors",
                  mode === m ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
                )}
              >
                {m === "interval" ? "every…" : m === "daily" ? "daily at…" : "once at…"}
              </button>
            ))}
          </div>

          {mode === "interval" && (
            <div className="flex items-center gap-1.5">
              <input
                type="number"
                min={1}
                value={everyN}
                onChange={(e) => setEveryN(e.target.value)}
                aria-label="Interval amount"
                className="w-20 rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
              <select
                value={everyUnit}
                onChange={(e) => setEveryUnit(e.target.value as "minutes" | "hours")}
                aria-label="Interval unit"
                className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              >
                <option value="minutes">minutes</option>
                <option value="hours">hours</option>
              </select>
            </div>
          )}
          {mode === "daily" && (
            <input
              type="time"
              value={dailyAt}
              onChange={(e) => setDailyAt(e.target.value)}
              aria-label="Daily time"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          )}
          {mode === "once" && (
            <input
              type="datetime-local"
              value={onceAt}
              onChange={(e) => setOnceAt(e.target.value)}
              aria-label="Once date and time"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          )}
        </div>
      </div>

      {editing && (
        <p className="mt-2 text-[10px] text-muted">
          Editing changes the intent and replaces the cadence with the timing chosen above.
        </p>
      )}
      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />}{" "}
          {editing ? "Save changes" : "Create schedule"}
        </Button>
      </div>
    </div>
  );
}
