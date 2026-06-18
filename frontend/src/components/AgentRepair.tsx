import { useRef, useState } from "react";
import { Wrench, Play, Square, RotateCcw, Undo2, AlertTriangle, CheckCircle2, Repeat } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";
import { getJSON, postJSON } from "@/lib/api";
import { cn, clip, fmtTime } from "@/lib/utils";
import { newTurn, foldChatFrame, turnText, streamRun, type ChatTurn } from "@/lib/chat";
import {
  buildRepairBrief,
  parseRepairProposal,
  applyProposal,
  proposalSummary,
  type RepairProposal,
  type RepairContext,
} from "@/lib/agentrepair";
import type { AgentProfile } from "@/views/Roster";
import type { RunLite } from "@/lib/agentdetail";

interface DenialRow {
  capability?: string;
  tool?: string;
  reason?: string;
  hard_denied?: boolean;
}
interface ToolErrRow {
  tool?: string;
  output?: string;
}
interface RoutingSnapshot {
  chains?: Record<string, string[]>;
}

export function repairReadinessPassport(
  profile: Pick<AgentProfile, "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent" | "retry_policy" | "health_policy" | "self_repair">,
  evidence: number,
): { value: string; detail: string; tone: "good" | "warn" | "bad" | "muted" } {
  const manager = profile.parent_agent || profile.owner_agent || "";
  if (profile.retired) {
    return {
      value: "repair blocked",
      detail: "revive this agent before requesting repair",
      tone: "bad",
    };
  }
  if (profile.kind === "subagent" || profile.managed || profile.direct_callable === false) {
    return {
      value: "manager repair",
      detail: manager ? `request repair through ${manager}` : "request repair through parent/owner",
      tone: "warn",
    };
  }
  const retry = profile.retry_policy?.max_attempts && profile.retry_policy.max_attempts > 1
    ? `retry ${profile.retry_policy.max_attempts}x`
    : "single attempt";
  const doctor = profile.health_policy?.doctor_agent
    ? `doctor ${profile.health_policy.doctor_agent}`
    : "no doctor";
  const selfRepair = profile.self_repair?.enabled
    ? `self-repair${profile.self_repair.max_attempts ? ` ${profile.self_repair.max_attempts}x` : ""}`
    : "self-repair off";
  const escalation = profile.self_repair?.escalate_to ? `escalate ${profile.self_repair.escalate_to}` : "";
  const value = evidence > 0 ? `ready · ${evidence} evidence` : "ready · proactive";
  const tone = profile.health_policy?.doctor_agent || profile.self_repair?.enabled || (profile.retry_policy?.max_attempts || 0) > 1
    ? "good"
    : "warn";
  return {
    value,
    detail: [retry, doctor, selfRepair, escalation].filter(Boolean).join(" · "),
    tone,
  };
}

// AgentRepair (M960) is the Self-Repair / Iterate tab: it runs the agent AS
// ITSELF on a brief built from its own recent failures, lets it fix its own
// skills/scripts/workdir files with its tools, and — per the owner's "fully
// autonomous" choice — auto-applies any identity change (soul / model /
// fallbacks) it proposes, with a one-click Undo as the reversal valve. "Iterate
// ×N" runs N rounds back-to-back, each shown the prior round's outcome.
export function AgentRepair({
  slug,
  profile,
  fail,
  denials,
  toolErrors,
  runs,
  configIssues = [],
  taskModelChain,
  onApplied,
}: {
  slug: string;
  profile: AgentProfile;
  fail?: RunLite;
  denials: DenialRow[] | null;
  toolErrors: ToolErrRow[] | null;
  runs: number;
  configIssues?: string[];
  taskModelChain?: string[];
  onApplied: () => void;
}) {
  const ui = useUI();
  const [turn, setTurn] = useState<ChatTurn | null>(null);
  const [running, setRunning] = useState(false);
  const [round, setRound] = useState(0); // current round index during an iterate sweep
  const [iterN, setIterN] = useState(2);
  const [err, setErr] = useState<string | null>(null);
  const [requestingRepair, setRequestingRepair] = useState(false);
  // The last applied identity change + the profile snapshot to restore on Undo.
  const [applied, setApplied] = useState<{
    prop: RepairProposal;
    prev: AgentProfile;
    prevRouting?: { taskType: string; chain?: string[] };
  } | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  function buildBrief(priorRounds: string[]): string {
    const ctx: RepairContext = {
      profile: {
        slug,
        name: profile.name,
        soul: profile.soul,
        model: profile.model,
        fallbacks: profile.fallbacks,
        task_type: profile.task_type,
        task_model_chain: taskModelChain,
        workdir: profile.workdir,
        memory_scope: profile.memory_scope,
        owner_agent: profile.owner_agent,
        parent_agent: profile.parent_agent,
        direct_callable: profile.direct_callable,
        retry_policy: profile.retry_policy,
        health_policy: profile.health_policy,
        self_repair: profile.self_repair,
        noise_policy: profile.noise_policy,
      },
      fail: fail ? { correlation_id: fail.correlation_id, status: fail.status, started_unix_ms: fail.started_unix_ms } : undefined,
      denials: (denials || []).map((d) => ({ capability: d.capability, tool: d.tool, reason: d.reason, hard_denied: d.hard_denied })),
      toolErrors: (toolErrors || []).map((t) => ({ tool: t.tool, output: t.output })),
      configIssues,
      runs,
      priorRounds,
    };
    return buildRepairBrief(ctx);
  }

  // runOnce streams a single repair round and returns its final answer text (for
  // the next round to build on). Auto-applies any identity proposal it carries.
  async function runOnce(priorRounds: string[]): Promise<string> {
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    let acc = newTurn();
    setTurn(acc);
    await streamRun(
      { intent: buildBrief(priorRounds), agent: slug },
      (f) => {
        acc = foldChatFrame(acc, f);
        setTurn({ ...acc, tools: acc.tools.map((c) => ({ ...c })) });
      },
      ctrl.signal,
    );
    const finalText = turnText(acc);
    const prop = parseRepairProposal(finalText);
    if (prop) await autoApply(prop);
    return finalText;
  }

  // autoApply writes the proposed identity change to the agent's own profile.
  // agents/edit replaces mutable fields wholesale, so we send the FULL current
  // profile with the proposal merged in (applyProposal). The pre-change profile
  // is captured for Undo.
  async function autoApply(prop: RepairProposal) {
    try {
      let prevRouting: { taskType: string; chain?: string[] } | undefined;
      const next = applyProposal(profile, prop);
      await postJSON("/api/agents/edit", { ref: slug, profile: stripForEdit(next) });
      if (prop.task_model_chain !== undefined) {
        const taskType = (prop.task_type || next.task_type || profile.task_type || "").trim();
        if (!taskType) throw new Error("task_model_chain proposed without a task_type");
        const routing = await getJSON<RoutingSnapshot>("/api/routing");
        prevRouting = { taskType, chain: routing.chains?.[taskType] ? [...(routing.chains?.[taskType] || [])] : undefined };
        const chains = { ...(routing.chains || {}) };
        chains[taskType] = prop.task_model_chain;
        await postJSON("/api/routing/set", { chains });
      }
      setApplied({ prop, prev: profile, prevRouting });
      ui.toast(`Applied to ${slug}: ${proposalSummary(prop)}`, "success");
      onApplied();
    } catch (e) {
      setErr(`auto-apply failed: ${(e as Error).message}`);
    }
  }

  async function undo() {
    if (!applied) return;
    try {
      await postJSON("/api/agents/edit", { ref: slug, profile: stripForEdit(applied.prev) });
      if (applied.prevRouting) {
        const routing = await getJSON<RoutingSnapshot>("/api/routing");
        const chains = { ...(routing.chains || {}) };
        if (applied.prevRouting.chain && applied.prevRouting.chain.length > 0) chains[applied.prevRouting.taskType] = applied.prevRouting.chain;
        else delete chains[applied.prevRouting.taskType];
        await postJSON("/api/routing/set", { chains });
      }
      ui.toast(`Reverted ${slug} to the previous identity`, "success");
      setApplied(null);
      onApplied();
    } catch (e) {
      ui.toast(`undo failed: ${(e as Error).message}`, "error");
    }
  }

  async function selfRepair(rounds: number) {
    setErr(null);
    setApplied(null);
    setRunning(true);
    const prior: string[] = [];
    try {
      for (let i = 0; i < rounds; i++) {
        setRound(i + 1);
        const out = await runOnce(prior);
        prior.push(clip(out, 600));
        if (abortRef.current?.signal.aborted) break;
      }
    } catch (e) {
      if (!abortRef.current?.signal.aborted) setErr((e as Error).message);
    } finally {
      setRunning(false);
      setRound(0);
      abortRef.current = null;
    }
  }

  function stop() {
    abortRef.current?.abort();
    setRunning(false);
  }

  const governedRepairBlocked = profile.retired
    ? "revive this agent before requesting repair"
    : profile.kind === "subagent" || profile.managed || profile.direct_callable === false
      ? "managed sub-agent; request repair through its parent/owner"
      : "";

  async function requestGovernedRepair() {
    if (governedRepairBlocked) return;
    setRequestingRepair(true);
    setErr(null);
    try {
      const res = await postJSON<{ correlation_id?: string }>("/api/agents/repair", {
        ref: slug,
        reason: `operator requested governed repair from ${slug} repair tab`,
      });
      ui.toast(res.correlation_id ? `Repair accepted (${res.correlation_id})` : "Repair accepted", "success");
      onApplied();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setRequestingRepair(false);
    }
  }

  const evidence = (fail ? 1 : 0) + (denials?.length || 0) + (toolErrors?.length || 0) + (configIssues?.length || 0);
  const readiness = repairReadinessPassport(profile, evidence);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="min-w-0">
          <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
            <AlertTriangle className="size-3" /> governed doctor run
          </div>
          <div className="mt-0.5 text-[11px] text-muted">
            Queue the runtime repair path with this agent's retry, health, owner, and policy guardrails.
          </div>
          <div
            title={readiness.detail}
            className={cn(
              "mt-2 inline-flex max-w-full items-center gap-1.5 rounded-md border border-border bg-card px-2 py-1 text-[11px]",
              readiness.tone === "good" && "border-good/30 bg-good/10 text-good",
              readiness.tone === "warn" && "border-warn/35 bg-warn/10 text-warn",
              readiness.tone === "bad" && "border-bad/35 bg-bad/10 text-bad",
            )}
          >
            <span className="font-medium">{readiness.value}</span>
            <span className="truncate text-muted">{readiness.detail}</span>
          </div>
        </div>
        <Button
          size="sm"
          variant="ghost"
          disabled={requestingRepair || !!governedRepairBlocked}
          onClick={requestGovernedRepair}
          title={governedRepairBlocked || "Request a governed doctor/repair run for this agent"}
        >
          <Wrench className="size-3.5" /> Repair now
        </Button>
      </div>

      {/* What's wrong — the case for repair. */}
      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-1.5 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
          <Wrench className="size-3" /> self-repair evidence
        </div>
        {evidence === 0 ? (
          <p className="text-[11px] text-muted">
            No failures, denials, or tool errors recorded — a repair run will hunt for latent weaknesses (unclear soul, missing
            skills, brittle scripts) instead.
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5 text-[11px]">
            {fail && <Badge variant="bad">last run failed</Badge>}
            {(denials?.length || 0) > 0 && <Badge variant="default">{denials!.length} denial(s)</Badge>}
            {(toolErrors?.length || 0) > 0 && <Badge variant="default">{toolErrors!.length} tool error(s)</Badge>}
            {(configIssues?.length || 0) > 0 && <Badge variant="bad">{configIssues!.length} config issue(s)</Badge>}
          </div>
        )}
      </div>

      {/* Controls. */}
      <div className="flex flex-wrap items-center gap-2">
        {!running ? (
          <>
            <Button size="sm" onClick={() => selfRepair(1)}>
              <Play className="size-3.5" /> Self-repair
            </Button>
            <div className="flex items-center gap-1 rounded-md border border-border bg-panel/40 px-1.5 py-0.5">
              <Repeat className="size-3 text-muted" />
              <span className="text-[10px] text-muted">Iterate</span>
              <input
                type="number"
                min={2}
                max={9}
                value={iterN}
                onChange={(e) => setIterN(Math.max(2, Math.min(9, Number(e.target.value) || 2)))}
                className="w-10 rounded border border-border bg-card px-1 py-0.5 text-center text-[11px] outline-none focus:border-accent"
              />
              <Button size="sm" variant="ghost" onClick={() => selfRepair(iterN)} title={`Run ${iterN} repair rounds back-to-back`}>
                ×{iterN}
              </Button>
            </div>
          </>
        ) : (
          <Button size="sm" variant="ghost" onClick={stop} title="Abort the repair run">
            <Square className="size-3.5" /> Stop{round ? ` (round ${round})` : ""}
          </Button>
        )}
        {turn && !running && (
          <Button size="sm" variant="ghost" onClick={() => selfRepair(1)} title="Run another round">
            <RotateCcw className="size-3.5" /> Again
          </Button>
        )}
      </div>

      <p className="text-[10px] text-muted">
        Runs as <span className="font-mono text-foreground/80">{slug}</span> through the governed loop (policy/HITL apply). It
        edits its own files; durable changes it proposes (soul/model/fallbacks/task routing/config overrides) are applied automatically
        — Undo reverts the profile fields.
      </p>

      {err && <ErrorText>{err}</ErrorText>}

      {/* Applied-change banner + Undo. */}
      {applied && (
        <div className="flex items-start gap-2 rounded-lg border border-good/40 bg-good/5 p-2.5">
          <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-good" />
          <div className="min-w-0 flex-1">
            <div className="text-[11px] font-medium text-good">Applied to its identity</div>
            <div className="text-[11px] text-muted">{proposalSummary(applied.prop)}</div>
            {applied.prop.soul !== undefined && (
              <pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap rounded bg-panel p-1.5 font-mono text-[10px] text-foreground/80">
                {clip(applied.prop.soul, 600)}
              </pre>
            )}
          </div>
          <Button size="sm" variant="ghost" onClick={undo} title="Revert to the previous identity">
            <Undo2 className="size-3.5" /> Undo
          </Button>
        </div>
      )}

      {/* Live run transcript. */}
      {turn && <RepairTranscript turn={turn} running={running} />}

      {!turn && !running && evidence === 0 && (
        <EmptyState icon={Wrench} title="Nothing broken right now" hint="You can still run a repair pass to harden this agent proactively." />
      )}
    </div>
  );
}

// stripForEdit drops fields agents/edit rejects (id/slug/enabled/retired are
// protected; created/updated are kernel-owned), leaving the mutable profile the
// handler applies wholesale.
export function stripForEdit(p: AgentProfile): Record<string, unknown> {
  return {
    name: p.name,
    soul: p.soul,
    model: p.model,
    fallbacks: p.fallbacks,
    task_type: p.task_type,
    max_cost_mc: p.max_cost_mc,
    max_daily_mc: p.max_daily_mc,
    memory_scope: p.memory_scope,
    workdir: p.workdir,
    owner_agent: p.owner_agent,
    parent_agent: p.parent_agent,
    direct_callable: p.direct_callable,
    retry_policy: p.retry_policy,
    health_policy: p.health_policy,
    self_repair: p.self_repair,
    noise_policy: p.noise_policy,
    description: p.description,
    instructions: p.instructions,
    tool_allow: p.tool_allow,
    tool_deny: p.tool_deny,
    trust_ceiling: p.trust_ceiling,
    config_overrides: p.config_overrides,
    lifecycle: p.lifecycle,
    tasklist: p.tasklist,
  };
}

function RepairTranscript({ turn, running }: { turn: ChatTurn; running: boolean }) {
  const text = turnText(turn);
  return (
    <div className="space-y-2 rounded-lg border border-border bg-panel/20 p-2.5">
      <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-wider text-muted">
        {running && <span className="size-1.5 animate-pulse rounded-full bg-accent" />}
        repair run {turn.status === "error" ? "— error" : running ? "— working…" : "— done"}
      </div>

      {/* Tool calls the agent made (what it actually changed). */}
      {turn.tools.length > 0 && (
        <ul className="space-y-1">
          {turn.tools.map((c) => (
            <li key={c.callId} className="flex items-start gap-2 text-[11px]">
              <span
                className={cn(
                  "rounded px-1.5 py-0.5 font-mono text-[10px]",
                  c.error ? "bg-bad/15 text-bad" : c.allow === false ? "bg-bad/10 text-bad" : "bg-card text-foreground/80",
                )}
              >
                {c.tool}
              </span>
              <span className="min-w-0 flex-1 truncate text-muted" title={c.output || c.input}>
                {clip(c.output || c.input || "", 120)}
              </span>
              {c.hardDenied && <AlertTriangle className="size-3 shrink-0 text-bad" />}
            </li>
          ))}
        </ul>
      )}

      {text && (
        <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded bg-card p-2 font-mono text-[11px] text-foreground/85">{text}</pre>
      )}

      {turn.status === "done" && (
        <div className="text-[10px] text-muted">
          {turn.iters ? `${turn.iters} iteration(s)` : ""}
          {turn.model ? ` · ${turn.model}` : ""}
          {turn.ts ? ` · ${fmtTime(turn.ts)}` : ""}
        </div>
      )}
    </div>
  );
}
