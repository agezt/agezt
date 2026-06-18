// ACP (Agent Client Protocol) agent discovery — types + helpers for the ACP
// Agents view. The backend (kernel/acpcatalog) detects which ACP coding agents
// are installed on the host; this mirrors its wire shape.

export interface ACPAgentStatus {
  slug: string;
  name: string;
  bin: string;
  command: string;
  description: string;
  install?: string;
  docs?: string;
  installed: boolean;
  version?: string;
  path?: string;
  active: boolean;
}

export interface ACPInventory {
  os: string;
  active_command?: string;
  agents: ACPAgentStatus[];
  installed_count: number;
  missing_count: number;
}

export interface ACPCensus {
  total: number;
  installed: number;
  missing: number;
}

export function acpCensus(inv: ACPInventory | null): ACPCensus {
  const agents = inv?.agents ?? [];
  const installed = agents.filter((a) => a.installed).length;
  return { total: agents.length, installed, missing: agents.length - installed };
}

// acpUsageHint returns the one-line "how do I use this" string for an agent:
// installed agents show how a run/tool delegates to them; missing ones show how
// to install. Pure + tested.
export function acpUsageHint(a: ACPAgentStatus): string {
  if (!a.installed) {
    return a.install ? `Install: ${a.install}` : "Not installed on this host.";
  }
  if (a.active) {
    return "Default ACP agent — the acp_agent tool uses it automatically.";
  }
  return `Use it: acp_agent with agent="${a.slug}".`;
}
