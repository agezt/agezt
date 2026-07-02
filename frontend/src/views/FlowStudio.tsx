import { useEffect, useMemo, useState } from "react";
import { Play, Sparkles, Wand2, RefreshCw, Workflow, X, FileJson2 } from "lucide-react";
import { postJSON, getJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
import { Page } from "@/components/ui/page";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/input";
import { Badge, statusVariant } from "@/components/ui/badge";
import { Muted, ErrorText } from "@/components/JsonView";
import { PlanDag, type Plan } from "@/components/PlanDag";
import { prettyJSON } from "@/lib/utils";

interface PlanRow {
  correlation_id?: string;
  plan_name?: string;
  status?: string;
  node_count?: number;
  duration_ms?: number;
}

// Flow Studio: author a plan from an intent, edit the JSON, see it as a live DAG
// (React Flow), refine it, then run it — watching each node recolour as
// node.*/plan.* events arrive on the SSE feed.
export function FlowStudio() {
  const { subscribe } = useEvents();
  const [intent, setIntent] = useState("");
  const [model, setModel] = useState("");
  const [planText, setPlanText] = useState("");
  const [feedback, setFeedback] = useState("");
  const [msg, setMsg] = useState("");
  const [msgErr, setMsgErr] = useState(false);
  const [busy, setBusy] = useState(false);
  const [running, setRunning] = useState(false);
  const [nodeStatus, setNodeStatus] = useState<Record<string, string>>({});
  const [history, setHistory] = useState<PlanRow[]>([]);
  const [composeOpen, setComposeOpen] = useState(false);
  const [refineOpen, setRefineOpen] = useState(false);

  const plan = useMemo<Plan>(() => {
    try {
      return JSON.parse(planText) as Plan;
    } catch {
      return {};
    }
  }, [planText]);

  function say(text: string, err = false) {
    setMsg(text);
    setMsgErr(err);
  }

  async function loadHistory() {
    try {
      const d = await getJSON<{ plans?: PlanRow[] }>("/api/plan_history", { limit: "8" });
      setHistory(d.plans || []);
    } catch {
      /* non-fatal */
    }
  }

  useEffect(() => {
    loadHistory();
  }, []);

  // Live node recolour from the journal stream.
  useEffect(() => {
    return subscribe((e: AgentEvent) => {
      const k = e.kind || "";
      if (k === "plan.started") {
        setNodeStatus({});
        say("running…");
        return;
      }
      if (k === "plan.completed") {
        say("completed ✓");
        loadHistory();
        return;
      }
      if (k === "plan.failed") {
        say("plan failed ✗", true);
        loadHistory();
        return;
      }
      if (!k.startsWith("node.")) return;
      const id = e.payload?.node_id as string | undefined;
      if (!id) return;
      const state = k === "node.started" ? "running" : k === "node.completed" ? "done" : k === "node.failed" ? "failed" : "";
      if (state) setNodeStatus((prev) => ({ ...prev, [id]: state }));
    });
  }, [subscribe]);

  async function generate() {
    if (!intent.trim()) return say("enter an intent first", true);
    setBusy(true);
    say("generating…");
    try {
      const body: Record<string, string> = { intent: intent.trim() };
      if (model.trim()) body.model = model.trim();
      const d = await postJSON<{ plan_json: string; node_count: number }>("/api/plan/generate", body);
      setPlanText(prettyJSON(d.plan_json));
      setNodeStatus({});
      say(`generated ${d.node_count ?? "?"} node(s)`);
    } catch (e) {
      say((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }

  async function refine() {
    if (!planText.trim()) return say("nothing to refine", true);
    if (!feedback.trim()) return say("enter a refine instruction", true);
    setBusy(true);
    say("refining…");
    try {
      const body: Record<string, string> = { plan_json: planText.trim(), feedback: feedback.trim() };
      if (model.trim()) body.model = model.trim();
      const d = await postJSON<{ plan_json: string; node_count: number }>("/api/plan/refine", body);
      setPlanText(prettyJSON(d.plan_json));
      setNodeStatus({});
      say(`refined → ${d.node_count ?? "?"} node(s)`);
      setFeedback("");
    } catch (e) {
      say((e as Error).message, true);
    } finally {
      setBusy(false);
    }
  }

  async function run() {
    if (!planText.trim()) return say("nothing to run", true);
    setRunning(true);
    say("starting run…");
    try {
      await postJSON("/api/plan/run", { plan_json: planText.trim() });
      say("run finished ✓");
    } catch (e) {
      say(`run: ${(e as Error).message}`, true);
    } finally {
      setRunning(false);
      loadHistory();
    }
  }

  return (
    <Page
      icon={Workflow}
      title="Flow Studio"
      description="Describe a task; the planner drafts an editable flow you can refine and run."
      mode="fill"
      width="full"
      actions={
        <>
          <Button size="sm" variant="ghost" onClick={loadHistory} title="Refresh history">
            <RefreshCw className="size-3.5" /> Refresh
          </Button>
          <Button size="sm" onClick={() => setComposeOpen(true)}>
            <Sparkles className="size-3.5" /> Compose
          </Button>
        </>
      }
    >
      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-2">
        {/* Authoring column */}
        <Card className="min-h-0">
          <CardHeader>
            <CardTitle>Plan editor</CardTitle>
            {msg ? (
              msgErr ? <ErrorText>{msg}</ErrorText> : <Muted>{msg}</Muted>
            ) : null}
          </CardHeader>
          <CardBody className="flex min-h-0 flex-col gap-2">
            {planText.trim() ? (
              <Textarea
                value={planText}
                onChange={(e) => setPlanText(e.target.value)}
                placeholder="generated plan appears here — editable, then Refine or Run"
                className="min-h-[260px] flex-1 font-mono text-xs"
              />
            ) : (
              <button
                className="flex min-h-[260px] flex-1 flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border bg-panel/35 p-6 text-center transition-colors hover:border-accent hover:bg-panel/55"
                onClick={() => setComposeOpen(true)}
              >
                <span className="grid size-12 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
                  <FileJson2 className="size-6" />
                </span>
                <span className="text-sm font-semibold text-foreground">No flow drafted</span>
                <span className="max-w-sm text-xs text-muted">Compose from an intent, then inspect the JSON and DAG before running it.</span>
              </button>
            )}
            <div className="flex flex-wrap gap-2">
              <Button variant="accent" onClick={() => setComposeOpen(true)} disabled={busy} className="shrink-0">
                <Sparkles /> {planText.trim() ? "Regenerate" : "Compose"}
              </Button>
              <Button onClick={() => setRefineOpen(true)} disabled={busy || !planText.trim()} className="shrink-0">
                <Wand2 /> Refine
              </Button>
              <Button variant="good" onClick={run} disabled={running || !planText.trim()} className="shrink-0">
                <Play /> {running ? "running…" : "Run"}
              </Button>
            </div>
          </CardBody>
        </Card>

        {/* Diagram column */}
        <Card className="min-h-0">
          <CardHeader>
            <CardTitle>Diagram</CardTitle>
            <Muted>loop = box · gate = hexagon</Muted>
          </CardHeader>
          <CardBody className="p-0">
            <div className="h-full min-h-[300px]">
              <PlanDag plan={plan} status={nodeStatus} />
            </div>
          </CardBody>
        </Card>
      </div>

      {/* History */}
      <Card className="shrink-0">
        <CardHeader>
          <CardTitle>Recent plans</CardTitle>
          <Button variant="ghost" size="icon" className="ml-auto" onClick={loadHistory} title="Refresh">
            <RefreshCw />
          </Button>
        </CardHeader>
        <CardBody className="max-h-40">
          {history.length ? (
            <ul className="space-y-1">
              {history.map((p, i) => (
                <li key={p.correlation_id || i} className="flex items-center gap-2">
                  <Badge variant={statusVariant(p.status)}>{p.status || "?"}</Badge>
                  <span className="truncate">{p.plan_name || p.correlation_id || "plan"}</span>
                  <span className="ml-auto shrink-0 text-muted">
                    {p.node_count != null ? `${p.node_count} nodes` : ""}
                    {p.duration_ms ? ` · ${p.duration_ms}ms` : ""}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <Muted>no plan runs yet</Muted>
          )}
        </CardBody>
      </Card>

      {composeOpen && (
        <FlowModal title="Compose flow" icon={Sparkles} onClose={() => setComposeOpen(false)}>
          <div className="space-y-3">
            <label className="block text-[11px] text-muted">
              Intent
              <Textarea
                autoFocus
                value={intent}
                onChange={(e) => setIntent(e.target.value)}
                placeholder="audit my repo for secrets and propose fixes"
                className="mt-1 min-h-28"
              />
            </label>
            <label className="block text-[11px] text-muted">
              Model override
              <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="optional" className="mt-1" />
            </label>
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => setComposeOpen(false)}>Cancel</Button>
              <Button
                size="sm"
                variant="accent"
                onClick={async () => {
                  await generate();
                  if (intent.trim()) setComposeOpen(false);
                }}
                disabled={busy || !intent.trim()}
              >
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Sparkles className="size-3.5" />} Generate
              </Button>
            </div>
          </div>
        </FlowModal>
      )}

      {refineOpen && (
        <FlowModal title="Refine flow" icon={Wand2} onClose={() => setRefineOpen(false)}>
          <div className="space-y-3">
            <label className="block text-[11px] text-muted">
              Change request
              <Textarea
                autoFocus
                value={feedback}
                onChange={(e) => setFeedback(e.target.value)}
                placeholder="add an approval gate before deploy"
                className="mt-1 min-h-28"
              />
            </label>
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => setRefineOpen(false)}>Cancel</Button>
              <Button
                size="sm"
                onClick={async () => {
                  await refine();
                  if (feedback.trim()) setRefineOpen(false);
                }}
                disabled={busy || !feedback.trim() || !planText.trim()}
              >
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Wand2 className="size-3.5" />} Refine
              </Button>
            </div>
          </div>
        </FlowModal>
      )}
    </Page>
  );
}

function FlowModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: typeof Sparkles;
  onClose: () => void;
  children: React.ReactNode;
}) {
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
        className="modal-in mt-10 w-full max-w-xl rounded-lg border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="mb-3 flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
            <Icon className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close flow modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}
