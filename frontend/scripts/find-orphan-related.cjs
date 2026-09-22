// scripts/find-orphan-related.cjs — finds related[] entries that point at ids
// not in HELP. Run with `node scripts/find-orphan-related.cjs`.
const fs = require("node:fs");
const path = require("node:path");

const files = ["agents", "automation", "converse", "knowledge", "monitor", "system"];
const allIds = new Set();
for (const name of files) {
  const p = path.join(__dirname, "..", "src", "app", "help", `${name}.ts`);
  const c = fs.readFileSync(p, "utf8");
  const idRe = /^  ([a-z_-]+):\s*\{/gm;
  let m;
  while ((m = idRe.exec(c))) allIds.add(m[1]);
}

let total = 0;
for (const name of files) {
  const p = path.join(__dirname, "..", "src", "app", "help", `${name}.ts`);
  const c = fs.readFileSync(p, "utf8");
  const relRe = /related:\s*\[([\s\S]*?)\]/g;
  let rm;
  while ((rm = relRe.exec(c))) {
    const inner = rm[1];
    const itemRe = /id:\s*['"]([a-z_-]+)['"]/g;
    let im;
    while ((im = itemRe.exec(inner))) {
      const refId = im[1];
      if (!allIds.has(refId)) {
        console.log(`[${name}] related → "${refId}" — NOT in HELP anywhere`);
        total++;
      }
    }
  }
}
console.log(`\nTotal broken related: ${total}`);
console.log(`HELP has ${allIds.size} topics.`);
