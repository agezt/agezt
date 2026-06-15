import { Settings } from "lucide-react";
import { Panel, Stats, Count } from "@/components/Panel";
import { KeyValue, JsonView } from "@/components/JsonView";

// CATEGORIES buckets the AGEZT_* settings (sans prefix) into labelled groups so
// the Config view reads like a settings panel instead of a flat chip cloud. Rules
// are ordered — the FIRST whose any-substring matches wins — so put the more
// specific groups (Security's ALLOW/EDICT) before broader ones (Tools' HTTP).
const CATEGORIES: { label: string; match: string[] }[] = [
  { label: "Provider & Model", match: ["PROVIDER", "MODEL", "OLLAMA", "CATALOG", "PRICING", "CONTEXT"] },
  { label: "Channels", match: ["TELEGRAM", "SLACK", "DISCORD", "MATRIX", "EMAIL", "SMS", "WHATSAPP", "TEAMS", "WEBHOOK", "HOMEASSISTANT", "CHANNEL"] },
  { label: "Interfaces", match: ["WEB_ADDR", "API_ADDR", "REST", "MULTITENANT", "PEERS", "MESH"] },
  { label: "Autonomy & Learning", match: ["SCHEDULE", "STANDING", "PULSE", "FORGE", "MEMORY", "REFLECT", "BRIEF", "WORLD", "SKILL"] },
  { label: "Security & Policy", match: ["ALLOW_ALL", "EDICT", "APPROVAL", "ANOMALY", "HTTP_ALLOW", "BROWSER_ALLOW", "AWS", "FORCE_START"] },
  { label: "Tools & Plugins", match: ["CODING", "ACP", "PLUGIN", "BROWSER", "HTTP", "TOOL", "ENV_INJECT", "ARTIFACT"] },
];

// categorize returns the section label for a set env key (AGEZT_ stripped).
function categorize(key: string): string {
  const k = key.replace(/^AGEZT_/, "");
  for (const c of CATEGORIES) {
    if (c.match.some((m) => k.includes(m))) return c.label;
  }
  return "Other";
}

const SECTION_ORDER = [...CATEGORIES.map((c) => c.label), "Other"];

export function Config() {
  return (
    <Panel<Record<string, any>> title="Config" icon={Settings} description="Every AGEZT_* setting the daemon sees, grouped by area" path="/api/config">
      {(d) => {
        const setKeys = Object.keys(d.env || {})
          .filter((k) => d.env[k])
          .sort();
        // Bucket the set keys into ordered sections.
        const buckets = new Map<string, string[]>();
        for (const k of setKeys) {
          const sec = categorize(k);
          const arr = buckets.get(sec) || [];
          arr.push(k);
          buckets.set(sec, arr);
        }
        const sections = SECTION_ORDER.filter((s) => buckets.has(s));
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
              <Count>
                {setKeys.length} setting{setKeys.length === 1 ? "" : "s"} configured · {sections.length} group
                {sections.length === 1 ? "" : "s"}
              </Count>
              <div className="mt-2 space-y-3">
                {sections.map((sec) => {
                  const keys = buckets.get(sec)!;
                  return (
                    <div key={sec} className="glass rounded-xl p-3">
                      <div className="mb-1.5 flex items-center gap-2">
                        <h3 className="text-xs font-semibold uppercase tracking-wider text-accent">{sec}</h3>
                        <span className="text-[10px] text-muted">{keys.length}</span>
                      </div>
                      <div className="flex flex-wrap gap-1">
                        {keys.map((k) => (
                          <span
                            key={k}
                            className="rounded border border-border bg-panel px-1.5 py-0.5 font-mono text-[10px] text-foreground/80"
                            title={k}
                          >
                            {k.replace(/^AGEZT_/, "")}
                          </span>
                        ))}
                      </div>
                    </div>
                  );
                })}
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
