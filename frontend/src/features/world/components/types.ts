// types.ts — world type definitions (extracted from World.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

import type {
  WorldListResponseEntitiesItem,
} from "@/app/api/types.generated";

export type WorldEntity = {
  id?: WorldListResponseEntitiesItem["id"];
  name: WorldListResponseEntitiesItem["name"];
  kind?: WorldListResponseEntitiesItem["kind"];
  aliases?: WorldListResponseEntitiesItem["aliases"];
  attrs?: WorldListResponseEntitiesItem["attrs"];
  weight?: WorldListResponseEntitiesItem["weight"];};
