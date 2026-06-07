import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";
import { ActionButton } from "@/components/ActionButton";

export function Skills() {
  return (
    <Panel<Record<string, any>> title="Skills" path="/api/skills">
      {(d, reload) => {
        const sk = d.skills || [];
        return (
          <>
            <Count>
              {d.active != null ? `${d.active} active · ` : ""}
              {sk.length} total
            </Count>
            {sk.length ? (
              sk.map((s: any, i: number) => {
                const m = s.metrics || {};
                return (
                  <Row key={s.id || i}>
                    <Badge>{s.status || ""}</Badge>
                    <span>{s.name || ""}</span>
                    {s.status === "shadow" && (m.shadow_evals || 0) > 0 ? (
                      <Muted>
                        shadow {m.shadow_wins || 0}/{m.shadow_evals}
                      </Muted>
                    ) : null}
                    {s.id ? (
                      <span className="ml-auto flex gap-1">
                        {(s.status === "draft" || s.status === "shadow") && (
                          <ActionButton label="promote" variant="good" path="/api/skill/promote" params={{ id: s.id }} onDone={reload} />
                        )}
                        {s.status === "active" && (
                          <ActionButton label="quarantine" variant="danger" path="/api/skill/quarantine" params={{ id: s.id }} onDone={reload} />
                        )}
                        <ActionButton label="revert" path="/api/skill/revert" params={{ id: s.id }} onDone={reload} />
                      </span>
                    ) : null}
                  </Row>
                );
              })
            ) : (
              <Muted>no skills</Muted>
            )}
          </>
        );
      }}
    </Panel>
  );
}
