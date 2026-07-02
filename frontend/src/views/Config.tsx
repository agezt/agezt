import { Brain, Cable, FolderOpen, Gauge, Network, Puzzle, RefreshCw, Route, Settings, ShieldCheck, type LucideIcon } from "lucide-react";
import { Stats, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";
import { Page } from "@/components/ui/page";
import { SkeletonList } from "@/components/ui/skeleton";
import { usePanel } from "@/lib/usePanel";
import { cn } from "@/lib/utils";
import { ErrorText } from "@/components/JsonView";

// CATEGORIES buckets the AGEZT_* settings (sans prefix) into labelled groups so
// the Config view reads like a settings panel instead of a flat chip cloud. Rules
// are ordered — the FIRST whose any-substring matches wins — so put the more
// specific groups (Security's ALLOW/EDICT) before broader ones (Tools' HTTP).
const CATEGORIES: { label: string; match: string[] }[] = [
  { label: "Provider & Model", match: ["PROVIDER", "MODEL", "OLLAMA", "CATALOG", "PRICING", "CONTEXT"] },
  { label: "Channels", match: ["TELEGRAM", "SLACK", "DISCORD", "MATRIX", "EMAIL", "SMS", "WHATSAPP", "TEAMS", "WEBHOOK", "HOMEASSISTANT", "CHANNEL"] },
  { label: "Interfaces", match: ["WEB_ADDR", "WEB_ALLOWED_HOSTS", "WEB_PASSWORD", "TUNNEL", "API_ADDR", "REST", "MULTITENANT", "PEERS", "MESH"] },
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
const SECTION_ICON: Record<string, LucideIcon> = {
  "Provider & Model": Brain,
  Channels: Cable,
  Interfaces: Network,
  "Autonomy & Learning": Gauge,
  "Security & Policy": ShieldCheck,
  "Tools & Plugins": Puzzle,
  Other: Settings,
};

function compactKeys(keys: string[]): string[] {
  return keys.map((k) => k.replace(/^AGEZT_/, "")).slice(0, 8);
}

function objectCount(value: unknown): number {
  return value && typeof value === "object" ? Object.keys(value as Record<string, unknown>).length : 0;
}

export function Config() {
  const { data, error, loading, reload } = usePanel<Record<string, any>>("/api/config");
  return (
    <Page
      icon={Settings}
      title="Config"
      description="Every AGEZT_* setting the daemon sees, grouped by area"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Refresh">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      <Card glass>
        <CardBody className="space-y-3">
          {error ? (
            <ErrorText>{error}</ErrorText>
          ) : !data ? (
            <SkeletonList count={3} lines={2} />
          ) : (
            <ConfigBody d={data} />
          )}
        </CardBody>
      </Card>
    </Page>
  );
}

function ConfigBody({ d }: { d: Record<string, any> }) {
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
              <div className="mt-2 grid gap-2 md:grid-cols-2 xl:grid-cols-3">
                {sections.map((sec) => {
                  const keys = buckets.get(sec)!;
                  const Icon = SECTION_ICON[sec] || Settings;
                  const shown = compactKeys(keys);
                  const hidden = Math.max(0, keys.length - shown.length);
                  return (
                    <section
                      key={sec}
                      className="rounded-lg border border-border bg-panel/45 p-2"
                      aria-label={`Config group: ${sec}`}
                    >
                      <div className="mb-2 flex items-center gap-2">
                        <span className="grid size-7 place-items-center rounded-md border border-accent/30 bg-accent/10 text-accent">
                          <Icon className="size-3.5" />
                        </span>
                        <span className="min-w-0 flex-1 truncate text-xs font-semibold uppercase tracking-normal text-foreground">
                          {sec}
                        </span>
                        <Badge variant="default">{keys.length}</Badge>
                      </div>
                      <div className="flex flex-wrap gap-1">
                        {shown.map((k) => (
                          <span
                            key={k}
                            className="rounded border border-border bg-panel px-1.5 py-0.5 font-mono text-xs text-foreground/80"
                            title={`AGEZT_${k}`}
                          >
                            {k}
                          </span>
                        ))}
                        {hidden > 0 && (
                          <span className="rounded border border-border bg-card px-1.5 py-0.5 font-mono text-xs text-muted">
                            +{hidden}
                          </span>
                        )}
                      </div>
                    </section>
                  );
                })}
              </div>
            </div>

            {d.paths && (
              <section className="rounded-lg border border-border bg-panel/45 p-2" aria-label="Config paths">
                <div className="mb-2 flex items-center gap-2">
                  <FolderOpen className="size-4 text-accent" />
                  <Count>paths · {objectCount(d.paths)} entries</Count>
                </div>
                <div className="grid gap-1 sm:grid-cols-2">
                  {Object.entries(d.paths as Record<string, unknown>).slice(0, 6).map(([k, v]) => (
                    <div key={k} className="min-w-0 rounded-md border border-border/70 bg-card/50 px-2 py-1">
                      <div className="truncate text-xs uppercase tracking-normal text-muted">{k}</div>
                      <div className="truncate font-mono text-xs text-foreground" title={String(v)}>{String(v)}</div>
                    </div>
                  ))}
                </div>
              </section>
            )}
            {d.routing && (
              <section className="rounded-lg border border-border bg-panel/45 p-2" aria-label="Config routing">
                <div className="mb-2 flex items-center gap-2">
                  <Route className="size-4 text-accent" />
                  <Count>routing · {objectCount(d.routing)} entries</Count>
                </div>
                <div className="flex flex-wrap gap-1">
                  {Object.keys(d.routing as Record<string, unknown>).slice(0, 10).map((k) => (
                    <span key={k} className="rounded border border-border bg-card px-1.5 py-0.5 font-mono text-xs text-foreground/80">
                      {k}
                    </span>
                  ))}
                </div>
              </section>
            )}
          </>
  );
}
