// ConfigCenter.tsx — re-exports the page + types from this package's

// helpers + sub-components. Day 23 god file split carved the original

// 1000+ satır file into types.ts + page.tsx + this re-export; the public

// surface is preserved so consumers (features/configcenter/index.ts, nav.tsx)

// don't change.

export {
  reloadBoundariesFromSections,
  summarizeReloadBoundaries,
  ConfigCenter,
  agentConfigScopeLabel,
  summarizeAgentConfigEntries,
  FieldRow,
} from "./page";
export type {
  Field,
  ValueEntry,
  FieldType,
  ApplyMode,
} from "./types";
