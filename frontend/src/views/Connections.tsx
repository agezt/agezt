import { useEffect, useMemo, useState } from "react";
import { Network, Plug, Radio, Boxes, ArrowRight, RefreshCw, CheckCircle2, AlertTriangle, Circle } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { AnimatedNumber } from "@/components/AnimatedNumber";

// Connections — one cockpit for "what's actually wired up": AI providers
// (credentialed?), channels (live/configured?), and MCP servers (attached?).
// Read-only aggregation over the existing endpoints; each section links to the
// place you connect/manage it. Answers the "am I connected?" question at a glance.

interface Provider { id: string; name?: string; credentialed?: boolean }
interface ChannelRow { kind: string; display?: string; live?: boolean; configured?: boolean }
interface MCPServer { name: string; enabled?: boolean; attached?: boolean }
interface NodeRow { id?: string; name: string; local?: boolean; reachable?: boolean; url?: string; version?: string; status?: string }

function go(id: string) {
  return () => {
    location.hash = id;
  };
}

export function Connections() {
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [channels, setChannels] = useState<ChannelRow[] | null>(null);
  const [servers, setServers] = useState<MCPServer[] | null>(null);
  const [nodes, setNodes] = useState<NodeRow[] | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    setErr("");
    try {
      const [cat, ch, mcp, nodeRes] = await Promise.all([
        getJSON<{ providers?: Provider[] }>("/api/catalog").catch(() => ({ providers: [] })),
        getJSON<{ channels?: ChannelRow[] }>("/api/channels").catch(() => ({ channels: [] })),
        getJSON<{ servers?: MCPServer[] }>("/api/mcp").catch(() => ({ servers: [] })),
        getJSON<{ nodes?: NodeRow[] }>("/api/nodes").catch(() => ({ nodes: [] })),
      ]);
      setProviders(cat.providers || []);
      setChannels(ch.channels || []);
      setServers(mcp.servers || []);
      setNodes(nodeRes.nodes || []);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    load();
  }, []);

  const keyed = useMemo(() => (providers || []).filter((p) => p.credentialed), [providers]);
  const liveCh = useMemo(() => (channels || []).filter((c) => c.live), [channels]);
  const configuredCh = useMemo(() => (channels || []).filter((c) => c.configured && !c.live), [channels]);
  const attached = useMemo(() => (servers || []).filter((s) => s.attached), [servers]);
  const enabledOnly = useMemo(() => (servers || []).filter((s) => s.enabled && !s.attached), [servers]);
  const reachableNodes = useMemo(() => (nodes || []).filter((n) => n.reachable), [nodes]);
  const unreachableNodes = useMemo(() => (nodes || []).filter((n) => !n.reachable), [nodes]);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Network}
        title="Connections"
        actions={
          <Button variant="ghost" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
          </Button>
        }
      />

      {err && <p className="text-sm text-bad">{err}</p>}
      {loading && !providers ? (
        <SkeletonList count={3} />
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-4">
          <SectionCard
            icon={Plug}
            title="AI Providers"
            connected={keyed.length}
            total={(providers || []).length}
            connectedLabel="keyed"
            items={keyed.map((p) => ({ key: p.id, label: p.name || p.id, tone: "good" as const }))}
            emptyHint="No provider connected"
            actionLabel="Add provider"
            onAction={go("quickconnect")}
          />
          <SectionCard
            icon={Radio}
            title="Channels"
            connected={liveCh.length}
            total={(channels || []).length}
            connectedLabel="live"
            items={[
              ...liveCh.map((c) => ({ key: c.kind, label: c.display || c.kind, tone: "good" as const })),
              ...configuredCh.map((c) => ({ key: c.kind, label: `${c.display || c.kind} (restart to start)`, tone: "warn" as const })),
            ]}
            emptyHint="No channel live"
            actionLabel="Manage channels"
            onAction={go("channels")}
          />
          <SectionCard
            icon={Boxes}
            title="MCP Servers"
            connected={attached.length}
            total={(servers || []).length}
            connectedLabel="attached"
            items={[
              ...attached.map((s) => ({ key: s.name, label: s.name, tone: "good" as const })),
              ...enabledOnly.map((s) => ({ key: s.name, label: `${s.name} (enabled)`, tone: "warn" as const })),
            ]}
            emptyHint="No MCP server attached"
            actionLabel="Manage MCP"
            onAction={go("mcp")}
          />
          <SectionCard
            icon={Network}
            title="Nodes"
            connected={reachableNodes.length}
            total={(nodes || []).length}
            connectedLabel="reachable"
            items={[
              ...reachableNodes.map((n) => ({ key: n.id || n.name, label: `${n.name}${n.local ? " (local)" : ""}`, tone: "good" as const })),
              ...unreachableNodes.map((n) => ({ key: n.id || n.name, label: `${n.name} (${n.status || "unreachable"})`, tone: "warn" as const })),
            ]}
            emptyHint="No peer nodes configured"
            actionLabel="Manage nodes"
            onAction={go("config")}
          />
        </div>
      )}
    </div>
  );
}

// ConnectivityStrip is a compact one-line summary for the Cockpit: keyed
// providers · live channels · attached MCP, linking to the full Connections view.
export function ConnectivityStrip() {
  const [s, setS] = useState<{ providers: number; channels: number; mcp: number; nodes: number } | null>(null);
  useEffect(() => {
    Promise.all([
      getJSON<{ providers?: Provider[] }>("/api/catalog").catch(() => ({ providers: [] })),
      getJSON<{ channels?: ChannelRow[] }>("/api/channels").catch(() => ({ channels: [] })),
      getJSON<{ servers?: MCPServer[] }>("/api/mcp").catch(() => ({ servers: [] })),
      getJSON<{ nodes?: NodeRow[] }>("/api/nodes").catch(() => ({ nodes: [] })),
    ]).then(([cat, ch, mcp, nodeRes]) =>
      setS({
        providers: (cat.providers || []).filter((p) => p.credentialed).length,
        channels: (ch.channels || []).filter((c) => c.live).length,
        mcp: (mcp.servers || []).filter((m) => m.attached).length,
        nodes: (nodeRes.nodes || []).filter((n) => n.reachable).length,
      }),
    );
  }, []);
  if (!s) return null;
  return (
    <button
      onClick={go("connections")}
      className="flex w-full items-center gap-3 rounded-lg border border-border bg-panel/45 p-2.5 text-left text-xs transition-colors hover:border-accent/40"
      title="Open the Connections cockpit"
    >
      <Network className="size-3.5 text-accent" />
      <span className="font-semibold uppercase tracking-normal text-accent">Connections</span>
      <span className="text-muted">
        <AnimatedNumber value={s.providers} className="font-semibold text-fg" /> provider{s.providers === 1 ? "" : "s"} keyed ·{" "}
        <AnimatedNumber value={s.channels} className="font-semibold text-fg" /> channel{s.channels === 1 ? "" : "s"} live ·{" "}
        <AnimatedNumber value={s.mcp} className="font-semibold text-fg" /> MCP attached ·{" "}
        <AnimatedNumber value={s.nodes} className="font-semibold text-fg" /> node{s.nodes === 1 ? "" : "s"} reachable
      </span>
      <ArrowRight className="ml-auto size-3.5 text-muted" />
    </button>
  );
}

function SectionCard({
  icon: Icon,
  title,
  connected,
  total,
  connectedLabel,
  items,
  emptyHint,
  actionLabel,
  onAction,
}: {
  icon: typeof Plug;
  title: string;
  connected: number;
  total: number;
  connectedLabel: string;
  items: { key: string; label: string; tone: "good" | "warn" | "muted" }[];
  emptyHint: string;
  actionLabel: string;
  onAction: () => void;
}) {
  return (
    <Card glass className="flex flex-col gap-3 p-4">
      <div className="flex items-center gap-2">
        <Icon className="size-4 text-accent" />
        <span className="text-sm font-semibold">{title}</span>
        <span className="ml-auto text-[11px] text-muted">
          <AnimatedNumber value={connected} className="font-semibold text-fg" /> {connectedLabel}
          {total > 0 && <span className="text-muted"> / {total}</span>}
        </span>
      </div>
      <div className="flex min-h-16 flex-col gap-1">
        {items.length === 0 ? (
          <p className="text-[11px] text-muted">{emptyHint}</p>
        ) : (
          items.slice(0, 8).map((it) => (
            <div key={it.key + it.label} className="flex items-center gap-2 text-xs">
              {it.tone === "good" && <Badge variant="good"><CheckCircle2 className="size-3" /></Badge>}
              {it.tone === "warn" && <Badge variant="warn"><AlertTriangle className="size-3" /></Badge>}
              {it.tone === "muted" && <Badge variant="default"><Circle className="size-3" /></Badge>}
              <span className="truncate">{it.label}</span>
            </div>
          ))
        )}
        {items.length > 8 && <span className="text-xs text-muted">+{items.length - 8} more</span>}
      </div>
      <Button variant="ghost" size="sm" onClick={onAction} className="mt-auto justify-start">
        {actionLabel} <ArrowRight className="size-3.5" />
      </Button>
    </Card>
  );
}
