import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  CalendarClock,
  RefreshCw,
  Play,
  Pause,
  Trash2,
  Bot,
  Heart,
  Infinity as InfinityIcon,
  ShieldCheck,
  Plus,
  X,
  Pencil,
  Download,
  Upload,
  GitFork,
  Wrench,
  AlertTriangle,
  Clock3,
  Zap,
} from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { downloadText } from "@/lib/export";
import { cn, fmtDateTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { Badge, statusVariant } from "@/components/ui/badge";
import { ErrorText, KeyValue } from "@/components/JsonView";
import { Page } from "@/components/ui/page";
import { TabNav } from "@/components/ui/tab-nav";
import { MetricWidget, MetricGrid } from "@/components/ui/metric-widget";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { useScheduleFiresPager } from "@/lib/cursorPager";

interface Sched {
  id: string;
  intent?: string;
  model?: string;
  agent?: string;
  target?: string;
  workflow?: string;
  system_task?: string;
  tool?: string;
  payload?: unknown;
  cadence?: string;
  mode?: string;
  interval_sec?: number;
  at_minutes?: number;
  end_minutes?: number;
  days?: number;
  tz?: string;
  source?: string;
  enabled?: boolean;
  next_run_unix?: number;
  once_at_unix?: number;
  last_status?: string;
  frequency_warning?: string;
  fires?: number;
  assure?: number;
  executor?: string;
  uses_llm?: boolean;
  execution_contract?: string;
  target_status?: string;
  target_error?: string;
}

interface ScheduleFire {
  correlation_id?: string;
  schedule_id?: string;
  fired_unix_ms?: number;
  intent?: string;
  action?: string;
  model?: string;
  target?: string;
  agent?: string;
  workflow?: string;
  system_task?: string;
  tool?: string;
  executor?: string;
  category?: string;
  effect_class?: string;
  uses_llm?: boolean;
  status?: string;
  reason?: string;
  duration_ms?: number;
}

interface ScheduleAgent {
  slug: string;
  name?: string;
  enabled?: boolean;
  retired?: boolean;
  managed?: boolean;
  direct_callable?: boolean;
  kind?: string;
  tool_allow?: string[];
  tool_deny?: string[];
}

interface ScheduleWorkflow {
  id?: string;
  name: string;
  enabled?: boolean;
}

interface ScheduleTool {
  name: string;
  description?: string;
}

interface ScheduleSystemTaskInfo {
  name: string;
  label?: string;
  description?: string;
  category?: string;
  executor?: string;
  uses_llm?: boolean;
  effect_class?: string;
  effect?: string;
  recommended_interval_sec?: number;
}

const FALLBACK_SYSTEM_TASKS = ["catalog_sync", "artifact_collect", "memory_clean", "memory_tidy", "log_clean", "graveyard_scan"];
const FALLBACK_SYSTEM_TASK_INFO: ScheduleSystemTaskInfo[] = [
  {
    name: "catalog_sync",
    label: "Catalog sync",
    description: "Download the models.dev catalog, persist it, and reload provider/model metadata.",
    category: "catalog",
    executor: "daemon",
    uses_llm: false,
    effect_class: "config_update",
    effect: "Refreshes provider/model metadata from models.dev/api.json without waking an LLM agent.",
    recommended_interval_sec: 24 * 3600,
  },
  {
    name: "artifact_collect",
    label: "Artifact collect",
    description: "Index offloaded run artifacts so autonomous work remains searchable and inspectable.",
    category: "storage",
    executor: "daemon",
    uses_llm: false,
    effect_class: "local_index",
    effect: "Indexes local run artifacts as a typed daemon job; no agent identity is woken.",
    recommended_interval_sec: 6 * 3600,
  },
  {
    name: "memory_clean",
    label: "Memory clean",
    description: "Run memory maintenance and publish a compact maintenance summary.",
    category: "memory",
    executor: "daemon",
    uses_llm: false,
    effect_class: "memory_maintenance",
    effect: "Runs memory maintenance as a typed daemon task rather than an agent wake.",
    recommended_interval_sec: 24 * 3600,
  },
  {
    name: "memory_tidy",
    label: "Memory tidy",
    description: "Run lightweight memory hygiene without waking an LLM agent.",
    category: "memory",
    executor: "daemon",
    uses_llm: false,
    effect_class: "memory_maintenance",
    effect: "Runs lightweight memory hygiene without waking an LLM agent.",
    recommended_interval_sec: 12 * 3600,
  },
  {
    name: "log_clean",
    label: "Log clean",
    description: "Inspect journal/log pressure and publish a compact maintenance summary.",
    category: "logs",
    executor: "daemon",
    uses_llm: false,
    effect_class: "log_maintenance",
    effect: "Scans durable journal/log pressure without waking an LLM agent; physical deletion stays disabled for hash-chain safety.",
    recommended_interval_sec: 24 * 3600,
  },
  {
    name: "graveyard_scan",
    label: "Graveyard scan",
    description: "Report retired agents past the configured retention window. Notify-only — it never archives or deletes.",
    category: "graveyard",
    executor: "daemon",
    uses_llm: false,
    effect_class: "report_only",
    effect: "Lists graveyard identities older than the retention window and journals an eligibility report; removal stays an explicit operator action (no auto-deletion).",
    recommended_interval_sec: 24 * 3600,
  },
];

const SYSTEM_TASK_QUICK_PRESETS = [
  { task: "catalog_sync", label: "Sync models catalog" },
  { task: "artifact_collect", label: "Collect run artifacts" },
  { task: "memory_tidy", label: "Tidy memory" },
  { task: "log_clean", label: "Inspect log pressure" },
  { task: "graveyard_scan", label: "Scan graveyard retention" },
];

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

export interface ScheduleTargetCounts {
  agent: number;
  workflow: number;
  systemTask: number;
  tool: number;
}

export type ScheduleTargetFilter = "all" | "attention" | "agent" | "workflow" | "system_task" | "tool";

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

export function scheduleTargetCounts(items: Pick<Sched, "target">[]): ScheduleTargetCounts {
  const counts: ScheduleTargetCounts = { agent: 0, workflow: 0, systemTask: 0, tool: 0 };
  for (const s of items) {
    if (s.target === "workflow") counts.workflow++;
    else if (s.target === "system_task") counts.systemTask++;
    else if (s.target === "tool") counts.tool++;
    else counts.agent++;
  }
  return counts;
}

export function scheduleTargetMixLabel(counts: ScheduleTargetCounts): string {
  return [
    counts.agent > 0 ? `${counts.agent} agent` : "",
    counts.workflow > 0 ? `${counts.workflow} workflow` : "",
    counts.systemTask > 0 ? `${counts.systemTask} system` : "",
    counts.tool > 0 ? `${counts.tool} tool` : "",
  ].filter(Boolean).join(" / ") || "none";
}

export function scheduleSystemTaskPresetLabel(
  task: string,
  tasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
): string {
  const info = tasks.find((row) => row.name === task);
  const label = systemTaskDisplayName(task, tasks);
  const recommended = info?.recommended_interval_sec || 0;
  if (recommended > 0) {
    const parts = intervalParts(recommended);
    return `${label} · every ${parts.amount} ${parts.unit}`;
  }
  return label;
}

export function scheduleNeedsAttention(
  s: Pick<Sched, "target" | "agent" | "workflow" | "system_task" | "tool" | "mode" | "interval_sec" | "frequency_warning" | "target_status" | "target_error">,
  agents: ScheduleAgent[] = [],
  workflows: ScheduleWorkflow[] = [],
  tools: ScheduleTool[] = [],
  systemTasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
): boolean {
  const health = scheduleTargetHealthPassport(s, agents, workflows, tools, systemTasks);
  return health.tone === "bad" || !!scheduleFrequencyIssue(s, systemTasks, agents);
}

export function scheduleAttentionReasons(
  s: Pick<Sched, "target" | "agent" | "workflow" | "system_task" | "tool" | "mode" | "interval_sec" | "frequency_warning" | "target_status" | "target_error">,
  agents: ScheduleAgent[] = [],
  workflows: ScheduleWorkflow[] = [],
  tools: ScheduleTool[] = [],
  systemTasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
): string[] {
  const health = scheduleTargetHealthPassport(s, agents, workflows, tools, systemTasks);
  return [
    health.tone === "bad" ? health.detail : "",
    scheduleFrequencyIssue(s, systemTasks, agents),
  ].filter(Boolean);
}

export function scheduleAttentionCount(
  items: Sched[],
  agents: ScheduleAgent[] = [],
  workflows: ScheduleWorkflow[] = [],
  tools: ScheduleTool[] = [],
  systemTasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
): number {
  return items.filter((s) => scheduleNeedsAttention(s, agents, workflows, tools, systemTasks)).length;
}

export function filterScheduleItems(
  items: Sched[],
  filter: ScheduleTargetFilter,
  agents: ScheduleAgent[] = [],
  workflows: ScheduleWorkflow[] = [],
  tools: ScheduleTool[] = [],
  systemTasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
): Sched[] {
  if (filter === "all") return items;
  if (filter === "attention") {
    return items.filter((s) => scheduleNeedsAttention(s, agents, workflows, tools, systemTasks));
  }
  return items.filter((s) => {
    if (filter === "agent") return s.target !== "workflow" && s.target !== "system_task" && s.target !== "tool";
    return s.target === filter;
  });
}

export function systemTaskDisplayName(name?: string, tasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO): string {
  const raw = (name || "").trim();
  if (!raw) return "system task";
  const task = tasks.find((t) => t.name === raw);
  return task?.label || raw;
}

export function systemTaskExecutionLabel(task?: Pick<ScheduleSystemTaskInfo, "executor" | "uses_llm" | "category" | "effect_class">): string {
  const executor = task?.executor?.trim() || "daemon";
  const category = task?.category?.trim();
  const effectClass = task?.effect_class?.trim();
  const llm = task?.uses_llm ? "LLM" : "no LLM";
  return [executor, category, effectClass, llm].filter(Boolean).join(" · ");
}

// parseSchedulesJSON normalises an exported schedules file into a list of
// re-addable `schedule_add` arg objects. Accepts a bare array or a {schedules:[…]}
// wrapper (the list shape). For each entry it rebuilds the cadence args from the
// stored mode — interval (interval_sec), continuous (cooldown_sec), daily
// (at_minutes+days+tz), window (window_start/end+interval_sec+days+tz) or once
// (once_at_unix) — dropping kernel identity/runtime fields
// (id/source/enabled/fires/...). Agent-task schedules need a task;
// workflow/system_task/tool schedules need their typed target plus a valid
// cadence; throws on bad JSON / nothing valid.
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
    const target = typeof s.target === "string" ? s.target.trim() : "";
    const workflow = typeof s.workflow === "string" ? s.workflow.trim() : "";
    const systemTask = typeof s.system_task === "string" ? s.system_task.trim() : "";
    const tool = typeof s.tool === "string" ? s.tool.trim() : "";
    if (!intent && target !== "workflow" && target !== "system_task" && target !== "tool") continue;
    const num = (k: string) => (typeof s[k] === "number" ? (s[k] as number) : undefined);
    const mode = typeof s.mode === "string" ? s.mode : "";
    const args: Record<string, unknown> = {};
    if (intent) args.intent = intent;
    if (typeof s.model === "string" && s.model) args.model = s.model;
    if (typeof s.agent === "string" && s.agent) args.agent = s.agent;
    if (target === "workflow") {
      if (!workflow) continue;
      args.target = "workflow";
      args.workflow = workflow;
      if ("payload" in s) args.payload = s.payload;
    }
    if (target === "system_task") {
      if (!systemTask) continue;
      args.target = "system_task";
      args.system_task = systemTask;
      delete args.agent;
      delete args.model;
    }
    if (target === "tool") {
      if (!tool) continue;
      args.target = "tool";
      args.tool = tool;
      delete args.model;
      if ("payload" in s) args.payload = s.payload;
    }
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
    } else if (mode === "continuous") {
      const sec = num("interval_sec");
      if (!sec || sec < 1) continue;
      args.cooldown_sec = sec;
    } else if (mode === "" || mode === "interval") {
      const sec = num("interval_sec");
      if (!sec || sec < 1) continue;
      args.interval_sec = sec;
    } else {
      continue; // unknown mode
    }
    out.push(args);
  }
  if (out.length === 0) throw new Error("no re-addable schedules (each needs an agent task or typed target plus a valid cadence) found");
  return out;
}

export function scheduleTargetLabel(s: Pick<Sched, "target" | "workflow" | "system_task" | "tool" | "agent">): string {
  if (s.target === "workflow") return "workflow";
  if (s.target === "system_task") return "system task";
  if (s.target === "tool") return "tool";
  return s.agent ? "agent wake" : "agent task";
}

export function scheduleActionTitle(s: Pick<Sched, "id" | "intent" | "target" | "workflow" | "system_task" | "tool" | "agent">): string {
  if (s.target === "workflow" && s.workflow) return `Run workflow ${s.workflow}`;
  if (s.target === "system_task" && s.system_task) return `Run system task ${systemTaskDisplayName(s.system_task)}`;
  if (s.target === "tool" && s.tool) return `Run tool ${s.tool}`;
  if (s.agent && s.intent) return `Wake ${s.agent}: ${s.intent}`;
  if (s.intent) return s.intent;
  return s.id;
}






export function scheduleTargetHealthPassport(
  s: Pick<Sched, "target" | "agent" | "workflow" | "system_task" | "tool" | "target_status" | "target_error">,
  agents: ScheduleAgent[] = [],
  workflows: ScheduleWorkflow[] = [],
  tools: ScheduleTool[] = [],
  systemTasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const apiError = s.target_error?.trim();
  if (apiError || s.target_status === "blocked") {
    return {
      value: "target blocked",
      detail: apiError || "daemon validation reports this schedule target is blocked",
      tone: "bad",
    };
  }
  const agent = s.agent ? agents.find((a) => a.slug === s.agent) : undefined;
  const agentIssue = s.agent ? scheduleResumeIssue(s, agents) : "";
  if (agentIssue) return { value: "target blocked", detail: agentIssue, tone: "bad" };
  if (s.target === "workflow") {
    const name = (s.workflow || "").trim();
    if (!name) return { value: "target missing", detail: "workflow target is empty", tone: "bad" };
    const workflow = workflows.find((w) => w.name === name || w.id === name);
    if (!workflow) return { value: "target missing", detail: `workflow ${name} is not registered`, tone: "bad" };
    if (workflow.enabled === false) return { value: "target paused", detail: `workflow ${name} is disabled`, tone: "bad" };
    return {
      value: "target ready",
      detail: s.agent ? `workflow ${name} will run as ${s.agent}` : `workflow ${name} will run under system identity`,
      tone: s.agent ? "warn" : "good",
    };
  }
  if (s.target === "system_task") {
    const name = (s.system_task || "").trim();
    if (!name) return { value: "target missing", detail: "system task target is empty", tone: "bad" };
    const task = systemTasks.find((t) => t.name === name);
    if (!task) return { value: "target missing", detail: `system task ${name} is not registered`, tone: "bad" };
    return {
      value: "target ready",
      detail: `${systemTaskDisplayName(name, systemTasks)} is available as a typed daemon task`,
      tone: "good",
    };
  }
  if (s.target === "tool") {
    const name = (s.tool || "").trim();
    if (!name) return { value: "target missing", detail: "tool target is empty", tone: "bad" };
    const tool = tools.find((t) => t.name === name);
    if (!tool) return { value: "target missing", detail: `tool ${name} is not registered`, tone: "bad" };
    const toolIssue = s.agent ? scheduleToolAgentIssue(name, s.agent, agents) : "";
    if (toolIssue) return { value: "target blocked", detail: toolIssue, tone: "bad" };
    return {
      value: "target ready",
      detail: s.agent ? `tool ${name} can run under ${s.agent}'s tool policy` : `tool ${name} can run under system tool policy`,
      tone: s.agent ? "warn" : "good",
    };
  }
  if (s.agent) {
    return {
      value: "target ready",
      detail: `${agentLabel(agents, s.agent)} can be woken by cron`,
      tone: agent ? "good" : "warn",
    };
  }
  return {
    value: "target unbound",
    detail: "no roster agent is bound; schedule will use daemon/default runtime context",
    tone: "muted",
  };
}


export function scheduleFrequencyIssue(
  s: Pick<Sched, "mode" | "interval_sec" | "target" | "system_task" | "agent" | "frequency_warning">,
  systemTasks: ScheduleSystemTaskInfo[] = FALLBACK_SYSTEM_TASK_INFO,
  agents: ScheduleAgent[] = [],
): string {
  if (s.frequency_warning) return s.frequency_warning;
  const sec = s.interval_sec || 0;
  if (sec <= 0 || (s.mode && s.mode !== "interval" && s.mode !== "window" && s.mode !== "continuous")) return "";
  if (s.target === "system_task") {
    const info = systemTasks.find((task) => task.name === s.system_task);
    const recommended = info?.recommended_interval_sec || 0;
    if (recommended > 0 && sec < recommended) {
      return `${systemTaskDisplayName(s.system_task, systemTasks)} is scheduled more often than its recommended cadence`;
    }
  }
  const agent = s.agent ? agents.find((a) => a.slug === s.agent) : undefined;
  if (agent?.kind === "system" && sec < 8 * 3600) {
    return `${agentLabel(agents, s.agent || "")} is a system agent scheduled inside the guardian quiet window`;
  }
  if (s.target !== "workflow" && s.target !== "system_task" && s.target !== "tool" && sec < 15 * 60) {
    return "agent wake schedule is very frequent";
  }
  return "";
}

export function scheduleFireMeta(f: Pick<ScheduleFire, "target" | "agent" | "model" | "workflow" | "system_task" | "tool" | "executor" | "category" | "effect_class" | "uses_llm" | "schedule_id" | "duration_ms">): string[] {
  const target =
    f.target === "workflow"
      ? f.workflow
        ? `workflow ${f.workflow}`
        : "workflow"
      : f.target === "system_task"
        ? f.system_task
          ? `system ${systemTaskDisplayName(f.system_task)}`
          : "system task"
        : f.target === "tool"
          ? f.tool
            ? `tool ${f.tool}`
            : "tool"
          : "agent";
  return [
    target,
    f.executor ? [f.executor, f.category || "", f.effect_class || "", f.uses_llm === false ? "no LLM" : f.uses_llm === true ? "LLM" : ""].filter(Boolean).join(" · ") : "",
    f.agent ? `as ${f.agent}` : "",
    f.model ? `model ${f.model}` : "",
    f.schedule_id ? `id ${f.schedule_id}` : "",
    typeof f.duration_ms === "number" ? `${Math.round(f.duration_ms)}ms` : "",
  ].filter(Boolean);
}

// SCHEDULE_ROW_WINDOW is how many schedule rows render at once. /api/schedules
// has no cursor, so the whole list arrives in one fetch — the window keeps a
// big fleet of schedules from ballooning the DOM; a Load-more footer grows it
// client-side. Attention counts and rollups always use the FULL list.
const SCHEDULE_ROW_WINDOW = 60;

// Schedules is the autonomy cockpit: every cron-like job — whether it wakes an
// agent, runs a workflow, invokes a tool, or performs a system task — with its
// cadence, next fire, last outcome and origin, plus run-now / pause-resume /
// remove controls so unattended work stays governable.
export function Schedules() {
  const ui = useUI();
  const [items, setItems] = useState<Sched[] | null>(null);
  const [profiles, setProfiles] = useState<ScheduleAgent[]>([]);
  const [workflows, setWorkflows] = useState<ScheduleWorkflow[]>([]);
  const [tools, setTools] = useState<ScheduleTool[]>([]);
  const [systemTasks, setSystemTasks] = useState<string[]>(FALLBACK_SYSTEM_TASKS);
  const [systemTaskInfo, setSystemTaskInfo] = useState<ScheduleSystemTaskInfo[]>(FALLBACK_SYSTEM_TASK_INFO);
  // Recent firings are cursor-paginated via useScheduleFiresPager (the hook
  // owns its own polling + live-event reload). reload() below fetches only the
  // schedules, profiles, workflows, tools, and system-task catalog — the pager
  // drives the fires list independently.
  const {
    paged: fireRows,
    loadMore: loadMoreFires,
    loadingMore: loadingMoreFires,
    moreError: firesError,
    hasMore: hasMoreFires,
  } = useScheduleFiresPager(20);
  const fires = fireRows as unknown as ScheduleFire[];
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [targetFilter, setTargetFilter] = useState<ScheduleTargetFilter>("all");
  const [rowWin, setRowWin] = useState(SCHEDULE_ROW_WINDOW);

  // Reset the render window whenever the target filter changes so a new
  // filter always starts from the top of its result set.
  useEffect(() => {
    setRowWin(SCHEDULE_ROW_WINDOW);
  }, [targetFilter]);
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
      const [d, a, w, t, st] = await Promise.all([
        getJSON<{ schedules?: Sched[] }>("/api/schedules"),
        getJSON<{ profiles?: ScheduleAgent[] }>("/api/agents").catch(() => ({ profiles: [] })),
        getJSON<{ workflows?: ScheduleWorkflow[] }>("/api/workflows").catch(() => ({ workflows: [] })),
        getJSON<{ tools?: ScheduleTool[] }>("/api/tools_catalog").catch(() => ({ tools: [] })),
        getJSON<{ system_tasks?: string[]; system_task_info?: ScheduleSystemTaskInfo[] }>("/api/schedule/system_tasks").catch(() => ({
          system_tasks: FALLBACK_SYSTEM_TASKS,
          system_task_info: FALLBACK_SYSTEM_TASK_INFO,
        })),
      ]);
      setItems(d.schedules || []);
      setProfiles(a.profiles || []);
      setWorkflows(w.workflows || []);
      setTools(t.tools || []);
      const nextSystemTasks = (st.system_tasks || []).filter(Boolean);
      setSystemTasks(nextSystemTasks.length ? nextSystemTasks : FALLBACK_SYSTEM_TASKS);
      const nextSystemTaskInfo = (st.system_task_info || []).filter((task) => task?.name);
      setSystemTaskInfo(nextSystemTaskInfo.length ? nextSystemTaskInfo : FALLBACK_SYSTEM_TASK_INFO);
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

  async function pauseAttentionSchedules() {
    const targets = (items || []).filter(
      (s) => s.enabled !== false && scheduleNeedsAttention(s, profiles, workflows, tools, systemTaskInfo),
    );
    if (targets.length === 0) {
      ui.toast("No enabled attention schedules to pause", "success");
      return;
    }
    if (!(await ui.confirm({
      title: `Pause ${targets.length} attention schedule${targets.length === 1 ? "" : "s"}?`,
      message: "Schedules with missing targets, blocked agent/tool policy, or noisy cadence will stop firing until resumed.",
      confirmLabel: "Pause attention",
    }))) return;
    setBusy("attention");
    try {
      await Promise.all(targets.map((s) => postAction("/api/schedule/enable", { id: s.id, enabled: "false" })));
      ui.toast(`Paused ${targets.length} attention schedule${targets.length === 1 ? "" : "s"}`, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  const attentionCount = items ? scheduleAttentionCount(items, profiles, workflows, tools, systemTaskInfo) : 0;
  const enabledAttentionCount = items
    ? items.filter((s) => s.enabled !== false && scheduleNeedsAttention(s, profiles, workflows, tools, systemTaskInfo)).length
    : 0;
  const shownItems = useMemo(
    () => (items ? filterScheduleItems(items, targetFilter, profiles, workflows, tools, systemTaskInfo) : []),
    [items, profiles, targetFilter, tools, workflows, systemTaskInfo],
  );
  const visibleItems = useMemo(() => shownItems.slice(0, rowWin), [shownItems, rowWin]);
  const editingSchedule = useMemo(
    () => (editingId ? (items || []).find((s) => s.id === editingId) || null : null),
    [editingId, items],
  );

  return (
    <Page
      icon={CalendarClock}
      title="Schedules"
      width="wide"
      actions={
        <>
            <Button size="sm" onClick={() => setShowForm(true)} title="Create a schedule">
              <Plus className="size-3.5" /> New schedule
            </Button>
            {attentionCount > 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={pauseAttentionSchedules}
                disabled={busy === "attention" || enabledAttentionCount === 0}
                title={enabledAttentionCount > 0 ? "Pause enabled schedules that need attention" : "All attention schedules are already paused"}
              >
                <Pause className="size-3.5" /> Pause attention
              </Button>
            )}
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
      >

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
        <div>
          {/* Summary band (M917): the schedule fleet at a glance — how many are
              live, paused, and about to fire within the hour. */}
          {(() => {
            const c = scheduleCounts(items, now);
            const targets = scheduleTargetCounts(items);
            const attention = scheduleAttentionCount(items, profiles, workflows, tools, systemTaskInfo);
            return (
              <MetricGrid cols="repeat(auto-fill, minmax(120px, 1fr))">
                <MetricWidget icon={CalendarClock} label="total" value={c.total} tone="muted" />
                <MetricWidget icon={Play} label="enabled" value={c.enabled} tone={c.enabled > 0 ? "accent" : "muted"} />
                <MetricWidget icon={Pause} label="paused" value={c.paused} tone={c.paused > 0 ? "warn" : "muted"} />
                <MetricWidget icon={AlertTriangle} label="attention" value={attention} tone={attention > 0 ? "bad" : "muted"} />
                <MetricWidget icon={Bot} label="targets" value={scheduleTargetMixLabel(targets)} tone={targets.workflow + targets.systemTask + targets.tool > 0 ? "accent" : "muted"} />
              </MetricGrid>
            );
          })()}
          {(() => {
            const targets = scheduleTargetCounts(items);
            const attention = scheduleAttentionCount(items, profiles, workflows, tools, systemTaskInfo);
            const filters: { id: ScheduleTargetFilter; label: string; icon: typeof Bot; count: number }[] = [
              { id: "all", label: "All", icon: CalendarClock, count: items.length },
              { id: "attention", label: "Attention", icon: AlertTriangle, count: attention },
              { id: "agent", label: "Agent", icon: Bot, count: targets.agent },
              { id: "workflow", label: "Workflow", icon: GitFork, count: targets.workflow },
              { id: "system_task", label: "System task", icon: RefreshCw, count: targets.systemTask },
              { id: "tool", label: "Tool", icon: Wrench, count: targets.tool },
            ];
            return (
              <TabNav
                tabs={filters.map((f) => ({
                  id: f.id,
                  label: f.label,
                  icon: f.icon,
                  count: f.count,
                  content: null,
                }))}
                value={targetFilter}
                onValueChange={(v) => setTargetFilter(v as ScheduleTargetFilter)}
              />
            );
          })()}
          {fires.length > 0 && (
            <ScheduleEventPanel
              icon={Zap}
              title="Recent firings"
              status={`${fires.length} event${fires.length === 1 ? "" : "s"}`}
            >
              <div className="grid gap-1.5 md:grid-cols-2">
                {fires.map((f, idx) => (
                  <div
                    key={f.correlation_id || `${f.schedule_id || "fire"}-${idx}`}
                    className="flex min-w-0 items-start gap-2 rounded-md bg-background/45 px-2 py-1.5 text-xs"
                  >
                    <Badge variant={statusVariant(f.status || "")}>{f.status || "fired"}</Badge>
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium text-foreground/85">{f.action || f.intent || f.schedule_id || f.correlation_id || "Scheduled job"}</div>
                      <div className="truncate text-xs text-muted">
                        {f.fired_unix_ms ? fmtDateTime(f.fired_unix_ms) : "time unknown"}
                      </div>
                      <div className="truncate text-xs text-muted/85">
                        {scheduleFireMeta(f).join(" · ")}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              <LoadMoreFooter
                hasMore={hasMoreFires}
                loadingMore={loadingMoreFires}
                moreError={firesError}
                onLoadMore={loadMoreFires}
                pageSize={20}
                label="firings"
              />
            </ScheduleEventPanel>
          )}
          {shownItems.length === 0 ? (
            <EmptyState icon={CalendarClock} title="No matching schedules" hint="Try a different target filter." />
          ) : (
          <ul className="space-y-2">
            {visibleItems.map((s) => {
              const resumeIssue = scheduleResumeIssue(s, profiles);
              const targetLabel = scheduleTargetLabel(s);
              const actionTitle = scheduleActionTitle(s);
              const frequencyIssue = scheduleFrequencyIssue(s, systemTaskInfo, profiles);
              const targetHealth = scheduleTargetHealthPassport(s, profiles, workflows, tools, systemTaskInfo);
              const attentionReasons = scheduleAttentionReasons(s, profiles, workflows, tools, systemTaskInfo);
              return (
              <li key={s.id} className="glass rounded-xl p-3">
                <div className="flex items-center gap-2">
                  <Badge>
                    {s.mode === "continuous" && <InfinityIcon className="mr-1 inline size-3 align-[-1px]" />}
                    {s.cadence || s.mode || "?"}
                  </Badge>
                  <span
                    className={cn(
                      "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs font-semibold uppercase tracking-normal",
                      sourceTone(s.source),
                    )}
                    title={`source: ${s.source || "?"}`}
                  >
                    {s.source === "agent" && <Bot className="size-3" />}
                    {s.source || "?"}
                  </span>
                  <span className="inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 text-xs font-semibold text-foreground/75" title={`job target: ${targetLabel}`}>
                    {targetLabel}
                  </span>
                  {s.mode === "continuous" && s.enabled !== false && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-bad/10 px-1.5 py-0.5 text-xs font-semibold text-bad"
                      title={`alive — ${s.fires ?? 0} cycle${s.fires === 1 ? "" : "s"} completed`}
                    >
                      <Heart className="size-3 animate-pulse fill-current" />
                      {s.fires ?? 0}
                    </span>
                  )}
                  {(s.assure ?? 0) > 0 && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-good/10 px-1.5 py-0.5 text-xs font-semibold text-good"
                      title={`do-it-for-sure: each firing verifies completion and retries up to ${s.assure}×`}
                    >
                      <ShieldCheck className="size-3" />
                      assured {s.assure}×
                    </span>
                  )}
                  {s.enabled === false && <span className="text-xs text-muted">(paused)</span>}
                  {s.agent && (
                    <span className="inline-flex items-center gap-1.5">
                      <span className="inline-flex items-center gap-1 rounded-full bg-accent/10 px-1.5 py-0.5 text-xs font-semibold text-accent" title={`runs as ${s.agent}`}>
                        <Bot className="size-3" />
                        {agentLabel(profiles, s.agent)}
                      </span>
                      <ScheduleAgentStateBadge issue={resumeIssue} />
                    </span>
                  )}
                  {s.target === "workflow" && s.workflow && (
                    <span className="inline-flex items-center gap-1 rounded-full bg-good/10 px-1.5 py-0.5 text-xs font-semibold text-good" title={`runs workflow ${s.workflow}`}>
                      <GitFork className="size-3" />
                      {s.workflow}
                    </span>
                  )}
                  {s.target === "system_task" && s.system_task && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 text-xs font-semibold text-muted"
                      title={`runs system task ${s.system_task} · ${systemTaskExecutionLabel(systemTaskInfo.find((task) => task.name === s.system_task))}`}
                    >
                      <RefreshCw className="size-3" />
                      {systemTaskDisplayName(s.system_task, systemTaskInfo)}
                    </span>
                  )}
                  {s.target === "tool" && s.tool && (
                    <span className="inline-flex items-center gap-1 rounded-full bg-warn/10 px-1.5 py-0.5 text-xs font-semibold text-warn" title={`runs tool ${s.tool}`}>
                      <Wrench className="size-3" />
                      {s.tool}
                    </span>
                  )}
                  {s.model && (
                    <span className="inline-flex items-center gap-1 rounded-full bg-panel px-1.5 py-0.5 font-mono text-xs font-semibold text-muted" title={`model override: ${s.model}`}>
                      model {s.model}
                    </span>
                  )}
                  {s.last_status && <Badge variant={statusVariant(s.last_status)}>{s.last_status}</Badge>}
                  {frequencyIssue && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-warn/10 px-1.5 py-0.5 text-xs font-semibold text-warn"
                      title={frequencyIssue}
                    >
                      <AlertTriangle className="size-3" />
                      frequent
                    </span>
                  )}
                  {targetHealth.tone === "bad" && (
                    <span
                      className="inline-flex items-center gap-1 rounded-full bg-bad/10 px-1.5 py-0.5 text-xs font-semibold text-bad"
                      title={targetHealth.detail}
                    >
                      <AlertTriangle className="size-3" />
                      target
                    </span>
                  )}
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
                      disabled={busy === s.id || (s.enabled === false && !!resumeIssue)}
                      title={s.enabled === false ? resumeIssue || "Resume" : "Pause"}
                      className="text-muted transition-colors hover:text-foreground disabled:opacity-50"
                    >
                      {s.enabled === false ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
                    </button>
                    <button
                      onClick={() => setEditingId(s.id)}
                      disabled={busy === s.id}
                      title="Edit"
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
                <div className="mt-1.5 text-sm font-medium">{actionTitle}</div>
                {s.intent && actionTitle !== s.intent && (
                  <div className="mt-0.5 text-sm text-muted">label: {s.intent}</div>
                )}
                {attentionReasons.length > 0 && (
                  <div className="mt-1.5 flex items-start gap-1.5 rounded-md border border-warn/30 bg-warn/10 px-2 py-1.5 text-sm text-warn">
                    <AlertTriangle className="mt-0.5 size-3 shrink-0" />
                    <span>{attentionReasons.join(" · ")}</span>
                  </div>
                )}
                <div className="mt-1 flex flex-wrap items-center gap-x-3 text-xs text-muted">
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
                </div>
                {forecast?.id === s.id && (
                  <ol className="mt-1.5 space-y-0.5 rounded-md border border-border/60 bg-panel/40 p-2 text-sm">
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
                {/* The row's single raw escape hatch — stored fields once, no narration. */}
                <Disclosure
                  className="mt-1"
                  summary={<span className="text-xs text-muted">details</span>}
                >
                  <div className="rounded-md border border-border/60 bg-panel/40 p-2 text-xs">
                    <KeyValue
                      pairs={(
                        [
                          ["id", <span key="id" className="font-mono">{s.id}</span>],
                          ["cadence", s.cadence || s.mode || "—"],
                          ["source", s.source || "—"],
                          s.executor ? ["executor", s.executor] : null,
                          s.uses_llm !== undefined ? ["LLM", s.uses_llm ? "yes" : "no"] : null,
                          s.model ? ["model", <span key="model" className="font-mono">{s.model}</span>] : null,
                          s.payload !== undefined && s.payload !== null
                            ? [
                                "payload",
                                <span key="payload" className="break-all font-mono">
                                  {JSON.stringify(s.payload)}
                                </span>,
                              ]
                            : null,
                        ] as ([string, React.ReactNode] | null)[]
                      ).filter((p): p is [string, React.ReactNode] => p !== null)}
                    />
                  </div>
                </Disclosure>
              </li>
              );
            })}
          </ul>
          )}
          {shownItems.length > SCHEDULE_ROW_WINDOW && (
            <LoadMoreFooter
              hasMore={rowWin < shownItems.length}
              loadingMore={false}
              onLoadMore={() => setRowWin((w) => w + SCHEDULE_ROW_WINDOW)}
              pageSize={Math.min(SCHEDULE_ROW_WINDOW, Math.max(1, shownItems.length - rowWin))}
              label="schedules"
            />
          )}
        </div>
      )}

      {showForm && (
        <ScheduleModal title="New schedule" onClose={() => setShowForm(false)}>
          <NewScheduleForm
            onCreated={() => {
              setShowForm(false);
              ui.toast("Schedule created", "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
            agents={profiles}
            workflows={workflows}
            tools={tools}
            systemTasks={systemTasks}
            systemTaskInfo={systemTaskInfo}
          />
        </ScheduleModal>
      )}

      {editingSchedule && (
        <ScheduleModal title={`Edit ${editingSchedule.intent || editingSchedule.id}`} onClose={() => setEditingId(null)}>
          <NewScheduleForm
            editId={editingSchedule.id}
            initialIntent={editingSchedule.intent}
            initialModel={editingSchedule.model || ""}
            initialAgent={editingSchedule.agent}
            initialTarget={editingSchedule.target === "workflow" ? "workflow" : editingSchedule.target === "system_task" ? "system_task" : editingSchedule.target === "tool" ? "tool" : "agent"}
            initialWorkflow={editingSchedule.workflow || ""}
            initialPayload={editingSchedule.payload === undefined ? "" : JSON.stringify(editingSchedule.payload, null, 2)}
            initialSystemTask={editingSchedule.system_task || "catalog_sync"}
            initialTool={editingSchedule.tool || ""}
            initialMode={editingSchedule.mode}
            initialIntervalSec={editingSchedule.interval_sec}
            initialAtMinutes={editingSchedule.at_minutes}
            initialEndMinutes={editingSchedule.end_minutes}
            initialDays={editingSchedule.days}
            initialTz={editingSchedule.tz}
            initialOnceAtUnix={editingSchedule.once_at_unix || (editingSchedule.mode === "once" ? editingSchedule.next_run_unix : undefined)}
            agents={profiles}
            workflows={workflows}
            tools={tools}
            systemTasks={systemTasks}
            systemTaskInfo={systemTaskInfo}
            onCreated={() => {
              setEditingId(null);
              ui.toast("Schedule updated", "success");
              void reload();
            }}
            onError={(m) => ui.toast(m, "error")}
          />
        </ScheduleModal>
      )}
    </Page>
  );
}

function ScheduleEventPanel({
  icon: Icon,
  title,
  status,
  children,
}: {
  icon: typeof Zap;
  title: string;
  status: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-accent/35 bg-accent/5 text-accent">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}

function ScheduleModal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="modal-overlay fixed inset-0 z-[160] flex items-start justify-center overflow-y-auto bg-black/55 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="modal-in mt-10 w-full max-w-4xl rounded-lg border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="mb-3 flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
            <CalendarClock className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close schedule modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}


function agentLabel(agents: ScheduleAgent[], slug: string): string {
  const a = agents.find((p) => p.slug === slug);
  return a?.name ? `${a.name} (${slug})` : slug;
}

function scheduleResumeIssue(schedule: Pick<Sched, "agent">, agents: ScheduleAgent[]): string {
  if (!schedule.agent) return "";
  const agent = agents.find((p) => p.slug === schedule.agent);
  if (!agent) return `agent ${schedule.agent} is missing`;
  if (agent.retired) return `agent ${schedule.agent} is retired`;
  if (agent.enabled === false) return `agent ${schedule.agent} is paused`;
  if (scheduleAgentManaged(agent)) return `agent ${schedule.agent} is a managed sub-agent`;
  return "";
}

function ScheduleAgentStateBadge({ issue }: { issue: string }) {
  if (!issue) return <Badge variant="good">agent ready</Badge>;
  return <Badge variant="bad">{issue.replace(/^agent\s+\S+\s+/, "")}</Badge>;
}

export function scheduleAgentManaged(agent: Pick<ScheduleAgent, "kind" | "managed" | "direct_callable">): boolean {
  return agent.kind === "subagent" || !!agent.managed || agent.direct_callable === false;
}

function scheduleAgentDisabled(agent: ScheduleAgent): boolean {
  return agent.enabled === false || !!agent.retired || scheduleAgentManaged(agent);
}

function scheduleAgentStateLabel(agent: ScheduleAgent): string {
  if (agent.enabled === false) return " (paused)";
  if (agent.retired) return " (retired)";
  if (scheduleAgentManaged(agent)) return " (managed)";
  return "";
}

export function scheduleSelectedAgentIssue(slug: string, agents: ScheduleAgent[]): string {
  const ref = slug.trim();
  if (!ref) return "";
  const agent = agents.find((p) => p.slug === ref);
  if (!agent) return "";
  if (agent.retired) return `agent ${ref} is retired`;
  if (agent.enabled === false) return `agent ${ref} is paused`;
  if (scheduleAgentManaged(agent)) return `agent ${ref} is a managed sub-agent`;
  return "";
}

export function scheduleToolAgentIssue(tool: string, agentSlug: string, agents: ScheduleAgent[]): string {
  const toolName = tool.trim();
  const slug = agentSlug.trim();
  if (!toolName || !slug) return "";
  const agent = agents.find((p) => p.slug === slug);
  if (!agent) return "";
  const lower = toolName.toLowerCase();
  const deny = new Set((agent.tool_deny || []).map((name) => name.trim().toLowerCase()).filter(Boolean));
  if (deny.has(lower)) return `agent ${slug} cannot schedule tool ${toolName}: agent tool denylist`;
  const allow = new Set((agent.tool_allow || []).map((name) => name.trim().toLowerCase()).filter(Boolean));
  if (allow.size > 0 && !allow.has(lower)) return `agent ${slug} cannot schedule tool ${toolName}: not in agent tool allowlist`;
  return "";
}

function workflowLabel(workflows: ScheduleWorkflow[], name: string): string {
  const w = workflows.find((p) => p.name === name || p.id === name);
  return w?.id && w.id !== w.name ? `${w.name} (${w.id})` : name;
}

function systemTaskLabel(tasks: ScheduleSystemTaskInfo[], name: string): string {
  const task = tasks.find((t) => t.name === name);
  return task?.label ? `${task.label} (${name})` : name;
}

type ScheduleMode = "interval" | "continuous" | "window" | "daily" | "once";
export type ScheduleTarget = "agent" | "workflow" | "system_task" | "tool";

interface ScheduleChoice {
  value: string;
  label: string;
  detail?: string;
  disabled?: boolean;
  icon?: ReactNode;
}

function ScheduleChoicePicker({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: ScheduleChoice[];
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1 text-sm text-muted">
      <span>{label}</span>
      <div className="grid gap-1.5" role="group" aria-label={label}>
        {options.map((option) => {
          const selected = value === option.value;
          return (
            <button
              key={option.value || "__empty"}
              type="button"
              disabled={option.disabled}
              aria-pressed={selected}
              onClick={() => onChange(option.value)}
              className={cn(
                "flex min-h-10 items-start gap-2 rounded-lg border px-2.5 py-2 text-left text-xs transition",
                selected
                  ? "border-accent bg-accent/10 text-foreground"
                  : "border-border bg-panel/45 text-muted hover:border-accent/50 hover:text-foreground",
                option.disabled && "cursor-not-allowed opacity-45 hover:border-border hover:text-muted",
              )}
            >
              {option.icon && <span className="mt-0.5 shrink-0 text-accent">{option.icon}</span>}
              <span className="min-w-0">
                <span className="block truncate font-semibold">{option.label}</span>
                {option.detail && <span className="mt-0.5 block truncate text-[11px] text-muted">{option.detail}</span>}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function ScheduleUnitPicker({
  label,
  value,
  onChange,
}: {
  label: string;
  value: "minutes" | "hours";
  onChange: (value: "minutes" | "hours") => void;
}) {
  return (
    <div className="inline-flex overflow-hidden rounded-md border border-border" role="group" aria-label={label}>
      {(["minutes", "hours"] as const).map((unit) => (
        <button
          key={unit}
          type="button"
          aria-pressed={value === unit}
          onClick={() => onChange(unit)}
          className={cn(
            "px-2 py-1 text-xs transition-colors",
            value === unit ? "bg-accent/15 text-accent" : "bg-panel text-muted hover:text-foreground",
          )}
        >
          {unit}
        </button>
      ))}
    </div>
  );
}

export function scheduleIntentFieldHint(target: ScheduleTarget): string {
  if (target === "agent") return "This is the task handed to the selected agent when cron wakes it.";
  if (target === "workflow") return "Optional label only; the workflow definition supplies the actual steps.";
  if (target === "system_task") return "Optional label only; the daemon runs the selected system task as a typed cron call.";
  return "Optional label only; the selected tool and payload define the call.";
}

export function schedulePayloadContract(target: ScheduleTarget, payloadText: string): string {
  if (target !== "workflow" && target !== "tool") return "";
  const kind = target === "workflow" ? "workflow" : "tool";
  const clean = payloadText.trim();
  if (!clean) return `cron passes no ${kind} payload`;
  try {
    const parsed = JSON.parse(clean);
    const shape = Array.isArray(parsed) ? "array" : parsed && typeof parsed === "object" ? "object" : typeof parsed;
    return `cron passes ${shape} JSON ${kind} payload`;
  } catch {
    return `invalid ${kind} payload JSON`;
  }
}

function safeParsePayloadShape(payloadText: string): unknown {
  try {
    return JSON.parse(payloadText);
  } catch {
    return undefined;
  }
}


export function scheduleFormCadenceLabel(
  mode: ScheduleMode,
  everyN: string,
  everyUnit: "minutes" | "hours",
  dailyAt: string,
  windowStart: string,
  windowEnd: string,
  onceAt: string,
): string {
  const amount = String(Number(everyN) > 0 ? everyN : "0").trim();
  if (mode === "interval") return `every ${amount} ${everyUnit}`;
  if (mode === "continuous") return `cycle after ${amount} ${everyUnit}`;
  if (mode === "window") return `every ${amount} ${everyUnit} in ${windowStart}-${windowEnd}`;
  if (mode === "daily") return `daily at ${dailyAt}`;
  return onceAt ? `once at ${onceAt}` : "once";
}

function scheduleInitialMode(mode?: string): ScheduleMode {
  if (mode === "continuous") return "continuous";
  if (mode === "window") return "window";
  if (mode === "daily") return "daily";
  if (mode === "once") return "once";
  return "interval";
}

function intervalParts(sec?: number): { amount: string; unit: "minutes" | "hours" } {
  if (!sec || sec < 1) return { amount: "30", unit: "minutes" };
  if (sec % 3600 === 0) return { amount: String(sec / 3600), unit: "hours" };
  return { amount: String(Math.max(1, Math.round(sec / 60))), unit: "minutes" };
}

function minutesToTime(minutes?: number): string {
  if (minutes === undefined || minutes < 0) return "09:00";
  const h = Math.floor(minutes / 60) % 24;
  const m = minutes % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

function timeToMinutes(value: string): number | null {
  if (!/^\d{1,2}:\d{2}$/.test(value)) return null;
  const [h, m] = value.split(":").map(Number);
  if (h < 0 || h > 23 || m < 0 || m > 59) return null;
  return h * 60 + m;
}

function unixToLocalInput(unix?: number): string {
  if (!unix) return "";
  const d = new Date(unix * 1000);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// NewScheduleForm creates OR edits a cron-like scheduled job from the UI (M715
// create; M728 edit). The target is structured: agent task, workflow, system
// task, or tool. The free-text field is the agent task only for agent targets;
// for other targets it is just an operator label.
export function NewScheduleForm({
  onCreated,
  onError,
  editId,
  initialIntent,
  initialModel,
  initialAgent,
  initialTarget,
  initialWorkflow,
  initialPayload,
  initialSystemTask,
  initialTool,
  initialMode,
  initialIntervalSec,
  initialAtMinutes,
  initialEndMinutes,
  initialDays,
  initialTz,
  initialOnceAtUnix,
  agents = [],
  workflows = [],
  tools = [],
  systemTasks = FALLBACK_SYSTEM_TASKS,
  systemTaskInfo = FALLBACK_SYSTEM_TASK_INFO,
}: {
  onCreated: () => void;
  onError: (msg: string) => void;
  // When set, the form edits this schedule instead of creating a new one (M728).
  editId?: string;
  initialIntent?: string;
  initialModel?: string;
  initialAgent?: string;
  initialTarget?: ScheduleTarget;
  initialWorkflow?: string;
  initialPayload?: string;
  initialSystemTask?: string;
  initialTool?: string;
  initialMode?: string;
  initialIntervalSec?: number;
  initialAtMinutes?: number;
  initialEndMinutes?: number;
  initialDays?: number;
  initialTz?: string;
  initialOnceAtUnix?: number;
  agents?: ScheduleAgent[];
  workflows?: ScheduleWorkflow[];
  tools?: ScheduleTool[];
  systemTasks?: string[];
  systemTaskInfo?: ScheduleSystemTaskInfo[];
}) {
  const editing = !!editId;
  const [intent, setIntent] = useState(initialIntent ?? "");
  const [modelRef, setModelRef] = useState(initialModel ?? "");
  const [target, setTarget] = useState<ScheduleTarget>(initialTarget ?? "agent");
  const [agentRef, setAgentRef] = useState(initialAgent ?? "");
  const [workflowRef, setWorkflowRef] = useState(initialWorkflow ?? "");
  const [payloadText, setPayloadText] = useState(initialPayload ?? "");
  const [systemTask, setSystemTask] = useState(initialSystemTask || systemTasks[0] || "catalog_sync");
  const [toolRef, setToolRef] = useState(initialTool ?? "");
  const initialInterval = intervalParts(initialIntervalSec);
  const [mode, setMode] = useState<ScheduleMode>(scheduleInitialMode(initialMode));
  const [everyN, setEveryN] = useState(initialInterval.amount);
  const [everyUnit, setEveryUnit] = useState<"minutes" | "hours">(initialInterval.unit);
  const [dailyAt, setDailyAt] = useState(minutesToTime(initialAtMinutes));
  const [windowStart, setWindowStart] = useState(minutesToTime(initialAtMinutes));
  const [windowEnd, setWindowEnd] = useState(minutesToTime(initialEndMinutes ?? 1020));
  const [windowDays] = useState(initialDays ?? 0);
  const [windowTz] = useState(initialTz ?? "");
  const [onceAt, setOnceAt] = useState(unixToLocalInput(initialOnceAtUnix));
  const [timingDirty, setTimingDirty] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const firstAgentSlug = agents.find((a) => !scheduleAgentDisabled(a))?.slug || "";
  const firstWorkflowName = workflows.find((w) => w.enabled !== false)?.name || "";
  const firstToolName = tools[0]?.name || "";
  const effectiveSystemTasks = systemTasks.length ? systemTasks : FALLBACK_SYSTEM_TASKS;
  const effectiveSystemTaskInfo = systemTaskInfo.length ? systemTaskInfo : FALLBACK_SYSTEM_TASK_INFO;
  const selectedSystemTaskInfo =
    effectiveSystemTaskInfo.find((task) => task.name === systemTask) ||
    FALLBACK_SYSTEM_TASK_INFO.find((task) => task.name === systemTask);

  useEffect(() => {
    if (!editing && target === "agent" && !agentRef && firstAgentSlug) setAgentRef(firstAgentSlug);
  }, [agentRef, editing, firstAgentSlug, target]);
  useEffect(() => {
    if (!editing && target === "workflow" && !workflowRef && firstWorkflowName) setWorkflowRef(firstWorkflowName);
  }, [editing, firstWorkflowName, target, workflowRef]);
  useEffect(() => {
    if (!editing && target === "tool" && !toolRef && firstToolName) setToolRef(firstToolName);
  }, [editing, firstToolName, target, toolRef]);
  useEffect(() => {
    if (!editing && target === "system_task" && !effectiveSystemTasks.includes(systemTask)) {
      setSystemTask(effectiveSystemTasks[0] || "catalog_sync");
    }
  }, [editing, effectiveSystemTasks, systemTask, target]);
  useEffect(() => {
    if (editing || timingDirty || target !== "system_task" || mode !== "interval") return;
    const recommended = selectedSystemTaskInfo?.recommended_interval_sec || 0;
    if (recommended <= 0) return;
    const parts = intervalParts(recommended);
    setEveryN(parts.amount);
    setEveryUnit(parts.unit);
  }, [editing, mode, selectedSystemTaskInfo?.recommended_interval_sec, target, timingDirty]);

  const intervalSec = Math.max(1, Number(everyN) || 0) * (everyUnit === "hours" ? 3600 : 60);
  const windowStartMinutes = timeToMinutes(windowStart);
  const windowEndMinutes = timeToMinutes(windowEnd);
  const validTiming =
    (mode === "interval" && Number(everyN) > 0) ||
    (mode === "continuous" && Number(everyN) > 0) ||
    (mode === "window" &&
      Number(everyN) > 0 &&
      windowStartMinutes !== null &&
      windowEndMinutes !== null &&
      windowStartMinutes < windowEndMinutes) ||
    (mode === "daily" && /^\d{1,2}:\d{2}$/.test(dailyAt)) ||
    (mode === "once" && onceAt !== "");
  const valid =
    validTiming &&
    (target === "system_task" ||
      (target === "workflow" && workflowRef.trim() !== "") ||
      (target === "tool" && toolRef.trim() !== "") ||
      intent.trim() !== "");
  const showRunAsAgent = target === "workflow" || target === "tool";
  const taskLabel = target === "agent" ? "Agent task" : "Schedule label";
  const taskHint = scheduleIntentFieldHint(target);
  const selectedAgentIssue = scheduleSelectedAgentIssue(agentRef, agents);
  const selectedToolAgentIssue = target === "tool" ? scheduleToolAgentIssue(toolRef, agentRef, agents) : "";
  const payloadContract = schedulePayloadContract(target, payloadText);
  const payloadInvalid = payloadContract.includes("invalid");
  const formCadence = scheduleFormCadenceLabel(mode, everyN, everyUnit, dailyAt, windowStart, windowEnd, onceAt);
  const agentOptions: ScheduleChoice[] = [
    { value: "", label: "No roster agent", detail: "Run without binding a roster identity", icon: <Bot className="size-3.5" /> },
    ...agents.map((a) => ({
      value: a.slug,
      label: agentLabel(agents, a.slug),
      detail: scheduleAgentStateLabel(a).trim().replace(/[()]/g, "") || "ready for direct scheduling",
      disabled: scheduleAgentDisabled(a),
      icon: <Bot className="size-3.5" />,
    })),
    ...(initialAgent && !agents.some((a) => a.slug === initialAgent)
      ? [{ value: initialAgent, label: initialAgent, detail: "existing binding", icon: <Bot className="size-3.5" /> }]
      : []),
  ];

  async function create() {
    if (!valid) return;
    const agentIssue = scheduleSelectedAgentIssue(agentRef, agents);
    if ((target === "agent" || showRunAsAgent) && agentIssue) {
      onError(agentIssue);
      return;
    }
    const toolAgentIssue = target === "tool" ? scheduleToolAgentIssue(toolRef, agentRef, agents) : "";
    if (toolAgentIssue) {
      onError(toolAgentIssue);
      return;
    }
    setSubmitting(true);
    try {
      const args: Record<string, unknown> = {};
      if (intent.trim()) args.intent = intent.trim();
      if ((target === "agent" || target === "workflow") && modelRef.trim()) args.model = modelRef.trim();
      if (editing) args.id = editId;
      if (target === "system_task") {
        args.target = "system_task";
        args.system_task = systemTask;
        if (editing) args.agent = "";
        if (editing) args.model = "";
      } else if (target === "workflow") {
        args.target = "workflow";
        args.workflow = workflowRef.trim();
        if (agentRef.trim() || editing) args.agent = agentRef.trim();
        if (payloadText.trim()) {
          try {
            args.payload = JSON.parse(payloadText);
          } catch (e) {
            onError(`Invalid workflow payload JSON: ${(e as Error).message}`);
            return;
          }
        }
      } else if (target === "tool") {
        args.target = "tool";
        args.tool = toolRef.trim();
        if (agentRef.trim() || editing) args.agent = agentRef.trim();
        if (editing) args.model = "";
        if (payloadText.trim()) {
          try {
            args.payload = JSON.parse(payloadText);
          } catch (e) {
            onError(`Invalid tool payload JSON: ${(e as Error).message}`);
            return;
          }
        }
      } else if (agentRef.trim() || editing) {
        args.agent = agentRef.trim();
        if (editing) args.target = "";
      }
      if (!editing || timingDirty) {
        if (mode === "interval") {
          args.interval_sec = intervalSec;
        } else if (mode === "continuous") {
          args.cooldown_sec = intervalSec;
        } else if (mode === "window") {
          if (windowStartMinutes === null || windowEndMinutes === null || windowStartMinutes >= windowEndMinutes) {
            return onError("Invalid window time range");
          }
          args.interval_sec = intervalSec;
          args.window_start = windowStartMinutes;
          args.window_end = windowEndMinutes;
          args.days = windowDays;
          if (windowTz.trim()) args.tz = windowTz.trim();
        } else if (mode === "daily") {
          const [h, m] = dailyAt.split(":").map(Number);
          args.at_minutes = h * 60 + m;
          args.days = 0; // every day
        } else {
          const ms = Date.parse(onceAt);
          if (Number.isNaN(ms)) return onError("Invalid date/time");
          args.once_at_unix = Math.floor(ms / 1000);
        }
      }
      await postJSON(editing ? "/api/schedule/edit" : "/api/schedule/add", args);
      onCreated();
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  function changeTarget(next: ScheduleTarget) {
    setTarget(next);
    if (next === "tool" || next === "system_task") setModelRef("");
    if (!editing && (next === "workflow" || next === "tool" || next === "system_task")) setAgentRef("");
    if (!editing && next === "agent" && !agentRef && firstAgentSlug) setAgentRef(firstAgentSlug);
    if (next === "workflow" && !workflowRef && firstWorkflowName) setWorkflowRef(firstWorkflowName);
    if (next === "tool" && !toolRef && firstToolName) setToolRef(firstToolName);
  }

  function applySystemTaskPreset(task: string, label: string) {
    const info =
      effectiveSystemTaskInfo.find((row) => row.name === task) ||
      FALLBACK_SYSTEM_TASK_INFO.find((row) => row.name === task);
    setTarget("system_task");
    setAgentRef("");
    setModelRef("");
    setSystemTask(task);
    setIntent(label);
    setMode("interval");
    const recommended = info?.recommended_interval_sec || 0;
    if (recommended > 0) {
      const parts = intervalParts(recommended);
      setEveryN(parts.amount);
      setEveryUnit(parts.unit);
    }
    setTimingDirty(true);
  }

  return (
    <div className="glass rounded-xl p-3">
      <div className="grid gap-2 sm:grid-cols-[minmax(150px,190px)_minmax(180px,240px)_1fr]">
        <ScheduleChoicePicker
          label="Schedule target"
          value={target}
          onChange={(value) => changeTarget(value as ScheduleTarget)}
          options={[
            { value: "agent", label: "Agent task", detail: "Wake a roster identity", icon: <Bot className="size-3.5" /> },
            { value: "workflow", label: "Workflow", detail: "Trigger a saved graph", icon: <GitFork className="size-3.5" /> },
            { value: "system_task", label: "System task", detail: "Daemon cron job", icon: <ShieldCheck className="size-3.5" /> },
            { value: "tool", label: "Tool", detail: "Typed tool call", icon: <Wrench className="size-3.5" /> },
          ]}
        />
        {target === "agent" ? (
          <div className="flex flex-col gap-1 text-sm text-muted">
            <ScheduleChoicePicker label="Roster agent" value={agentRef} onChange={setAgentRef} options={agentOptions} />
            {selectedAgentIssue && (
              <span className="text-xs leading-snug text-warn">{selectedAgentIssue}</span>
            )}
            {selectedToolAgentIssue && (
              <span className="text-xs leading-snug text-warn">{selectedToolAgentIssue}</span>
            )}
          </div>
        ) : target === "workflow" ? (
          <ScheduleChoicePicker
            label="Workflow"
            value={workflowRef}
            onChange={setWorkflowRef}
            options={[
              { value: "", label: "Select workflow", detail: "Choose a graph before saving", icon: <GitFork className="size-3.5" /> },
              ...workflows.map((w) => ({
                value: w.name,
                label: workflowLabel(workflows, w.name),
                detail: w.enabled === false ? "disabled" : "enabled workflow",
                disabled: w.enabled === false,
                icon: <GitFork className="size-3.5" />,
              })),
              ...(initialWorkflow && !workflows.some((w) => w.name === initialWorkflow || w.id === initialWorkflow)
                ? [{ value: initialWorkflow, label: initialWorkflow, detail: "existing binding", icon: <GitFork className="size-3.5" /> }]
                : []),
            ]}
          />
        ) : target === "system_task" ? (
          <div className="flex flex-col gap-1 text-sm text-muted">
            <ScheduleChoicePicker
              label="System task"
              value={systemTask}
              onChange={setSystemTask}
              options={[
                ...effectiveSystemTasks.map((task) => ({
                  value: task,
                  label: systemTaskLabel(effectiveSystemTaskInfo, task),
                  detail: systemTaskExecutionLabel(effectiveSystemTaskInfo.find((info) => info.name === task)),
                  icon: <ShieldCheck className="size-3.5" />,
                })),
                ...(initialSystemTask && !effectiveSystemTasks.includes(initialSystemTask)
                  ? [{ value: initialSystemTask, label: initialSystemTask, detail: "existing binding", icon: <ShieldCheck className="size-3.5" /> }]
                  : []),
              ]}
            />
            {selectedSystemTaskInfo?.description && (
              <span className="text-xs leading-snug text-muted/80">
                {selectedSystemTaskInfo.description}
              </span>
            )}
            <span className="text-xs leading-snug text-muted/80">
              {systemTaskExecutionLabel(selectedSystemTaskInfo)}
              {selectedSystemTaskInfo?.effect ? ` - ${selectedSystemTaskInfo.effect}` : ""}
            </span>
            {!!selectedSystemTaskInfo?.recommended_interval_sec && (
              <span className="text-xs leading-snug text-muted/80">
                Recommended cadence: every {intervalParts(selectedSystemTaskInfo.recommended_interval_sec).amount}{" "}
                {intervalParts(selectedSystemTaskInfo.recommended_interval_sec).unit}
              </span>
            )}
          </div>
        ) : (
          <ScheduleChoicePicker
            label="Tool"
            value={toolRef}
            onChange={setToolRef}
            options={[
              { value: "", label: "Select tool", detail: "Choose a typed tool call", icon: <Wrench className="size-3.5" /> },
              ...tools.map((t) => ({
                value: t.name,
                label: t.name,
                detail: t.description || "tool",
                icon: <Wrench className="size-3.5" />,
              })),
              ...(initialTool && !tools.some((t) => t.name === initialTool)
                ? [{ value: initialTool, label: initialTool, detail: "existing binding", icon: <Wrench className="size-3.5" /> }]
                : []),
            ]}
          />
        )}
        <label className="flex flex-col gap-1 text-sm text-muted">
          {taskLabel}
          <textarea
            value={intent}
            onChange={(e) => setIntent(e.target.value)}
            placeholder={target === "agent" ? "Task to hand to the selected agent when this fires..." : "Optional schedule label..."}
            aria-label={taskLabel}
            className="h-16 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
          <span className="text-xs leading-snug text-muted/80">{taskHint}</span>
        </label>
      </div>

      {!editing && (
        <div className="mt-2 rounded-md border border-border bg-panel/35 px-2 py-1.5">
          <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
            <Wrench className="size-3" /> Daemon cron presets
          </div>
          <div className="flex flex-wrap gap-1.5">
            {SYSTEM_TASK_QUICK_PRESETS.filter((preset) => effectiveSystemTasks.includes(preset.task)).map((preset) => (
              <Button
                key={preset.task}
                type="button"
                variant="ghost"
                size="sm"
                title={`Create typed system task schedule: ${scheduleSystemTaskPresetLabel(preset.task, effectiveSystemTaskInfo)}`}
                onClick={() => applySystemTaskPreset(preset.task, preset.label)}
              >
                <RefreshCw className="size-3.5" /> {scheduleSystemTaskPresetLabel(preset.task, effectiveSystemTaskInfo)}
              </Button>
            ))}
          </div>
        </div>
      )}

      {/* One cadence confirmation line + payload validation — no prose manifests. */}
      <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted">
        <span className="inline-flex items-center gap-1.5">
          <CalendarClock className="size-3.5 text-accent" /> {formCadence}
        </span>
        {payloadInvalid && <span className="text-bad">{payloadContract}</span>}
      </div>

      {showRunAsAgent && (
        <div className={cn("mt-2 grid gap-2", target === "workflow" && "sm:grid-cols-2")}>
          <div className="flex flex-col gap-1 text-sm text-muted">
            <ScheduleChoicePicker
              label="Run as agent"
              value={agentRef}
              onChange={setAgentRef}
              options={[
                { value: "", label: "System identity", detail: "Daemon policy boundary", icon: <ShieldCheck className="size-3.5" /> },
                ...agentOptions.slice(1),
              ]}
            />
            {selectedAgentIssue && (
              <span className="text-xs leading-snug text-warn">{selectedAgentIssue}</span>
            )}
            {selectedToolAgentIssue && (
              <span className="text-xs leading-snug text-warn">{selectedToolAgentIssue}</span>
            )}
          </div>
          {target === "workflow" && (
            <label className="flex flex-col gap-1 text-sm text-muted">
              Model override
              <input
                value={modelRef}
                onChange={(e) => setModelRef(e.target.value)}
                placeholder="agent/default model"
                aria-label="Model override"
                className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
              />
            </label>
          )}
        </div>
      )}

      {target === "agent" && (
        <label className="mt-2 flex flex-col gap-1 text-sm text-muted sm:max-w-[240px]">
          Model override
          <input
            value={modelRef}
            onChange={(e) => setModelRef(e.target.value)}
            placeholder="agent/default model"
            aria-label="Model override"
            className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
        </label>
      )}

      {(target === "workflow" || target === "tool") && (
        <label className="mt-2 flex flex-col gap-1 text-sm text-muted">
          {target === "workflow" ? "Workflow payload JSON" : "Tool payload JSON"}
          <textarea
            value={payloadText}
            onChange={(e) => setPayloadText(e.target.value)}
            placeholder='{"force":true}'
            aria-label={target === "workflow" ? "Workflow payload JSON" : "Tool payload JSON"}
            className="h-16 w-full resize-y rounded-md border border-border bg-panel p-2 font-mono text-xs text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
          />
          <span className={cn("text-xs leading-snug", payloadContract.startsWith("invalid") ? "text-bad" : "text-muted/80")}>
            {payloadContract}
          </span>
        </label>
      )}

      <div className="mt-2 flex flex-col gap-1 text-sm text-muted">
        When
        <div className="flex flex-wrap items-center gap-1.5">
          <div className="inline-flex overflow-hidden rounded-md border border-border">
            {(["interval", "continuous", "window", "daily", "once"] as const).map((m) => (
              <button
                key={m}
                onClick={() => {
                  setMode(m);
                  setTimingDirty(true);
                }}
                className={cn(
                  "px-2 py-1 text-xs transition-colors",
                  mode === m ? "bg-accent/15 text-accent" : "text-muted hover:text-foreground",
                )}
              >
                {m === "interval"
                  ? "every…"
                  : m === "continuous"
                    ? "cycle…"
                    : m === "window"
                      ? "within window…"
                      : m === "daily"
                        ? "daily at…"
                        : "once at…"}
              </button>
            ))}
          </div>

          {(mode === "interval" || mode === "continuous") && (
            <div className="flex items-center gap-1.5">
              <input
                type="number"
                min={1}
                value={everyN}
                onChange={(e) => {
                  setEveryN(e.target.value);
                  setTimingDirty(true);
                }}
                aria-label={mode === "continuous" ? "Cycle cooldown amount" : "Interval amount"}
                className="w-20 rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
              <ScheduleUnitPicker
                label={mode === "continuous" ? "Cycle cooldown unit" : "Interval unit"}
                value={everyUnit}
                onChange={(unit) => {
                  setEveryUnit(unit);
                  setTimingDirty(true);
                }}
              />
              {mode === "continuous" && <span className="text-sm text-muted">after each completed run</span>}
            </div>
          )}
          {mode === "window" && (
            <div className="flex flex-wrap items-center gap-1.5">
              <input
                type="number"
                min={1}
                value={everyN}
                onChange={(e) => {
                  setEveryN(e.target.value);
                  setTimingDirty(true);
                }}
                aria-label="Window interval amount"
                className="w-20 rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
              <ScheduleUnitPicker
                label="Window interval unit"
                value={everyUnit}
                onChange={(unit) => {
                  setEveryUnit(unit);
                  setTimingDirty(true);
                }}
              />
              <input
                type="time"
                value={windowStart}
                onChange={(e) => {
                  setWindowStart(e.target.value);
                  setTimingDirty(true);
                }}
                aria-label="Window start time"
                className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
              <span className="text-sm text-muted">to</span>
              <input
                type="time"
                value={windowEnd}
                onChange={(e) => {
                  setWindowEnd(e.target.value);
                  setTimingDirty(true);
                }}
                aria-label="Window end time"
                className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
              />
            </div>
          )}
          {mode === "daily" && (
            <input
              type="time"
              value={dailyAt}
              onChange={(e) => {
                setDailyAt(e.target.value);
                setTimingDirty(true);
              }}
              aria-label="Daily time"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          )}
          {mode === "once" && (
            <input
              type="datetime-local"
              value={onceAt}
              onChange={(e) => {
                setOnceAt(e.target.value);
                setTimingDirty(true);
              }}
              aria-label="Once date and time"
              className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
            />
          )}
        </div>
      </div>

      {editing && (
        <p className="mt-2 text-xs text-muted">
          Editing saves the selected job target and optional label. Cadence is only changed when you edit the timing controls.
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
