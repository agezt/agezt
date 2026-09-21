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
      { id: "roster", label: "Roster" },
    ],
  },

  research: {
    title: "Research",
    intro:
      "The grounded-answer surface. Ask a question and the daemon drafts sub-questions, gathers sources, verifies its claims, and writes a final answer you can audit. Unlike Chat it is single-shot — no conversation, no follow-up turns, just one careful answer.",
    sections: [
      {
        heading: "Inputs",
        items: [
          {
            term: "Question",
            desc: "The intent you want grounded. Open-ended works; a question mark helps the daemon treat it as a probe rather than an action.",
          },
          {
            term: "Sub-questions / Sources / Claims to verify",
            desc: "How many sub-questions to decompose into, how many sources to pull, and how many of the final claims to push back through verification.",
          },
          {
            term: "Verify claims (toggle)",
            desc: "Off: faster, but the answer cites without checking. On (default): each claim gets a yes/no from the verifier and lands in the Verified claims list with its verdict.",
          },
        ],
      },
      {
        heading: "Reading the answer",
        items: [
          {
            term: "Sub-questions",
            desc: "Each one the planner wrote, with its own intermediate answer. Use them to see how the final answer was assembled.",
          },
          {
            term: "Sources",
            desc: "Cards with title, URL, snippet, and a confidence percentage. Lower confidence = weaker grounding; cross-check before quoting.",
          },
          {
            term: "Verified claims",
            desc: "Green rows are confirmed by the verifier; amber rows failed and the answer hedges around them.",
          },
          {
            term: "Transcript",
            desc: "The raw planner output, collapsed by default. Open it when an answer surprises you — that is what the daemon actually saw.",
          },
        ],
      },
    ],
    tips: [
      "Recent questions land in the sidebar so a follow-up investigation is one click, not a re-ask.",
      "Treat confidence below 60% as 'directional, not authoritative'.",
    ],
    related: [
      { id: "analyst", label: "Analyst" },
      { id: "reflect", label: "Reflect" },
    ],
  },

  analyst: {
    title: "Analyst",
    intro:
      "Read the journal, not the model. Analyst is a view over the daemon's event log — every policy decision, provider fallback, tool invoke, run start/end, approval request — so you can see what has actually been happening and where the pressure points are.",
    sections: [
      {
        heading: "Filtering",
        items: [
          {
            term: "Pattern",
            desc: "Substring search across kind, actor, subject, correlation_id, and note. Lowercase, no regex.",
          },
          {
            term: "Kind",
            desc: "Pick a specific event kind from the dropdown — the dropdown is populated from whatever kinds are in the current window so it never offers an empty bucket.",
          },
          {
            term: "Limit",
            desc: "How many of the most recent events to load. 50 is a good default; 500 only when chasing a multi-hour investigation.",
          },
        ],
      },
      {
        heading: "Reading the rails",
        items: [
          {
            term: "Event kinds",
            desc: "Histogram of what kinds are firing. A sudden spike in policy.decision or provider.fallback is a smell worth clicking into.",
          },
          {
            term: "Top actors",
            desc: "Who is emitting events — agents, governors, routers, channels. An actor that fires constantly is either the busiest worker or a runaway loop.",
          },
          {
            term: "The table",
            desc: "One row per event with when, kind, actor, subject, and note. Click a row to see its full JSON in the journal search drawer.",
          },
        ],
      },
    ],
    tips: [
      "Sort by recency by default; when you spot something interesting, narrow by its correlation_id to get just that one story.",
    ],
    related: [
      { id: "research", label: "Research" },
      { id: "reflect", label: "Reflect" },
    ],
  },

  reflect: {
    title: "Reflect",
    intro:
      "Read what the daemon has been telling itself. Reflect surfaces the agent's self-talk — lessons written, corrections made, memories superseded, decisions logged — so the operator can audit not just what the daemon did, but what it learned from doing it.",
    sections: [
      {
        heading: "What lands here",
        items: [
          {
            term: "Lessons written",
            desc: "Memory entries that match 'learned / lesson / remember / next time' patterns. Green-bordered cards.",
          },
          {
            term: "Corrections",
            desc: "Memory or journal entries that match 'mistake / wrong / rolled back / fixed' patterns. Amber-bordered cards.",
          },
          {
            term: "Superseded memories",
            desc: "Memory entries that supersede an older one or are superseded by a newer one — the chain is visible so you can follow what replaced what.",
          },
          {
            term: "Top subjects",
            desc: "Which entities the daemon has been thinking about. A spike on a new subject usually means a new agent is on stage.",
          },
        ],
      },
      {
        heading: "Reading the journal section",
        items: [
          {
            term: "Reflective kinds",
            desc: "memory.write, memory.supersede, policy.decision, run.fail, approval.request, approval.decide — the events where the daemon was reasoning about itself.",
          },
        ],
      },
    ],
    tips: [
      "The lesson list is the closest thing to a written culture doc the daemon maintains. Read it before you re-prompt.",
    ],
    related: [
      { id: "research", label: "Research" },
      { id: "analyst", label: "Analyst" },
      { id: "memory", label: "Memory" },
    ],
  },

};
