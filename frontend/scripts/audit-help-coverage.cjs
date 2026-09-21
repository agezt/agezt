// Cross-reference NAV view ids against HELP topic ids in src/app/help/*.ts.
// Reports NAV ids without a topic and topics orphaned (no longer in NAV).
const fs = require("fs");

const HELP_FILES = ["monitor.ts","converse.ts","agents.ts","automation.ts","knowledge.ts","system.ts"];
// Keys that look like topic identifiers but are actually reserved words
// for the help-topic shape. Watch out: any added word that is also a real
// help-topic key (like 'policy' or 'prompts') will be wrongly filtered out,
// so double-check against `node scripts/audit-help-coverage.cjs` output.
const SKIP_KEYS = new Set([
  "examples","id","label","icon","keywords","title","intro","term","desc",
  "heading","paragraphs","items","sections","tips","related","render","subTabs",
  "agent","defaults","fields","alerts","provider","products",
]);
function isTopicKey(k) {
  if (!k) return false;
  if (SKIP_KEYS.has(k)) return false;
  if (typeof k !== "string") return false;
  return /^[a-z][\w-]+$/.test(k);
}

const topics = {};
for (const f of HELP_FILES) {
  const src = fs.readFileSync("src/app/help/" + f, "utf8");
  for (const m of src.matchAll(/^  (?:"([a-z][\w-]+)"|([a-z][\w-]+)): \{[\s]*\n/gm)) {
    const id = m[1] || m[2];
    if (isTopicKey(id)) topics[id] = (topics[id] || 0) + 1;
  }
}

const nav = fs.readFileSync("src/nav.tsx", "utf8");
const navIds = [...nav.matchAll(/^ {10}id: "([^"]+)"/gm)].map((m) => m[1]);

const noTopic = navIds.filter((id) => !topics[id]);
const orphans = Object.keys(topics).filter((id) => !navIds.includes(id));

console.log("NAV ids:   " + navIds.length);
console.log("HELP topics: " + Object.keys(topics).length);
console.log();
console.log("NAV ids WITHOUT help topic: " + (noTopic.length ? JSON.stringify(noTopic) : "none"));
console.log("HELP topics NOT in NAV:       " + (orphans.length ? JSON.stringify(orphans) : "none"));
