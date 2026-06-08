import { useEffect, useState } from "react";
import { Boxes, RefreshCw } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { joinCatalog, levelTone, type CatalogTool, type CatalogRow, type ToolUsage } from "@/lib/catalog";

// Catalog is the agent's capability surface: every tool it can call, what the
// tool does, the Edict capability that governs it, the current trust level
// (granted/restricted at runtime via Policy), and how much it's been used. The
// "what can my agent do, and under what policy" view — fully observable.
export function Catalog() {
  const [rows, setRows] = useState<CatalogRow[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const [cat, edict, stats] = await Promise.all([
        getJSON<{ tools?: CatalogTool[] }>("/api/tools_catalog"),
        getJSON<{ levels?: Record<string, string> }>("/api/edict_show"),
        getJSON<{ by_tool?: Record<string, ToolUsage> }>("/api/tools"),
      ]);
      setRows(joinCatalog(cat.tools || [], edict.levels, stats.by_tool));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 10000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Boxes className="size-4 text-accent" /> Capability catalog
        </h2>
        <span className="text-xs text-muted">{rows ? `${rows.length} tools the agent can use` : ""}</span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !rows ? (
        <Muted>loading…</Muted>
      ) : rows.length === 0 ? (
        <Muted>no tools registered</Muted>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="grid grid-cols-1 gap-2 lg:grid-cols-2">
            {rows.map((r) => (
              <li key={r.name} className="rounded-lg border border-border bg-card p-3">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm font-semibold">{r.name}</span>
                  {r.level && (
                    <span
                      className={cn(
                        "rounded border px-1.5 py-0.5 text-[10px] font-semibold tabular-nums",
                        levelTone(r.level),
                      )}
                      title="current trust level (edit in Policy)"
                    >
                      {r.level}
                    </span>
                  )}
                  <span className="ml-auto text-[10px] tabular-nums text-muted">
                    {r.calls > 0 ? (
                      <>
                        {r.calls} call{r.calls === 1 ? "" : "s"}
                        {r.errors > 0 && <span className="text-bad"> · {r.errors} err</span>}
                      </>
                    ) : (
                      "unused"
                    )}
                  </span>
                </div>
                {r.capability && (
                  <div className="mt-1 text-[10px] text-muted">
                    capability <span className="font-mono text-foreground/80">{r.capability}</span>
                  </div>
                )}
                <p className="mt-1.5 text-xs leading-snug text-foreground/85">{r.description || "—"}</p>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
