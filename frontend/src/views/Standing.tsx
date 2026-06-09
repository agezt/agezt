import { useEffect, useRef, useState } from "react";
import { Anchor, RefreshCw, Pause, Play, Trash2, Clock, Zap, ShieldCheck, Plus, X, Pencil, Save, Download, Upload } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { downloadText } from "@/lib/export";
import { Button } from "@/components/ui/button";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";

interface Trigger {
  type?: string;
  schedule?: string;
  subject?: string;
}
interface Order {
  id: string;
  name?: string;
  enabled?: boolean;
  triggers?: Trigger[];
  initiative?: { mode?: string };
  plan?: string;
  assure?: number;
}
interface WhyEvent {
  seq?: number;
  kind?: string;
  ts_unix_ms?: number;
  payload?: Record<string, unknown>;
}

// kindLabel turns a standing.* event kind into a short human label for the history.
function kindLabel(kind?: string): string {
  const k = (kind || "").replace(/^standing\./, "");
  return k || "event";
}

// parseStandingJSON normalises an exported standing-orders file into an array of
// order objects ready to re-add. Accepts a bare array, a {standing:[…]} wrapper, or
// a {orders:[…]} wrapper (the list shape). Keeps only entries with a name and at
// least one trigger (what the daemon validates), stripping kernel-assigned fields
// (id/timestamps) so a re-add mints fresh ones. Throws on bad JSON / nothing valid.
export function parseStandingJSON(text: string): Record<string, unknown>[] {
  const data = JSON.parse(text);
  const arr = Array.isArray(data)
    ? data
    : Array.isArray((data as { standing?: unknown[] })?.standing)
      ? (data as { standing: unknown[] }).standing
      : Array.isArray((data as { orders?: unknown[] })?.orders)
        ? (data as { orders: unknown[] }).orders
        : null;
  if (!arr) throw new Error("expected an array of orders (or a {standing:[…]} / {orders:[…]} wrapper)");
  const out: Record<string, unknown>[] = [];
  for (const raw of arr) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;
    const o = raw as Record<string, unknown>;
    const name = typeof o.name === "string" ? o.name.trim() : "";
    const triggers = Array.isArray(o.triggers) ? o.triggers : [];
    if (!name || triggers.length === 0) continue;
    // Drop kernel-assigned identity/lifecycle fields; keep the declarative shape.
    const { id, enabled, created_ms, updated_ms, ...rest } = o;
    void id;
    void enabled;
    void created_ms;
    void updated_ms;
    out.push({ ...rest, name, triggers });
  }
  if (out.length === 0) throw new Error("no valid orders (each needs a name and at least one trigger) found");
  return out;
}

// Standing is the autonomy cockpit for Chronos standing orders: persistent goals
// that fire on a trigger (a cron schedule or a matching journal event) and act
// at their initiative level. Each order shows its triggers, autonomy mode and
// plan, with pause-resume / remove controls — so the operator can see and govern
// what the daemon does unprompted.
export function Standing() {
  const ui = useUI();
  const [orders, setOrders] = useState<Order[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  // Order life-story (M746): the order id whose journal history is shown + events.
  const [history, setHistory] = useState<{ id: string; events: WhyEvent[] } | null>(null);

  async function toggleHistory(id: string) {
    if (history?.id === id) {
      setHistory(null);
      return;
    }
    try {
      const d = await getJSON<{ events?: WhyEvent[] }>("/api/standing/why", { id });
      setHistory({ id, events: d.events || [] });
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ orders?: Order[] }>("/api/standing");
      setOrders(d.orders || []);
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

  const enabledCount = orders?.filter((o) => o.enabled).length ?? 0;
  const fileRef = useRef<HTMLInputElement>(null);

  function exportOrders() {
    downloadText("agezt-standing.json", JSON.stringify({ version: 1, standing: orders ?? [] }, null, 2), "application/json");
  }

  // Restore standing orders from a file: re-add each (the daemon mints fresh ids and
  // validates). Note it ADDS — importing onto a daemon that already has them creates
  // duplicates; that's why this lives behind an explicit Import action.
  async function importOrders(file: File) {
    try {
      const list = parseStandingJSON(await file.text());
      let added = 0;
      for (const order of list) {
        try {
          await postJSON("/api/standing/add", { order });
          added++;
        } catch {
          /* skip an order the daemon rejects; keep importing the rest */
        }
      }
      ui.toast(`Imported ${added}/${list.length} standing order${list.length === 1 ? "" : "s"}`, added ? "success" : "error");
      void reload();
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Anchor className="size-4 text-accent" /> Standing orders
        </h2>
        <span className="text-xs text-muted">
          {orders ? `${orders.length} total` : ""}
          {orders && orders.length > 0 && <span className="text-good"> · {enabledCount} active</span>}
        </span>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          aria-hidden="true"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) void importOrders(f);
            e.target.value = "";
          }}
        />
        <Button variant="ghost" size="sm" className="ml-auto" onClick={() => fileRef.current?.click()} title="Import standing orders from a file">
          <Upload className="size-3.5" /> Import
        </Button>
        <Button variant="ghost" size="sm" onClick={exportOrders} disabled={!orders || orders.length === 0} title="Export standing orders to a file">
          <Download className="size-3.5" /> Export
        </Button>
        <Button size="sm" onClick={() => setShowForm((v) => !v)} title="Create a standing order">
          {showForm ? <X className="size-3.5" /> : <Plus className="size-3.5" />} New order
        </Button>
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {showForm && (
        <NewOrderForm
          onCreated={(name) => {
            setShowForm(false);
            ui.toast(`Standing order “${name}” created`, "success");
            void reload();
          }}
          onError={(m) => ui.toast(m, "error")}
        />
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !orders ? (
        <SkeletonList count={4} lines={2} />
      ) : orders.length === 0 ? (
        <EmptyState
          icon={Anchor}
          title="No standing orders yet"
          hint={
            <>
              Standing orders are persistent goals the daemon pursues on a trigger. Hit{" "}
              <span className="font-medium text-foreground/80">New order</span> above to create one — or use{" "}
              <code className="rounded bg-panel px-1 py-0.5">agt standing add</code>.
            </>
          }
        />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-2">
            {orders.map((o) => (
              <li key={o.id} className="rounded-lg border border-border bg-card p-3">
                <div className="flex items-center gap-2">
                  <Badge variant={o.enabled ? "good" : "default"}>{o.enabled ? "active" : "paused"}</Badge>
                  <span className="text-sm font-semibold">{o.name || o.id}</span>
                  {o.initiative?.mode && (
                    <span className="rounded bg-panel px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                      {o.initiative.mode}
                    </span>
                  )}
                  {(o.assure ?? 0) > 0 && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-good/10 px-1.5 py-0.5 text-[10px] font-semibold text-good"
                      title={`do-it-for-sure: each firing verifies completion and retries up to ${o.assure}×`}
                    >
                      <ShieldCheck className="size-3" />
                      assured {o.assure}×
                    </span>
                  )}
                  <div className="ml-auto flex items-center gap-1.5">
                    <button
                      onClick={() =>
                        act(o.id, "/api/standing/enable", { enabled: o.enabled ? "false" : "true" }, {
                          success: o.enabled ? "Order paused" : "Order resumed",
                        })
                      }
                      disabled={busy === o.id}
                      title={o.enabled ? "Pause" : "Resume"}
                      className="text-muted transition-colors hover:text-foreground disabled:opacity-50"
                    >
                      {o.enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
                    </button>
                    <button
                      onClick={() => setEditingId((cur) => (cur === o.id ? null : o.id))}
                      disabled={busy === o.id}
                      title={editingId === o.id ? "Close editor" : "Edit"}
                      className={cn(
                        "transition-colors disabled:opacity-50",
                        editingId === o.id ? "text-accent" : "text-muted hover:text-accent",
                      )}
                    >
                      <Pencil className="size-3.5" />
                    </button>
                    <button
                      onClick={() =>
                        act(o.id, "/api/standing/remove", undefined, {
                          confirm: {
                            title: "Remove this standing order?",
                            message: o.name
                              ? `“${o.name}” will stop firing and be permanently deleted.`
                              : "This standing order will stop firing and be permanently deleted.",
                            confirmLabel: "Remove",
                            danger: true,
                          },
                          success: "Standing order removed",
                        })
                      }
                      disabled={busy === o.id}
                      title="Remove"
                      className="text-muted transition-colors hover:text-bad disabled:opacity-50"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                </div>
                <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                  {(o.triggers || []).map((t, i) => (
                    <span
                      key={i}
                      className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-[10px]"
                      title={`trigger: ${t.type}`}
                    >
                      {t.type === "event" ? (
                        <Zap className="size-3 text-accent" />
                      ) : (
                        <Clock className="size-3 text-accent" />
                      )}
                      {t.type === "event" ? t.subject : t.schedule}
                    </span>
                  ))}
                </div>
                {o.plan && <p className="mt-1.5 line-clamp-2 text-xs text-foreground/80">{o.plan}</p>}
                <div className="mt-1 flex items-center gap-3 text-[10px]">
                  <button onClick={() => toggleHistory(o.id)} className="text-accent/80 transition-colors hover:text-accent" title="Show this order's life story from the journal">
                    {history?.id === o.id ? "hide history" : "history"}
                  </button>
                  <span className="font-mono text-muted opacity-70">{o.id}</span>
                </div>
                {history?.id === o.id && (
                  <ol className="mt-1.5 space-y-0.5 rounded-md border border-border/60 bg-panel/40 p-2 text-[11px]">
                    {history.events.length === 0 ? (
                      <li className="text-muted">no journal events for this order yet</li>
                    ) : (
                      history.events.map((ev, i) => (
                        <li key={ev.seq ?? i} className="flex items-center gap-2">
                          <span className="rounded bg-panel px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-accent">
                            {kindLabel(ev.kind)}
                          </span>
                          {typeof ev.payload?.action === "string" && (
                            <span className="text-foreground/80">{ev.payload.action as string}</span>
                          )}
                          <span className="ml-auto font-mono text-[10px] text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                        </li>
                      ))
                    )}
                  </ol>
                )}
                {editingId === o.id && (
                  <div className="mt-2">
                    <EditOrderForm
                      order={o}
                      onSaved={(name) => {
                        setEditingId(null);
                        ui.toast(`Standing order “${name}” updated`, "success");
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

// NewOrderForm builds and submits a standing order from the UI — so defining what
// the daemon does autonomously no longer needs the CLI. Captures the essentials
// (name, trigger, plan, autonomy mode); the control plane validates and persists.
export function NewOrderForm({
  onCreated,
  onError,
}: {
  onCreated: (name: string) => void;
  onError: (msg: string) => void;
}) {
  const [name, setName] = useState("");
  const [triggerType, setTriggerType] = useState<"cron" | "event">("cron");
  const [triggerValue, setTriggerValue] = useState("");
  const [plan, setPlan] = useState("");
  const [mode, setMode] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const valid = name.trim() !== "" && triggerValue.trim() !== "";

  async function create() {
    if (!valid) return;
    const trigger =
      triggerType === "cron"
        ? { type: "cron", schedule: triggerValue.trim() }
        : { type: "event", subject: triggerValue.trim() };
    const order: Record<string, unknown> = { name: name.trim(), triggers: [trigger] };
    if (plan.trim()) order.plan = plan.trim();
    if (mode) order.initiative = { mode };

    setSubmitting(true);
    try {
      await postJSON("/api/standing/add", { order });
      onCreated(name.trim());
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Name
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Morning briefing"
            aria-label="Order name"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Autonomy
          <select
            value={mode}
            onChange={(e) => setMode(e.target.value)}
            aria-label="Initiative mode"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          >
            <option value="">default (inform only)</option>
            <option value="inform_only">inform only</option>
            <option value="ask">ask first</option>
            <option value="act_or_ask">act, or ask if unsure</option>
          </select>
        </label>
      </div>

      <div className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Trigger
        <div className="flex items-center gap-1.5">
          <div className="inline-flex overflow-hidden rounded-md border border-border">
            {(["cron", "event"] as const).map((t) => (
              <button
                key={t}
                onClick={() => setTriggerType(t)}
                className={cn(
                  "px-2 py-1 text-xs transition-colors",
                  triggerType === t ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
                )}
              >
                {t === "cron" ? "schedule" : "event"}
              </button>
            ))}
          </div>
          <input
            value={triggerValue}
            onChange={(e) => setTriggerValue(e.target.value)}
            placeholder={triggerType === "cron" ? "cron, e.g. 0 9 * * *" : "event subject, e.g. run.failed"}
            aria-label="Trigger value"
            className="min-w-0 flex-1 rounded-md border border-border bg-panel px-2 py-1 font-mono text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </div>
      </div>

      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Plan
        <textarea
          value={plan}
          onChange={(e) => setPlan(e.target.value)}
          placeholder="What the agent should do each time this fires…"
          aria-label="Order plan"
          className="h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
        />
      </label>

      <div className="mt-2 flex items-center justify-end gap-2">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Create order
        </Button>
      </div>
    </div>
  );
}

// EditOrderForm edits an existing order's human-tunable fields in place (M729):
// name, plan, autonomy mode and the do-it-for-sure (assure) retry budget. Triggers,
// observers and scope are left as-is (this is the "tune what it does", not "re-wire
// when it fires", surface), and pause/resume keeps its own control. Posts to the
// standing_edit command, which applies the subset and re-validates.
export function EditOrderForm({
  order,
  onSaved,
  onError,
}: {
  order: Order;
  onSaved: (name: string) => void;
  onError: (msg: string) => void;
}) {
  const [name, setName] = useState(order.name ?? "");
  const [plan, setPlan] = useState(order.plan ?? "");
  const [mode, setMode] = useState(order.initiative?.mode ?? "");
  const [assure, setAssure] = useState(String(order.assure ?? 0));
  const [submitting, setSubmitting] = useState(false);

  const valid = name.trim() !== "";

  async function save() {
    if (!valid) return;
    // Send the full editable state: each key present means "set to this". An empty
    // mode clears it to the default; an empty plan clears the plan — both valid.
    const args: Record<string, unknown> = {
      id: order.id,
      name: name.trim(),
      plan: plan.trim(),
      mode,
      assure: Math.max(0, Math.floor(Number(assure) || 0)),
    };
    setSubmitting(true);
    try {
      await postJSON("/api/standing/edit", args);
      onSaved(name.trim());
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Name
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            aria-label="Edit order name"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          />
        </label>
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Autonomy
          <select
            value={mode}
            onChange={(e) => setMode(e.target.value)}
            aria-label="Edit initiative mode"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
          >
            <option value="">default (inform only)</option>
            <option value="inform_only">inform only</option>
            <option value="ask">ask first</option>
            <option value="act_or_ask">act, or ask if unsure</option>
          </select>
        </label>
      </div>

      <label className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Plan
        <textarea
          value={plan}
          onChange={(e) => setPlan(e.target.value)}
          placeholder="What the agent should do each time this fires…"
          aria-label="Edit order plan"
          className="h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
        />
      </label>

      <label className="mt-2 flex w-40 flex-col gap-1 text-[11px] text-muted">
        Assure (retries)
        <input
          type="number"
          min={0}
          value={assure}
          onChange={(e) => setAssure(e.target.value)}
          aria-label="Edit assure retries"
          title="do-it-for-sure: each firing verifies completion and retries the gap up to this many times (0 = single pass)"
          className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
        />
      </label>

      <div className="mt-2 flex items-center justify-end gap-2">
        <Button size="sm" onClick={save} disabled={!valid || submitting}>
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save changes
        </Button>
      </div>
    </div>
  );
}
