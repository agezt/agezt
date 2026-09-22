// Strip related-chips pointing at retired view ids in help topics.
// Used once after the Day 28 IA pass retired the eight legacy rows whose
// labels misrepresented the content they rendered. The matching is by
// object shape, not by line, so it's robust to whitespace / wrapping.
const fs = require("fs");

const deadIds = [
  "health",
  "alerts",
  "search",
  "storage",
  "taste",
  "inbox",
  "wizards",
  "overview",
  "messages",
];

const files = [
  "src/app/help/monitor.ts",
  "src/app/help/converse.ts",
  "src/app/help/agents.ts",
  "src/app/help/automation.ts",
  "src/app/help/knowledge.ts",
  "src/app/help/system.ts",
];

let total = 0;
for (const f of files) {
  const src = fs.readFileSync(f, "utf8");
  let out = src;
  for (const id of deadIds) {
    // Match each inline related-link object line-block.
    // Pattern: optional indent, `{ id: "<id>", ...`, closing brace, optional comma + newline.
    const re = new RegExp(
      "\\n?[ \\t]*\\{[\\s\\S]{0,200}?id:[\\s\\t]*[\"']" +
        id +
        "[\"'][\\s\\S]{0,400}?\\}[\\s\\t]*,?[\\s\\t]*\\n?",
      "g",
    );
    const before = out;
    out = out.replace(re, "\n");
    if (out !== before) total++;
  }
  // Collapse the runs of blank lines left behind.
  out = out.replace(/\n{3,}/g, "\n\n");
  fs.writeFileSync(f, out);
}
console.log("stripped deadIds across", files.length, "files; matches:", total);
