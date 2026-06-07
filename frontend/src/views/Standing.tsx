import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";

export function Standing() {
  return (
    <Panel<Record<string, any>> title="Standing" path="/api/standing">
      {(d) => {
        const orders = d.orders || [];
        return (
          <>
            <Count>
              {d.enabled_count != null ? `${d.enabled_count} enabled · ` : ""}
              {orders.length} total
            </Count>
            {orders.length ? (
              orders.map((o: any, i: number) => {
                const trigs = o.triggers || [];
                const ini = o.initiative || {};
                return (
                  <Row key={o.id || i}>
                    <Badge variant={o.enabled ? "good" : "default"}>{o.enabled ? "on" : "off"}</Badge>
                    <span>{o.name || o.id || ""}</span>
                    {trigs.length ? <Muted>{trigs.length} trigger(s)</Muted> : null}
                    {ini.mode ? <Muted>{ini.mode}</Muted> : null}
                  </Row>
                );
              })
            ) : (
              <Muted>no standing orders</Muted>
            )}
          </>
        );
      }}
    </Panel>
  );
}
