import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

// The console's module boundaries, enforced.
//
// The frontend grew feature by feature, and each feature reached for what it
// needed wherever it found it: a colour map in `lib/`, a formatter in another
// feature's `lib/`, a badge component in a third feature's `components/`. The
// result was a graph where every feature knew about every other feature, and a
// change in one was a change in all.
//
// Two rules, and only two:
//
//   R1  a feature does not reach into another feature (`features/X` must not
//       import `features/Y`). A feature owns its own vocabulary; what two
//       features share belongs in `lib/` (the shared layer) or in
//       `components/` (the design system).
//
//   R2  the shared layer does not reach up into a feature: `components/` and
//       `lib/` must not import `features/`. A shared component that needs a
//       feature's helper needs it in `lib/`.
//
// The known debt is listed in ALLOWED below, and the list is EMPTY: the debt
// was paid in this slice. The ratchet keeps two teeth either way — an import
// that is NOT on the list fails here (new debt cannot land), and a listed
// import that no longer happens fails too (the debt was paid and the list
// was not updated — a stale allowance is a lie about the code). So the list
// is a debt ledger that must stay empty, not a place to park the next
// violation: a violation that needs an allowance gets fixed instead.

const SRC = new URL("./", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

/** Every source file under src/, tests included: the module graph is the graph. */
function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else if (/\.tsx?$/.test(entry) && !entry.endsWith(".d.ts")) out.push(p);
  }
  return out;
}

const FILES = walk(SRC);

/** The import specifiers of one file, in source order. */
function importsOf(src: string): string[] {
  const out: string[] = [];
  const re = /(?:\bfrom\s*|\bimport\s*|\bimport\(\s*|\brequire\(\s*)["']([^"']+)["']/g;
  for (const m of src.matchAll(re)) out.push(m[1]);
  return out;
}

/** Resolve an import specifier to a file under src/, or null when it is external. */
function resolveSpec(spec: string, fromFile: string): string | null {
  let base: string;
  if (spec.startsWith("@/")) base = join(SRC, spec.slice(2));
  else if (spec.startsWith(".")) base = resolve(dirname(fromFile), spec);
  else return null; // a package
  for (const candidate of [base, `${base}.ts`, `${base}.tsx`, join(base, "index.ts"), join(base, "index.tsx")]) {
    try {
      if (statSync(candidate).isFile()) return candidate;
    } catch {
      // not there; try the next spelling
    }
  }
  return null;
}

/** The feature a file belongs to, or null when it is not inside features/. */
function featureOf(file: string): string | null {
  const rel = file.slice(SRC.length).replace(/\\/g, "/");
  const m = /^features\/([^/]+)\//.exec(rel);
  return m ? m[1] : null;
}

/** The shared layer a file belongs to: "components", "lib" or null. */
function layerOf(file: string): string | null {
  const rel = file.slice(SRC.length).replace(/\\/g, "/");
  const m = /^(components|lib)\//.exec(rel);
  return m ? m[1] : null;
}

interface Violation {
  /** The importing file, relative to src/. */
  from: string;
  /** The imported file, relative to src/. */
  to: string;
  /** The rule the import breaks. */
  rule: "R1" | "R2";
}

/** Every boundary violation in the tree. */
function violations(): Violation[] {
  const out: Violation[] = [];
  for (const file of FILES) {
    const src = readFileSync(file, "utf8");
    const fromFeature = featureOf(file);
    const layer = layerOf(file);
    if (!fromFeature && !layer) continue;
    for (const spec of importsOf(src)) {
      const target = resolveSpec(spec, file);
      if (!target) continue;
      const toFeature = featureOf(target);
      if (!toFeature) continue;
      const rel = (f: string) => f.slice(SRC.length).replace(/\\/g, "/");
      if (fromFeature && toFeature && fromFeature !== toFeature) {
        out.push({ from: rel(file), to: rel(target), rule: "R1" });
      } else if (!fromFeature && layer) {
        out.push({ from: rel(file), to: rel(target), rule: "R2" });
      }
    }
  }
  return out;
}

/**
 * The known debt, one entry per (importer, imported) pair.
 *
 * Every entry here is a boundary the code crosses today. The ratchet below
 * fails when an entry stops happening — so the list shrinks, deliberately, as
 * the debt is paid. It is EMPTY today: the last 67 crossings (35 feature →
 * feature, 32 shared-layer → feature) were paid by moving the shared pieces
 * down into `lib/` and `components/`, and nothing new has been borrowed
 * since.
 */
const ALLOWED: string[] = [];

describe("frontend boundaries", () => {
  it("has files to check", () => {
    expect(FILES.length).toBeGreaterThan(200);
  });

  it("keeps a feature out of another feature, and the shared layer out of the features", () => {
    const found = violations().map((v) => `${v.rule} ${v.from} -> ${v.to}`);
    const allowed = new Set(ALLOWED);
    const unexpected = found.filter((v) => !allowed.has(v));
    expect(unexpected, "a new boundary crossing — move the shared piece into lib/ or components/").toEqual([]);
    const stale = ALLOWED.filter((a) => !found.includes(a));
    expect(stale, "the allowance lists a crossing that no longer happens — delete the line").toEqual([]);
  });
});
