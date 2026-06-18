import { useEffect, useMemo, useRef, useState } from "react";
import { Anchor, RefreshCw, Pause, Play, Trash2, Clock, Zap, ShieldCheck, Plus, X, Pencil, Save, Download, Upload, Users, AlertTriangle } from "lucide-react";
import { AgentPicker } from "@/components/AgentPicker";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { downloadText } from "@/lib/export";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
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
  agent?: string; // M790: firings run AS this roster agent
  assure?: number;
  cooldown_sec?: number;
  frequency_warning?: string;
  target_status?: string;
  target_error?: string;
}
interface StandingAgent {
  slug: string;
  enabled?: boolean;
  retired?: boolean;
  managed?: boolean;
  direct_callable?: boolean;
  kind?: string;
}
interface WhyEvent {
  seq?: number;
  kind?: string;
  ts_unix_ms?: number;
  payload?: Record<string, unknown>;
}

type StandingFilter = "all" | "attention";

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

// Standing is the autonomy cockpit for durable wake rules. A standing order is
// not an agent identity; it binds cron or journal-event triggers to a governed
// plan and optional roster agent. Each order shows its triggers, autonomy mode
// and plan, with pause-resume / remove controls so the operator can govern what
// the daemon wakes unprompted.
export function Standing() {
  const ui = useUI();
  const [orders, setOrders] = useState<Order[] | null>(null);
  const [agents, setAgents] = useState<StandingAgent[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [filter, setFilter] = useState<StandingFilter>("all");
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
      const [standingRes, agentRes] = await Promise.allSettled([
        getJSON<{ orders?: Order[] }>("/api/standing"),
        getJSON<{ profiles?: StandingAgent[] }>("/api/agents"),
      ]);
      if (standingRes.status === "rejected") throw standingRes.reason;
      setOrders(standingRes.value.orders || []);
      setAgents(agentRes.status === "fulfilled" ? agentRes.value.profiles || [] : null);
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
  const agentBySlug = useMemo(() => {
    if (!agents) return null;
    return new Map(agents.map((a) => [a.slug, a]));
  }, [agents]);
  const attentionCount = orders ? standingAttentionCount(orders, agentBySlug) : 0;
  const enabledAttentionCount = orders ? orders.filter((o) => o.enabled && standingNeedsAttention(o, agentBySlug)).length : 0;
  const shownOrders = useMemo(
    () => (orders ? filterStandingOrders(orders, filter, agentBySlug) : []),
    [agentBySlug, filter, orders],
  );
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

  async function pauseAttentionStanding() {
    const targets = (orders || []).filter((o) => o.enabled && standingNeedsAttention(o, agentBySlug));
    if (targets.length === 0) {
      ui.toast("No enabled attention standing orders to pause", "success");
      return;
    }
    if (!(await ui.confirm({
      title: `Pause ${targets.length} attention standing order${targets.length === 1 ? "" : "s"}?`,
      message: "Wake rules with blocked agents or noisy cadence will stop firing until resumed.",
      confirmLabel: "Pause attention",
    }))) return;
    setBusy("attention");
    try {
      await Promise.all(targets.map((o) => postAction("/api/standing/enable", { id: o.id, enabled: "false" })));
      ui.toast(`Paused ${targets.length} attention standing order${targets.length === 1 ? "" : "s"}`, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
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
      <PageHeader
        icon={Anchor}
        title="Standing orders"
        description={
          <>
            {orders ? `${orders.length} total` : ""}
            {orders && orders.length > 0 && <span className="text-good"> · {enabledCount} active</span>}
            {!orders && "Persistent goals the daemon pursues on a trigger"}
          </>
        }
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={() => fileRef.current?.click()} title="Import standing orders from a file">
              <Upload className="size-3.5" /> Import
            </Button>
            <Button variant="ghost" size="sm" onClick={exportOrders} disabled={!orders || orders.length === 0} title="Export standing orders to a file">
              <Download className="size-3.5" /> Export
            </Button>
            <Button size="sm" onClick={() => setShowForm((v) => !v)} title="Create a standing order">
              {showForm ? <X className="size-3.5" /> : <Plus className="size-3.5" />} New order
            </Button>
            {attentionCount > 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={pauseAttentionStanding}
                disabled={busy === "attention" || enabledAttentionCount === 0}
                title={enabledAttentionCount > 0 ? "Pause enabled standing orders that need attention" : "All attention standing orders are already paused"}
              >
                <Pause className="size-3.5" /> Pause attention
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

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
              Standing orders are durable wake rules, not agent identities. Hit{" "}
              <span className="font-medium text-foreground/80">New order</span> above to create one — or use{" "}
              <code className="rounded bg-panel px-1 py-0.5">agt standing add</code>.
            </>
          }
        />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <div className="mb-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <StandingStat label="wake rules" value={orders.length} />
            <StandingStat label="active" value={enabledCount} accent={enabledCount > 0} />
            <StandingStat label="paused" value={orders.length - enabledCount} />
            <StandingStat label="attention" value={attentionCount} accent={attentionCount > 0} />
          </div>
          <div className="mb-3 flex flex-wrap items-center gap-1.5">
            {[
              { id: "all" as const, label: "All", count: orders.length },
              { id: "attention" as const, label: "Attention", count: attentionCount },
            ].map((f) => (
              <button
                key={f.id}
                onClick={() => setFilter(f.id)}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors",
                  filter === f.id ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
                )}
              >
                {f.label}
                <span className="rounded-full bg-card px-1.5 text-[10px] tabular-nums">{f.count}</span>
              </button>
            ))}
          </div>
          <ul className="space-y-2">
            {shownOrders.map((o) => {
              const resumeIssue = standingResumeIssue(o, agentBySlug);
              const frequencyIssue = standingFrequencyIssue(o);
              const attentionReasons = standingAttentionReasons(o, agentBySlug);
              const wakeLedger = standingWakeLedger(o, agentBySlug);
              return (
              <li key={o.id} className="glass rounded-xl p-3">
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
                  {(o.cooldown_sec ?? 0) > 0 && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 text-[10px] font-semibold text-muted"
                      title="minimum gap between event-triggered firings"
                    >
                      <Clock className="size-3" />
                      cooldown {formatCooldown(o.cooldown_sec || 0)}
                    </span>
                  )}
                  {frequencyIssue && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-warn/10 px-1.5 py-0.5 text-[10px] font-semibold text-warn"
                      title={frequencyIssue}
                    >
                      <AlertTriangle className="size-3" />
                      frequent
                    </span>
                  )}
                  <div className="ml-auto flex items-center gap-1.5">
                    <button
                      onClick={() => act(o.id, "/api/standing/fire", undefined, { success: "Standing order fired — it's running now" })}
                      disabled={busy === o.id}
                      title="Run now (fire this order immediately, ignoring its triggers)"
                      className="text-muted transition-colors hover:text-accent disabled:opacity-50"
                    >
                      <Zap className="size-3.5" />
                    </button>
                    <button
                      onClick={() =>
                        act(o.id, "/api/standing/enable", { enabled: o.enabled ? "false" : "true" }, {
                          success: o.enabled ? "Order paused" : "Order resumed",
                        })
                      }
                      disabled={busy === o.id || (!o.enabled && !!resumeIssue)}
                      title={o.enabled ? "Pause" : resumeIssue || "Resume"}
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
                {o.agent && (
                  <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span className="inline-flex items-center gap-1 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-[10px] text-accent">
                      <Users className="size-3" />
                      runs as {o.agent}
                    </span>
                    <StandingAgentBadge agent={o.agent} agents={agentBySlug} />
                  </div>
                )}
                <StandingWakeLedger items={wakeLedger} id={o.id} />
                {attentionReasons.length > 0 && (
                  <div className="mt-1.5 flex items-start gap-1.5 rounded-md border border-warn/30 bg-warn/10 px-2 py-1.5 text-[11px] text-warn">
                    <AlertTriangle className="mt-0.5 size-3 shrink-0" />
                    <span>{attentionReasons.join(" · ")}</span>
                  </div>
                )}
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
              );
            })}
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
  const [agent, setAgent] = useState(""); // M790: firings run AS this roster agent
  const [cooldownSec, setCooldownSec] = useState("");
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
    if (agent.trim()) order.agent = agent.trim();
    if (mode) order.initiative = { mode };
    const cooldown = Math.max(0, Math.floor(Number(cooldownSec) || 0));
    if (cooldown > 0) order.cooldown_sec = cooldown;

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

  function applyMailboxPreset(kind: "dm" | "help") {
    const slug = agent.trim();
    if (!slug) return;
    setTriggerType("event");
    setTriggerValue(kind === "dm" ? `board.dm.${slug}` : `board.help.${slug}`);
    if (!name.trim()) setName(kind === "dm" ? `${slug} mailbox` : `${slug} help queue`);
    if (!plan.trim()) {
      setPlan(
        kind === "dm"
          ? "Read the triggering board message by id from the trigger payload, handle the request, reply if a reply is needed, then ack the message."
          : "Read the triggering help request by id from the trigger payload, resolve or route it, reply with the outcome, then stop.",
      );
    }
  }

  return (
    <div className="glass rounded-xl border-accent/30 p-3">
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

      <div className="mt-2 flex items-center gap-2 text-[11px] text-muted">
        Run as agent
        <AgentPicker value={agent} onChange={setAgent} />
        <span className="opacity-70">firings use its soul, model chain, memory, and budget</span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5 text-[11px] text-muted">
        <span>Mailbox trigger</span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={!agent.trim()}
          title={agent.trim() ? `Use board.dm.${agent.trim()}` : "Select an agent first"}
          onClick={() => applyMailboxPreset("dm")}
        >
          <Anchor className="size-3.5" /> DM
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={!agent.trim()}
          title={agent.trim() ? `Use board.help.${agent.trim()}` : "Select an agent first"}
          onClick={() => applyMailboxPreset("help")}
        >
          <Users className="size-3.5" /> Help
        </Button>
      </div>

      <label className="mt-2 flex w-48 flex-col gap-1 text-[11px] text-muted">
        Event cooldown (sec)
        <input
          type="number"
          min={0}
          value={cooldownSec}
          onChange={(e) => setCooldownSec(e.target.value)}
          aria-label="Event cooldown seconds"
          title="minimum gap between event-triggered firings; 0 uses the daemon default"
          className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
        />
      </label>

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
  const [agent, setAgent] = useState(order.agent ?? ""); // M790: who firings run AS
  const [assure, setAssure] = useState(String(order.assure ?? 0));
  const [cooldownSec, setCooldownSec] = useState(String(order.cooldown_sec ?? 0));
  const [submitting, setSubmitting] = useState(false);

  const valid = name.trim() !== "";

  async function save() {
    if (!valid) return;
    // Send the full editable state: each key present means "set to this". An empty
    // mode clears it to the default; an empty plan/agent clears it — both valid.
    const args: Record<string, unknown> = {
      id: order.id,
      name: name.trim(),
      plan: plan.trim(),
      agent: agent.trim(),
      mode,
      assure: Math.max(0, Math.floor(Number(assure) || 0)),
      cooldown_sec: Math.max(0, Math.floor(Number(cooldownSec) || 0)),
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
    <div className="glass rounded-xl border-accent/30 p-3">
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

      <div className="mt-2 flex items-center gap-2 text-[11px] text-muted">
        Run as agent
        <AgentPicker value={agent} onChange={setAgent} />
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

      <label className="mt-2 flex w-48 flex-col gap-1 text-[11px] text-muted">
        Event cooldown (sec)
        <input
          type="number"
          min={0}
          value={cooldownSec}
          onChange={(e) => setCooldownSec(e.target.value)}
          aria-label="Edit event cooldown seconds"
          title="minimum gap between event-triggered firings; 0 uses the daemon default"
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

export function standingResumeIssue(order: Order, agents: Map<string, StandingAgent> | null): string {
  const apiError = (order.target_error || "").trim();
  if (apiError || order.target_status === "blocked") return apiError || "standing order target is blocked";
  const slug = (order.agent || "").trim();
  if (!slug || !agents) return "";
  const agent = agents.get(slug);
  if (!agent) return `agent ${slug} is missing`;
  if (agent.retired) return `agent ${slug} is retired`;
  if (agent.enabled === false) return `agent ${slug} is paused`;
  if (standingAgentManaged(agent)) return `agent ${slug} is a managed sub-agent`;
  return "";
}

export function standingAttentionReasons(order: Order, agents: Map<string, StandingAgent> | null): string[] {
  return [
    standingResumeIssue(order, agents),
    standingFrequencyIssue(order),
  ].filter(Boolean);
}

export function standingNeedsAttention(order: Order, agents: Map<string, StandingAgent> | null): boolean {
  return standingAttentionReasons(order, agents).length > 0;
}

export function standingAttentionCount(orders: Order[], agents: Map<string, StandingAgent> | null): number {
  return orders.filter((o) => standingNeedsAttention(o, agents)).length;
}

export interface StandingWakeLedgerEntry {
  label: string;
  value: string;
  detail: string;
  tone: "good" | "warn" | "bad" | "accent" | "muted";
}

export function standingTriggerSummary(triggers: Trigger[] = []): { value: string; detail: string; tone: "good" | "warn" | "accent" | "muted" } {
  const clean = triggers.filter((t) => (t.type === "event" && (t.subject || "").trim()) || (t.type === "cron" && (t.schedule || "").trim()));
  if (clean.length === 0) return { value: "no trigger", detail: "standing order has no typed trigger", tone: "warn" };
  if (clean.length === 1) {
    const t = clean[0];
    if (t.type === "event") {
      const subject = (t.subject || "").trim();
      const mailbox = subject.startsWith("board.dm.") || subject.startsWith("board.help.");
      return {
        value: mailbox ? `mailbox event ${subject}` : `event ${subject}`,
        detail: "journal event wakes this standing rule; the rule is not an agent identity",
        tone: mailbox ? "accent" : "good",
      };
    }
    return {
      value: `cron ${t.schedule}`,
      detail: "cron tick wakes this standing rule; schedule owns timing only",
      tone: "good",
    };
  }
  const events = clean.filter((t) => t.type === "event").length;
  const crons = clean.filter((t) => t.type === "cron").length;
  return {
    value: `${clean.length} triggers`,
    detail: [`${events} event`, `${crons} cron`].filter((part) => !part.startsWith("0 ")).join(" · "),
    tone: "good",
  };
}

export function standingWakeLedger(order: Order, agents: Map<string, StandingAgent> | null): StandingWakeLedgerEntry[] {
  const trigger = standingTriggerSummary(order.triggers || []);
  const issue = standingResumeIssue(order, agents);
  const frequency = standingFrequencyIssue(order);
  const eventTriggered = (order.triggers || []).some((t) => t.type === "event");
  const cooldown = eventTriggered
    ? order.cooldown_sec && order.cooldown_sec > 0
      ? formatCooldown(order.cooldown_sec)
      : "daemon default"
    : "cron cadence";
  const plan = (order.plan || "").trim();
  return [
    {
      label: "trigger",
      value: trigger.value,
      detail: trigger.detail,
      tone: trigger.tone,
    },
    {
      label: "runner",
      value: order.agent ? `agent ${order.agent}` : "daemon/default",
      detail: order.agent
        ? `standing wakes ${order.agent}; that agent owns soul, memory, tools, model route, retry, and repair`
        : "standing fires under daemon/default context; no durable agent identity is defined here",
      tone: issue ? "bad" : order.agent ? "good" : "muted",
    },
    {
      label: "cooldown",
      value: cooldown,
      detail: eventTriggered ? "event bursts are rate-bounded before they can wake work" : "cron cadence is controlled by the trigger schedule",
      tone: frequency ? "warn" : "good",
    },
    {
      label: "plan",
      value: plan ? "wake task set" : "no plan",
      detail: plan || "the trigger only wakes; no task text is attached",
      tone: plan ? "good" : "warn",
    },
    {
      label: "guard",
      value: issue ? "target blocked" : frequency ? "frequency review" : "armed",
      detail: issue || frequency || "target and cadence are ready",
      tone: issue ? "bad" : frequency ? "warn" : "good",
    },
  ];
}

function filterStandingOrders(orders: Order[], filter: StandingFilter, agents: Map<string, StandingAgent> | null): Order[] {
  if (filter === "attention") return orders.filter((o) => standingNeedsAttention(o, agents));
  return orders;
}

function standingAgentManaged(agent: StandingAgent): boolean {
  return agent.kind === "subagent" || !!agent.managed || agent.direct_callable === false;
}

export function standingFrequencyIssue(order: Pick<Order, "triggers" | "cooldown_sec" | "frequency_warning">): string {
  if (order.frequency_warning) return order.frequency_warning;
  const triggers = order.triggers || [];
  const explicitCooldown = order.cooldown_sec || 0;
  if (triggers.some((t) => t.type === "event") && explicitCooldown > 0 && explicitCooldown < 15 * 60) {
    return `event cooldown ${formatCooldown(explicitCooldown)} is below the default 15m guard`;
  }
  const cron = triggers.find((t) => t.type === "cron" && (t.schedule || "").trim());
  const first = (cron?.schedule || "").trim().split(/\s+/)[0] || "";
  if (first === "*" || first === "*/1" || first === "0/1") {
    return "cron trigger may wake this standing order every minute";
  }
  return "";
}

function StandingAgentBadge({
  agent,
  agents,
}: {
  agent: string;
  agents: Map<string, StandingAgent> | null;
}) {
  const issue = standingResumeIssue({ id: "", agent }, agents);
  if (!issue) return <Badge variant="good">agent ready</Badge>;
  return <Badge variant="bad">{issue.replace(/^agent\s+\S+\s+/, "")}</Badge>;
}

function StandingWakeLedger({ items, id }: { items: StandingWakeLedgerEntry[]; id: string }) {
  return (
    <div className="mt-1.5 rounded-md border border-border/60 bg-panel/35 p-1.5" aria-label={`${id} wake rule ledger`}>
      <div className="mb-1 text-[9px] font-semibold uppercase tracking-wider text-muted/80">Wake rule ledger</div>
      <div className="grid gap-1 sm:grid-cols-2 xl:grid-cols-5">
        {items.map((item) => (
          <div
            key={item.label}
            title={item.detail}
            className={cn(
              "min-h-[44px] min-w-0 rounded-md border border-border/50 bg-card/45 px-2 py-1.5",
              item.tone === "good" && "border-good/25 bg-good/5",
              item.tone === "bad" && "border-bad/30 bg-bad/5",
              item.tone === "warn" && "border-warn/35 bg-warn/10",
              item.tone === "accent" && "border-accent/30 bg-accent/10",
            )}
          >
            <div className="truncate text-[9px] font-semibold uppercase tracking-wider text-muted/80">{item.label}</div>
            <div
              className={cn(
                "mt-0.5 truncate text-[11px] font-medium text-foreground/90",
                item.tone === "good" && "text-good",
                item.tone === "bad" && "text-bad",
                item.tone === "warn" && "text-warn",
                item.tone === "accent" && "text-accent",
                item.tone === "muted" && "text-muted",
              )}
            >
              {item.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function StandingStat({ label, value, accent }: { label: string; value: string | number; accent?: boolean }) {
  return (
    <div className="rounded-lg border border-border bg-panel/60 px-3 py-2">
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent ? "text-accent" : "text-foreground")}>{value}</div>
    </div>
  );
}

function formatCooldown(sec: number): string {
  if (sec <= 0) return "default";
  if (sec % 3600 === 0) return `${sec / 3600}h`;
  if (sec % 60 === 0) return `${sec / 60}m`;
  return `${sec}s`;
}
