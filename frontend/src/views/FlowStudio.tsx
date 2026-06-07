import { useEffect, useMemo, useState } from "react";
import { Play, Sparkles, Wand2, RefreshCw } from "lucide-react";
import { postJSON, getJSON } from "@/lib/api";
import { useEvents, type AgentEvent } from "@/lib/events";
import { Card, CardHeader, CardTitle, CardBody } from "@/components/ui/card";
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
    <div className="flex h-full flex-col gap-3">
      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-2">
        {/* Authoring column */}
        <Card className="min-h-0">
          <CardHeader>
            <CardTitle>Flow Studio</CardTitle>
            {msg ? (
              msgErr ? <ErrorText>{msg}</ErrorText> : <Muted>{msg}</Muted>
            ) : null}
          </CardHeader>
          <CardBody className="flex min-h-0 flex-col gap-2">
            <div className="flex gap-2">
              <Input
                value={intent}
                onChange={(e) => setIntent(e.target.value)}
                placeholder="describe a task to plan… e.g. audit my repo for secrets and propose fixes"
              />
              <Input
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder="model (optional)"
                className="w-32 shrink-0"
              />
              <Button variant="accent" onClick={generate} disabled={busy} className="shrink-0">
                <Sparkles /> Generate
              </Button>
            </div>
            <Textarea
              value={planText}
              onChange={(e) => setPlanText(e.target.value)}
              placeholder="generated plan appears here — editable, then Refine or Run"
              className="min-h-[200px] flex-1"
            />
            <div className="flex gap-2">
              <Input
                value={feedback}
                onChange={(e) => setFeedback(e.target.value)}
                placeholder="refine: describe a change… e.g. add an approval gate before deploy"
              />
              <Button onClick={refine} disabled={busy} className="shrink-0">
                <Wand2 /> Refine
              </Button>
              <Button variant="good" onClick={run} disabled={running} className="shrink-0">
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
    </div>
  );
}
