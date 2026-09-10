// Setup.tsx — re-exports the page + types from this package's

// helpers + sub-components. Day 23 god file split carved the original

// 1000+ satır file into types.ts + page.tsx + this re-export; the public

// surface is preserved so consumers (features/setup/index.ts, nav.tsx)

// don't change.

export {
  Setup,
} from "./page";
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
  type SetupCatalog,
  type SetupFallbackCandidate,
  type SetupModel,
  type SetupProvider,
} from "../lib/setup";
