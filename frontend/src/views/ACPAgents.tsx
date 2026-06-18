import { useEffect, useMemo, useState } from "react";
import {
  Plug, RefreshCw, Cpu, PackageCheck, PackageX, Boxes, ExternalLink, Star, Copy,
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty";
import { SkeletonList } from "@/components/ui/skeleton";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import { acpCensus, acpUsageHint, type ACPInventory, type ACPAgentStatus } from "@/lib/acp";

// ACP Agents — discovery of the Agent Client Protocol coding agents installed on
// this host (Gemini CLI, Claude Code's adapter, Codex, …). AGEZT can delegate a
// task to any installed one via the acp_agent tool. Detection runs in the Go
// backend (kernel/acpcatalog); this view renders what's available and how to use
// it. Read-only.
export function ACPAgents() {
  const ui = useUI();
  const [inv, setInv] = useState<ACPInventory | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<ACPInventory>("/api/acp/agents");
      setInv(d);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
  }, []);

  const agents = inv?.agents || [];
  const cen = useMemo(() => acpCensus(inv), [inv]);

  return (
    <div className="space-y-3">
      <PageHeader
        icon={Plug}
        title="ACP Agents"
        description="External coding agents that speak the Agent Client Protocol. Delegate work to any installed one via the acp_agent tool."
        actions={
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Re-scan host">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        }
      />

      {inv && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1 text-[11px] text-muted">
            <Cpu className="size-3" /> {inv.os}
          </span>
          {inv.active_command ? (
            <span className="inline-flex items-center gap-1 text-[11px] text-muted">
              default:{" "}
              <code className="rounded bg-card px-1.5 py-0.5 font-mono text-[10px] text-accent">{inv.active_command}</code>
            </span>
          ) : (
            <span className="text-[11px] text-muted">
              no default configured (set AGEZT_ACP_AGENT_CMD, or pick an installed agent per call)
            </span>
          )}
        </div>
      )}

      {inv && (
        <div className="grid grid-cols-3 gap-2">
          <BigStat icon={Boxes} label="catalog" value={cen.total} />
          <BigStat icon={PackageCheck} label="installed" value={cen.installed} accent={cen.installed > 0} />
          <BigStat icon={PackageX} label="missing" value={cen.missing} />
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !inv ? (
        <SkeletonList count={3} lines={2} />
      ) : agents.length === 0 ? (
        <EmptyState icon={Plug} title="No ACP agents in the catalog" hint="The catalog is empty." />
      ) : (
        <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
          {agents.map((a) => (
            <AgentCard key={a.slug} a={a} onCopy={(text, what) => {
              void navigator.clipboard?.writeText(text);
              ui.toast(`${what} copied`, "success");
            }} />
          ))}
        </div>
      )}

      <p className="text-[11px] text-muted">
        Any ACP-speaking binary works even if it's not in this catalog — set{" "}
        <code className="font-mono">AGEZT_ACP_AGENT_CMD</code> to its launch command. Installed catalog agents can be
        selected per call with <code className="font-mono">acp_agent agent="&lt;slug&gt;"</code>.
      </p>
    </div>
  );
}

function AgentCard({ a, onCopy }: { a: ACPAgentStatus; onCopy: (text: string, what: string) => void }) {
  return (
    <div className={cn("glass flex flex-col gap-2 rounded-xl p-3", !a.installed && "border-border/60")}>
      <div className="flex items-center gap-2">
        <span className="text-sm font-semibold">{a.name}</span>
        {a.active && (
          <span title="Configured default ACP agent" className="inline-flex items-center gap-0.5 text-accent2">
            <Star className="size-3.5 fill-current" />
          </span>
        )}
        {a.installed ? <Badge variant="good">installed</Badge> : <Badge variant="default" className="text-muted">missing</Badge>}
        <span className="ml-auto font-mono text-[10px] text-muted">{a.slug}</span>
      </div>

      {a.description && <div className="text-[11px] text-muted">{a.description}</div>}

      {a.installed && a.version && (
        <div className="truncate font-mono text-[10px] text-foreground/70" title={a.path}>{a.version}</div>
      )}

      {/* Launch command (installed) or install hint (missing) */}
      {a.installed ? (
        <button
          onClick={() => onCopy(a.command, "command")}
          title="Copy launch command"
          className="group flex items-center gap-1 truncate rounded bg-card px-1.5 py-1 text-left font-mono text-[10px] text-muted hover:text-accent"
        >
          <Copy className="size-3 shrink-0 opacity-0 group-hover:opacity-100" />
          <span className="truncate">$ {a.command}</span>
        </button>
      ) : a.install ? (
        <button
          onClick={() => onCopy(a.install!, "install command")}
          title="Copy install command"
          className="group flex items-center gap-1 truncate rounded bg-card px-1.5 py-1 text-left font-mono text-[10px] text-muted hover:text-accent"
        >
          <Copy className="size-3 shrink-0 opacity-0 group-hover:opacity-100" />
          <span className="truncate">{a.install}</span>
        </button>
      ) : null}

      <div className="mt-auto flex items-center gap-2 pt-1 text-[10px] text-muted">
        <span className="truncate">{acpUsageHint(a)}</span>
        {a.docs && (
          <a
            href={a.docs}
            target="_blank"
            rel="noreferrer"
            className="ml-auto inline-flex items-center gap-0.5 shrink-0 hover:text-accent"
            title="Open docs"
          >
            docs <ExternalLink className="size-3" />
          </a>
        )}
      </div>
    </div>
  );
}

function BigStat({ icon: Icon, label, value, accent }: { icon: typeof Boxes; label: string; value: number | string; accent?: boolean }) {
  return (
    <div className={cn("rounded-xl p-2.5", accent ? "border border-accent/50 bg-card" : "glass")}>
      <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
        <Icon className={cn("size-3", accent && "text-accent")} /> {label}
      </div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}
