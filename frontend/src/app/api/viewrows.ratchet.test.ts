// The browser may not spell a daemon row by hand.
//
// The daemon's rows live in one place — the generated catalog
// (`types.generated.ts`, rendered from the op registry). A browser view that
// copies such a row by hand is a second contract: it can promise a field the
// daemon never sends, or hide a field the daemon always sends. The browser
// names the fields it reads; the daemon says what they are.
//
// So the rule is mechanical: a hand-written type that copies a daemon row
// must take its members' types from the daemon's vocabulary — an indexed
// access into the row (`Row["field"]`), a `Pick` of it, or a type that does
// the same. What is left over is the browser's OWN vocabulary (the badge
// summaries it builds, the queue it keeps, the voice catalogue it ships), and
// it is listed in LEDGER below.
//
// The ledger has two teeth, like every other ratchet in this repo: a copy
// that is NOT on the list fails here (new debt cannot land), and a listed
// type that no longer copies — or that has been bound to the daemon — fails
// too (the debt was paid and the list was not updated; a stale allowance is
// a lie about the code).
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = new URL("../../", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");
const GENERATED = join(SRC, "app", "api", "types.generated.ts");

/** The browser's own vocabulary: shapes the daemon never sends. */
const LEDGER: Record<string, string> = {
  // A one-line summary the browser BUILDS (label + detail + a tone) for a
  // badge. It is not the daemon's next_action row — it is the browser's own
  // sentence about one. Five views speak that sentence.
  "features/agents/components/agentdetail/capability.tsx": "WorkflowToolAccessSummary",
  "features/agents/components/agentdetail/lifecycle.tsx": "AgentLifecycleActionResultSummary",
  "features/agents/components/roster/removal.ts": "AgentRemovalDecisionSummary",
  "features/workflows/components/types.ts": "WorkflowChainKind",
  "lib/fleet.ts": "FleetAgentIdentityCardSummary",
};

/** Every source file under src/: the module graph is the graph. */
function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else if (/\.tsx?$/.test(entry) && !entry.endsWith(".d.ts")) out.push(p);
  }
  return out;
}

/** The text of a `{ … }` block, comments and strings respected. */
function blockOf(src: string, open: number): string {
  let depth = 0;
  for (let i = open; i < src.length; i++) {
    const c = src[i];
    if (c === "/" && src[i + 1] === "/") {
      i = src.indexOf("\n", i);
      if (i === -1) return "";
      continue;
    }
    if (c === "/" && src[i + 1] === "*") {
      i = src.indexOf("*/", i);
      if (i === -1) return "";
      i++;
      continue;
    }
    if (c === '"' || c === "'" || c === "`") {
      const quote = c;
      i++;
      while (i < src.length && src[i] !== quote) {
        if (src[i] === "\\") i++;
        i++;
      }
      continue;
    }
    if (c === "{") depth++;
    else if (c === "}") {
      depth--;
      if (depth === 0) return src.slice(open + 1, i);
    }
  }
  return "";
}

/** The members of an object body: name, optional, type text. */
function membersOf(body: string): { name: string; optional: boolean; type: string }[] {
  const out: { name: string; optional: boolean; type: string }[] = [];
  // comments are not members, and a comment after a `;` would otherwise hide
  // the member that follows it
  const clean = body
    .replace(/\/\*[\s\S]*?\*\//g, " ")
    .replace(/\/\/[^\n]*/g, " ");
  let depth = 0;
  let start = 0;
  const parts: string[] = [];
  for (let i = 0; i < clean.length; i++) {
    const c = clean[i];
    if (c === '"' || c === "'" || c === "`") {
      const quote = c;
      i++;
      while (i < clean.length && clean[i] !== quote) {
        if (clean[i] === "\\") i++;
        i++;
      }
      continue;
    }
    if (c === "{" || c === "(" || c === "[" || c === "<") depth++;
    else if (c === "}" || c === ")" || c === "]" || c === ">") depth--;
    else if (c === ";" && depth === 0) {
      parts.push(clean.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(clean.slice(start));
  for (const part of parts) {
    const m = /^\s*([A-Za-z_$][\w$]*)(\?)?\s*:\s*([\s\S]+)$/.exec(part);
    if (m) out.push({ name: m[1], optional: m[2] === "?", type: m[3].trim() });
  }
  return out;
}

const generated = readFileSync(GENERATED, "utf8");

/** The daemon's rows: name → the fields it carries. */
const ROWS = new Map<string, Set<string>>();
for (const m of generated.matchAll(/export interface ([A-Za-z_$][\w$]*)\s*\{/g)) {
  const body = blockOf(generated, m.index + m[0].length - 1);
  const fields = new Set<string>();
  for (const member of membersOf(body)) fields.add(member.name);
  ROWS.set(m[1], fields);
}

/** Every hand-written type/interface with an object body. */
interface Decl {
  file: string;
  name: string;
  members: { name: string; optional: boolean; type: string }[];
  extendsRow: boolean;
}
const DECLS: Decl[] = [];
for (const path of walk(SRC)) {
  if (path === GENERATED) continue;
  const src = readFileSync(path, "utf8");
  const file = path.slice(SRC.length).replace(/\\/g, "/");
  for (const m of src.matchAll(/(?:export\s+)?(?:type|interface)\s+([A-Za-z_$][\w$]*)\s*(?:=\s*)?\{/g)) {
    const body = blockOf(src, m.index + m[0].length - 1);
    const head = src.slice(m.index, m.index + m[0].length);
    const extendsRow = /extends\s+(?:Omit\s*<\s*)?(?:Partial\s*<)?[A-Za-z_$][\w$]*/.test(head) &&
      [...ROWS.keys()].some((r) => new RegExp(`extends[^\\n]*${r}\\b`).test(head));
    DECLS.push({ file, name: m[1], members: membersOf(body), extendsRow });
  }
}

/** A member's type comes from the daemon when it indexes a row, Picks one, or names one. */
function daemonTyped(type: string): boolean {
  if (/\["/.test(type)) return true;
  if (/\bPick\s*</.test(type)) return true;
  for (const row of ROWS.keys()) {
    if (new RegExp(`\\b${row}\\b`).test(type)) return true;
  }
  return false;
}

/** A member is hand-written when its type is the browser's own vocabulary. */
function handwritten(type: string, boundNames: Set<string>): boolean {
  if (daemonTyped(type)) return false;
  for (const name of boundNames) {
    if (new RegExp(`\\b${name}\\b`).test(type)) return false;
  }
  return true;
}

// A type is BOUND when every member takes the daemon's types. A member may
// name another browser view — so the answer is a fixpoint.
const boundNames = new Set<string>();
for (let pass = 0; pass < 10; pass++) {
  let changed = false;
  for (const decl of DECLS) {
    if (boundNames.has(decl.name)) continue;
    if (decl.extendsRow) {
      boundNames.add(decl.name);
      changed = true;
      continue;
    }
    if (decl.members.length && decl.members.every((m) => !handwritten(m.type, boundNames))) {
      boundNames.add(decl.name);
      changed = true;
    }
  }
  if (!changed) break;
}

/** The daemon row a type copies: every hand-written member is one of its fields.
 *
 * Three members is the floor. Two field names in common ("id", "label") are a
 * coincidence of English, not a copy — the browser's own shapes have those
 * all over — and a ratchet that cries wolf gets ignored.
 */
function copiedRow(decl: Decl): string | null {
  const own = decl.members.filter((m) => handwritten(m.type, boundNames));
  if (own.length < 3) return null;
  let best: string | null = null;
  for (const [row, fields] of ROWS) {
    if (own.every((m) => fields.has(m.name))) {
      if (best === null || fields.size < (ROWS.get(best) as Set<string>).size) best = row;
    }
  }
  return best;
}

describe("the browser's views of the daemon's rows", () => {
  it("never spell a daemon row by hand", () => {
    const violations: string[] = [];
    const stale: string[] = [];
    const ledger = new Set(
      Object.entries(LEDGER).map(([file, name]) => `${file}:${name}`),
    );
    const seen = new Set<string>();
    for (const decl of DECLS) {
      const row = copiedRow(decl);
      if (!row) continue;
      const key = `${decl.file}:${decl.name}`;
      seen.add(key);
      if (boundNames.has(decl.name)) continue;
      if (ledger.has(key)) continue;
      violations.push(
        `${key} copies the daemon's ${row} by hand — take the members' types from the daemon`,
      );
    }
    for (const key of ledger) {
      if (!seen.has(key)) stale.push(`${key} is on the ledger but no longer copies a daemon row`);
    }
    expect([...violations, ...stale]).toEqual([]);
  });

  it("keeps the daemon's rows in the generated catalog", () => {
    expect(ROWS.size).toBeGreaterThan(300);
  });
});
