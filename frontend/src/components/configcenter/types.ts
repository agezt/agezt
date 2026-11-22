// types.ts — configcenter type definitions (extracted from ConfigCenter.tsx). Day 23 god file split.
//
// Exported types/interfaces + the enum-like type aliases they reference
// (FieldType, ApplyMode) live here so the interface bodies don't depend on
// page.tsx internals. Internal-only types (Section, ReloadBoundary,
// ConfigSchemaResponse, SetResult, AgentConfigEntry, Category) stay in
// page.tsx. Public surface (page.tsx re-exports) unchanged.

export type FieldType = "text" | "password" | "number" | "bool" | "csv" | "select";
export type ApplyMode = "live" | "restart";

export type Field = {
  env: string;
  label: string;
  type: string;
  secret: boolean;
  required: boolean;
  help?: string;
  apply: string;
  options?: string[];
  read_only?: boolean;
  locked?: boolean;
};
export type ValueEntry = {
  env: string;
  secret: boolean;
  env_pinned: boolean;
  set: boolean;
  value?: string;
};
