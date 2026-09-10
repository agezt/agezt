// types.ts — world type definitions (extracted from World.tsx). Day 23 god file split.
//
// Only exported type/interface declarations live here. Internal types
// stay in page.tsx because they aren't consumed outside the
// package's barrel. Public surface (page.tsx re-exports) unchanged.

export interface WorldEntity {
  id?: string;
  name: string;
  kind?: string;
  aliases?: string[];
  attrs?: Record<string, string>;
  weight?: number;
}
