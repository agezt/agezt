import { useState, type ReactNode } from "react";
import { Play, AlertTriangle, Wrench, CheckCheck, Archive, Trash2, Plus, Repeat } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { type AgentProfile } from "@/views/Roster";
import { type AgentOperationalTask } from "@/lib/agentdetail";
import { DetailOptionPicker } from "@/components/agentdetail/shared";

export function AgentTaskList({
  tasks,
  busy,
  onAction,
  onAdd,
}: {
  tasks: NonNullable<AgentProfile["tasklist"]>;
  busy?: string | null;
  onAction?: (id: string, op: "update" | "remove", status?: string) => void;
  onAdd?: (title: string, scope: "cycle" | "total") => void;
}) {
  const cycle = tasks.filter(
    (t) => (t.scope || "total") === "cycle" && t.status !== "retired",
  );
  const total = tasks.filter(
    (t) => (t.scope || "total") === "total" && t.status !== "retired",
  );
  return (
    <div className="space-y-2">
      {onAdd && <TaskComposer busy={busy} onAdd={onAdd} />}
      <div className="grid gap-2 sm:grid-cols-2">
        <TaskGroup title="every-cycle tasks" tasks={cycle} busy={busy} onAction={onAction} />
        <TaskGroup title="total tasklist" tasks={total} busy={busy} onAction={onAction} />
      </div>
    </div>
  );
}

function TaskComposer({
  busy,
  onAdd,
}: {
  busy?: string | null;
  onAdd: (title: string, scope: "cycle" | "total") => void;
}) {
  const [title, setTitle] = useState("");
  const [scope, setScope] = useState<"cycle" | "total">("cycle");
  const adding = busy === `add:${scope}`;
  return (
    <div className="rounded-md border border-border bg-panel/45 p-2">
      <div className="mb-1 text-xs uppercase tracking-normal text-muted">
        add durable task
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          aria-label="New agent task"
          placeholder="Task title..."
          className="min-w-[14rem] flex-1 rounded-md border border-border bg-card px-2 py-1 text-xs text-foreground outline-none placeholder:text-muted/60 focus-visible:border-accent"
        />
        <div className="min-w-[14rem]">
          <DetailOptionPicker
            label="New task scope"
            value={scope}
            onChange={setScope}
            columns="grid-cols-2"
            options={[
              { value: "cycle", label: "Every cycle", detail: "repeat task", icon: <Repeat className="size-3.5" /> },
              { value: "total", label: "Total", detail: "finish once", icon: <CheckCheck className="size-3.5" /> },
            ]}
          />
        </div>
        <Button
          type="button"
          size="sm"
          disabled={adding || !title.trim()}
          onClick={() => {
            const submitted = title.trim();
            onAdd(submitted, scope);
            setTitle("");
          }}
        >
          {adding ? <Wrench className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add task
        </Button>
      </div>
    </div>
  );
}

export function OperationalTaskList({
  tasks,
  compact = false,
}: {
  tasks: AgentOperationalTask[];
  compact?: boolean;
}) {
  return (
    <TaskGroup title="operational queue" tasks={tasks} compact={compact} />
  );
}

function TaskGroup({
  title,
  tasks,
  compact = false,
  busy,
  onAction,
}: {
  title: string;
  tasks: Array<{
    id?: string;
    title: string;
    description?: string;
    status?: string;
  }>;
  compact?: boolean;
  busy?: string | null;
  onAction?: (id: string, op: "update" | "remove", status?: string) => void;
}) {
  const summary = taskGroupSummary(tasks);
  return (
    <div>
      <div className="mb-1 flex items-center gap-2">
        <span className="text-xs uppercase tracking-normal text-muted">
          {title}
        </span>
        {summary && (
          <span className="ml-auto truncate text-xs text-muted" title={summary}>
            {summary}
          </span>
        )}
      </div>
      {tasks.length === 0 ? (
        <div className="rounded-md bg-panel p-2.5 text-xs text-muted">
          empty
        </div>
      ) : (
        <ul className="space-y-1 rounded-md bg-panel p-2.5 text-xs text-foreground/85">
          {tasks.map((t, i) => (
            <li
              key={t.id || `${t.title}-${i}`}
              className={cn(
                "flex gap-2",
                !compact && t.description && "items-start",
              )}
            >
              <span className={cn("shrink-0 rounded px-1.5 py-0.5 font-mono text-xs uppercase", taskStatusTone(t.status))}>
                {t.status || "todo"}
              </span>
              <span className="min-w-0">
                <span>{t.title}</span>
                {!compact && t.description && (
                  <span className="mt-0.5 block text-[11px] text-muted">
                    {t.description}
                  </span>
                )}
              </span>
              {!compact && onAction && t.id && (
                <span className="ml-auto flex shrink-0 items-center gap-1">
                  <TaskAction
                    title="Mark doing"
                    busy={busy === `update:${t.id}:doing`}
                    onClick={() => onAction(t.id || "", "update", "doing")}
                  >
                    <Play className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Mark done"
                    busy={busy === `update:${t.id}:done`}
                    onClick={() => onAction(t.id || "", "update", "done")}
                  >
                    <CheckCheck className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Mark blocked"
                    busy={busy === `update:${t.id}:blocked`}
                    onClick={() => onAction(t.id || "", "update", "blocked")}
                  >
                    <AlertTriangle className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Retire task"
                    busy={busy === `update:${t.id}:retired`}
                    onClick={() => onAction(t.id || "", "update", "retired")}
                  >
                    <Archive className="size-3" />
                  </TaskAction>
                  <TaskAction
                    title="Remove task"
                    busy={busy === `remove:${t.id}:`}
                    danger
                    onClick={() => onAction(t.id || "", "remove")}
                  >
                    <Trash2 className="size-3" />
                  </TaskAction>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function taskGroupSummary(tasks: Array<{ status?: string }>): string {
  if (tasks.length === 0) return "";
  const counts = tasks.reduce<Record<string, number>>((acc, task) => {
    const status = (task.status || "todo").trim().toLowerCase() || "todo";
    acc[status] = (acc[status] || 0) + 1;
    return acc;
  }, {});
  return ["todo", "doing", "blocked", "done"]
    .map((status) => (counts[status] ? `${counts[status]} ${status}` : ""))
    .filter(Boolean)
    .join(" · ");
}

function taskStatusTone(status?: string): string {
  switch ((status || "todo").toLowerCase()) {
    case "doing":
      return "bg-accent/10 text-accent";
    case "done":
      return "bg-good/10 text-good";
    case "blocked":
      return "bg-bad/10 text-bad";
    default:
      return "bg-card text-muted";
  }
}

function TaskAction({
  title,
  busy,
  danger,
  onClick,
  children,
}: {
  title: string;
  busy?: boolean;
  danger?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      disabled={busy}
      className={cn(
        "rounded border border-border bg-card p-1 text-muted transition-colors hover:border-accent hover:text-accent disabled:opacity-50",
        danger && "hover:border-bad hover:text-bad",
      )}
    >
      {busy ? <Wrench className="size-3 animate-spin" /> : children}
    </button>
  );
}

export function ToolPolicyBox({
  title,
  items,
  empty,
}: {
  title: string;
  items: string[];
  empty: string;
}) {
  return (
    <div>
      <div className="mb-1 text-xs uppercase tracking-normal text-muted">
        {title}
      </div>
      {items.length === 0 ? (
        <div className="rounded-md bg-panel p-2.5 text-xs text-muted">
          {empty}
        </div>
      ) : (
        <div className="rounded-md bg-panel p-2.5 text-xs text-foreground/85">
          {items.join(", ")}
        </div>
      )}
    </div>
  );
}

