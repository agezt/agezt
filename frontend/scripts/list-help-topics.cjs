// Walk help topic files and surface line numbers for any topic entry,
// plus any orphan content blocks left behind by a half-eaten delete.
const fs = require("fs");
const files = [
  "monitor.ts",
  "converse.ts",
  "agents.ts",
  "automation.ts",
  "knowledge.ts",
  "system.ts",
];
for (const f of files) {
  const src = fs.readFileSync("src/app/help/" + f, "utf8");
  const lines = src.split("\n");
  const name = f.padEnd(15);
  console.log("=== " + name + " (" + lines.length + " lines) ===");
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(/^\s+(("[^"]+"|\w+)): \{$/);
    if (m) console.log("  " + (i + 1) + ": " + m[1]);
  }
}
