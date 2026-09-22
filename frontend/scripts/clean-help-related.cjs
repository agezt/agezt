// scripts/clean-help-related.cjs — strips related entries that point to ids no
// longer in NAV. One-shot script invoked from PowerShell; not part of the build.
const fs = require("node:fs");
const path = require("node:path");

const removedIds = new Set([
  "jarvis", "chat", "mission", "seats", "okr", "budget", "insights",
  "board", "approvals", "conductor", "research", "analyst", "reflect",
  "toolforge", "toolbox", "providers", "routing", "tools", "catalog",
  "cache", "backup", "workboard", "persona", "flow",
]);

let total = 0;
const files = ["agents", "automation", "converse", "knowledge", "monitor", "system"];
for (const name of files) {
  const p = path.join(__dirname, "..", "src", "app", "help", `${name}.ts`);
  let c = fs.readFileSync(p, "utf8");
  const before = c;
  c = c.replace(/related:\s*\[([\s\S]*?)\]/g, (full, inner) => {
    const filtered = inner
      .split("\n")
      .filter((line) => {
        const m = line.match(/id:\s*['"]([a-z_-]+)['"]/);
        if (m && removedIds.has(m[1])) return false;
        return true;
      })
      .join("\n");
    return "related: [" + filtered + "]";
  });
  if (c !== before) {
    fs.writeFileSync(p, c);
    console.log("Cleaned related in", name);
    total++;
  }
}
console.log("Done, cleaned", total, "files");
