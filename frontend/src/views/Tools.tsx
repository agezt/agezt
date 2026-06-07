import { Panel, Stats, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Muted } from "@/components/JsonView";
import { LogDetail } from "@/components/LogDetail";
import { byDescValue, pct } from "@/lib/format";
import { clip, fmtTime } from "@/lib/utils";

export function Tools() {
  return (
    <Panel<Record<string, any>> title="Tools" path="/api/tools">
      {(d) => {
        const byTool: Record<string, any> = d.by_tool || {};
        const names = Object.keys(byTool).sort(
          (a, b) => (byTool[b].calls || 0) - (byTool[a].calls || 0),
        );
        return (
          <>
            <Stats
              pairs={[
                ["calls", d.total || 0],
                ["errored", d.errored || 0],
                ["error rate", pct(d.error_rate, d.total)],
              ]}
            />
            <Count>{d.tools || names.length} tool(s) used</Count>
            {names.length ? (
              names.map((n) => {
                const t = byTool[n] || {};
                return (
                  <Row key={n}>
                    <Badge variant="accent">{t.calls || 0}</Badge>
                    <span>{n}</span>
                    {t.errors ? <span className="text-bad">{t.errors} err</span> : null}
                    {t.avg_ms != null ? <span className="ml-auto text-muted">{t.avg_ms}ms</span> : null}
                  </Row>
                );
              })
            ) : (
              <Muted>no tool calls yet</Muted>
            )}
            <LogDetail
              label="invocation log"
              path="/api/tool_log"
              params={{ limit: "40" }}
              extract={(x) => x.invocations || []}
              render={(ev: any, i) => (
                <div key={i} className="flex gap-2 border-b border-border/40 py-0.5">
                  <span className="text-muted">{fmtTime(ev.ts_unix_ms)}</span>
                  <span className={ev.error ? "text-bad" : "text-accent"}>
                    {ev.tool || "?"} {ev.error ? "✗" : "✓"}
                    {ev.duration_ms ? ` (${ev.duration_ms}ms)` : ""}
                  </span>
                  <span className="truncate text-muted">
                    {[ev.input, ev.output].filter(Boolean).map((s: any) => clip(s, 60)).join(" → ")}
                  </span>
                </div>
              )}
            />
          </>
        );
      }}
    </Panel>
  );
}
