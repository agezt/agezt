import { useEffect, useState, type ReactNode } from "react";
import { Network, Send, Loader2, CheckCircle2, XCircle, Play, X } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { useUI } from "@/components/ui/feedback";
import { Page } from "@/components/ui/page";
import { Disclosure } from "@/components/ui/disclosure";
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
  const [taskOpen, setTaskOpen] = useState(false);
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
      setTaskOpen(false);
      ui.toast(res.passed ? "Conductor verified the answer" : "Conductor finished — not verified", res.passed ? "success" : "info");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  const noModels = roles && !roles.thinker && !roles.worker && !roles.verifier;

  return (
    <Page
      icon={Network}
      title="Conductor"
      description="Thinker → Worker → Verifier on different models — solve a hard task and verify it (running the code when it can)."
      className="gap-4"
      actions={
        <Button size="sm" onClick={() => setTaskOpen(true)}>
          <Send className="size-3.5" /> New task
        </Button>
      }
    >

      {noModels && (
        <Card glass className="p-3 text-sm text-muted">
          No keyed providers configured yet — the Conductor needs at least one model. Add one in Providers/Models, or
          set roles explicitly below.
        </Card>
      )}

      {taskOpen && (
      <ConductorModal title="New conductor task" onClose={() => setTaskOpen(false)}>
        <textarea
          value={task}
          onChange={(e) => setTask(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) go();
          }}
          rows={3}
          aria-label="Conductor task"
          placeholder="Describe a hard, verifiable task — e.g. “Write and test a Python function that returns the nth prime.”"
          className="w-full resize-y rounded-lg border border-border bg-panel p-3 text-sm outline-none focus:border-accent/50"
        />

        <div className="flex flex-wrap items-center gap-3 text-sm">
          <RoundPicker value={maxRounds} onChange={setMaxRounds} />
          <label className="flex items-center gap-1.5 text-muted">
            <input type="checkbox" checked={plan} onChange={(e) => setPlan(e.target.checked)} />
            <span>Tailor role instructions first</span>
          </label>
          <div className="ml-auto">
            <Button onClick={go} disabled={running || !task.trim()}>
              {running ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
              {running ? "Conducting…" : "Conduct"}
            </Button>
          </div>
        </div>

        <Disclosure summary={<span className="text-xs font-semibold uppercase tracking-normal text-muted">Role models</span>}>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <RolePicker label="Thinker" value={thinker} onChange={setThinker} auto={roles?.thinker} tint={ROLE_META.thinker.tint} />
            <RolePicker label="Worker" value={worker} onChange={setWorker} auto={roles?.worker} tint={ROLE_META.worker.tint} />
            <RolePicker label="Verifier" value={verifier} onChange={setVerifier} auto={roles?.verifier} tint={ROLE_META.verifier.tint} />
          </div>
        </Disclosure>
      </ConductorModal>
      )}

      {error && <ErrorText>{error}</ErrorText>}

      {run && <RunView run={run} />}
    </Page>
  );
}

function RoundPicker({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-muted" role="group" aria-label="Max rounds">
      <span className="mr-0.5 text-xs">Max rounds</span>
      {[1, 2, 3, 4, 5, 6].map((n) => (
        <button
          key={n}
          type="button"
          aria-pressed={value === n}
          onClick={() => onChange(n)}
          className={cn(
            "grid size-7 place-items-center rounded-md border text-xs font-semibold transition-colors",
            value === n
              ? "border-accent bg-accent/15 text-accent"
              : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
          )}
        >
          {n}
        </button>
      ))}
    </div>
  );
}

function ConductorModal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Network className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Give the panel a hard task; role overrides stay tucked away until needed.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close conductor modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="flex min-h-0 flex-col gap-3 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}

// RolePicker selects a role's model from the shared ModelPicker (keyed models +
// named fallback chains) via its modal. value "" means "auto" — the daemon fills
// the role from the keyed providers; the card shows what that auto choice is.
function RolePicker({
  label,
  value,
  onChange,
  auto,
  tint,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  auto?: string;
  tint: string;
}) {
  return (
    <div className="flex flex-col gap-1.5 rounded-lg border border-border bg-panel/40 p-3">
      <span className={cn("text-xs font-semibold", tint)}>{label}</span>
      <ModelPicker
        value={value}
        onChange={onChange}
        activeModel={auto || undefined}
        triggerClassName="h-9 w-full max-w-none rounded-lg px-3 text-sm"
      />
      <span className="text-xs text-muted">{value ? "overridden" : `Auto: ${auto || "a keyed provider"}`}</span>
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
          <div className="text-xs font-semibold uppercase tracking-normal text-muted">Answer</div>
          <Markdown source={run.answer} />
        </Card>
      )}

      {run.plan && (
        <Card glass className="gap-2 p-4">
          <div className="text-xs font-semibold uppercase tracking-normal text-muted">Coordination plan</div>
          <Markdown source={run.plan} />
        </Card>
      )}

      {/* Transcript (streams live) */}
      {run.steps.length > 0 && (
        <div className="flex flex-col gap-2">
          <div className="px-1 text-xs font-semibold uppercase tracking-normal text-muted">Transcript</div>
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
