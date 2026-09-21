// Generate a per-row audit: section, row label, row id, view count, view ids + labels.
const fs = require("fs");

const nav = fs.readFileSync("src/nav.tsx", "utf8");
const lines = nav.split("\n");
let curSection = "";
let curRow = "";
const sections = {};
for (const line of lines) {
  const sm = line.match(/^ {4}label: "([^"]+)",$/);
  if (sm) curSection = sm[1];
  const rm = line.match(/row\("([^"]+)"/);
  if (rm) curRow = rm[1];
  const vm = line.match(/^ {10}id: "([^"]+)",$/);
  if (vm) {
    if (!sections[curSection]) sections[curSection] = [];
    sections[curSection].push({ row: curRow, id: vm[1] });
  }
}

// Find label right after id
function labelFor(id) {
  for (const sec of Object.keys(sections)) {
    for (const r of sections[sec]) {
      if (r.id === id) return r;
    }
  }
  return null;
}

const renderLabels = (() => {
  const map = {};
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    for (let j = 1; j < 6; j++) {
      const next = lines[i + j];
      if (!next) break;
      const m = next.match(/^ {10}label: "([^"]+)",$/);
      if (m && line.match(/^ {10}id: "([^"]+)",$/)) {
        const idM = line.match(/^ {10}id: "([^"]+)",$/);
        map[idM[1]] = m[1];
      }
    }
  }
  return map;
})();

// Group by row id
const rowsById = {};
for (const sec of Object.keys(sections)) {
  for (const r of sections[sec]) {
    if (!rowsById[r.row]) rowsById[r.row] = [];
    rowsById[r.row].push({ ...r, section: sec, label: renderLabels[r.id] || r.id });
  }
}

console.log("=== PER-ROW NAV SHAPE — every tab accounted for ===\n");
let totalRows = 0, totalTabs = 0;
const sectionOrder = ["Talk", "Observe", "Automate", "Govern", "Agents", "Knowledge", "Connect", "Admin"];
for (const sec of sectionOrder) {
  if (!sections[sec]) continue;
  const seenRows = new Set();
  console.log(`### ${sec}`);
  for (const r of sections[sec]) {
    if (seenRows.has(r.row + r.section)) continue;
    seenRows.add(r.row + r.section);
    const tabs = rowsById[r.row].filter((t) => t.section === sec);
    const labels = tabs.map((t) => `${t.id} ("${t.label}")`).join(", ");
    console.log(`  - ${r.row.padEnd(22)} [${tabs.length} ${tabs.length === 1 ? "tab" : "tabs"}]: ${labels}`);
    if (!seenRows.size) totalRows++;
  }
  totalRows += seenRows.size;
  totalTabs += sections[sec].length;
  console.log();
}
console.log(`total rows: ${Object.keys(rowsById).length}, total tabs: ${totalTabs}`);
