// Connections.tsx — re-exports the page + types from this package's

// helpers + sub-components. Day 23 god file split carved the original

// 1000+ satır file into types.ts + page.tsx + this re-export; the public

// surface is preserved so consumers (features/connections/index.ts, nav.tsx)

// don't change.

export {
  Connections,
  ConnectivityStrip,
} from "./page";
