import { Globe } from "lucide-react";
import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { ActionButton } from "@/components/ActionButton";
import { WorldGraph } from "@/components/WorldGraph";
import { BreakdownBar } from "@/components/Widgets";

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
