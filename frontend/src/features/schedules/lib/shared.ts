// The Schedules view's data model and derivations: the wire shapes the schedule
// endpoints return, and the pure functions that turn them into what the console
// shows — counts, labels, attention reasons, health passports, filters.
//
// Split out of Schedules.tsx (refactor Phase 4.1), mirroring views/roster/.
// Everything here is pure and unit-tested; nothing renders. Keeping it apart
// from the view means a derivation can be reasoned about — and its test read —
// without scrolling past six hundred lines of JSX.

import type {
  AgentListResponseProfilesItem,
  ScheduleAddResponse,
  ScheduleFiresResponseFiresItem,
  ScheduleSystemTasksResponseSystemTaskInfoItem,
  ToolListResponseToolsItem,
  WorkflowListResponseWorkflowsItem,
} from "@/app/api/types.generated";

export interface Sched {
  id: ScheduleAddResponse["id"];
  intent?: ScheduleAddResponse["intent"];
  model?: ScheduleAddResponse["model"];
  agent?: ScheduleAddResponse["agent"];
  target?: ScheduleAddResponse["target"];
  workflow?: ScheduleAddResponse["workflow"];
  system_task?: ScheduleAddResponse["system_task"];
  tool?: ScheduleAddResponse["tool"];
  payload?: ScheduleAddResponse["payload"];
  cadence?: ScheduleAddResponse["cadence"];
  mode?: ScheduleAddResponse["mode"];
  interval_sec?: ScheduleAddResponse["interval_sec"];
  at_minutes?: ScheduleAddResponse["at_minutes"];
  end_minutes?: ScheduleAddResponse["end_minutes"];
  days?: ScheduleAddResponse["days"];
  tz?: ScheduleAddResponse["tz"];
  source?: ScheduleAddResponse["source"];
  enabled?: ScheduleAddResponse["enabled"];
  next_run_unix?: ScheduleAddResponse["next_run_unix"];

  last_status?: ScheduleAddResponse["last_status"];
  frequency_warning?: ScheduleAddResponse["frequency_warning"];
  fires?: ScheduleAddResponse["fires"];
  assure?: ScheduleAddResponse["assure"];
  executor?: ScheduleAddResponse["executor"];
  uses_llm?: ScheduleAddResponse["uses_llm"];
  execution_contract?: ScheduleAddResponse["execution_contract"];
  target_status?: ScheduleAddResponse["target_status"];
  target_error?: ScheduleAddResponse["target_error"];}

export type ScheduleFire = {
  correlation_id?: ScheduleFiresResponseFiresItem["correlation_id"];
  schedule_id?: ScheduleFiresResponseFiresItem["schedule_id"];
  fired_unix_ms?: ScheduleFiresResponseFiresItem["fired_unix_ms"];
  intent?: ScheduleFiresResponseFiresItem["intent"];
  action?: ScheduleFiresResponseFiresItem["action"];
  model?: ScheduleFiresResponseFiresItem["model"];
  target?: ScheduleFiresResponseFiresItem["target"];
  agent?: ScheduleFiresResponseFiresItem["agent"];
  workflow?: ScheduleFiresResponseFiresItem["workflow"];
  system_task?: ScheduleFiresResponseFiresItem["system_task"];
  tool?: ScheduleFiresResponseFiresItem["tool"];
  executor?: ScheduleFiresResponseFiresItem["executor"];
  category?: ScheduleFiresResponseFiresItem["category"];
  effect_class?: ScheduleFiresResponseFiresItem["effect_class"];
  uses_llm?: ScheduleFiresResponseFiresItem["uses_llm"];
  status?: ScheduleFiresResponseFiresItem["status"];
  reason?: ScheduleFiresResponseFiresItem["reason"];
  duration_ms?: ScheduleFiresResponseFiresItem["duration_ms"];};

export interface ScheduleAgent {
  slug: AgentListResponseProfilesItem["slug"];
  name?: AgentListResponseProfilesItem["name"];
  enabled?: AgentListResponseProfilesItem["enabled"];
  retired?: AgentListResponseProfilesItem["retired"];
  managed?: AgentListResponseProfilesItem["managed"];
  direct_callable?: AgentListResponseProfilesItem["direct_callable"];
  kind?: AgentListResponseProfilesItem["kind"];
  tool_allow?: AgentListResponseProfilesItem["tool_allow"];
  tool_deny?: AgentListResponseProfilesItem["tool_deny"];}

export interface ScheduleWorkflow {
  id?: WorkflowListResponseWorkflowsItem["id"];
  name: WorkflowListResponseWorkflowsItem["name"];
  enabled?: WorkflowListResponseWorkflowsItem["enabled"];}

export type ScheduleTool = {
  name: ToolListResponseToolsItem["name"];
  description?: ToolListResponseToolsItem["description"];};

export type ScheduleSystemTaskInfo = {
  name: ScheduleSystemTasksResponseSystemTaskInfoItem["name"];
  label?: ScheduleSystemTasksResponseSystemTaskInfoItem["label"];
  description?: ScheduleSystemTasksResponseSystemTaskInfoItem["description"];
  category?: ScheduleSystemTasksResponseSystemTaskInfoItem["category"];
  executor?: ScheduleSystemTasksResponseSystemTaskInfoItem["executor"];
  uses_llm?: ScheduleSystemTasksResponseSystemTaskInfoItem["uses_llm"];
  effect_class?: ScheduleSystemTasksResponseSystemTaskInfoItem["effect_class"];
  effect?: ScheduleSystemTasksResponseSystemTaskInfoItem["effect"];
  recommended_interval_sec?: ScheduleSystemTasksResponseSystemTaskInfoItem["recommended_interval_sec"];};

export const FALLBACK_SYSTEM_TASKS = ["catalog_sync", "artifact_collect", "memory_clean", "memory_tidy", "log_clean", "graveyard_scan"];
export const FALLBACK_SYSTEM_TASK_INFO: ScheduleSystemTaskInfo[] = [
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

export const SYSTEM_TASK_QUICK_PRESETS = [
  { task: "catalog_sync", label: "Sync models catalog" },
  { task: "artifact_collect", label: "Collect run artifacts" },
  { task: "memory_tidy", label: "Tidy memory" },
  { task: "log_clean", label: "Inspect log pressure" },
  { task: "graveyard_scan", label: "Scan graveyard retention" },
];

// sourceTone colours the origin badge: an agent-scheduled run (the agent used
// the `schedule` tool to arrange its own future work) is the notable one, so it
// gets the accent; operator/env are muted.
export function sourceTone(src?: string): string {
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
export const SCHEDULE_ROW_WINDOW = 60;

// Schedules is the autonomy cockpit: every cron-like job — whether it wakes an
// agent, runs a workflow, invokes a tool, or performs a system task — with its
// cadence, next fire, last outcome and origin, plus run-now / pause-resume /
// remove controls so unattended work stays governable.

export function agentLabel(agents: ScheduleAgent[], slug: string): string {
  const a = agents.find((p) => p.slug === slug);
  return a?.name ? `${a.name} (${slug})` : slug;
}

export function scheduleResumeIssue(schedule: Pick<Sched, "agent">, agents: ScheduleAgent[]): string {
  if (!schedule.agent) return "";
  const agent = agents.find((p) => p.slug === schedule.agent);
  if (!agent) return `agent ${schedule.agent} is missing`;
  if (agent.retired) return `agent ${schedule.agent} is retired`;
  if (agent.enabled === false) return `agent ${schedule.agent} is paused`;
  if (scheduleAgentManaged(agent)) return `agent ${schedule.agent} is a managed sub-agent`;
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

export function intervalParts(sec?: number): { amount: string; unit: "minutes" | "hours" } {
  if (!sec || sec < 1) return { amount: "30", unit: "minutes" };
  if (sec % 3600 === 0) return { amount: String(sec / 3600), unit: "hours" };
  return { amount: String(Math.max(1, Math.round(sec / 60))), unit: "minutes" };
}

export function scheduleAgentManaged(agent: Pick<ScheduleAgent, "kind" | "managed" | "direct_callable">): boolean {
  return agent.kind === "subagent" || !!agent.managed || agent.direct_callable === false;
}
