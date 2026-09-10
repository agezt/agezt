// app/help/automation.ts — the Automation section of the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).
// Topics: workflows, schedules, standing
//
// Each file exports a Record<string, HelpTopic> with just the
// topics that fall under the Automation section. The aggregator in
// help.ts spreads all six into the single HELP record. Topics
// remain plain data so they stay testable and tree-shakeable;
// <HelpDrawer> owns all presentation.

import type { HelpTopic } from "./types";

export const Automation: Record<string, HelpTopic> = {
  workflows: {
    title: "Workflows",
    intro:
      "The typed-node DAG editor: build automations from triggers (manual, cron, event, webhook), tool steps, LLM steps, branches, loops, and approval gates — every run journaled.",
    sections: [
      {
        heading: "The list",
        items: [
          {
            term: "Workflow cards",
            desc: "Trigger kind and detail, node count, enabled/disabled badge, with enable/disable and remove actions.",
          },
        ],
      },
      {
        heading: "The canvas",
        items: [
          {
            term: "Node palette & config",
            desc: "Drag nodes from the left palette; configure the selected node in the right panel, wiring data between nodes via handles.",
          },
          {
            term: "Reliability per node",
            desc: "Timeout, retry count, and retry delay are set per node — production workflows fail predictably, not silently.",
          },
          {
            term: "Node inspection",
            desc: "After a run, the node panel shows that node's actual input, output, and attempt count.",
          },
          {
            term: "Dry-run a node",
            desc: "Test a single node with mock upstream data before committing the whole workflow.",
          },
          {
            term: "Copilot",
            desc: "Describe the workflow (or a change) in natural language; the copilot drafts it onto the canvas. Drafts are never auto-saved — you review, then Save.",
          },
          {
            term: "Save & Run",
            desc: "Runs are asynchronous; nodes recolor live on the canvas as the run progresses. The Runs drawer lists past runs and can replay one onto the canvas.",
          },
        ],
      },
    ],
    tips: [
      "Webhook-triggered workflows give external systems a URL to fire — check the trigger config for the endpoint.",
    ],
    related: [
      { id: "schedules", label: "Schedules" },
      { id: "flow", label: "Flow Studio" },
      { id: "standing", label: "Standing" },
    ],
  },

  schedules: {
    title: "Schedules",
    intro:
      "Every typed cron job — agent wake, workflow run, daemon task, or tool call — with live countdowns, pause/resume, run-now, and full fire history.",
    sections: [
      {
        heading: "Reading the cockpit",
        items: [
          {
            term: "Summary tiles",
            desc: "Total, enabled, paused, and due-within-the-hour counts.",
          },
          {
            term: "Schedule cards",
            desc: "Cadence badge, typed target badge, source badge (operator / env / agent), assure badge, last fire status, and a live \"fires in …\" countdown.",
          },
          {
            term: "Fire preview",
            desc: "Toggle \"next fires\" to preview the next five fire times for any schedule.",
          },
          {
            term: "History",
            desc: "Past fires of each schedule, pulled from the journal.",
          },
        ],
      },
      {
        heading: "Managing",
        items: [
          {
            term: "Controls",
            desc: "Run Now fires immediately; Pause/Resume, Edit (replaces the cadence), and Remove do what they say.",
          },
          {
            term: "New schedule",
            desc: "Interval, daily-at-time, or once modes — each scheduling a typed target rather than embedding a new identity.",
          },
          {
            term: "Import / export",
            desc: "Schedules round-trip as JSON. Import is additive and dedupes by intent only — re-importing the same bundle can create near-duplicates if cadences differ.",
          },
        ],
      },
    ],
    tips: [
      "Agent-created schedules wear a distinct accent badge — worth reviewing periodically, since the daemon can arm future cron jobs for itself.",
    ],
    related: [
      { id: "standing", label: "Standing" },
      { id: "autonomy", label: "Autonomy" },
      { id: "workflows", label: "Workflows" },
    ],
  },

  standing: {
    title: "Standing",
    intro:
      "Standing orders: durable wake rules that fire on a cron schedule or event trigger, optionally running as a specific roster agent with a chosen autonomy mode.",
    sections: [
      {
        heading: "Order anatomy",
        items: [
          {
            term: "Triggers",
            desc: "Cron schedule and/or event subject — at least one is required. Both show as color-coded icons on the card.",
          },
          {
            term: "Autonomy mode",
            desc: "inform_only (report, don't act), ask (request approval), or act_or_ask (act within policy, escalate past it).",
          },
          {
            term: "Agent binding",
            desc: "An order can run AS a roster agent — that agent's soul, model, memory, and budget all apply to the firing.",
          },
          {
            term: "Assure",
            desc: "A retry count: how many times the daemon re-attempts an order whose outcome didn't verify.",
          },
        ],
      },
      {
        heading: "Managing",
        items: [
          {
            term: "Controls",
            desc: "Run Now, Pause/Resume, Edit (name, plan, agent, mode, assure), Remove.",
          },
          {
            term: "History",
            desc: "Toggle to see the order's recent firings from the journal.",
          },
          {
            term: "Import / export",
            desc: "JSON round-trip; import is additive, keyed on name + trigger.",
          },
        ],
      },
    ],
    tips: [
      "Schedules run typed targets on a clock; standing orders are durable wake rules that can react to events and carry autonomy policy.",
    ],
    related: [
      { id: "schedules", label: "Schedules" },
      { id: "roster", label: "Roster" },
      { id: "autonomy", label: "Autonomy" },
    ],
  },

};
