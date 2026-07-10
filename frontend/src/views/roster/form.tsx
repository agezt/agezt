import { useState, type ReactNode } from "react";
import { X, Plus, Pencil, Zap, GitBranch, Activity, RefreshCw, Archive, ShieldCheck, AlertTriangle, Pause, CalendarClock, Wrench, IdCard, ListTree, Cpu, CheckCheck, type LucideIcon } from "lucide-react";
import { postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Advanced, Disclosure } from "@/components/ui/disclosure";
import { ModelPicker } from "@/components/ModelPicker";
import { isChainRef } from "@/lib/chains";
import { slugOk, usdToMc, type AgentLifecycle, type AgentProfile, type AgentTask } from "./shared";
import { agentTaskContractSummary } from "./passports";

function nonNegativeInt(s: string, label: string): number | string {
  const t = s.trim();
  if (t === "") return 0;
  const n = Number(t);
  if (!Number.isInteger(n) || n < 0) return `${label} must be a non-negative integer`;
  return n;
}

const inputCls =
  "rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent";

export function RosterModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: LucideIcon;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-sm">
      <div className="w-full max-w-3xl rounded-xl border border-border bg-panel shadow-2xl">
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <Icon className="size-4 text-accent" />
          <h2 className="text-sm font-semibold text-foreground">{title}</h2>
          <Button className="ml-auto" size="icon" variant="ghost" onClick={onClose} aria-label={`Close ${title}`}>
            <X className="size-4" />
          </Button>
        </div>
        <div className="max-h-[78vh] overflow-y-auto p-4">{children}</div>
      </div>
    </div>
  );
}

// profileFields collects the shared New/Edit form fields into the wire shape.
// Exported for unit tests (the model/fallbacks mapping, incl. the @chain
// self-contained rule, is the meaningful logic).
export function profileFields(f: {
  name: string;
  soul: string;
  instructions: string;
  model: string;
  fallbacks: string;
  taskType: string;
  maxCost: string;
  maxDaily: string;
  memoryScope: string;
  workdir: string;
  ownerAgent: string;
  parentAgent: string;
  directCallable: string;
  retryAttempts: string;
  retryBackoff: string;
  retryBaseDelay: string;
  retryMaxDelay: string;
  retryOn?: string;
  healthDoctor: string;
  healthFailureThreshold: string;
  selfRepairEnabled: string;
  selfRepairAttempts: string;
  selfRepairEscalate: string;
  trustCeiling: string;
  toolAllow: string;
  toolDeny: string;
  configOverrides: string;
  lifecycleMode: string;
  lifecycleMaxCycles: string;
  cycleTasks: string;
  totalTasks: string;
  description: string;
}): Record<string, unknown> | string {
  const mc = usdToMc(f.maxCost);
  if (mc === null) return "max cost must be a dollar amount like 0.50";
  const dailyMc = usdToMc(f.maxDaily);
  if (dailyMc === null) return "max daily must be a dollar amount like 5.00";
  const retryAttempts = nonNegativeInt(f.retryAttempts, "retry attempts");
  if (typeof retryAttempts === "string") return retryAttempts;
  const retryBaseDelay = nonNegativeInt(f.retryBaseDelay, "retry base delay");
  if (typeof retryBaseDelay === "string") return retryBaseDelay;
  const retryMaxDelay = nonNegativeInt(f.retryMaxDelay, "retry max delay");
  if (typeof retryMaxDelay === "string") return retryMaxDelay;
  const healthFailureThreshold = nonNegativeInt(f.healthFailureThreshold, "health failure threshold");
  if (typeof healthFailureThreshold === "string") return healthFailureThreshold;
  const selfRepairAttempts = nonNegativeInt(f.selfRepairAttempts, "self-repair attempts");
  if (typeof selfRepairAttempts === "string") return selfRepairAttempts;
  const lifecycleMaxCycles = nonNegativeInt(f.lifecycleMaxCycles, "max cycles");
  if (typeof lifecycleMaxCycles === "string") return lifecycleMaxCycles;
  if (f.directCallable === "false" && !f.ownerAgent.trim() && !f.parentAgent.trim()) {
    return "managed sub-agent needs an owner or parent agent";
  }
  const p: Record<string, unknown> = {
    name: f.name.trim(),
    soul: f.soul.trim(),
    instructions: lines(f.instructions),
    model: f.model.trim(),
    task_type: f.taskType.trim(),
    max_cost_mc: mc,
    max_daily_mc: dailyMc,
    memory_scope: f.memoryScope.trim(),
    workdir: f.workdir.trim(),
    owner_agent: f.ownerAgent.trim(),
    parent_agent: f.parentAgent.trim(),
    direct_callable: f.directCallable !== "false",
    description: f.description.trim(),
    trust_ceiling: f.trustCeiling.trim(),
    tool_allow: csvList(f.toolAllow),
    tool_deny: csvList(f.toolDeny),
    config_overrides: configMap(f.configOverrides),
  };
  const tasklist = [
    ...taskLines(f.cycleTasks, "cycle"),
    ...taskLines(f.totalTasks, "total"),
  ];
  if (tasklist.length > 0) p.tasklist = tasklist;
  const lifecycleMode = f.lifecycleMode.trim() || "persistent";
  const effectiveLifecycleMode = lifecycleMode === "persistent" && lifecycleMaxCycles > 0 ? "cycle" : lifecycleMode;
  if (f.lifecycleMode.trim() || lifecycleMaxCycles) {
    p.lifecycle = {
      mode: effectiveLifecycleMode,
      retire_on_complete: effectiveLifecycleMode === "retire_on_complete",
      max_cycles: lifecycleMaxCycles,
    };
  }
  const retryOn = csvList(f.retryOn || "");
  if (retryAttempts || retryBaseDelay || retryMaxDelay || f.retryBackoff.trim() || retryOn.length > 0) {
    p.retry_policy = {
      max_attempts: retryAttempts,
      backoff: f.retryBackoff.trim() || "exponential",
      base_delay_sec: retryBaseDelay,
      max_delay_sec: retryMaxDelay,
      retry_on: retryOn,
    };
  }
  if (f.healthDoctor.trim() || healthFailureThreshold) {
    p.health_policy = {
      doctor_agent: f.healthDoctor.trim(),
      failure_threshold: healthFailureThreshold,
    };
  }
  if (f.selfRepairEnabled === "true" || selfRepairAttempts || f.selfRepairEscalate.trim()) {
    p.self_repair = {
      enabled: f.selfRepairEnabled === "true",
      max_attempts: selfRepairAttempts,
      escalate_to: f.selfRepairEscalate.trim(),
    };
  }
  // A "@chain" model is self-contained — its fallback ladder lives in the chain,
  // so per-agent fallbacks are ignored (and the form hides the field).
  if (!isChainRef(p.model as string)) {
    const fb = f.fallbacks
      .split(",")
      .map((m) => m.trim())
      .filter(Boolean);
    if (fb.length > 0) p.fallbacks = fb;
  }
  return p;
}

function lines(s: string): string[] {
  return s
    .split(/\r?\n/)
    .map((x) => x.trim())
    .filter(Boolean);
}

function csvList(s: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of s
    .split(/[,\r\n]+/)
    .map((x) => x.trim())
    .filter(Boolean)) {
    const key = item.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(item);
  }
  return out;
}

function configMap(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const raw of s.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) continue;
    const eq = line.indexOf("=");
    if (eq < 0) {
      out[line] = "";
      continue;
    }
    out[line.slice(0, eq).trim()] = line.slice(eq + 1).trim();
  }
  return out;
}

function configText(m?: Record<string, string>): string {
  if (!m) return "";
  return Object.entries(m)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
}

function taskLines(s: string, scope: "cycle" | "total"): AgentTask[] {
  return lines(s).map((title) => ({ title, scope, status: "todo" }));
}

function tasksText(tasks: AgentTask[] | undefined, scope: "cycle" | "total"): string {
  return (tasks || [])
    .filter((t) => (t.scope || "total") === scope && t.status !== "retired")
    .map((t) => t.title)
    .join("\n");
}

function AgentOptionPicker({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string; detail?: string; icon?: ReactNode }[];
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1 text-[11px] text-muted">
      <span>{label}</span>
      <div className="grid gap-1.5 sm:grid-cols-2" role="group" aria-label={label}>
        {options.map((option) => {
          const selected = value === option.value;
          return (
            <button
              key={option.value}
              type="button"
              aria-pressed={selected}
              onClick={() => onChange(option.value)}
              className={cn(
                "flex min-h-10 items-start gap-2 rounded-lg border px-2.5 py-2 text-left text-xs transition",
                selected
                  ? "border-accent bg-accent/10 text-foreground"
                  : "border-border bg-panel/45 text-muted hover:border-accent/50 hover:text-foreground",
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

function AgentFormBlock({
  title,
  icon: Icon,
  summary,
  children,
}: {
  title: string;
  icon: LucideIcon;
  summary: string;
  children: ReactNode;
}) {
  return (
    <Disclosure
      className="rounded-lg border border-border bg-panel/40"
      summaryClassName="px-2.5 py-2 hover:bg-card/60"
      contentClassName="px-2.5 pb-2"
      summary={
        <span className="flex min-w-0 items-center gap-2">
          <Icon className="size-3.5 shrink-0 text-accent" />
          <span className="truncate text-xs font-semibold text-foreground">{title}</span>
          <span className="ml-auto truncate text-[11px] text-muted">{summary}</span>
        </span>
      }
    >
      {children}
    </Disclosure>
  );
}

// AgentFormFields renders the shared editable fields for New/Edit.
function AgentFormFields(props: {
  state: Record<string, string>;
  set: (key: string, value: string) => void;
}) {
  const { state, set } = props;
  const field = (label: string, key: string, placeholder: string, aria: string) => (
    <label className="flex flex-col gap-1 text-[11px] text-muted">
      {label}
      <input
        value={state[key] || ""}
        onChange={(e) => set(key, e.target.value)}
        placeholder={placeholder}
        aria-label={aria}
        className={inputCls}
      />
    </label>
  );
  const modelIsChain = isChainRef(state.model || "");
  const lifecycleMode = state.lifecycleMode || "persistent";
  const maxCycles = Number.parseInt(state.lifecycleMaxCycles || "0", 10);
  const effectiveLifecycleMode = lifecycleMode === "persistent" && Number.isFinite(maxCycles) && maxCycles > 0 ? "cycle" : lifecycleMode;
  const taskContractPreview = agentTaskContractSummary({
    lifecycle: {
      mode: effectiveLifecycleMode as AgentLifecycle["mode"],
      retire_on_complete: effectiveLifecycleMode === "retire_on_complete",
      max_cycles: Number.isFinite(maxCycles) && maxCycles > 0 ? maxCycles : 0,
    },
    tasklist: [
      ...taskLines(state.cycleTasks || "", "cycle"),
      ...taskLines(state.totalTasks || "", "total"),
    ],
  });
  const instructionCount = lines(state.instructions || "").length;
  const allowCount = csvList(state.toolAllow || "").length;
  const denyCount = csvList(state.toolDeny || "").length;
  const overrideCount = Object.keys(configMap(state.configOverrides || "")).length;
  const cycleTaskCount = lines(state.cycleTasks || "").length;
  const totalTaskCount = lines(state.totalTasks || "").length;
  return (
    <>
      <div className="grid gap-2 sm:grid-cols-2">
        {field("Name", "name", "e.g. The Researcher", "Agent name")}
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Model
          <div className="flex h-[30px] items-center">
            <ModelPicker value={state.model || ""} activeModel="daemon default" onChange={(id) => set("model", id)} />
          </div>
          {modelIsChain && <span className="text-xs text-accent">chain is self-contained — fallbacks come from @{state.model.slice(1)}</span>}
        </label>
        {modelIsChain
          ? null
          : field("Fallback models (comma-separated)", "fallbacks", "m1, m2", "Fallback models")}
        {field("Task type", "taskType", "e.g. research, code", "Task type")}
        {field("Max cost per run (USD)", "maxCost", "e.g. 0.50 — blank = no cap", "Max cost per run")}
        {field("Max cost per day (USD)", "maxDaily", "e.g. 5.00 — blank = no cap", "Max cost per day")}
        {field("Memory scope", "memoryScope", "blank = the slug", "Memory scope")}
        {field("Workdir (workspace-relative)", "workdir", "e.g. research", "Workdir")}
        {field("Owner agent", "ownerAgent", "supervisor slug", "Owner agent")}
        {field("Parent agent", "parentAgent", "leader slug for managed workers", "Parent agent")}
        <AgentOptionPicker
          label="Direct call policy"
          value={state.directCallable || "true"}
          onChange={(value) => set("directCallable", value)}
          options={[
            { value: "true", label: "Direct", detail: "Can be called from chat/API", icon: <Zap className="size-3.5" /> },
            { value: "false", label: "Managed only", detail: "Owner or parent wakes it", icon: <GitBranch className="size-3.5" /> },
          ]}
        />
        <AgentOptionPicker
          label="Agent lifecycle"
          value={state.lifecycleMode || "persistent"}
          onChange={(value) => set("lifecycleMode", value)}
          options={[
            { value: "persistent", label: "Persistent", detail: "Stays alive after runs", icon: <Activity className="size-3.5" /> },
            { value: "cycle", label: "Cycle", detail: "Repeats on wake", icon: <RefreshCw className="size-3.5" /> },
            { value: "retire_on_complete", label: "One-shot", detail: "Retires on completion", icon: <Archive className="size-3.5" /> },
          ]}
        />
        {field("Max cycles", "lifecycleMaxCycles", "0 = unlimited", "Max cycles")}
        <AgentOptionPicker
          label="Trust ceiling"
          value={state.trustCeiling || "L4"}
          onChange={(value) => set("trustCeiling", value)}
          options={[
            { value: "L4", label: "L4 allow", detail: "Autonomous inside policy", icon: <ShieldCheck className="size-3.5" /> },
            { value: "L3", label: "L3 scoped", detail: "Ask on scoped risk", icon: <ShieldCheck className="size-3.5" /> },
            { value: "L2", label: "L2 ask", detail: "Ask before risky work", icon: <AlertTriangle className="size-3.5" /> },
            { value: "L1", label: "L1 gated", detail: "Ask almost always", icon: <AlertTriangle className="size-3.5" /> },
            { value: "L0", label: "L0 deny", detail: "No autonomous action", icon: <Pause className="size-3.5" /> },
          ]}
        />
        {field("Description", "description", "what this agent is for", "Description")}
      </div>
      <Advanced label="Resilience & repair" className="mt-2">
      <div className="grid gap-2 sm:grid-cols-3">
        {field("Retry attempts", "retryAttempts", "0 = no run retry", "Retry attempts")}
        <AgentOptionPicker
          label="Retry backoff"
          value={state.retryBackoff || "exponential"}
          onChange={(value) => set("retryBackoff", value)}
          options={[
            { value: "exponential", label: "Exponential", detail: "Back off harder each retry", icon: <Activity className="size-3.5" /> },
            { value: "fixed", label: "Fixed", detail: "Same delay every retry", icon: <CalendarClock className="size-3.5" /> },
          ]}
        />
        {field("Retry base delay (sec)", "retryBaseDelay", "e.g. 30", "Retry base delay")}
        {field("Retry max delay (sec)", "retryMaxDelay", "e.g. 1800", "Retry max delay")}
        {field("Retry on", "retryOn", "error, timeout", "Retry on")}
        {field("Doctor agent", "healthDoctor", "guardian-doctor", "Doctor agent")}
        {field("Failure threshold", "healthFailureThreshold", "e.g. 5", "Failure threshold")}
        <AgentOptionPicker
          label="Self repair"
          value={state.selfRepairEnabled || "false"}
          onChange={(value) => set("selfRepairEnabled", value)}
          options={[
            { value: "false", label: "Off", detail: "Escalate manually", icon: <Pause className="size-3.5" /> },
            { value: "true", label: "Enabled", detail: "Doctor can repair", icon: <Wrench className="size-3.5" /> },
          ]}
        />
        {field("Self-repair attempts", "selfRepairAttempts", "e.g. 2", "Self-repair attempts")}
        {field("Escalate to", "selfRepairEscalate", "owner/agent slug", "Escalate to")}
      </div>
      </Advanced>
      <div className="mt-2 grid gap-2 lg:grid-cols-2">
        <AgentFormBlock
          title="Identity core"
          icon={IdCard}
          summary={(state.soul || "").trim() ? "soul set" : "no soul yet"}
        >
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Soul — who this agent is
            <textarea
              value={state.soul || ""}
              onChange={(e) => set("soul", e.target.value)}
              placeholder="You are Researcher. You dig deep and cite sources."
              aria-label="Agent soul"
              rows={3}
              className={cn(inputCls, "resize-y")}
            />
          </label>
        </AgentFormBlock>
        <AgentFormBlock
          title="Standing rules"
          icon={ListTree}
          summary={`${instructionCount} rule${instructionCount === 1 ? "" : "s"}`}
        >
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Standing instructions — durable operating rules
            <textarea
              value={state.instructions || ""}
              onChange={(e) => set("instructions", e.target.value)}
              placeholder="One instruction per line"
              aria-label="Agent instructions"
              rows={3}
              className={cn(inputCls, "resize-y")}
            />
          </label>
        </AgentFormBlock>
      </div>
      <Advanced label="Tools, tasks & overrides" className="mt-2">
      <div className="grid gap-2">
        <AgentFormBlock title="Tool policy" icon={ShieldCheck} summary={`${allowCount} allow · ${denyCount} deny`}>
          <div className="grid gap-2 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[11px] text-muted">
              Tool allowlist
              <textarea
                value={state.toolAllow || ""}
                onChange={(e) => set("toolAllow", e.target.value)}
                placeholder="shell, memory, mcp_fake_greet"
                aria-label="Tool allowlist"
                rows={2}
                className={cn(inputCls, "resize-y")}
              />
            </label>
            <label className="flex flex-col gap-1 text-[11px] text-muted">
              Tool denylist
              <textarea
                value={state.toolDeny || ""}
                onChange={(e) => set("toolDeny", e.target.value)}
                placeholder="notify, shell"
                aria-label="Tool denylist"
                rows={2}
                className={cn(inputCls, "resize-y")}
              />
            </label>
          </div>
        </AgentFormBlock>
        <AgentFormBlock title="Runtime overrides" icon={Cpu} summary={`${overrideCount} override${overrideCount === 1 ? "" : "s"}`}>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Agent config overrides
            <textarea
              value={state.configOverrides || ""}
              onChange={(e) => set("configOverrides", e.target.value)}
              placeholder={"AGEZT_X_MODE=agent-only\nAGEZT_X_BATCH=8"}
              aria-label="Agent config overrides"
              rows={3}
              className={cn(inputCls, "resize-y font-mono text-xs")}
            />
          </label>
        </AgentFormBlock>
        <AgentFormBlock title="Durable tasks" icon={CheckCheck} summary={`${cycleTaskCount} cycle · ${totalTaskCount} total`}>
          <div className="grid gap-2 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-[11px] text-muted">
              Every-cycle tasks
              <textarea
                value={state.cycleTasks || ""}
                onChange={(e) => set("cycleTasks", e.target.value)}
                placeholder="One task per line"
                aria-label="Every-cycle tasks"
                rows={3}
                className={cn(inputCls, "resize-y")}
              />
            </label>
            <label className="flex flex-col gap-1 text-[11px] text-muted">
              Total tasklist
              <textarea
                value={state.totalTasks || ""}
                onChange={(e) => set("totalTasks", e.target.value)}
                placeholder="One task per line"
                aria-label="Total tasklist"
                rows={3}
                className={cn(inputCls, "resize-y")}
              />
            </label>
          </div>
        </AgentFormBlock>
      </div>
      <div
        className="mt-2 rounded-md border border-border bg-panel/60 px-2.5 py-2 text-xs text-muted"
        aria-label="Task contract preview"
      >
        <div className="mb-0.5 text-xs font-semibold uppercase tracking-normal text-muted">
          Task contract
        </div>
        <div className="text-foreground/85">{taskContractPreview}</div>
      </div>
      </Advanced>
    </>
  );
}

// NewAgentForm creates a roster profile (M785). Exported for tests and reuse
// (the M714 "creatable from UI" recipe).
export function NewAgentForm({
  onCreated,
  onError,
}: {
  onCreated: (slug: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));
  const slug = (state.slug || "").trim();
  const valid = slugOk(slug);

  async function create() {
    if (!valid) return;
    const fields = profileFields({
      name: state.name || "",
      soul: state.soul || "",
      instructions: state.instructions || "",
      model: state.model || "",
      fallbacks: state.fallbacks || "",
      taskType: state.taskType || "",
      maxCost: state.maxCost || "",
      maxDaily: state.maxDaily || "",
      memoryScope: state.memoryScope || "",
      workdir: state.workdir || "",
      ownerAgent: state.ownerAgent || "",
      parentAgent: state.parentAgent || "",
      directCallable: state.directCallable || "true",
      retryAttempts: state.retryAttempts || "",
      retryBackoff: state.retryBackoff || "",
      retryBaseDelay: state.retryBaseDelay || "",
      retryMaxDelay: state.retryMaxDelay || "",
      retryOn: state.retryOn || "",
      healthDoctor: state.healthDoctor || "",
      healthFailureThreshold: state.healthFailureThreshold || "",
      selfRepairEnabled: state.selfRepairEnabled || "",
      selfRepairAttempts: state.selfRepairAttempts || "",
      selfRepairEscalate: state.selfRepairEscalate || "",
      trustCeiling: state.trustCeiling || "L4",
      toolAllow: state.toolAllow || "",
      toolDeny: state.toolDeny || "",
      configOverrides: state.configOverrides || "",
      lifecycleMode: state.lifecycleMode || "",
      lifecycleMaxCycles: state.lifecycleMaxCycles || "",
      cycleTasks: state.cycleTasks || "",
      totalTasks: state.totalTasks || "",
      description: state.description || "",
    });
    if (typeof fields === "string") {
      onError(fields);
      return;
    }
    setSubmitting(true);
    try {
      await postJSON("/api/agents/add", { profile: { slug, ...fields } });
      onCreated(slug);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="glass glow-accent rounded-xl p-3">
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Slug — the agent's permanent handle (lowercase; cannot be changed later)
        <input
          value={state.slug || ""}
          onChange={(e) => set("slug", e.target.value)}
          placeholder="e.g. researcher"
          aria-label="Agent slug"
          className={cn(inputCls, slug !== "" && !valid && "border-bad")}
        />
      </label>
      <div className="mt-2">
        <AgentFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={create} disabled={!valid || submitting}>
          <Plus className="h-3.5 w-3.5" /> Create agent
        </Button>
      </div>
    </div>
  );
}

// EditAgentForm edits a profile's mutable fields (the slug is the agent's
// address — immutable, shown but not editable).
export function EditAgentForm({
  profile,
  onSaved,
  onError,
}: {
  profile: AgentProfile;
  onSaved: (slug: string) => void;
  onError: (msg: string) => void;
}) {
  const [state, setState] = useState<Record<string, string>>({
    name: profile.name || "",
    soul: profile.soul || "",
    instructions: (profile.instructions || []).join("\n"),
    model: profile.model || "",
    fallbacks: (profile.fallbacks || []).join(", "),
    taskType: profile.task_type || "",
    maxCost: profile.max_cost_mc ? String(profile.max_cost_mc / 1e9) : "",
    maxDaily: profile.max_daily_mc ? String(profile.max_daily_mc / 1e9) : "",
    memoryScope: profile.memory_scope || "",
    workdir: profile.workdir || "",
    ownerAgent: profile.owner_agent || "",
    parentAgent: profile.parent_agent || "",
    directCallable: profile.direct_callable === false ? "false" : "true",
    retryAttempts: profile.retry_policy?.max_attempts ? String(profile.retry_policy.max_attempts) : "",
    retryBackoff: profile.retry_policy?.backoff || "exponential",
    retryBaseDelay: profile.retry_policy?.base_delay_sec ? String(profile.retry_policy.base_delay_sec) : "",
    retryMaxDelay: profile.retry_policy?.max_delay_sec ? String(profile.retry_policy.max_delay_sec) : "",
    retryOn: (profile.retry_policy?.retry_on || []).join(", "),
    healthDoctor: profile.health_policy?.doctor_agent || "",
    healthFailureThreshold: profile.health_policy?.failure_threshold ? String(profile.health_policy.failure_threshold) : "",
    selfRepairEnabled: profile.self_repair?.enabled ? "true" : "false",
    selfRepairAttempts: profile.self_repair?.max_attempts ? String(profile.self_repair.max_attempts) : "",
    selfRepairEscalate: profile.self_repair?.escalate_to || "",
    trustCeiling: profile.trust_ceiling || "L4",
    toolAllow: (profile.tool_allow || []).join(", "),
    toolDeny: (profile.tool_deny || []).join(", "),
    configOverrides: configText(profile.config_overrides),
    lifecycleMode: profile.lifecycle?.mode || (profile.lifecycle?.retire_on_complete ? "retire_on_complete" : "persistent"),
    lifecycleMaxCycles: profile.lifecycle?.max_cycles ? String(profile.lifecycle.max_cycles) : "",
    cycleTasks: tasksText(profile.tasklist, "cycle"),
    totalTasks: tasksText(profile.tasklist, "total"),
    description: profile.description || "",
  });
  const [submitting, setSubmitting] = useState(false);
  const set = (k: string, v: string) => setState((s) => ({ ...s, [k]: v }));

  async function save() {
    const fields = profileFields({
      name: state.name,
      soul: state.soul,
      instructions: state.instructions || "",
      model: state.model,
      fallbacks: state.fallbacks,
      taskType: state.taskType,
      maxCost: state.maxCost,
      maxDaily: state.maxDaily,
      memoryScope: state.memoryScope,
      workdir: state.workdir,
      ownerAgent: state.ownerAgent,
      parentAgent: state.parentAgent,
      directCallable: state.directCallable || "true",
      retryAttempts: state.retryAttempts || "",
      retryBackoff: state.retryBackoff || "",
      retryBaseDelay: state.retryBaseDelay || "",
      retryMaxDelay: state.retryMaxDelay || "",
      retryOn: state.retryOn || "",
      healthDoctor: state.healthDoctor || "",
      healthFailureThreshold: state.healthFailureThreshold || "",
      selfRepairEnabled: state.selfRepairEnabled || "",
      selfRepairAttempts: state.selfRepairAttempts || "",
      selfRepairEscalate: state.selfRepairEscalate || "",
      trustCeiling: state.trustCeiling || "L4",
      toolAllow: state.toolAllow || "",
      toolDeny: state.toolDeny || "",
      configOverrides: state.configOverrides || "",
      lifecycleMode: state.lifecycleMode || "",
      lifecycleMaxCycles: state.lifecycleMaxCycles || "",
      cycleTasks: state.cycleTasks || "",
      totalTasks: state.totalTasks || "",
      description: state.description,
    });
    if (typeof fields === "string") {
      onError(fields);
      return;
    }
    setSubmitting(true);
    try {
      await postJSON("/api/agents/edit", { ref: profile.slug, profile: fields });
      onSaved(profile.slug);
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="glass glow-accent rounded-xl p-3">
      <div className="text-[11px] text-muted">
        Editing <span className="font-mono text-foreground">{profile.slug}</span> (slug is permanent)
      </div>
      <div className="mt-2">
        <AgentFormFields state={state} set={set} />
      </div>
      <div className="mt-3 flex justify-end">
        <Button size="sm" onClick={save} disabled={submitting}>
          <Pencil className="h-3.5 w-3.5" /> Save
        </Button>
      </div>
    </div>
  );
}
