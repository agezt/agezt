// app/help/monitor.ts — the Monitor section of the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).
// Topics: mission, health, activity, autonomy, taste, seats, okr, alerts, feed, insights, runs, budget
//
// Each file exports a Record<string, HelpTopic> with just the
// topics that fall under the Monitor section. The aggregator in
// help.ts spreads all six into the single HELP record. Topics
// remain plain data so they stay testable and tree-shakeable;
// <HelpDrawer> owns all presentation.

import type { HelpTopic } from "./types";

export const Monitor: Record<string, HelpTopic> = {

  mission: {
    title: "Mission Control",
    intro:
      "What the system is doing right now — event throughput, in-flight runs, anything that needs your eyes, and the live event stream. Refreshes every ten seconds; the live tail uses the same SSE feed the rest of the console listens to.",
    sections: [
      {
        heading: "Top strip",
        items: [
          { term: "Events / sec", desc: "Rolling 60-second rate of agent events on the SSE feed. Higher means busier; a sudden drop to zero usually means the daemon stalled." },
          { term: "Runs in flight", desc: "Active runs across all agents — what's actively working." },
          { term: "Awaiting you", desc: "Approvals and HITL prompts queued for your eyes. Zero means nothing wants you." },
          { term: "Spend today", desc: "Cumulative cost of all runs since midnight." },
        ],
      },
      {
        heading: "Attention queue",
        items: [
          { term: "Last 5 minutes", desc: "The five-minute window for HITL prompts, errored runs, and failed fallbacks. Each row is one attention item with a kind tag so you can route it to the right surface." },
          { term: "Nothing to see", desc: "When the list is empty, the daemon is running cleanly and no agent has flagged anything for human eyes." },
        ],
      },
      {
        heading: "Live stream",
        items: [
          { term: "Last 60 seconds", desc: "The same SSE tail the rest of the console uses; mission control slices it to the most recent 30 events so it stays readable at a glance." },
          { term: "Kind tags", desc: "run.* are accent-coloured (an agent is busy), error.* bad-coloured (something went wrong), warn.* warn-coloured (degraded). The colour is a route hint, not decoration." },
        ],
      },
    ],
    tips: [
      "For the chronological feed (older than 60 seconds) switch to Observe › Runs; mission control is for right-this-instant.",
      "A sustained 'Events / sec = 0' with 'Runs in flight > 0' is a stuck run — the agent is busy but not making progress.",
    ],
    related: [
      { id: "runs", label: "Runs" },
      { id: "feed", label: "Live Stream" },
    ],
  },

  activity: {
    title: "Activity",
    intro:
      "The live fleet monitor: every in-flight run, its sub-agents, iterations, and spend — updating in real time as the event stream arrives.",
    sections: [
      {
        heading: "Watching runs",
        items: [
          {
            term: "Run hierarchy",
            desc: "Runs are grouped parent-first with delegated sub-agents indented beneath, so a deep delegation tree stays readable.",
          },
          {
            term: "Expand for detail",
            desc: "Click a row to open the full run detail — tool calls, policy verdicts, and the final answer.",
          },
          {
            term: "Cancel",
            desc: "Each running row has a cancel button to stop just that run without halting the whole daemon.",
          },
          {
            term: "Counters",
            desc: "Running / completed / failed counts tick live; elapsed time updates every second while anything is in flight.",
          },
        ],
      },
    ],
    tips: [
      "The page seeds from the run list on load, then folds the live event stream on top — so it's accurate even for runs that started before you opened it.",
    ],
    related: [
      { id: "runs", label: "Runs" },
      { id: "agents", label: "Agents" },
      { id: "overseer", label: "Overseer" },
    ],
  },

  autonomy: {
    title: "Autonomy",
    intro:
      "What the daemon did on its own initiative — schedules firing, standing orders, skill lifecycle, completion checks, pulse briefings — plus the controls that tune that initiative.",
    sections: [
      {
        heading: "The timeline",
        items: [
          {
            term: "Category chips",
            desc: "Filter the curated, newest-first feed by source: schedule, standing, assure, skill, pulse, or board.",
          },
        ],
      },
      {
        heading: "Tuning the pulse",
        items: [
          {
            term: "Pause / resume / beat now",
            desc: "Stop or restart the autonomous heartbeat, or fire a single beat on demand — \"beat now\" works even while paused.",
          },
          {
            term: "Cadence & proactivity",
            desc: "Set how often the pulse beats (10s to 1h) and how chatty the daemon should be (quiet / balanced / chatty). Live-tuned; resets to defaults on restart.",
          },
          {
            term: "Quiet hours",
            desc: "Define hours during which the daemon keeps initiative to itself.",
          },
          {
            term: "Observers",
            desc: "Add disk-watch or command-probe observers the pulse evaluates each beat; runtime-added ones can be removed here.",
          },
          {
            term: "Digest flush",
            desc: "Force-deliver the pending digest instead of waiting for its schedule.",
          },
        ],
      },
    ],
    tips: [
      "This feed is curated — for the raw firehose of every event, use Live Stream instead.",
    ],
    related: [
      { id: "schedules", label: "Schedules" },
      { id: "standing", label: "Standing" },
      { id: "feed", label: "Live Stream" },
    ],
  },

  feed: {
    title: "Live Stream",
    intro:
      "The raw journal firehose: every event the daemon writes, color-coded by category, streaming in live. The most truthful — and busiest — view in the console.",
    sections: [
      {
        heading: "Taming the stream",
        items: [
          {
            term: "Pause / resume",
            desc: "Pause freezes the current snapshot so you can scroll and read without rows shifting under you; resume catches back up.",
          },
          {
            term: "Category chips",
            desc: "Toggle whole categories on and off — each chip shows a color dot and live count, dimming when disabled.",
          },
          {
            term: "Search",
            desc: "Substring filter across event kind, subject, actor, and id.",
          },
          {
            term: "Correlation focus",
            desc: "Click the last-6 of any row's correlation id to filter the stream to that run only.",
          },
          {
            term: "Expand a row",
            desc: "Click to reveal sequence number, actor, category, and the full payload as an explorable JSON tree.",
          },
        ],
      },
    ],
    tips: [
      "Error-kind events get a red tint so failures are visible even at full scroll speed.",
      "Category counts are computed over the unfiltered stream — toggling chips never changes the numbers.",
    ],
    related: [
      { id: "mission", label: "Mission Control" },
      { id: "runs", label: "Runs" },
    ],
  },

  runs: {
    title: "Runs",
    intro:
      "Every run the daemon has executed — in-flight and finished — with search and expandable full detail.",
    sections: [
      {
        heading: "Finding a run",
        items: [
          {
            term: "Filter box",
            desc: "Client-side search over intent, status, and correlation id, with a live match count.",
          },
          {
            term: "Run rows",
            desc: "Status badge, the intent (or correlation id), duration, and start time. Click to expand the full detail: each LLM round, tool call, policy verdict, and the final answer.",
          },
        ],
      },
      {
        heading: "Deep links",
        paragraphs: [
          "Other pages (Alerts, Dashboard, the ⌘K palette's \"Open run …\" commands) deep-link here and auto-expand the run in question.",
        ],
      },
    ],
    tips: [
      "For a cinematic step-through of a single run, open it in Replay instead.",
    ],
    related: [
      { id: "replay", label: "Replay" },
      { id: "activity", label: "Activity" },
    ],
  },

};
