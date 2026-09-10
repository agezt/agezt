// types.ts — mcp type definitions (extracted from Mcp.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

export interface MCPServer {
  id: string;
  name: string;
  command?: string;
  args?: string[];
  // url + transport: remote (Streamable HTTP) servers (M904). transport is
  // "stdio" | "http"; url is set only for http.
  url?: string;
  transport?: string;
  enabled?: boolean;
  description?: string;
  attached?: boolean;
  tool_count?: number;
  // env values are redacted by the backend; only the key names come back (M898).
  env_keys?: string[];
  // header values are redacted too; only the names come back for remote servers (M904).
  header_keys?: string[];
  // tool_allow: optional per-server allowlist of tool names to expose (M899).
  tool_allow?: string[];
  // lazy: collapse this server's tools into one mcp_<name> dispatcher (M906).
  lazy?: boolean;
}
export type CatalogCategory = "core" | "web" | "data" | "dev" | "apps";
export interface CatalogEntry {
  name: string;
  category: CatalogCategory;
  // command/args: a stdio preset. url/headers: a remote (http) preset (M904).
  // Exactly one shape is set per entry.
  command?: string;
  args?: string;
  url?: string;
  description: string;
  needs?: string;
  // env: names of environment variables this server needs — prefilled (with
  // blank values) into the register form's env field for the operator to fill.
  env?: string[];
  // headers: names of HTTP headers a remote preset needs — prefilled (with blank
  // values, "Name: ") into the register form's headers field.
  headers?: string[];
}
