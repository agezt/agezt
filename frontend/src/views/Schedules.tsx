import { Panel, Row, Count } from "@/components/Panel";
import { Badge, statusVariant } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";
import { fmtDateTime } from "@/lib/utils";

export function Schedules() {
  return (
    <Panel<Record<string, any>> title="Schedules" path="/api/schedules">
      {(d) => {
        const ss = d.schedules || [];
        return (
          <>
            <Count>{ss.length} schedule(s)</Count>
            {ss.length ? (
              ss.map((s: any, i: number) => (
                <Row key={s.id || i}>
                  <Badge>{s.cadence || s.mode || "?"}</Badge>
                  <span>{s.intent || s.id || ""}</span>
                  {s.enabled === false ? (
                    <Muted>(paused)</Muted>
                  ) : s.next_run_unix ? (
                    <span className="text-muted">next {fmtDateTime(s.next_run_unix * 1000)}</span>
                  ) : null}
                  {s.last_status ? (
                    <Badge variant={statusVariant(s.last_status)}>{s.last_status}</Badge>
                  ) : null}
                </Row>
              ))
            ) : (
              <Muted>no schedules</Muted>
            )}
          </>
        );
      }}
    </Panel>
  );
}
