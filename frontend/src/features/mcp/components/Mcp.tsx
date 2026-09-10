// Mcp.tsx — re-exports the page + types from this package's

// helpers + sub-components. Day 23 god file split carved the original

// 1000+ satır file into types.ts + page.tsx + this re-export; the public

// surface is preserved so consumers (features/mcp/index.ts, nav.tsx)

// don't change.

export {
  CATEGORY_LABELS,
  CATALOG,
  serverNameOk,
  splitArgs,
  splitTools,
  parseEnv,
  parseHeaders,
  urlOk,
  transportOf,
  filterCatalog,
  NewServerForm,
  Mcp,
} from "./page";
export type {
  MCPServer,
  CatalogCategory,
  CatalogEntry,
} from "./types";
