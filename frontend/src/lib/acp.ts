// ACP (Agent Client Protocol) agent discovery — types + helpers for the ACP
// Agents view. The backend (kernel/acpcatalog) detects which ACP coding agents
// are installed on the host; the shapes below are its own rows, narrowed to the
// fields the view reads.
import type { AcpAgentsResponse, AcpAgentsResponseAgentsItem } from "@/app/api/types.generated";

export type ACPAgentStatus = {
  slug: AcpAgentsResponseAgentsItem["slug"];
  name: AcpAgentsResponseAgentsItem["name"];
  bin: AcpAgentsResponseAgentsItem["bin"];
  command: AcpAgentsResponseAgentsItem["command"];
  description: AcpAgentsResponseAgentsItem["description"];
  install?: AcpAgentsResponseAgentsItem["install"];
  docs?: AcpAgentsResponseAgentsItem["docs"];
  installed: AcpAgentsResponseAgentsItem["installed"];
  version?: AcpAgentsResponseAgentsItem["version"];
  path?: AcpAgentsResponseAgentsItem["path"];
  active: AcpAgentsResponseAgentsItem["active"];
};

export type ACPInventory = {
  os: AcpAgentsResponse["os"];
  active_command?: AcpAgentsResponse["active_command"];
  agents: AcpAgentsResponseAgentsItem[];
  installed_count: AcpAgentsResponse["installed_count"];
  missing_count: AcpAgentsResponse["missing_count"];
};

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
