// app/help/knowledge.ts — the Knowledge section of the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).
// Topics: memory, world, skills, reflect
//
// Each file exports a Record<string, HelpTopic> with just the
// topics that fall under the Knowledge section. The aggregator in
// help.ts spreads all six into the single HELP record. Topics
// remain plain data so they stay testable and tree-shakeable;
// <HelpDrawer> owns all presentation.

import type { HelpTopic } from "./types";

export const Knowledge: Record<string, HelpTopic> = {
  memory: {
    title: "Memory",
    intro:
      "Every durable fact the agent has stored — searchable, teachable, revisable, and exportable. This is its long-term memory, in the open.",
    sections: [
      {
        heading: "Browsing",
        items: [
          {
            term: "Memory cards",
            desc: "Type badge, subject, the fact itself, confidence percentage, age, creator/updater, and tags.",
          },
          {
            term: "Search",
            desc: "Filters across subject, content, and tags.",
          },
        ],
      },
      {
        heading: "Curating",
        items: [
          {
            term: "Teach",
            desc: "Add a fact directly: optional subject, a type, and the content.",
          },
          {
            term: "Revise",
            desc: "Edits create a new record that supersedes the old one — the original is retained for audit, never silently rewritten.",
          },
          {
            term: "Forget",
            desc: "Soft-deletes a memory; Prune later hard-removes soft-deleted records older than 30 days (dry-run first, then confirm).",
          },
          {
            term: "Import / export",
            desc: "Round-trips as memory.json. Memories are content-addressed, so re-importing the same file is a no-op rather than a duplicate flood.",
          },
        ],
      },
    ],
    tips: [
      "Chat shows \"learned\" chips when a conversation produces new memories — they land here.",
    ],
    related: [
      { id: "world", label: "World" },
      { id: "reflect", label: "Reflection" },
      { id: "data", label: "Data Lake" },
    ],
  },

  world: {
    title: "World",
    intro:
      "The knowledge graph: entities the agent knows about — people, projects, repos, orgs, devices, channels, topics, tasks — and the relations between them.",
    sections: [
      {
        heading: "Exploring",
        items: [
          {
            term: "Graph & breakdown",
            desc: "With two or more entities you get a visual graph, plus a breakdown bar and filter chips by kind.",
          },
          {
            term: "Search",
            desc: "Matches names, kinds, and aliases, with a live match count.",
          },
          {
            term: "Entity editor",
            desc: "The pencil on any entity edits its aliases (comma-separated) and arbitrary key/value attributes.",
          },
        ],
      },
      {
        heading: "Building the graph",
        items: [
          {
            term: "Add entity",
            desc: "Pick a kind, name it, add.",
          },
          {
            term: "Relate",
            desc: "Connect two entities with a verb (owns, depends_on, member_of, relates_to, …). Each relation row has a forget button.",
          },
          {
            term: "Import / export",
            desc: "JSON round-trip, content-addressed by kind+name (entities) and from/verb/to (relations) — idempotent on re-import.",
          },
        ],
      },
    ],
    tips: [
      "Agents grow this graph on their own as they work; reflection slowly decays the salience of entities that stop appearing.",
    ],
    related: [
      { id: "memory", label: "Memory" },
      { id: "reflect", label: "Reflection" },
    ],
  },

  skills: {
    title: "Skills",
    intro:
      "The learned-procedure library: reusable skills the agent has authored or learned, each moving through a lifecycle — draft → shadow → active — with usage evidence at every step.",
    sections: [
      {
        heading: "The lifecycle",
        items: [
          {
            term: "Status",
            desc: "Draft (not in use), shadow (evaluated silently alongside real runs), active (in the recall pool), quarantined (pulled), archived. The stacked bar up top shows the distribution.",
          },
          {
            term: "Promote / quarantine / revert",
            desc: "Promote moves a skill up the ladder; quarantine pulls a misbehaving one; revert rolls back the most recent change.",
          },
          {
            term: "Author & edit",
            desc: "Write a skill by hand or edit an existing one — a code change demotes it back to draft for re-proving.",
          },
        ],
      },
      {
        heading: "Evidence",
        items: [
          {
            term: "Skill cards",
            desc: "Version, shadow wins/evals, usage count, last used, trigger phrases, required tools, and a collapsible procedure body.",
          },
          {
            term: "Idle banner",
            desc: "Skills that are active but never (or long) unused are flagged with a one-click retire — they clutter the recall pool without earning their place.",
          },
          {
            term: "Private vs shared",
            desc: "A skill an agent learned in its own runs belongs to that agent (badge with the slug) and is retrieved only when IT acts — the same private-by-default wall as per-agent memory. Unbadged skills are the shared pool every agent and the default daemon identity draws from. Authoring a skill can target an agent via the 'Private to agent' field.",
          },
        ],
      },
    ],
    tips: [
      "Shadow mode is the safety net: a skill must win evaluations alongside real traffic before you trust it with real work.",
      "Search by an agent's slug to see exactly what that agent has taught itself.",
    ],
    related: [
      { id: "reflect", label: "Reflection" },
      { id: "roster", label: "Roster" },
      { id: "toolforge", label: "Tool Forge" },
    ],
  },

  reflect: {
    title: "Reflection",
    intro:
      "The daemon's self-review: it folds its own journal into observations and advisory proposals about how it could run better.",
    sections: [
      {
        heading: "The report",
        items: [
          {
            term: "Observation tiles",
            desc: "Window events, tasks done/failed, briefings, skills used, approvals granted/denied, and world entities — the raw material of the reflection.",
          },
          {
            term: "Proposals",
            desc: "Each carries an area badge, the observation that motivated it, and a suggestion. Proposals are advisory only — the daemon never auto-applies them.",
          },
          {
            term: "Run Now",
            desc: "Triggers a fresh reflection pass on demand instead of waiting for the next scheduled one.",
          },
        ],
      },
    ],
    tips: [
      "The single exception to \"advisory only\" is world-model salience decay — entities that stop appearing slowly fade, which is safe by construction.",
    ],
    related: [
      { id: "skills", label: "Skills" },
      { id: "world", label: "World" },
      { id: "autonomy", label: "Autonomy" },
    ],
  },

};
