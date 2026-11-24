// The browser's request rows come from the daemon, and they carry types.
//
// The daemon's operation registry declares what every console request body
// accepts; the generated catalog (`types.generated.ts`) renders those schemas
// into the browser's request rows. A request row whose member is `any` means
// the daemon declared that property WITHOUT a type — the browser then sends
// whatever it likes and the daemon reads the field as a silent default. The
// daemon's own guard (`kernel/controlplane/request_schema_ratchet_test.go`)
// holds the registry side; this one holds the browser's side, so a stale
// catalog or a hand-written copy cannot smuggle the same hole back in.
//
// Two teeth:
//
//  1. NO UNTYPED MEMBER. Every member of every request row the browser can
//     send names its type. A free-form object is fine (`Record<string, any>`);
//     a bare `any` is not — that is a property the daemon never typed.
//
//  2. NO HAND-WRITTEN COPY. The browser may not declare a request row of its
//     own: the row is the daemon's, and the browser names a PATH, not a shape.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = new URL("../../", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");
const GENERATED = join(SRC, "app", "api", "types.generated.ts");
const GENERATED_REL = "app/api/types.generated.ts";

const generated = readFileSync(GENERATED, "utf8");

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

/** The body of a declared interface, by name. */
function interfaceBody(name: string): string | null {
  const m = new RegExp(`export interface ${name} \\{`).exec(generated);
  if (!m) return null;
  return blockOf(generated, m.index + m[0].length - 1);
}

/** The type an alias points at, by name. */
function aliasTarget(name: string): string | null {
  const m = new RegExp(`export type ${name} = ([A-Za-z0-9_]+);`).exec(generated);
  return m ? m[1] : null;
}

/** The members of an object body: name, optional, type text. */
function membersOf(body: string): { name: string; optional: boolean; type: string }[] {
  const out: { name: string; optional: boolean; type: string }[] = [];
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

/** The paths the console accepts a JSON body for, and the row each one takes. */
function requestMap(): Map<string, string> {
  const out = new Map<string, string>();
  const m = /export interface OperationRequestMap \{([\s\S]*?)\n\}/.exec(generated);
  if (!m) return out;
  for (const line of m[1].split("\n")) {
    const row = /^\s*"([^"]+)":\s*([A-Za-z0-9_]+);/.exec(line);
    if (row) out.set(row[1], row[2]);
  }
  return out;
}

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

/** The declared type of a member, ignoring the optional marker. */
function bareType(type: string): string {
  return type.replace(/\s+/g, " ").trim();
}

describe("the browser's request rows", () => {
  it("names a type for every member of every request row", () => {
    const rows = requestMap();
    expect(rows.size).toBeGreaterThan(0);
    const untyped: string[] = [];
    for (const [path, row] of rows) {
      let name = row;
      let body = interfaceBody(name);
      while (body === null) {
        const target = aliasTarget(name);
        if (!target) break;
        name = target;
        body = interfaceBody(name);
      }
      if (body === null) {
        untyped.push(`${path}: the request row ${row} has no declaration`);
        continue;
      }
      for (const member of membersOf(body)) {
        const type = bareType(member.type);
        // A free-form object is `Record<string, any>` — a typed shape. A bare
        // `any` is a property the daemon never typed.
        if (type === "any" || /(^|\|)\s*any\s*(\||$)/.test(type)) {
          untyped.push(`${path}: ${name}.${member.name} is ${type}`);
        }
      }
    }
    expect(untyped).toEqual([]);
  });

  it("has no hand-written copy of a request row", () => {
    const rows = new Set<string>();
    for (const name of requestMap().values()) {
      rows.add(name);
      const target = aliasTarget(name);
      if (target) rows.add(target);
    }
    const copies: string[] = [];
    for (const file of walk(join(SRC, "app"))) {
      const rel = file.replace(/\\/g, "/").split("/src/")[1];
      if (rel === GENERATED_REL) continue;
      const text = readFileSync(file, "utf8");
      for (const m of text.matchAll(/^\s*(?:export\s+)?(?:interface|type)\s+([A-Za-z_$][\w$]*)/gm)) {
        if (rows.has(m[1])) copies.push(`${rel}: ${m[1]}`);
      }
    }
    expect(copies).toEqual([]);
  });
});
