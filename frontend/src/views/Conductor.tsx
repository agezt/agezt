import { useEffect, useState } from "react";
import { Network, Send, Loader2, CheckCircle2, XCircle, ChevronRight, Play } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import { ModelPicker } from "@/components/ModelPicker";
import { useConductorStore, startConductorRun, applyConductorResult, genConductorCorr } from "@/lib/conductorStore";
import { type ConductorRun, type ConductorStep, progressLabel } from "@/lib/conductor";

// Conductor view (M997): the operator's seat at the asymmetric, verify-driven
// panel (kernel/runtime M997). A Thinker plans, a Worker solves, and a Verifier
// checks (running the worker's code when it can), looping until accepted. The
// loop streams live via conductor.* events (folded by lib/conductorStore, ABOVE
// the router so it survives navigation); the blocking POST then upgrades the
// transcript to its full form. The agent reaches the same engine via the
// `conductor` tool.

interface RolesPreview {
  thinker: string;
  worker: string;
  verifier: string;
  available_models?: string[];
}

// Per-role accent so the transcript reads at a glance.
const ROLE_META: Record<string, { label: string; tint: string }> = {
  thinker: { label: "Thinker", tint: "text-accent" },
  worker: { label: "Worker", tint: "text-accent2" },
  verifier: { label: "Verifier", tint: "text-foreground" },
};

export function Conductor() {
  const ui = useUI();
  const { runs, activeCorr } = useConductorStore();
  const run = activeCorr ? runs[activeCorr] : null;

  const [roles, setRoles] = useState<RolesPreview | null>(null);
  const [task, setTask] = useState("");
  const [thinker, setThinker] = useState("");
  const [worker, setWorker] = useState("");
  const [verifier, setVerifier] = useState("");
  const [maxRounds, setMaxRounds] = useState(2);
  const [plan, setPlan] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    getJSON<RolesPreview>("/api/conductor/roles")
      .then(setRoles)
      .catch(() => setRoles(null));
  }, []);

  async function go() {
    const t = task.trim();
    if (!t) return;
    setRunning(true);
    setError("");
    const corr = genConductorCorr();
    // Seed the run so the live view appears immediately, then let conductor.*
    // events stream into it while the (blocking) ask runs.
    startConductorRun(corr, t, {
      thinker: thinker.trim() || roles?.thinker || "",
      worker: worker.trim() || roles?.worker || "",
      verifier: verifier.trim() || roles?.verifier || "",
    }, maxRounds);
    try {
      const res = await postJSON<{
        answer?: string;
        passed?: boolean;
        plan?: string;
        rounds?: number;
        roles?: Record<string, string>;
        steps?: ConductorStep[];
        correlation_id?: string;
      }>("/api/conductor/ask", {
        task: t,
        thinker: thinker.trim(),
        worker: worker.trim(),
        verifier: verifier.trim(),
        max_rounds: maxRounds,
        plan,
        corr,
      });
      applyConductorResult(corr, res);
      ui.toast(res.passed ? "Conductor verified the answer" : "Conductor finished — not verified", res.passed ? "success" : "info");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  const noModels = roles && !roles.thinker && !roles.worker && !roles.verifier;

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        icon={Network}
        title="Conductor"
        description="Thinker → Worker → Verifier on different models — solve a hard task and verify it (running the code when it can)."
      />

      {noModels && (
        <Card glass className="p-3 text-sm text-muted">
          No keyed providers configured yet — the Conductor needs at least one model. Add one in Providers/Models, or
          set roles explicitly below.
        </Card>
      )}

      {/* Task + controls */}
      <Card glass className="gap-3 p-4">
        <textarea
          value={task}
          onChange={(e) => setTask(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) go();
          }}
          rows={3}
          placeholder="Describe a hard, verifiable task — e.g. “Write and test a Python function that returns the nth prime.”"
          className="w-full resize-y rounded-lg border border-border bg-panel p-3 text-sm outline-none focus:border-accent/50"
        />

        <div className="flex flex-wrap items-center gap-3 text-sm">
          <label className="flex items-center gap-1.5 text-muted">
            <span>Max rounds</span>
            <select
              value={maxRounds}
              onChange={(e) => setMaxRounds(Number(e.target.value))}
              className="rounded-md border border-border bg-panel px-2 py-1 text-foreground outline-none focus:border-accent/50"
            >
              {[1, 2, 3, 4, 5, 6].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-1.5 text-muted">
            <input type="checkbox" checked={plan} onChange={(e) => setPlan(e.target.checked)} />
            <span>Tailor role instructions first</span>
          </label>
          <button
            type="button"
            onClick={() => setShowAdvanced((v) => !v)}
            className="flex items-center gap-1 text-muted hover:text-foreground"
          >
            <ChevronRight className={cn("size-3.5 transition-transform", showAdvanced && "rotate-90")} />
            Role models
          </button>
          <div className="ml-auto">
            <Button onClick={go} disabled={running || !task.trim()}>
              {running ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
              {running ? "Conducting…" : "Conduct"}
            </Button>
          </div>
        </div>

        {showAdvanced && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <RolePicker label="Thinker" value={thinker} onChange={setThinker} auto={roles?.thinker} />
            <RolePicker label="Worker" value={worker} onChange={setWorker} auto={roles?.worker} />
            <RolePicker label="Verifier" value={verifier} onChange={setVerifier} auto={roles?.verifier} />
          </div>
        )}
      </Card>

      {error && <ErrorText>{error}</ErrorText>}

      {run && <RunView run={run} />}
    </div>
  );
}

// RolePicker selects a role's model from the shared ModelPicker (keyed models +
// named fallback chains). value "" means "auto" — the daemon fills the role from
// the keyed providers; the picker shows that auto choice as the default hint.
function RolePicker({
  label,
  value,
  onChange,
  auto,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  auto?: string;
}) {
  return (
    <div className="flex flex-col gap-1 text-xs text-muted">
      <span>{label}</span>
      <ModelPicker value={value} onChange={onChange} activeModel={auto || undefined} />
    </div>
  );
}

function RunView({ run }: { run: ConductorRun }) {
  return (
    <div className="flex flex-col gap-3">
      {/* Verdict / live header */}
      <Card glass className="flex-row flex-wrap items-center gap-3 p-3">
        {!run.done ? (
          <span className="flex items-center gap-1.5 font-semibold text-accent">
            <Loader2 className="size-5 animate-spin" /> {progressLabel(run)}
          </span>
        ) : run.passed ? (
          <span className="flex items-center gap-1.5 font-semibold text-emerald-500">
            <CheckCircle2 className="size-5" /> Verified
          </span>
        ) : (
          <span className="flex items-center gap-1.5 font-semibold text-amber-500">
            <XCircle className="size-5" /> Not verified
          </span>
        )}
        <span className="text-xs text-muted">
          {run.maxRounds} round{run.maxRounds === 1 ? "" : "s"}
        </span>
        <div className="ml-auto flex flex-wrap items-center gap-1.5 text-xs">
          {(["thinker", "worker", "verifier"] as const).map((r) => (
            <Badge key={r} className={cn("font-mono", ROLE_META[r].tint)}>
              {ROLE_META[r].label}: {run.roles?.[r] || "auto"}
            </Badge>
          ))}
        </div>
      </Card>

      {/* Final answer (once available) */}
      {run.answer && (
        <Card glass className="gap-2 p-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted">Answer</div>
          <Markdown source={run.answer} />
        </Card>
      )}

      {run.plan && (
        <Card glass className="gap-2 p-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted">Coordination plan</div>
          <Markdown source={run.plan} />
        </Card>
      )}

      {/* Transcript (streams live) */}
      {run.steps.length > 0 && (
        <div className="flex flex-col gap-2">
          <div className="px-1 text-xs font-semibold uppercase tracking-wide text-muted">Transcript</div>
          {run.steps.map((s, i) => (
            <StepCard key={`${s.role}#${s.round}#${i}`} step={s} />
          ))}
        </div>
      )}
    </div>
  );
}

function StepCard({ step }: { step: ConductorStep }) {
  const meta = ROLE_META[step.role] || { label: step.role, tint: "text-foreground" };
  const isVerifier = step.role === "verifier";
  return (
    <Card glass className="gap-2 p-3">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Play className={cn("size-3.5", meta.tint)} />
        <span className={cn("font-semibold", meta.tint)}>{meta.label}</span>
        <span className="font-mono text-muted">{step.model}</span>
        <span className="text-muted">· round {step.round}</span>
        {isVerifier && step.verdict && (
          <Badge className={cn("ml-1", step.verdict === "pass" ? "text-emerald-500" : "text-amber-500")}>
            {step.verdict.toUpperCase()}
          </Badge>
        )}
      </div>

      {step.error && <ErrorText>{step.error}</ErrorText>}

      {isVerifier ? (
        <>
          {step.reason && <p className="text-sm text-muted">{step.reason}</p>}
          {step.exec?.ran && (
            <div className="rounded-md border border-border bg-panel/60 p-2">
              <div className="mb-1 text-[11px] text-muted">
                ran {step.exec.language || "code"} — {step.exec.ok ? "clean exit" : "reported failure"}
              </div>
              {step.exec.output && (
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-xs">
                  {step.exec.output}
                </pre>
              )}
            </div>
          )}
        </>
      ) : (
        step.text && <Markdown source={step.text} />
      )}
    </Card>
  );
}
