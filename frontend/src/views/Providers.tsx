import { Panel, Stats, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";
import { LogDetail } from "@/components/LogDetail";
import { byDescValue, pct } from "@/lib/format";
import { fmtTime } from "@/lib/utils";

export function Providers() {
  return (
    <Panel<Record<string, any>> title="Providers" path="/api/providers">
      {(d) => {
        const byPrimary: Record<string, number> = d.by_primary || {};
        const names = byDescValue(byPrimary);
        const fbBy: Record<string, number> = d.fallbacks_by_primary || {};
        const fbNames = byDescValue(fbBy);
        return (
          <>
            <Stats
              pairs={[
                ["routed", d.routed || 0],
                ["fallbacks", d.fallbacks || 0],
                ["fallback rate", pct(d.fallback_rate, d.routed)],
              ]}
            />
            <Count>{names.length} provider(s) serving</Count>
            {names.length ? (
              names.map((n) => (
                <Row key={n}>
                  <Badge variant="accent">{byPrimary[n]}</Badge>
                  <span>{n}</span>
                </Row>
              ))
            ) : (
              <Muted>no routing decisions yet</Muted>
            )}
            {fbNames.length > 0 && (
              <div>
                <Count>fallbacks by provider</Count>
                {fbNames.map((n) => (
                  <Row key={n}>
                    <Badge variant="bad">{fbBy[n]}</Badge>
                    <span>{n}</span>
                  </Row>
                ))}
              </div>
            )}
            <LogDetail
              label="routing log"
              path="/api/provider_log"
              params={{ limit: "40" }}
              extract={(x) => x.events || []}
              render={(ev: any, i) => (
                <div key={i} className="flex gap-2 border-b border-border/40 py-0.5">
                  <span className="text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                  {ev.kind === "fallback" ? (
                    <span className="text-bad">
                      fallback {ev.failed || "?"} → {ev.next || "?"} {ev.reason ? `· ${ev.reason}` : ""}
                    </span>
                  ) : (
                    <span className="text-accent">
                      route → {ev.primary || "?"} {ev.task_type ? `· ${ev.task_type}` : ""}
                    </span>
                  )}
                </div>
              )}
            />
          </>
        );
      }}
    </Panel>
  );
}
