import { Panel, Row } from "@/components/Panel";
import { Muted } from "@/components/JsonView";
import { ActionButton } from "@/components/ActionButton";

export function Memory() {
  return (
    <Panel<Record<string, any>> title="Memory" path="/api/memory">
      {(d, reload) => {
        const ms = d.records || (Array.isArray(d) ? d : []);
        return ms.length ? (
          ms.map((m: any, i: number) => (
            <Row key={m.id || i}>
              <span>{m.content || m.subject || m.text || JSON.stringify(m)}</span>
              {m.id ? (
                <span className="ml-auto">
                  <ActionButton label="forget" variant="danger" path="/api/memory/forget" params={{ id: m.id }} onDone={reload} />
                </span>
              ) : null}
            </Row>
          ))
        ) : (
          <Muted>no memories</Muted>
        );
      }}
    </Panel>
  );
}
