import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";
import { ActionButton } from "@/components/ActionButton";

export function Approvals() {
  return (
    <Panel<Record<string, any>> title="Approvals" path="/api/approvals">
      {(d, reload) => {
        const items = d.pending || [];
        return (
          <>
            <Count>{items.length} pending</Count>
            {items.length ? (
              items.map((a: any, i: number) => (
                <Row key={a.id || i}>
                  <Badge variant="warn">{a.capability || a.tool_name || "?"}</Badge>
                  <span>{a.reason || a.tool_name || a.id || ""}</span>
                  {a.id ? (
                    <span className="ml-auto flex gap-1">
                      <ActionButton label="approve" variant="good" path="/api/decide" params={{ id: a.id, decision: "grant" }} onDone={reload} />
                      <ActionButton label="deny" variant="danger" path="/api/decide" params={{ id: a.id, decision: "deny" }} onDone={reload} />
                    </span>
                  ) : null}
                </Row>
              ))
            ) : (
              <Muted>nothing awaiting approval</Muted>
            )}
          </>
        );
      }}
    </Panel>
  );
}
