// Fix two system.ts topics whose related arrays were emptied by the strip pass.
// One-shot run; not part of the Day 28 build pipeline.
const fs = require("fs");

const path = "src/app/help/system.ts";
let src = fs.readFileSync(path, "utf8");

const replacement1Old = [
  "    related: [",
  "    ],",
  "  },",
  "",
  "  backup: {",
].join("\n");

const replacement1New = [
  "    related: [",
  '      { id: "skills", label: "Skills" },',
  "    ],",
  "  },",
  "",
  "  backup: {",
].join("\n");

if (src.includes(replacement1Old)) {
  src = src.replace(replacement1Old, replacement1New);
  console.log("filled backup related");
} else {
  console.log("missing replacement1 (backup)");
}

const replacement2Old = [
  "    related: [",
  "    ],",
  "  },",
  "",
  '  "execution-profiles": {',
].join("\n");

const replacement2New = [
  "    related: [",
  '      { id: "approvals", label: "Approvals" },',
  "    ],",
  "  },",
  "",
  '  "execution-profiles": {',
].join("\n");

if (src.includes(replacement2Old)) {
  src = src.replace(replacement2Old, replacement2New);
  console.log("filled execution-profiles related");
} else {
  console.log("missing replacement2 (execution-profiles)");
}

fs.writeFileSync(path, src);
