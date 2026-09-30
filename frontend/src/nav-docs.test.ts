import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { NAV_GROUPS } from "@/nav";

// ---------------------------------------------------------------------------
// Documented-surface drift guard.
//
// Why this file exists
// --------------------
// This repo has been burned by the same failure shape four times, and it is
// recorded as such in CHANGELOG/unreleased/current.md:
//
//   2026-07-26  `tools/sdkparity` extracted routes by grepping for
//               `mux.HandleFunc("…")`. A refactor changed every registration
//               to `router.Handle(path, policy, handler)`, so the pattern
//               matched NOTHING, the generated table came out empty, and
//               `-check` then reported the (still correct) SDK-PARITY.md as
//               stale and printed a remedy that would have deleted all
//               thirteen routes. A seventeen-day regression passed because
//               nothing asserted a non-empty extraction.
//
//   Day 23/28   the WebUI IA cleanup removed nav rows and their backend
//               routes. README.md and docs/CONSOLE-IA.md kept describing the
//               pre-cleanup console. nav.test.ts (this file's sibling)
//               checks only INTERNAL consistency — unique ids, <=6 rows per
//               group, the REMOVED_VIEW_IDS set — so the shipped surface
//               drifting away from the ADVERTISED surface was invisible.
//
// The shared blind spot is never the code. It is a hand-maintained claim
// (a number written in prose) with no gate that fails when it drifts.
//
// What this gate asserts
// ----------------------
// Every number the operator-facing docs attach to a surface noun (views,
// rows/destinations/sidebar entries, sections) must equal what nav.tsx
// actually exports. It does not care WHICH direction the team moves: restore
// the retired views and the docs must say so, or shrink the docs and the gate
// goes green. It only refuses the state where the two disagree silently.
//
// Note this is currently RED by design. It is reporting live drift, not
// asserting a policy. Closing it means either restoring the console surface
// or correcting the documentation to match it — a product decision, not a
// mechanical one, so this gate does not make that call for you.
// ---------------------------------------------------------------------------

const REPO_ROOT = path.resolve(import.meta.dirname, "..", "..");

/** The surface nav.tsx actually ships, measured rather than asserted. */
const measured = {
  sections: NAV_GROUPS.length,
  rows: NAV_GROUPS.reduce((n, g) => n + g.rows.length, 0),
  views: NAV_GROUPS.reduce((n, g) => n + g.rows.reduce((m, r) => m + r.views.length, 0), 0),
};

/**
 * Nouns in the docs that are really surface counts, mapped to the measured
 * field they must equal. `rows` / `destinations` / `sidebar entries` are the
 * same count (a sidebar entry IS a row since the row-fold); `views` /
 * `nav views` / `nav items` / `lazy views` are the other.
 */
const NOUNS: Array<{ re: RegExp; field: keyof typeof measured }> = [
  { re: /\bsections?\b/gi, field: "sections" },
  { re: /\b(?:rows?|destinations?|sidebar entries)\b/gi, field: "rows" },
  { re: /\b(?:nav\s+)?views?\b|\bnav items\b/gi, field: "views" },
];

/**
 * Extract `(line, number, noun)` triples for every surface count in a doc.
 * Only digits within 3 of the same word are picked up, so prose like
 * "8 sections" and "64 views, folded into 36 rows" both parse, while
 * "82 components" or "0 lost addresses" are ignored (different noun).
 *
 * Fenced code blocks are skipped. They are diagrams, directory trees and
 * worked examples — docs/CONSOLE-IA.md section 1 is a fenced tree describing
 * the console *as it was*, and that history is worth keeping verbatim. The
 * rule this gate encodes is: **prose is normative, fences are illustrative.**
 */
function surfaceClaims(md: string): Array<{ line: number; text: string; count: number; field: keyof typeof measured }> {
  const out: Array<{ line: number; text: string; count: number; field: keyof typeof measured }> = [];
  let inFence = false;
  md.split(/\r?\n/).forEach((text, idx) => {
    if (/^\s*```/.test(text)) { inFence = !inFence; return; }
    if (inFence) return;
    for (const { re, field } of NOUNS) {
      re.lastIndex = 0;
      let m: RegExpExecArray | null;
      while ((m = re.exec(text)) !== null) {
        // look for a count immediately before the noun, allowing a comma and
        // intervening words: "64 views, folded into 36 rows"
        const before = text.slice(Math.max(0, m.index - 24), m.index);
        const cm = before.match(/(\d{1,3})\s*$/);
        if (cm) out.push({ line: idx + 1, text: text.trim(), count: Number(cm[1]), field });
      }
    }
  });
  return out;
}

const DOCS = ["README.md", "docs/CONSOLE-IA.md", "docs/CONSOLE.md"];

describe("documented console surface matches the shipped surface", () => {
  it("ships a non-empty surface to compare against", () => {
    // A guard that silently matches nothing is the sdkparity bug above. If
    // NAV_GROUPS is ever emptied or reshaped into something unmeasurable,
    // fail here rather than letting every check below pass vacuously.
    expect(measured.sections).toBeGreaterThan(0);
    expect(measured.rows).toBeGreaterThan(0);
    expect(measured.views).toBeGreaterThanOrEqual(measured.rows);
  });

  for (const doc of DOCS) {
    it(`${doc} states no surface count that contradicts nav.tsx`, () => {
      const md = readFileSync(path.join(REPO_ROOT, doc), "utf8");
      const claims = surfaceClaims(md);
      const drift = claims.filter((c) => c.count !== measured[c.field]);

      expect(
        drift.map((d) => `  ${doc}:${d.line} says ${d.count} ${d.field}, nav.tsx has ${measured[d.field]}\n    "${d.text}"`),
        `${doc} advertises a console surface that nav.tsx does not ship. ` +
          `Measured: ${measured.sections} sections / ${measured.rows} rows / ${measured.views} views. ` +
          `Either restore the view in nav.tsx or correct the number here.`,
      ).toEqual([]);
    });
  }
});
