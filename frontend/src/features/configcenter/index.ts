// features/configcenter — Day 14 eleventh feature carve-out.
//
// Owns:
//   - ConfigCenter page (ConfigCenter.tsx, 1061 satır — god file;
//     carries the config center browser UI, Field + ValueEntry
//     types, reloadBoundariesFromSections + summarizeReloadBoundaries
//     helpers, agentConfigScopeLabel + summarizeAgentConfigEntries
//     formatters, FieldRow sub-component)
//   - lib/configbackup.ts (75 satır — the config backup bundle:
//     ConfigBundle interface, parseConfigBundle + fetchConfigBundle
//     + applyConfigBundle functions; the snapshot + restore
//     pipeline)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/configcenter and never needs to know about the
// sub-paths):
//   - components/ConfigCenter.tsx — the page view
//   - FieldRow + Field + ValueEntry (re-exported) — FieldRow is
//     used by features/voice/components/VoiceSetup.tsx (the
//     voice-setup form), Field + ValueEntry are used in the
//     same VoiceSetup surface
//   - reloadBoundariesFromSections + summarizeReloadBoundaries +
//     agentConfigScopeLabel + summarizeAgentConfigEntries
//     (re-exported) — the config center helpers
//   - types.ts — Field, ValueEntry, ConfigBundle
//
// The 1 lib file (configbackup) is the feature's domain
// business logic; external code reaches it via
// @/features/configcenter/lib/configbackup (allowed but not
// advertised).
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//   - @/components/ConfigInventory — shared cross-feature component
//     that ConfigCenter imports for the inventory panel; stays
//     in components/ (8+ users across features)
//   - lib/snapshot.ts — applyConfigBundle + parseConfigBundle
//     consumer for snapshot generation
//   - views/Backup.tsx — parseConfigBundle + fetchConfigBundle +
//     applyConfigBundle consumer for the backup flow
//   - App.tsx — same 3 imports for the top-level config restore
//     button
//   - features/voice/components/VoiceSetup.tsx — FieldRow + Field +
//     ValueEntry consumer for the voice setup form
//
// The 1061-line ConfigCenter.tsx god file is the biggest single
// risk in this feature. A future Day-26+ split should separate:
//   - components/ConfigCenter/page.tsx (the ConfigCenter() default)
//   - components/ConfigCenter/rows.tsx (FieldRow + the row
//     sub-components)
//   - components/ConfigCenter/reload.ts (reloadBoundariesFromSections
//     + summarizeReloadBoundaries)
//   - components/ConfigCenter/scope.ts (agentConfigScopeLabel +
//     summarizeAgentConfigEntries)
//   - components/ConfigCenter.tsx (re-export the page)
// Day 14's commit only MOVES the file; the split is deferred so
// typecheck + test verification stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { ConfigCenter } from "./components/ConfigCenter";
export {
  FieldRow,
  reloadBoundariesFromSections,
  summarizeReloadBoundaries,
  agentConfigScopeLabel,
  summarizeAgentConfigEntries,
} from "./components/ConfigCenter";
export * from "./types";
