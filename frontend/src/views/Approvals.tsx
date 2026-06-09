import { ShieldCheck } from "lucide-react";
import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty";
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
                      <ActionButton label="approve" variant="good" path="/api/decide" params={{ id: a.id, decision: "grant" }} onDone={reload} success="Request approved" />
                      <ActionButton label="deny" variant="danger" path="/api/decide" params={{ id: a.id, decision: "deny" }} onDone={reload} success="Request denied" />
                    </span>
                  ) : null}
                </Row>
              ))
            ) : (
              <EmptyState
                icon={ShieldCheck}
                title="Nothing awaiting approval"
                hint="When the agent hits a capability gated by your ask policy, the request will appear here for you to approve or deny."
              />
            )}
          </>
        );
      }}
    </Panel>
  );
}
