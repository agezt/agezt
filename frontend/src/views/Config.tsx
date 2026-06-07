import { Panel, Stats, Count } from "@/components/Panel";
import { KeyValue, JsonView } from "@/components/JsonView";

export function Config() {
  return (
    <Panel<Record<string, any>> title="Config" path="/api/config">
      {(d) => {
        const env = Object.keys(d.env || {})
          .filter((k) => d.env[k])
          .sort();
        return (
          <>
            <Stats
              pairs={[
                ["model", d.model || "—"],
                ["system prompt", d.system_prompt_set ? "set" : "—"],
                ["tools", d.tool_count ?? "—"],
                ["plugins", d.plugin_count ?? "—"],
                ["ask policy", d.ask_policy || "—"],
              ]}
            />
            <div>
              <Count>{env.length} env var(s) set</Count>
              <div className="mt-1 flex flex-wrap gap-1">
                {env.map((k) => (
                  <span key={k} className="rounded border border-border bg-panel px-1.5 py-0.5 text-[10px]">
                    {k}
                  </span>
                ))}
              </div>
            </div>
            {d.paths && (
              <div>
                <Count>paths</Count>
                <KeyValue pairs={Object.entries(d.paths).map(([k, v]) => [k, String(v)])} />
              </div>
            )}
            {d.routing && <JsonView value={d.routing} />}
          </>
        );
      }}
    </Panel>
  );
}
