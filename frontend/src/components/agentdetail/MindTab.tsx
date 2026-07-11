import { useState } from "react";
import { ArrowUpRight, Brain, FileCode, Heart, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import { TabNav } from "@/components/ui/tab-nav";
import { KeyValue } from "@/components/JsonView";
import { type AgentProfile } from "@/views/Roster";
import { agentScope, type AgentConfigOverrideSummary, type MemoryRecord, type SkillLite } from "@/lib/agentdetail";
import { ConfigOverrideBox, LifecycleConfigEditor } from "@/components/agentdetail/shared";
import { AgentTaskList, ToolPolicyBox } from "@/components/agentdetail/tasks";
import { FilesTab } from "@/components/agentdetail/FilesTab";
import { MemoryTab } from "@/components/agentdetail/MemoryTab";
import { SkillsTab } from "@/components/agentdetail/SkillsTab";
import { agentRetryPolicyDetail } from "@/components/agentdetail/lifecycle";

// MindTab — who this agent is and what it carries: soul, standing instructions,
// tasklist, and the memory/skills/files it owns. One inner tab row replaces the
// old Soul / Memory / Skills / Files top-level tabs.
export function MindTab({
  slug,
  profile,
  overrides,
  memory,
  skills,
  busy,
  taskBusy,
  onSaveExecProfile,
  onMutateTask,
  onAddTask,
  onAction,
  onChanged,
  onManage,
}: {
  slug: string;
  profile: AgentProfile;
  overrides: AgentConfigOverrideSummary;
  memory: MemoryRecord[] | null;
  skills: SkillLite[] | null;
  busy: boolean;
  taskBusy: string | null;
  onSaveExecProfile: (value: string) => void;
  onMutateTask: (id: string, op: "update" | "remove", status?: string) => void;
  onAddTask: (title: string, scope: "cycle" | "total") => void;
  onAction: (
    path: string,
    params: Record<string, string>,
    success: string,
  ) => Promise<void> | void;
  onChanged: () => void;
  onManage: (view: string) => void;
}) {
  const [sub, setSub] = useState("soul");
  const soulLines = (profile.soul || "").split("\n").length;
  return (
    <TabNav
      value={sub}
      onValueChange={setSub}
      tabs={[
        {
          id: "soul",
          label: "Soul",
          icon: Heart,
          content: (
            <div className="space-y-3">
              <div>
                <div className="mb-1 text-xs uppercase tracking-normal text-muted">
                  soul — identity core
                </div>
                {profile.soul ? (
                  soulLines > 12 ? (
                    <Disclosure
                      summary={
                        <span className="text-xs text-muted">
                          {soulLines} lines — show full soul
                        </span>
                      }
                    >
                      <pre className="max-h-[28rem] overflow-auto whitespace-pre-wrap rounded-md bg-panel p-2.5 font-mono text-[11px] text-foreground/85">
                        {profile.soul}
                      </pre>
                    </Disclosure>
                  ) : (
                    <pre className="max-h-[28rem] overflow-auto whitespace-pre-wrap rounded-md bg-panel p-2.5 font-mono text-[11px] text-foreground/85">
                      {profile.soul}
                    </pre>
                  )
                ) : (
                  <div className="text-xs text-muted">
                    no soul set — this agent inherits the default daemon identity
                  </div>
                )}
              </div>
              {(profile.instructions || []).length > 0 && (
                <div>
                  <div className="mb-1 text-xs uppercase tracking-normal text-muted">
                    standing instructions
                  </div>
                  <ul className="space-y-1 rounded-md bg-panel p-2.5 text-xs text-foreground/85">
                    {(profile.instructions || []).map((ins, i) => (
                      <li key={`${i}-${ins}`}>{ins}</li>
                    ))}
                  </ul>
                </div>
              )}
              <AgentTaskList
                tasks={profile.tasklist || []}
                busy={taskBusy}
                onAction={onMutateTask}
                onAdd={onAddTask}
              />
              <LifecycleConfigEditor
                slug={slug}
                profile={profile}
                busy={busy}
                onChanged={onChanged}
              />
              <div className="rounded-lg border border-border bg-panel/30 p-2.5">
                <KeyValue
                  pairs={[
                    [
                      "isolation",
                      <select
                        key="isolation"
                        aria-label="Execution isolation profile"
                        value={profile.execution_profile || ""}
                        disabled={busy}
                        onChange={(e) => onSaveExecProfile(e.target.value)}
                        className="h-7 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
                      >
                        <option value="">tool defaults</option>
                        <option value="local">local</option>
                        <option value="warden">warden</option>
                        <option value="container">container</option>
                      </select>,
                    ],
                    [
                      "memory scope",
                      <span key="scope" className="font-mono">
                        {agentScope(slug, profile.memory_scope)}
                      </span>,
                    ],
                    [
                      "workdir",
                      profile.workdir ? (
                        <span key="workdir" className="font-mono">{profile.workdir}</span>
                      ) : (
                        "—"
                      ),
                    ],
                    ["retry", agentRetryPolicyDetail(profile)],
                    [
                      "doctor",
                      profile.health_policy?.doctor_agent ? (
                        <span key="doctor" className="font-mono">
                          {profile.health_policy.doctor_agent}
                        </span>
                      ) : (
                        "—"
                      ),
                    ],
                    [
                      "self-repair",
                      profile.self_repair?.enabled
                        ? `enabled${profile.self_repair?.max_attempts ? ` · ${profile.self_repair.max_attempts} attempts` : ""}`
                        : "off",
                    ],
                  ]}
                />
              </div>
              {((profile.tool_allow || []).length > 0 ||
                (profile.tool_deny || []).length > 0) && (
                <div className="grid gap-2 sm:grid-cols-2">
                  <ToolPolicyBox
                    title="tool allowlist"
                    items={profile.tool_allow || []}
                    empty="all advertised tools"
                  />
                  <ToolPolicyBox
                    title="tool denylist"
                    items={profile.tool_deny || []}
                    empty="none blocked"
                  />
                </div>
              )}
              {((profile.config_overrides &&
                Object.keys(profile.config_overrides).length > 0) ||
                overrides.runtime.length > 0) && (
                <ConfigOverrideBox summary={overrides} />
              )}
              <Button variant="ghost" size="sm" onClick={() => onManage("roster")}>
                Edit in Roster <ArrowUpRight className="size-3.5" />
              </Button>
            </div>
          ),
        },
        {
          id: "memory",
          label: "Memory",
          icon: Brain,
          count: memory?.length,
          content: (
            <MemoryTab
              records={memory}
              scope={agentScope(slug, profile.memory_scope)}
              busy={busy}
              onAction={onAction}
              onManage={onManage}
            />
          ),
        },
        {
          id: "skills",
          label: "Skills",
          icon: Sparkles,
          count: skills?.length,
          content: (
            <SkillsTab
              skills={skills}
              busy={busy}
              onAction={onAction}
              onManage={onManage}
            />
          ),
        },
        {
          id: "files",
          label: "Files",
          icon: FileCode,
          content: <FilesTab workdir={profile.workdir} skills={skills} />,
        },
      ]}
    />
  );
}
