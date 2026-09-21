// Day 28 IA pass: stripping related-chips that pointed at retired ids left
// several topics with an empty `related: [\n],\n},` block. Each empty case
// gets a meaningful default pair chosen by hand so the Help drawer still
// shows two useful jump-offs when the topic is rendered.
const fs = require("fs");

const FILL = {
  "monitor.ts:mission": { topic: "mission", refs: [{ id: "runs", label: "Runs" }, { id: "feed", label: "Live Stream" }] },
  "monitor.ts:feed": { topic: "feed", refs: [{ id: "mission", label: "Mission Control" }, { id: "runs", label: "Runs" }] },
  "converse.ts:artifacts": { topic: "artifacts", refs: [{ id: "data", label: "Data Lake" }] },
  "knowledge.ts:research": { topic: "research", refs: [{ id: "analyst", label: "Analyst" }, { id: "reflect", label: "Reflect" }] },
  "knowledge.ts:analyst": { topic: "analyst", refs: [{ id: "research", label: "Research" }, { id: "reflect", label: "Reflect" }] },
  "system.ts:prompts": { topic: "prompts", refs: [{ id: "skills", label: "Skills" }] },
  "system.ts:policy": { topic: "policy", refs: [{ id: "approvals", label: "Approvals" }] },
  "agents.ts:replay": { topic: "replay", refs: [{ id: "runs", label: "Runs" }, { id: "activity", label: "Activity" }] },
};

const FILES = ["monitor.ts", "converse.ts", "agents.ts", "knowledge.ts", "system.ts"];
let touched = 0;
for (const file of FILES) {
  const path = "src/app/help/" + file;
  let src = fs.readFileSync(path, "utf8");
  for (const [key, info] of Object.entries(FILL)) {
    if (!key.startsWith(file + ":")) continue;
    const re = new RegExp(
      "(^  " + info.topic + ": \\{[\\s\\S]*?tips: \\[[\\s\\S]*?\\],\\s*)" +
      "related: \\[\\s*\\n\\],\\s*\\n  \\},",
      "m",
    );
    const before = src;
    src = src.replace(re, (match, prefix) => {
      const refs = info.refs
        .map((r, i) => `      { id: "${r.id}", label: "${r.label}" }${i < info.refs.length - 1 ? "," : ""}`)
        .join("\n");
      return `${prefix}related: [\n${refs},\n    ],\n  },`;
    });
    if (src !== before) {
      touched++;
      console.log("filled", file, "→", info.topic, "with", info.refs.map((r) => r.id).join(", "));
    }
  }
  fs.writeFileSync(path, src);
}
console.log("topics refilled:", touched);
