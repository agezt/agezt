// scripts/dump-nav.cjs — final nav audit: walk every NAV view, resolve its
// render target, and confirm the actual component file exists on disk.
// Prints PASS / FAIL per row so a single broken import surfaces immediately.
const fs = require("node:fs");
const path = require("node:path");

const ROOT = path.resolve(__dirname, "..");
const SRC = path.join(ROOT, "src");
const NAV_FILE = path.join(SRC, "nav.tsx");
const NAV_TEXT = fs.readFileSync(NAV_FILE, "utf8");

// Build the const X = Y map (excluding helpers).
const bindingLines = NAV_TEXT.split("\n").filter((l) =>
  /^const \w+ = \w+;$/.test(l) && !/^const (row|Artifacts|REMOVED_VIEW|REMOVED_VIEW_IDS) /.test(l),
);
const renderOf = {};
for (const line of bindingLines) {
  const m = line.match(/^const (\w+) = (\w+);$/);
  if (!m) continue;
  renderOf[m[1]] = m[2];
}

// Component name → source file resolver. Mirrors the `lazyNamed(...)` imports
// in nav.tsx: each component is loaded from features/<area>/components/<Name>.tsx
// or components/<Name>.tsx (for shared UI).
function resolveSource(renderTarget) {
  if (renderTarget === "EventFeed") return path.join(SRC, "components", "EventFeed.tsx");
  // A few render-target names don't match their file names; map them first.
  const aliases = {
    Backup: "Backups",
  };
  const fileBase = aliases[renderTarget] || renderTarget;
  // Convention: fileBase is a PascalCase component name. Try features/* first,
  // then src/components/*.
  const candidates = [
    path.join(SRC, "features", "agents", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "agents", "components", "roster", `${fileBase}.tsx`),
    path.join(SRC, "features", "channels", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "chat", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "configcenter", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "connections", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "council", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "data", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "incidents", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "jarvis", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "market", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "mcp", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "observe", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "models", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "memory", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "overseer", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "policy", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "runs", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "sandbox", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "schedules", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "setup", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "skills", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "standing", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "voice", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "workflows", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "world", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "autonomy", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "execution-profiles", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "govern", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "artifacts", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "knowledge", "components", `${fileBase}.tsx`),
    path.join(SRC, "features", "admin", "components", `${fileBase}.tsx`),
    path.join(SRC, "components", `${fileBase}.tsx`),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  return null;
}

// Walk nav.tsx for every view, resolve its render target, and check the file.
const lines = NAV_TEXT.split("\n");
let section = "";
let row = "";
const rows = [];

for (let i = 0; i < lines.length; i++) {
  const l = lines[i];

  const sm = l.match(/^ {4}id: "([^"]+)",$/);
  if (sm && /^ {4}label: "[^"]+",$/.test(lines[i + 1] || "") && /rows: \[/.test(lines[i + 4] || "")) {
    section = sm[1];
  }
  const rm = l.match(/^ {6}row\("([^"]+)", "([^"]+)", \w+, \[$/);
  if (rm) row = rm[1];

  if (/^ {10}id: "[^"]+",$/.test(l)) {
    const idM = l.match(/^ {10}id: "([^"]+)",$/);
    let label = "";
    let renderVar = "";
    for (let j = i + 1; j < i + 12; j++) {
      const ll = lines[j] || "";
      const lm = ll.match(/^ {10}label: "([^"]+)",$/);
      if (lm) label = lm[1];
      const rm2 = ll.match(/^ {10}render: (\w+),?$/);
      if (rm2) renderVar = rm2[1];
      if (label && renderVar) break;
    }
    if (renderVar) {
      const target = renderOf[renderVar] || renderVar;
      const isRemoved = target === "REMOVED_VIEW";
      const sourceFile = isRemoved ? null : resolveSource(target);
      rows.push({
        section,
        row,
        id: idM[1],
        label,
        target,
        isRemoved,
        sourceFile,
      });
    }
  }
}

console.log("\n=== NAV AUDIT — every visible view ===\n");
let lastSection = "";
for (const v of rows) {
  if (v.section !== lastSection) {
    console.log(`\n[${v.section.toUpperCase()}]`);
    lastSection = v.section;
  }
  if (v.isRemoved) {
    console.log(`  ✗ ${v.id.padEnd(22)} "${v.label.padEnd(20)}" → PLACEHOLDER`);
  } else if (v.sourceFile) {
    console.log(`  ✓ ${v.id.padEnd(22)} "${v.label.padEnd(20)}" → ${v.target.padEnd(20)} (${path.relative(ROOT, v.sourceFile).replace(/\\/g, "/")})`);
  } else {
    console.log(`  ⚠ ${v.id.padEnd(22)} "${v.label.padEnd(20)}" → ${v.target.padEnd(20)} FILE NOT FOUND`);
  }
}

const total = rows.length;
const real = rows.filter((r) => !r.isRemoved).length;
const removed = rows.filter((r) => r.isRemoved).length;
const missing = rows.filter((r) => !r.isRemoved && !r.sourceFile).length;
console.log(`\n--- summary ---`);
console.log(`Total: ${total}`);
console.log(`Real components: ${real}`);
console.log(`Placeholders:    ${removed}`);
console.log(`Missing files:   ${missing}`);
if (missing > 0) {
  process.exit(1);
}
