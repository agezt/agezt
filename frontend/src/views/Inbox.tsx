import { Panel, Row } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";

export function Inbox() {
  return (
    <Panel<Record<string, any>> title="Inbox" path="/api/inbox">
      {(d) => {
        const items = d.items || (Array.isArray(d) ? d : []);
        return items.length ? (
          items.map((it: any, i: number) => (
            <Row key={it.id || i}>
              <Badge>{it.channel || it.kind || ""}</Badge>
              <span>{it.summary || it.text || ""}</span>
            </Row>
          ))
        ) : (
          <Muted>inbox empty</Muted>
        );
      }}
    </Panel>
  );
}
