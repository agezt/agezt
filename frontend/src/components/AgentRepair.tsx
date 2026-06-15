import { useRef, useState } from "react";
import { Wrench, Play, Square, RotateCcw, Undo2, AlertTriangle, CheckCircle2, Repeat } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";
import { postJSON } from "@/lib/api";
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
  onApplied,
}: {
  slug: string;
  profile: AgentProfile;
  fail?: RunLite;
  denials: DenialRow[] | null;
  toolErrors: ToolErrRow[] | null;
  runs: number;
  onApplied: () => void;
}) {
  const ui = useUI();
  const [turn, setTurn] = useState<ChatTurn | null>(null);
  const [running, setRunning] = useState(false);
  const [round, setRound] = useState(0); // current round index during an iterate sweep
  const [iterN, setIterN] = useState(2);
  const [err, setErr] = useState<string | null>(null);
  // The last applied identity change + the profile snapshot to restore on Undo.
  const [applied, setApplied] = useState<{ prop: RepairProposal; prev: AgentProfile } | null>(null);
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
        workdir: profile.workdir,
        memory_scope: profile.memory_scope,
      },
      fail: fail ? { correlation_id: fail.correlation_id, status: fail.status, started_unix_ms: fail.started_unix_ms } : undefined,
      denials: (denials || []).map((d) => ({ capability: d.capability, tool: d.tool, reason: d.reason, hard_denied: d.hard_denied })),
      toolErrors: (toolErrors || []).map((t) => ({ tool: t.tool, output: t.output })),
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
      const next = applyProposal(profile, prop);
      await postJSON("/api/agents/edit", { ref: slug, profile: stripForEdit(next) });
      setApplied({ prop, prev: profile });
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

  const evidence = (fail ? 1 : 0) + (denials?.length || 0) + (toolErrors?.length || 0);

  return (
    <div className="space-y-3">
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
        edits its own files; identity changes (soul/model/fallbacks) it proposes are applied automatically — Undo reverts them.
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
function stripForEdit(p: AgentProfile): Record<string, unknown> {
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
    description: p.description,
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
