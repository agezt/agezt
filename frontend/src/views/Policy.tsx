import { Panel, Stats, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { LogDetail } from "@/components/LogDetail";
import { byDescValue, pct } from "@/lib/format";
import { fmtTime } from "@/lib/utils";

export function Policy() {
  return (
    <Panel<Record<string, any>> title="Policy" path="/api/policy">
      {(d) => {
        const byCap: Record<string, number> = d.denied_by_capability || {};
        const names = byDescValue(byCap);
        return (
          <>
            <Stats
              pairs={[
                ["allowed", d.allowed || 0],
                ["denied", `${d.denied || 0}${d.hard_denied ? ` (${d.hard_denied} hard)` : ""}`],
                ["denial rate", pct(d.denial_rate, d.total)],
              ]}
            />
            {names.length > 0 && (
              <div>
                <Count>denied by capability</Count>
                {names.map((n) => (
                  <Row key={n}>
                    <Badge variant="bad">{byCap[n]}</Badge>
                    <span>{n}</span>
                  </Row>
                ))}
              </div>
            )}
            <LogDetail
              label="decision log"
              path="/api/policy_log"
              params={{ limit: "40" }}
              extract={(x) => x.decisions || []}
              render={(ev: any, i) => (
                <div key={i} className="flex gap-2 border-b border-border/40 py-0.5">
                  <span className="text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                  <span className={ev.allow ? "text-good" : "text-bad"}>
                    {ev.allow ? "allow" : ev.hard_denied ? "DENY(hard)" : "DENY"} {ev.capability || ""}{" "}
                    {ev.tool || ""}
                  </span>
                  {ev.reason ? <span className="truncate text-muted">{ev.reason}</span> : null}
                </div>
              )}
            />
          </>
        );
      }}
    </Panel>
  );
}
