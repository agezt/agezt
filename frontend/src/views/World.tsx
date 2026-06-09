import { useState } from "react";
import { Globe, Plus, RefreshCw } from "lucide-react";
import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Muted } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { ActionButton } from "@/components/ActionButton";
import { WorldGraph } from "@/components/WorldGraph";
import { BreakdownBar } from "@/components/Widgets";
import { postJSON } from "@/lib/api";
import { useUI } from "@/components/ui/feedback";

// The entity kinds the world model recognises — offered when teaching it one.
const WORLD_KINDS = ["person", "project", "repo", "org", "account", "device", "channel", "topic", "task"];

// kindBreakdown counts entities by kind for the breakdown bar.
function kindBreakdown(ents: any[]): { label: string; count: number }[] {
  const c: Record<string, number> = {};
  for (const e of ents) c[e.kind || "entity"] = (c[e.kind || "entity"] || 0) + 1;
  return Object.entries(c).map(([label, count]) => ({ label, count }));
}

export function World() {
  return (
    <Panel<Record<string, any>> title="World" path="/api/world">
      {(d, reload) => {
        const ents = d.entities || [];
        const edges = d.edges || [];
        const rels = d.relations ?? d.relation_count ?? edges.length;
        return (
          <>
            <Count>
              {ents.length} entities · {rels} relations
            </Count>
            <WorldAddForm onAdded={reload} />

            {ents.length > 0 && <BreakdownBar segments={kindBreakdown(ents)} />}
            {ents.length >= 2 && (
              <div className="h-72 overflow-hidden rounded-md border border-border bg-panel">
                <WorldGraph entities={ents} edges={edges} />
              </div>
            )}
            {ents.length ? (
              ents.map((e: any, i: number) => (
                <Row key={e.id || i}>
                  <Badge>{e.kind || ""}</Badge>
                  <span>{e.name || ""}</span>
                  {e.weight != null ? <Muted>w={e.weight}</Muted> : null}
                  {e.id ? (
                    <span className="ml-auto">
                      <ActionButton
                        label="forget"
                        variant="danger"
                        path="/api/world/forget"
                        params={{ id: e.id }}
                        onDone={reload}
                        confirm={{
                          title: "Forget this entity?",
                          message: e.name
                            ? `“${e.name}” and its relations will be permanently removed from the world model.`
                            : "This entity and its relations will be permanently removed from the world model.",
                          confirmLabel: "Forget",
                          danger: true,
                        }}
                        success="Entity forgotten"
                      />
                    </span>
                  ) : null}
                </Row>
              ))
            ) : (
              <EmptyState
                icon={Globe}
                title="No entities yet"
                hint="The agent builds its world model as it learns about people, projects and systems — it'll fill in here."
              />
            )}
          </>
        );
      }}
    </Panel>
  );
}

// WorldAddForm teaches the agent about an entity — a person, project, device, … —
// from the UI (M721), the write side of the world model. Name + kind post to
// world_add; the entity then participates in recall and relations like any other.
export function WorldAddForm({ onAdded }: { onAdded: () => void }) {
  const { toast } = useUI();
  const [name, setName] = useState("");
  const [kind, setKind] = useState("person");
  const [submitting, setSubmitting] = useState(false);
  const valid = name.trim() !== "";

  async function add() {
    if (!valid) return;
    setSubmitting(true);
    try {
      await postJSON("/api/world/add", { name: name.trim(), kind });
      toast(`Added ${kind} “${name.trim()}”`, "success");
      setName("");
      onAdded();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mb-2 flex flex-wrap items-center gap-1.5">
      <select
        value={kind}
        onChange={(e) => setKind(e.target.value)}
        aria-label="Entity kind"
        className="h-8 rounded-md border border-border bg-panel px-1.5 text-sm outline-none focus-visible:border-accent"
      >
        {WORLD_KINDS.map((k) => (
          <option key={k} value={k}>
            {k}
          </option>
        ))}
      </select>
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") add();
        }}
        placeholder="Name (e.g. Acme Corp, my-repo, Ada)"
        aria-label="Entity name"
        className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
      />
      <Button size="sm" onClick={add} disabled={!valid || submitting} title="Add entity">
        {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add entity
      </Button>
    </div>
  );
}
