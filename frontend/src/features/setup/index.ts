// features/setup — Day 15 fourteenth feature carve-out.
//
// Owns:
//   - Setup page (Setup.tsx, 952 satır — god file; carries the
//     first-run setup wizard UI; the business logic + types
//     are mostly in lib/setup.ts)
//   - lib/setup.ts (137 satır — the setup catalog + provider
//     ranking + fallback helpers: SetupModel + SetupProvider +
//     SetupCatalog + SetupFallbackCandidate interfaces,
//     providerKeyEnv + anyCredentialed + rankProviders +
//     setupModelChain + setupFallbackCandidates +
//     defaultSetupFallbacks + uniqueSetupChainName +
//     setupTaskSelection + mergeSetupTaskRouting functions)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/setup and never needs to know about the sub-paths):
//   - components/Setup.tsx — the page view (also re-exported from
//     views/Wizards.tsx as the sub-component for the create-wizard)
//   - 9 functions from lib/setup re-exported (anyCredentialed,
//     defaultSetupFallbacks, mergeSetupTaskRouting, providerKeyEnv,
//     rankProviders, setupFallbackCandidates, setupModelChain,
//     setupTaskSelection, uniqueSetupChainName) — so callers can
//     import from the feature surface instead of lib/
//   - 4 types from lib/setup re-exported (SetupModel,
//     SetupProvider, SetupCatalog, SetupFallbackCandidate)
//
// The 1 lib file (setup) is the feature's domain business
// logic; external code reaches it via
// @/features/setup/lib/setup (allowed but not advertised).
// App.tsx is a known cross-feature consumer — it imports
// anyCredentialed + SetupCatalog from this lib for the top-level
// onboarding gate.
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 952-line Setup.tsx god file is the biggest single risk in
// this feature. A future Day-26+ split should pull the wizard
// state machine into components/Setup/page.tsx and leave the lib
// as the pure catalog/ranking surface. Day 15's commit only MOVES
// the file; the split is deferred so typecheck + test verification
// stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Setup } from "./components/Setup";
export {
  anyCredentialed,
  defaultSetupFallbacks,
  mergeSetupTaskRouting,
  providerKeyEnv,
  rankProviders,
  setupFallbackCandidates,
  setupModelChain,
  setupTaskSelection,
  uniqueSetupChainName,
} from "./components/Setup";
export * from "./types";
