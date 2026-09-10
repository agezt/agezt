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
      "A real-time operations terminal: the daemon's pulse rendered as live rates and animated sparklines over a rolling 60-second window.",
    sections: [
      {
        heading: "Reading the instruments",
        items: [
          {
            term: "Activity waveform",
            desc: "The hero chart shows events per second — current rate, peak, and average over the window.",
          },
          {
            term: "Metric cards",
            desc: "LLM calls, tokens, spend, tool calls, and delegations — each with its instantaneous value and a sparkline of the last minute.",
          },
          {
            term: "Connection badge",
            desc: "Shows whether the live event stream is connected. Everything on this page is fed by the stream — no polling.",
          },
        ],
      },
    ],
    tips: [
      "\"Now\" reflects the last fully-elapsed second; the newest bucket is still filling, so the needle trails real time by about a second.",
    ],
    related: [
      { id: "feed", label: "Live Stream" },
      { id: "health", label: "Health" },
      { id: "activity", label: "Activity" },
    ],
  },

  health: {
    title: "Health",
    intro:
      "The daemon's vital signs — success and error rates, provider resilience, uptime, activity pulse, and knowledge footprint — as gauges and sparklines.",
    sections: [
      {
        heading: "System counters",
        items: [
          {
            term: "Operational / model / daemon",
            desc: "Whether the kernel is running or halted, the model in effect, and the daemon build. These came from the separate 'System' view, which read the same /api/status and repeated five of this page's tiles; #system still lands here.",
          },
          {
            term: "Journal head, world, skills, tools",
            desc: "The knowledge footprint: how far the append-only journal has advanced, how many world entities and active skills exist, and how many tools are registered.",
          },
          {
            term: "Schedules",
            desc: "Reads three ways. 'N live' means schedules are firing, 'enabled/total' is a healthy idle count, and 'offline' means schedules are enabled but the cadence resident is not running — they will never fire until it is.",
          },
        ],
      },
      {
        heading: "Advanced (delegation, HTTP, credentials, routing)",
        paragraphs: [
          "Turn on Advanced mode to reveal the daemon's wiring: delegation limits (depth, fan-out, spend ceiling), the HTTP surface with each listener and whether it is loopback-only, the credential chain in effect, and the provider-routing detail with the reason for the last fallback.",
        ],
      },
      {
        heading: "The gauges",
        items: [
          {
            term: "Success / error / fallback rings",
            desc: "Success rate over all completed runs, error rate, and how often providers had to fall back — color-coded so problems read at a glance.",
          },
          {
            term: "Uptime tile",
            desc: "How long the daemon has been up, humanized (\"2d 3h 4m\").",
          },
          {
            term: "Activity pulse",
            desc: "A sparkline of journal events per 5 seconds — the system's heartbeat.",
          },
          {
            term: "Footprint tiles",
            desc: "Running runs, pending approvals, provider/model fallback counts, memory records, world entities, and active skills.",
          },
          {
            term: "Fallback breakdown",
            desc: "Bars per primary provider showing how often each one failed over, with the most recent failure reason.",
          },
        ],
      },
      {
        heading: "Doctor",
        paragraphs: [
          "The diagnostics panel actively checks the daemon's live state — the same \"what's wrong and how do I fix it\" pass as `agt doctor` on the CLI. Each finding comes with a severity and a concrete remedy; a clean bill of health is shown explicitly rather than as an empty box.",
        ],
      },
    ],
    tips: [
      "A red \"halted\" badge means the kernel is paused — resume from the header or the System page.",
    ],
    related: [
      { id: "health", label: "Health" },
      { id: "providers", label: "Providers" },
      { id: "mission", label: "Mission Control" },
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

  taste: {
    title: "Taste",
    intro:
      "Curated “what good looks like” exemplars. Each is a concrete sample of good work — a model answer, a snippet, a well-formed artifact — that AGEZT injects into runs so output quality is anchored to examples rather than left to chance.",
    sections: [
      {
        heading: "How it works",
        items: [
          {
            term: "Exemplar",
            desc: "A title plus the example body itself. Before a run acts, matching exemplars are prepended to the system prompt as “what good looks like”.",
          },
          {
            term: "Scope",
            desc: "Blank scope means the exemplar shapes every run; setting an agent slug restricts it to that agent's runs. Scoped exemplars are injected ahead of global ones.",
          },
          {
            term: "Distinct from memory and skills",
            desc: "Memory is facts the agent recalls; skills are procedures it follows; taste is the quality bar it matches. They layer together in the system prompt.",
          },
        ],
      },
      {
        heading: "Control",
        paragraphs: [
          "Injection is on by default and capped at a few exemplars per run; AGEZT_TASTE_INJECT=off disables it while keeping the store and the `agt taste` CLI live. Each injection is journaled as taste.injected, so `agt why` shows exactly which exemplars shaped a run.",
        ],
      },
    ],
    tips: [
      "Keep exemplars short and exemplary — the point is the quality signal, not a knowledge dump.",
    ],
  },

  seats: {
    title: "Seats",
    intro:
      "Execution seats are named presets that decide HOW a workboard task runs — its model tier, tool tier, and isolation surface — layered on top of the agent it's dispatched to. The agent stays the identity; the seat refines execution for that task.",
    sections: [
      {
        heading: "Built-in seats",
        items: [
          { term: "default", desc: "Inherit everything from the assigned agent — no overrides." },
          { term: "reader", desc: "Read-only research: search, fetch, read artifacts. No shell, writes, or code execution." },
          { term: "builder", desc: "Full tools on the local execution surface — repo edits, tests, shell." },
          { term: "isolated", desc: "Full tools inside the warden sandbox, for untrusted or high-risk work." },
        ],
      },
      {
        heading: "Custom seats",
        paragraphs: [
          "Add your own seat with an id and an isolation surface (local, warden, or container — remote backends aren't available to seats). Custom seats sit alongside the built-ins in the task seat picker; built-ins can't be deleted. Pin a seat on a task with the seat picker on the Workboard, or `workboard create --seat`.",
        ],
      },
    ],
    tips: [
      "Precedence at dispatch: a task's seat wins, otherwise the agent's own default isolation applies.",
    ],
  },

  okr: {
    title: "Objectives",
    intro:
      "Objectives and their key results — the goal layer that makes fleet work legible as progress instead of a flat task list. Key results roll up the completion of the workboard tasks you link to them.",
    sections: [
      {
        heading: "Structure",
        items: [
          {
            term: "Objective",
            desc: "A durable goal you want the fleet to reach. It owns key results and shows an overall progress percentage averaged across them.",
          },
          {
            term: "Key result",
            desc: "A measurable outcome under an objective. Its target is a number of linked tasks that must be done; target 0 means every linked task must be done.",
          },
          {
            term: "Linked tasks",
            desc: "Workboard tasks you attach to a key result. As they complete, the key result's progress bar fills.",
          },
        ],
      },
      {
        heading: "Proof rolls up",
        paragraphs: [
          "Progress is fed by DONE tasks. Because the workboard proof gate only lets a task with acceptance criteria reach done once those criteria are proven, a gated task rolling up here means it was genuinely proven — legitimately-completed ungated tasks count too. An objective flips to achieved the moment every key result crosses its target.",
        ],
      },
    ],
    tips: [
      "Create an objective, add key results, then paste workboard task ids into a key result to link them.",
    ],
  },

  alerts: {
    title: "Alerts",
    intro:
      "What the daemon flagged on its own: self-health problems, run failures, budget trips, halts. A proactive signal feed, distinct from the raw event stream.",
    sections: [
      {
        heading: "Triage",
        items: [
          {
            term: "Severity chips",
            desc: "Filter by critical, warning, or info.",
          },
          {
            term: "Alert cards",
            desc: "Title, detail, source, the event kind that produced it, and the timestamp.",
          },
          {
            term: "Open run",
            desc: "Alerts tied to a run carry a jump button straight to that run's detail.",
          },
        ],
      },
      {
        heading: "How the feed is built",
        paragraphs: [
          "On load the page backfills from the journal, then merges live events on top — deduplicated, newest first, capped at 100. The Alerts entry in the sidebar shows an unseen-count badge from anywhere; opening this page marks them seen.",
        ],
      },
    ],
    tips: ["\"No alerts — all quiet\" genuinely means nothing was flagged, not that the feed is broken."],
    related: [
      { id: "runs", label: "Runs" },
      { id: "health", label: "Health" },
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
      { id: "search", label: "Search" },
      { id: "mission", label: "Mission Control" },
    ],
  },

  insights: {
    title: "Insights",
    intro:
      "The analytics cockpit: spend over time, per-model breakdown, run outcomes, and throughput — computed entirely client-side from the run list.",
    sections: [
      {
        heading: "What's measured",
        items: [
          {
            term: "Headline tiles",
            desc: "Total runs, total spend, success rate, average duration, and average iterations per run.",
          },
          {
            term: "Cumulative spend",
            desc: "An area chart of spend accumulating over time, with the peak labeled.",
          },
          {
            term: "Outcomes bar",
            desc: "Completed vs failed vs still-running, in one stacked bar.",
          },
          {
            term: "Spend by model",
            desc: "The top five models by what they cost you.",
          },
        ],
      },
    ],
    tips: [
      "Success rate counts only finished runs (completed + failed) — in-flight runs don't dilute it.",
      "The page refreshes itself when runs complete or fail; there's no extra backend endpoint behind it.",
    ],
    related: [
      { id: "budget", label: "Budget" },
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
      { id: "insights", label: "Insights" },
    ],
  },

  budget: {
    title: "Budget",
    intro:
      "The spend cockpit: today's spend against the daily ceiling, a pace-based forecast, and a live knob to adjust the ceiling at runtime.",
    sections: [
      {
        heading: "Reading the gauge",
        items: [
          {
            term: "Ring gauge",
            desc: "Percentage of today's ceiling consumed — or the raw spend figure when the ceiling is set to Unlimited.",
          },
          {
            term: "Pace forecast",
            desc: "\"At this pace\" extrapolates today's spend across the rest of the UTC day and warns if the projection exceeds the ceiling. Hidden very early in the day when the extrapolation would be noise.",
          },
          {
            term: "Per-task caps",
            desc: "Bar rows show spend per task type against any per-type caps.",
          },
        ],
      },
      {
        heading: "Adjusting",
        items: [
          {
            term: "Set ceiling",
            desc: "Enter a dollar figure or pick a quick preset ($5/$20/$50/$100) — or set Unlimited. Applies live, no restart.",
          },
        ],
      },
    ],
    tips: [
      "The daily counter resets at UTC midnight, not your local midnight.",
    ],
    related: [
      { id: "insights", label: "Insights" },
      { id: "models", label: "Models" },
    ],
  },

};
