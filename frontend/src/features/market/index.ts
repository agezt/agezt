// features/market — Day 15 thirteenth feature carve-out.
//
// Owns:
//   - Market page (Market.tsx, 929 satır — god file; carries the
//     pack marketplace UI; the business logic is mostly in
//     lib/market.ts)
//   - lib/market.ts (99 satır — the market stream + frame
//     decoder + pack details fetcher: streamMarket +
//     stepFromFrame + fetchPackDetails + MarketStep + PackDetails
//     + VetReport types)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/market and never needs to know about the sub-paths):
//   - components/Market.tsx — the page view
//   - types.ts — MarketStep, PackDetails, VetReport
//
// The 1 lib file (market) is the feature's domain business
// logic; external code reaches it via
// @/features/market/lib/market (allowed but not advertised).
//
// Cross-feature deps:
//   - @/app/api, @/app/events, @/app/utils, @/components/ui/*
//     (shared UI primitives — same pattern as the rest of the
//     features)
//
// The 929-line Market.tsx god file is the biggest single risk in
// this feature. A future Day-26+ split should pull the page
// renderer into components/Market/page.tsx and leave the lib as
// the pure stream/decoder surface. Day 15's commit only MOVES
// the file; the split is deferred so typecheck + test verification
// stays focused on the carve-out.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Market } from "./components/Market";
export * from "./types";
