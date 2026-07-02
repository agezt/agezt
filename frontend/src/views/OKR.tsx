import { useEffect, useMemo, useState } from "react";
import { Target, Plus, RefreshCw, Archive, Link2, CheckCircle2 } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Page } from "@/components/ui/page";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";

export interface OKRKeyResultProgress {
  id?: string;
  title?: string;
  done?: number;
  total?: number;
  target?: number;
  percent?: number;
  achieved?: boolean;
}

export interface OKRObjective {
  id: string;
  title?: string;
  description?: string;
  owner?: string;
  status?: string;
  percent?: number;
  achieved?: boolean;
  key_result_count?: number;
  progress?: { key_results?: OKRKeyResultProgress[]; percent?: number; achieved?: boolean };
}

interface OKRListData {
  objectives?: OKRObjective[];
  count?: number;
}

export function okrAchievedCount(objectives: OKRObjective[] = []): number {
  return objectives.filter((o) => o.status === "achieved" || o.achieved).length;
}

export function okrAveragePercent(objectives: OKRObjective[] = []): number {
  const active = objectives.filter((o) => o.status !== "archived");
  if (active.length === 0) return 0;
  const sum = active.reduce((acc, o) => acc + (o.percent || 0), 0);
  return Math.round(sum / active.length);
}

interface OKRLinkableTask {
  id: string;
  title?: string;
  status?: string;
}

export function OKR() {
  const [objectives, setObjectives] = useState<OKRObjective[]>([]);
  const [tasks, setTasks] = useState<OKRLinkableTask[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [newTitle, setNewTitle] = useState("");

  useEffect(() => {
    getJSON<{ tasks?: OKRLinkableTask[] }>("/api/workboard")
      .then((d) => setTasks(Array.isArray(d.tasks) ? d.tasks : []))
      .catch(() => {});
  }, []);

  const avg = useMemo(() => okrAveragePercent(objectives), [objectives]);
  const achieved = useMemo(() => okrAchievedCount(objectives), [objectives]);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<OKRListData>("/api/okr");
      setObjectives(Array.isArray(d.objectives) ? d.objectives : []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function mutate(path: string, body: Record<string, unknown>, after?: () => void) {
    setBusy(true);
    try {
      await postJSON(path, body);
      after?.();
      await reload();
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  return (
    <Page
      icon={Target}
      title="Objectives"
      description="Goals and key results that roll up proven work."
      actions={
        <Button variant="ghost" size="sm" onClick={() => reload()} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      <section className="grid gap-2 sm:grid-cols-3">
        <Metric label="Objectives" value={objectives.filter((o) => o.status !== "archived").length} />
        <Metric label="Achieved" value={achieved} tone="good" />
        <Metric label="Avg progress" value={avg} suffix="%" tone="accent" />
      </section>

      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (newTitle.trim()) mutate("/api/okr/create", { title: newTitle.trim() }, () => setNewTitle(""));
        }}
      >
        <input
          value={newTitle}
          onChange={(e) => setNewTitle(e.target.value)}
          aria-label="New objective title"
          placeholder="New objective — e.g. Ship the proof loop"
          className="h-9 min-w-0 flex-1 rounded-md border border-border bg-panel px-3 text-sm outline-none focus-visible:border-accent"
        />
        <Button type="submit" size="sm" disabled={busy || !newTitle.trim()}>
          <Plus className="size-3.5" /> Objective
        </Button>
      </form>

      {err && <ErrorText>{err}</ErrorText>}

      {loading && objectives.length === 0 ? (
        <SkeletonList count={3} lines={3} />
      ) : objectives.length === 0 ? (
        <EmptyState icon={Target} title="No objectives yet" hint="Create an objective, add key results, and link workboard tasks — proven tasks roll their progress up here." />
      ) : (
        <div className="grid gap-3">
          {objectives.map((o) => (
            <ObjectiveCard key={o.id} obj={o} tasks={tasks} busy={busy} mutate={mutate} />
          ))}
        </div>
      )}
    </Page>
  );
}

function ObjectiveCard({
  obj,
  tasks,
  busy,
  mutate,
}: {
  obj: OKRObjective;
  tasks: OKRLinkableTask[];
  busy: boolean;
  mutate: (path: string, body: Record<string, unknown>, after?: () => void) => void;
}) {
  const [krTitle, setKrTitle] = useState("");
  const [krTarget, setKrTarget] = useState("0");
  const [linkKR, setLinkKR] = useState("");
  const [linkTask, setLinkTask] = useState("");
  const krs = obj.progress?.key_results || [];
  const linkable = tasks.filter((t) => t.status !== "archived");
  const percent = obj.percent || 0;
  const done = obj.status === "achieved" || !!obj.achieved;

  return (
    <section
      className={cn(
        "rounded-lg border bg-card/75 p-3",
        done ? "border-emerald-500/40" : "border-border",
      )}
    >
      <div className="mb-2 flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-semibold">{obj.title || obj.id}</h3>
            <Badge variant={done ? "good" : obj.status === "archived" ? "default" : "accent"}>{obj.status || "active"}</Badge>
          </div>
          {obj.description && <p className="mt-0.5 text-xs text-muted">{obj.description}</p>}
          <div className="mt-0.5 flex flex-wrap gap-x-3 text-[11px] text-muted">
            <span className="font-mono">{obj.id}</span>
            {obj.owner && <span>owner {obj.owner}</span>}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold tabular-nums">{percent}%</span>
          {obj.status !== "archived" && (
            <Button size="sm" variant="ghost" disabled={busy} title="Archive" onClick={() => mutate("/api/okr/archive", { id: obj.id })}>
              <Archive className="size-3.5" />
            </Button>
          )}
        </div>
      </div>

      <ProgressBar percent={percent} achieved={done} />

      <ul className="mt-2 space-y-1.5">
        {krs.length === 0 && <li className="text-xs text-muted">No key results yet.</li>}
        {krs.map((kr) => (
          <li key={kr.id} className="rounded-md border border-border/70 bg-background/45 p-2">
            <div className="flex items-center justify-between gap-2 text-xs">
              <span className="flex min-w-0 items-center gap-1.5">
                {kr.achieved && <CheckCircle2 className="size-3.5 shrink-0 text-emerald-500" />}
                <span className="truncate">{kr.title}</span>
              </span>
              <span className="shrink-0 tabular-nums text-muted">
                {kr.done ?? 0}/{kr.target ?? 0} · {kr.percent ?? 0}%
              </span>
            </div>
            <div className="mt-1">
              <ProgressBar percent={kr.percent || 0} achieved={!!kr.achieved} thin />
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-1 text-[10px] text-muted">
              <span className="font-mono">{kr.id}</span>
            </div>
          </li>
        ))}
      </ul>

      {obj.status !== "archived" && (
        <div className="mt-2 grid gap-1.5 border-t border-border/60 pt-2 sm:grid-cols-2">
          <div className="flex items-center gap-1.5">
            <input
              value={krTitle}
              onChange={(e) => setKrTitle(e.target.value)}
              aria-label="Key result title"
              placeholder="new key result"
              className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
            />
            <input
              value={krTarget}
              onChange={(e) => setKrTarget(e.target.value)}
              aria-label="Key result target"
              title="target task count (0 = all linked)"
              className="h-8 w-12 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
            />
            <Button
              size="sm"
              disabled={busy || !krTitle.trim()}
              title="Add key result"
              onClick={() => mutate("/api/okr/keyresult", { id: obj.id, title: krTitle.trim(), target: Number(krTarget) || 0 }, () => { setKrTitle(""); setKrTarget("0"); })}
            >
              <Plus className="size-3.5" />
            </Button>
          </div>
          <div className="flex items-center gap-1.5">
            <select
              value={linkKR}
              onChange={(e) => setLinkKR(e.target.value)}
              aria-label="Key result to link"
              disabled={krs.length === 0}
              className="h-8 w-28 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
            >
              <option value="">key result…</option>
              {krs.map((kr) => (
                <option key={kr.id} value={kr.id}>{kr.title}</option>
              ))}
            </select>
            <select
              value={linkTask}
              onChange={(e) => setLinkTask(e.target.value)}
              aria-label="Task to link"
              disabled={linkable.length === 0}
              className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
            >
              <option value="">{linkable.length ? "task…" : "no tasks"}</option>
              {linkable.map((t) => (
                <option key={t.id} value={t.id}>{t.title || t.id} · {t.status}</option>
              ))}
            </select>
            <Button
              size="sm"
              variant="ghost"
              disabled={busy || !linkKR.trim() || !linkTask.trim()}
              title="Link a workboard task to a key result"
              onClick={() => mutate("/api/okr/link", { id: obj.id, key_result: linkKR.trim(), task: linkTask.trim() }, () => { setLinkKR(""); setLinkTask(""); })}
            >
              <Link2 className="size-3.5" />
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}

function ProgressBar({ percent, achieved, thin }: { percent: number; achieved: boolean; thin?: boolean }) {
  const pct = Math.max(0, Math.min(100, percent));
  return (
    <div className={cn("w-full overflow-hidden rounded-full bg-border/60", thin ? "h-1.5" : "h-2.5")}>
      <div
        className={cn("h-full rounded-full transition-all", achieved ? "bg-emerald-500" : "bg-accent")}
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

function Metric({ label, value, suffix, tone }: { label: string; value: number; suffix?: string; tone?: "good" | "accent" }) {
  return (
    <div className="rounded-lg border border-border bg-card/70 p-3">
      <div className="text-xs uppercase tracking-normal text-muted">{label}</div>
      <div className={cn("mt-1 text-2xl font-semibold tabular-nums", tone === "good" ? "text-emerald-500" : tone === "accent" ? "text-accent" : "")}>
        {value}
        {suffix}
      </div>
    </div>
  );
}
