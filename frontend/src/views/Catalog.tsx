import { useEffect, useState } from "react";
import { Boxes, RefreshCw, ShieldCheck } from "lucide-react";
import { EmptyState } from "@/components/ui/empty";
import { getJSON, postAction } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { Page } from "@/components/ui/page";
import { ErrorText } from "@/components/JsonView";
import { Badge } from "@/components/ui/badge";
import { joinCatalog, levelTone, type CatalogTool, type CatalogRow, type ToolUsage } from "@/lib/catalog";

// The edict trust ladder (L0 deny … L4 allow). Mirrors the Policy view so a
// tool's permission can be granted/restricted from the catalog directly.
const LEVELS = ["L0", "L1", "L2", "L3", "L4"];

// Catalog is the agent's capability surface: every tool it can call, what the
// tool does, the Edict capability that governs it, the current trust level
// (granted/restricted at runtime via Policy), and how much it's been used. The
// "what can my agent do, and under what policy" view — fully observable.
export function Catalog() {
  const ui = useUI();
  const [rows, setRows] = useState<CatalogRow[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [levelMenu, setLevelMenu] = useState<string | null>(null);

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

  // Grant/restrict a capability's trust level (M641) — the same control as the
  // Policy view, but right where you see what the tool does. Refreshes after.
  async function setLevel(capability: string, level: string) {
    setBusy(capability);
    try {
      await postAction("/api/edict/set_level", { capability, level });
      ui.toast(`${capability} → ${level}`, "success");
      await reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <Page
      icon={Boxes}
      title="Capability catalog"
      width="wide"
      actions={
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !rows ? (
        <SkeletonGrid count={6} lines={1} />
      ) : rows.length === 0 ? (
        <EmptyState icon={Boxes} title="No tools registered" hint="No capabilities are wired into this agent's runtime yet." />
      ) : (
        <ul className="grid grid-cols-1 gap-2 lg:grid-cols-2">
            {rows.map((r) => (
              <li key={r.name} className="glass rounded-xl p-3">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm font-semibold">{r.name}</span>
                  {r.capability && (
                    <button
                      type="button"
                      disabled={busy === r.capability}
                      onClick={() => setLevelMenu((cur) => (cur === r.capability ? null : r.capability))}
                      aria-expanded={levelMenu === r.capability}
                      aria-label={`Trust level for ${r.capability}`}
                      title="trust level — grant (L4) or restrict (L0) this capability"
                      className={cn(
                        "inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-xs font-semibold tabular-nums transition-colors disabled:opacity-50",
                        levelTone(r.level),
                      )}
                    >
                      <ShieldCheck className="size-3" />
                      {r.level || "set"}
                    </button>
                  )}
                  <span className="ml-auto text-xs tabular-nums text-muted">
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
                  <div className="mt-1 text-xs text-muted">
                    capability <span className="font-mono text-foreground/80">{r.capability}</span>
                  </div>
                )}
                {r.capability && levelMenu === r.capability && (
                  <div className="mt-2 flex flex-wrap items-center gap-1.5" role="group" aria-label={`Set trust level for ${r.capability}`}>
                    {LEVELS.map((level) => (
                      <button
                        key={level}
                        type="button"
                        aria-pressed={r.level === level}
                        disabled={busy === r.capability}
                        onClick={() => {
                          setLevelMenu(null);
                          void setLevel(r.capability, level);
                        }}
                        className={cn(
                          "inline-flex h-7 items-center gap-1 rounded-md border px-2 text-xs font-semibold transition-colors",
                          r.level === level
                            ? "border-accent bg-accent/15 text-accent"
                            : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
                        )}
                      >
                        {level}
                      </button>
                    ))}
                    <Badge variant="default">policy</Badge>
                  </div>
                )}
                <p className="mt-1.5 text-xs leading-snug text-foreground/85">{r.description || "—"}</p>
              </li>
            ))}
        </ul>
      )}
    </Page>
  );
}
