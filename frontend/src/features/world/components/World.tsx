// World.tsx — re-exports the page + types from this package's

// helpers + sub-components. Day 23 god file split carved the original

// 1000+ satır file into types.ts + page.tsx + this re-export; the public

// surface is preserved so consumers (features/world/index.ts, nav.tsx)

// don't change.

export {
  kindBreakdown,
  filterEntities,
  parseWorldJSON,
  entityMatches,
  World,
  WorldEditForm,
  WorldAddForm,
  WorldRelateForm,
} from "./page";
export type {
  WorldEntity,
} from "./types";
