// app/help/agents.ts — the Agents section of the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).
// Topics: agents, agent, roster, overseer, council, conductor, research, toolforge, mcp, acp, sandbox, flow, replay, analyst, search
//
// Each file exports a Record<string, HelpTopic> with just the
// topics that fall under the Agents section. The aggregator in
// help.ts spreads all six into the single HELP record. Topics
// remain plain data so they stay testable and tree-shakeable;
// <HelpDrawer> owns all presentation.

import type { HelpTopic } from "./types";

export const Agents: Record<string, HelpTopic> = {
  agents: {
    title: "Agents",
    intro:
      "Your whole autonomous fleet in one place. The Fleet tab is a complete census of every agent and automation you own — each card spelling out how it gets triggered — so the page is full and useful even when nothing is running. The Live tab is the run monitor.",
    sections: [
      {
        heading: "Fleet — the census",
        paragraphs: [
          "Every durable agent/automation appears as a card, whatever its kind, with its trigger front and centre. This is what you have, at rest.",
        ],
        items: [
          {
            term: "Roster agents",
            desc: "Persistent identities (soul, model, budget). They wake by delegation or a direct run unless a standing order / schedule is bound to them.",
          },
          {
            term: "Standing orders",
            desc: "Durable wake rules that fire on a cron schedule or a journal event subject.",
          },
          {
            term: "Schedules",
            desc: "Typed cron jobs — interval, daily, once, window, or continuous — each with its next fire time and target.",
          },
          {
            term: "Workflows",
            desc: "DAGs whose trigger node is manual, cron, event, or webhook.",
          },
          {
            term: "System engines",
            desc: "The always-on workers: Pulse (the proactive heartbeat), Reaper (the read-only sentinel), and Overseer (the live supervisor).",
          },
        ],
      },
      {
        heading: "Reading a card",
        items: [
          {
            term: "Trigger chips",
            desc: "The answer to 'how does this run?' — cron specs, event subjects, webhook, cadence, or 'manual / delegated' when nothing wakes it automatically.",
          },
          {
            term: "State pill",
            desc: "running (a run is happening) · armed (enabled with an automatic trigger) · manual (enabled but you must start it) · paused (disabled) · retired (graveyard).",
          },
          {
            term: "Filters & search",
            desc: "Narrow by kind (roster / standing / schedules / workflows / system) or running-now, or type to search names, models, and triggers.",
          },
        ],
      },
      {
        heading: "The detail panel",
        items: [
          {
            term: "Command Center (roster agents)",
            desc: "Opening a roster agent unfolds its full per-agent console: Overview (status, budgets, how it runs), Soul (identity core), Triggers (standing wake rules and schedules, with history), Activity/Runs/Logs, Memory (its private records), Skills, Diagnostics (capability denials + tool errors — what went wrong), and Files.",
          },
          {
            term: "How does this run?",
            desc: "A plain-language explanation of each trigger and exactly what you'd do to fire it (the cron, the event, the webhook URL, or the run command).",
          },
          {
            term: "Manage",
            desc: "Jumps to the page that edits this kind — Roster, Standing, Schedules, or Flow Studio.",
          },
          {
            term: "View live",
            desc: "For a running roster agent, opens its live delegation graph on the Live tab.",
          },
        ],
      },
      {
        heading: "Live — the run monitor",
        items: [
          {
            term: "Run cards",
            desc: "Every lead run with its sub-agent fleet, delegation depth, iterations, and spend. Running sorts first, then most recent.",
          },
          {
            term: "Delegation graph",
            desc: "Click a card for a live graph of the run's delegation tree; select any node to read that sub-run's full detail.",
          },
        ],
      },
    ],
    tips: [
      "The Fleet tab fills from the durable list endpoints, so you see your army even when nothing is executing — that's the point.",
      "A 'manual / delegated' agent isn't broken — it's reserve force. Arm it from Standing (give an order that agent + a cron/event trigger) or reference it from a schedule.",
      "Only lead runs get cards on the Live tab — sub-agents fold into their parent's tree.",
    ],
    related: [
      { id: "roster", label: "Roster" },
      { id: "standing", label: "Standing" },
      { id: "schedules", label: "Schedules" },
      { id: "flow", label: "Flow Studio" },
      { id: "overseer", label: "Overseer" },
    ],
  },

  agent: {
    title: "Agent identity page",
    intro:
      "Everything about one agent on a single deep-linkable page (#agent/<slug>): who it is, how it runs, what it has done and will do, and a self-repair console.",
    sections: [
      {
        heading: "Tabs",
        items: [
          { term: "Overview", desc: "How it's triggered, run/spend stats, today's budget, identity (model, fallbacks, scope), and its most recent failure." },
          { term: "Soul", desc: "The agent's identity core — who it is, how it should act, and what role it owns. Edit it in Roster, or let a self-repair run rewrite it." },
          { term: "Triggers", desc: "Standing orders and schedules that fire it, plus a forecast of its upcoming runs (what it will do next)." },
          { term: "Model", desc: "Its primary model, fallback chain, the global per-task chain its task type resolves to, and the provider activity its runs produced." },
          { term: "Activity and Comms", desc: "What it has done (runs, consults, memory) and its board mailbox — messages it sent, was addressed, or received." },
          { term: "Diagnostics and Files", desc: "Capability posture and denials, tool errors, plus its workdir and skill/script bundle files." },
        ],
      },
      {
        heading: "Repair",
        items: [
          { term: "Self-repair", desc: "Runs the agent as itself on a brief built from its own failures; it fixes its own scripts and files with its tools." },
          { term: "Auto-apply and Undo", desc: "Identity changes (soul/model/fallbacks) the run proposes are applied to its profile automatically; Undo reverts them." },
          { term: "Iterate by N", desc: "Runs several repair rounds back-to-back, each shown what the previous round already tried so it builds on it." },
        ],
      },
    ],
    tips: [
      "Open this page by clicking any agent's avatar or name in the Roster, or the “Page” button in the Fleet detail panel.",
      "The URL is shareable and survives a reload — bookmark an agent you watch often.",
    ],
    related: [
      { id: "roster", label: "Roster" },
      { id: "agents", label: "Agents" },
      { id: "routing", label: "Routing" },
    ],
  },

  roster: {
    title: "Roster",
    intro:
      "The agent-identity console: create, edit, pause, retire, and revive named agents — each with its own soul, model, cost ceiling, and memory scope.",
    sections: [
      {
        heading: "Agent cards",
        items: [
          {
            term: "Identity",
            desc: "A deterministic-color avatar, immutable slug, live status, and identity-card summary: kind, sleep/wake/work/repair rail, lifecycle, task contract, model, and private skills.",
          },
          {
            term: "Authority",
            desc: "Tool allow/deny posture, trust ceiling, workspace, memory scope, data-lake access, config overrides, noise budget, and schedule pressure.",
          },
          {
            term: "Operations",
            desc: "Mailbox backlog, lineage, delegation route, retry/self-repair governance, next wake, health, and current live work when the agent is awake.",
          },
          {
            term: "Activity",
            desc: "Opens a per-agent timeline: runs, delegations, memory writes, and board messages attributed to that agent.",
          },
          {
            term: "Lifecycle buttons",
            desc: "Edit, Pause/Resume, Retire/Revive, Remove. Retire moves the identity to the graveyard and is reversible; Remove can also clean selected standing orders, schedules, private/authored memory, skills, config, workspace, and dependent sub-agents.",
          },
        ],
      },
      {
        heading: "Creating agents",
        items: [
          {
            term: "New Agent",
            desc: "Set slug (fixed forever after creation), soul, model, task type, budget (dollar amounts), and memory scope.",
          },
        ],
      },
    ],
    tips: [
      "Before creating a new agent, check whether an existing one can be updated — near-duplicates fragment memory and budgets.",
      "Run any agent from the CLI with `agt run --agent <slug>`.",
    ],
    related: [
      { id: "agents", label: "Agents" },
      { id: "standing", label: "Standing" },
      { id: "persona", label: "Default Identity" },
    ],
  },

  overseer: {
    title: "Overseer",
    intro:
      "The supervisory dashboard: active runs, roster status, and open help requests — one glance to know whether the fleet needs you.",
    sections: [
      {
        heading: "Panels",
        items: [
          {
            term: "Stat cards",
            desc: "Active runs, enabled agents, and open help requests.",
          },
          {
            term: "Active runs",
            desc: "Each in-flight run with its agent chip and model. Click through to the run detail.",
          },
          {
            term: "Needs attention",
            desc: "Open help requests from the agent board, with routing info, surfaced so a stuck agent never waits unnoticed.",
          },
          {
            term: "Recent activity",
            desc: "A ticker of significant events only — task started/completed/failed, sub-agent spawned, council consensus, board posts — not the raw firehose.",
          },
        ],
      },
    ],
    tips: [
      "The Overseer nav item carries a live badge with the number of runs in flight, visible from any page.",
    ],
    related: [
      { id: "agents", label: "Agents" },
      { id: "board", label: "Agent Board" },
      { id: "activity", label: "Activity" },
    ],
  },

  council: {
    title: "Council",
    intro:
      "Multi-model deliberation: pose a question to a panel of models from your keyed providers, let them debate across rounds, and read the chair's synthesis of consensus and dissent.",
    sections: [
      {
        heading: "Convening",
        items: [
          {
            term: "Members",
            desc: "Badges show each seat and the model occupying it — drawn from providers that have keys.",
          },
          {
            term: "Question & rounds",
            desc: "Write the question, choose 0–5 deliberation rounds (in later rounds members see each other's opinions), and click Convene.",
          },
        ],
      },
      {
        heading: "Reading the result",
        items: [
          {
            term: "Consensus & dissent",
            desc: "The chair's synthesis appears first; genuine disagreement is preserved in a separate dissent block rather than papered over.",
          },
          {
            term: "Transcript",
            desc: "Every member's opinion, grouped by round, including any per-member errors.",
          },
        ],
      },
    ],
    tips: [
      "Council needs at least one keyed provider — an empty member list means no credentials are configured yet.",
      "More rounds cost more: every member is a real model call per round.",
    ],
    related: [
      { id: "models", label: "Models" },
      { id: "providers", label: "Providers" },
    ],
  },
  conductor: {
    title: "Conductor",
    intro:
      "Asymmetric, verify-driven collaboration: a Thinker plans, a Worker solves, and a Verifier checks — running the worker's code when it can — looping until the answer is verified or the round cap is hit.",
    sections: [
      {
        heading: "Running a task",
        items: [
          {
            term: "Task",
            desc: "Describe a hard, verifiable task (coding, math, multi-step reasoning) where one model's answer isn't enough. Press Conduct (or ⌘/Ctrl+Enter).",
          },
          {
            term: "Roles",
            desc: "Each role defaults to a different keyed-provider model; override any with a model id or @chain under Role models.",
          },
          {
            term: "Max rounds & plan",
            desc: "Max rounds caps Worker↔Verifier retries (default 2). Plan adds a first call that tailors each role's instructions to the task.",
          },
        ],
      },
      {
        heading: "Reading the result",
        items: [
          {
            term: "Verified / Not verified",
            desc: "The verdict header shows whether the Verifier accepted the final answer, and how many rounds it took.",
          },
          {
            term: "Transcript",
            desc: "Each role's turn in order: the Thinker's plan, the Worker's attempts, and the Verifier's check — including real code-execution output when it ran the code.",
          },
        ],
      },
    ],
    tips: [
      "The Conductor needs at least one keyed provider — empty role models mean no credentials are configured yet.",
      "It shines on tasks the Verifier can actually run: ask the Worker for code with self-tests and the check becomes a real execution, not just an opinion.",
    ],
    related: [
      { id: "council", label: "Council" },
      { id: "models", label: "Models" },
      { id: "chains", label: "Fallback chains" },
    ],
  },

  research: {
    title: "Research",
    intro:
      "The deep-research harness: a question is broken into sub-questions, each gathers independent web sources, the answer may only state claims it can cite, and every cited claim is then adversarially verified against its own source.",
    sections: [
      {
        heading: "Running research",
        items: [
          {
            term: "Question",
            desc: "Ask an open question that needs current, cross-checked evidence rather than one model's memory. Press Research (or ⌘/Ctrl+Enter).",
          },
          {
            term: "Adversarial verification",
            desc: "On by default: each cited claim gets an independent, skeptical check that tries to refute it against the source text. Turn it off for a faster, unverified draft.",
          },
          {
            term: "Advanced caps",
            desc: "Bound the run: max sources gathered, max sub-questions explored, and max claims verified. Every search and fetch is still governed and journaled.",
          },
        ],
      },
      {
        heading: "Reading the report",
        items: [
          {
            term: "Confidence & verified",
            desc: "Confidence is the share of verified claims the sources actually supported (or, unverified, the share of gathered sources the answer cited). The badge shows whether the adversarial pass ran.",
          },
          {
            term: "Claims",
            desc: "Each cited claim with its verdict — supported, refuted, or uncertain — the sources it cited, and the verifier's one-line reason. Refuted claims are the ones to distrust.",
          },
          {
            term: "Sources",
            desc: "Every gathered source, numbered [S1], [S2], …, linking out to the original page. The answer's citations refer to these.",
          },
        ],
      },
    ],
    tips: [
      "Research needs a provider and the web_search + browser.read tools; both are governed by their own policy and leave a trail in `agt why`.",
      "The report text is treated as untrusted external content, so it can never inject instructions back into an agent that reads it.",
    ],
    related: [
      { id: "conductor", label: "Conductor" },
      { id: "council", label: "Council" },
      { id: "memory", label: "Memory" },
    ],
  },

  toolforge: {
    title: "Tool Forge",
    intro:
      "Mint new tools from scripts: draft → test → promote, with operator sign-off. Promoted tools go live for agents as forge_<name>, no restart needed.",
    sections: [
      {
        heading: "The pipeline",
        items: [
          {
            term: "New tool",
            desc: "Name (lowercase, digits, underscores), language (python / node / deno), description, the code itself, and an optional JSON-Schema for its input.",
          },
          {
            term: "Test",
            desc: "Feed it JSON input and run it — a PASS/FAIL badge plus the actual output. Only tested tools can be promoted.",
          },
          {
            term: "Promote / quarantine",
            desc: "Promotion makes the tool callable by agents; quarantine pulls it from circulation without deleting it.",
          },
          {
            term: "Edit",
            desc: "Any code change demotes the tool back to draft — it must be re-tested before it can be promoted again.",
          },
        ],
      },
    ],
    tips: [
      "The tested badge is your safety rail: nothing reaches agents without at least one passing run you witnessed.",
    ],
    related: [
      { id: "tools", label: "Tool usage" },
      { id: "sandbox", label: "Sandbox" },
      { id: "catalog", label: "Tool registry" },
    ],
  },

  mcp: {
    title: "MCP Servers",
    intro:
      "Attach Model Context Protocol servers — local (stdio) or remote (HTTP) — and their tools go live for agents immediately as mcp_<name>_<tool>, no restart.",
    sections: [
      {
        heading: "Adding a server",
        items: [
          {
            term: "Popular gallery",
            desc: "A curated catalog of verified presets, searchable and grouped by category — one click prefills the register form.",
          },
          {
            term: "Register form",
            desc: "stdio tab: command, args, env. HTTP tab: URL plus auth headers (write-only). Both take an optional tool allowlist and a lazy checkbox.",
          },
          {
            term: "Name rule",
            desc: "Server names must be short lowercase alphanumerics (no dashes/underscores) because the name becomes part of tool ids and policy mapping.",
          },
        ],
      },
      {
        heading: "Managing servers",
        items: [
          {
            term: "Attach / detach",
            desc: "Bring a server's tools in or out of circulation live. Confirmation dialogs spell out exactly what changes.",
          },
          {
            term: "Lazy mode",
            desc: "Collapses a server's whole tool set into a single mcp_<name> dispatcher tool — keeps the agent's tool list small for servers with dozens of tools.",
          },
          {
            term: "Auto-attach",
            desc: "Toggle whether the server attaches automatically on daemon start.",
          },
        ],
      },
    ],
    tips: [
      "stdio servers run with a scrubbed environment — secrets you didn't explicitly pass don't leak into them.",
    ],
    related: [
      { id: "tools", label: "Tool usage" },
      { id: "catalog", label: "Tool registry" },
      { id: "policy", label: "Policy" },
    ],
  },

  acp: {
    title: "ACP Agents",
    intro:
      "External coding agents that speak the Agent Client Protocol — Gemini CLI, Claude Code's adapter, Codex, and friends. Agezt detects the ones installed on this host and can delegate a task to any of them via the acp_agent tool.",
    sections: [
      {
        heading: "What you see",
        items: [
          {
            term: "Census",
            desc: "A headcount of how many ACP agents are installed versus known to the catalog, so you can tell at a glance what this host can reach.",
          },
          {
            term: "Per-agent cards",
            desc: "Each detected agent shows its binary, install state, and a copy-ready usage hint for the acp_agent tool. Missing agents show how to install them.",
          },
          {
            term: "Re-scan",
            desc: "The refresh button re-probes the host (LookPath + version checks); detection is read-only and runs in the Go backend (kernel/acpcatalog).",
          },
        ],
      },
      {
        heading: "Using one",
        items: [
          {
            term: "Delegation",
            desc: "An agent calls the acp_agent tool naming an installed ACP agent and a task; the external agent runs it and returns the result, governed by the same policy and budget as any other tool.",
          },
        ],
      },
    ],
    related: [
      { id: "mcp", label: "MCP Servers" },
      { id: "tools", label: "Tool usage" },
      { id: "agents", label: "Agents" },
    ],
  },

  sandbox: {
    title: "Sandbox",
    intro:
      "Persistent projects agents built with the code_exec tool — their files visible, previewable, and downloadable instead of buried on disk.",
    sections: [
      {
        heading: "Projects",
        items: [
          {
            term: "Project cards",
            desc: "Each shows file count, total size, and last-modified time, with a collapsible file list.",
          },
          {
            term: "File preview",
            desc: "Toggle any file open to read it inline (fetched on first open, cached after). Previews cap at 256 KiB with a truncation notice.",
          },
          {
            term: "Download",
            desc: "Grab any single file directly.",
          },
          {
            term: "Remove project",
            desc: "Deletes the project from the sandbox (with confirmation). Past runs that created it remain in the journal.",
          },
        ],
      },
    ],
    tips: [
      "Ask the agent to \"build me a script that…\" in Chat — the resulting project appears here.",
    ],
    related: [
      { id: "toolforge", label: "Tool Forge" },
      { id: "artifacts", label: "Artifacts & Files" },
    ],
  },

  flow: {
    title: "Flow Studio",
    intro:
      "The plan-authoring workbench: describe a task, let the AI generate a multi-step plan as a DAG, edit or refine it, then run it and watch nodes light up live.",
    sections: [
      {
        heading: "Authoring",
        items: [
          {
            term: "Generate",
            desc: "Type the intent (optionally pick a model) and generate a plan — a JSON DAG of nodes you can edit directly in the textarea.",
          },
          {
            term: "Refine",
            desc: "Give a natural-language instruction (\"add a verification step\") and the plan is rewritten around it.",
          },
          {
            term: "Run",
            desc: "Executes the current plan text — including any manual edits you made.",
          },
        ],
      },
      {
        heading: "Watching",
        items: [
          {
            term: "Live DAG",
            desc: "The right panel renders the plan as a graph; nodes recolor as they run, complete, or fail, driven by the live event stream.",
          },
          {
            term: "History",
            desc: "The last eight plans with status, node count, and duration.",
          },
        ],
      },
    ],
    tips: [
      "The plan JSON is the source of truth — what you see in the textarea is exactly what Run executes.",
    ],
    related: [
      { id: "workflows", label: "Workflows" },
      { id: "runs", label: "Runs" },
    ],
  },

  replay: {
    title: "Replay",
    intro:
      "The flight recorder: pick any run and step through its exact sequence — every LLM round, tool call, policy decision, and the spend as it accumulated.",
    sections: [
      {
        heading: "Using the recorder",
        items: [
          {
            term: "Run selector",
            desc: "Newest first; runs still in flight are marked with a dot. The newest run is auto-selected on load.",
          },
          {
            term: "The timeline",
            desc: "The recorder lays out every step in order with its payload, so you can audit precisely what the agent saw and did.",
          },
          {
            term: "Live runs",
            desc: "Selecting an in-flight run folds live events in as they happen — you watch the recording being made.",
          },
        ],
      },
    ],
    tips: [
      "Replay is the best post-mortem tool: when a run went sideways, the answer is in the step where the inputs stopped matching your expectations.",
    ],
    related: [
      { id: "runs", label: "Runs" },
      { id: "search", label: "Search" },
    ],
  },

  analyst: {
    title: "Analyst",
    intro:
      "An AI observability assistant: it gathers a live snapshot of the system — stats, tools, cache, runs — and has the daemon's own model answer your questions about it.",
    sections: [
      {
        heading: "Asking",
        items: [
          {
            term: "Question box",
            desc: "Ask anything about system state (\"why did spend spike today?\"). Suggested questions are offered until your first ask.",
          },
          {
            term: "Streaming answer",
            desc: "The analysis streams in as markdown, with a collapsible reasoning block and a footer showing cost, model, and iterations.",
          },
        ],
      },
      {
        heading: "What it can and can't do",
        paragraphs: [
          "The Analyst reasons over the snapshot only — it makes no tool calls and changes nothing. It's a reading of the instruments, not a hand on the controls.",
        ],
      },
    ],
    tips: [
      "Each question costs one model call — the price is shown under every answer.",
    ],
    related: [
      { id: "insights", label: "Insights" },
      { id: "health", label: "Health" },
    ],
  },

  search: {
    title: "Search",
    intro:
      "Full journal search: filter the daemon's entire event history by text, kind, actor, or correlation id — with payload expansion, causation tracing, and cryptographic integrity checks.",
    sections: [
      {
        heading: "Querying",
        items: [
          {
            term: "Filters",
            desc: "Free-text pattern, event kind, actor, and correlation id — combine them and press Enter or Search.",
          },
          {
            term: "Results",
            desc: "Color-coded by category; click a row to expand the full payload.",
          },
          {
            term: "Trace cause",
            desc: "Walks an event's causation chain back to its root — across correlation boundaries, so you can trace a run all the way back to the heartbeat that initiated it.",
          },
        ],
      },
      {
        heading: "Trust but verify",
        items: [
          {
            term: "Integrity check",
            desc: "Verifies the journal's hash chain server-side and reports the first broken link, if any.",
          },
          {
            term: "Export",
            desc: "Downloads a signed journal bundle that can be re-verified offline.",
          },
        ],
      },
    ],
    tips: [
      "Live Stream answers \"what's happening now\"; this page answers \"what happened, ever\".",
    ],
    related: [
      { id: "feed", label: "Live Stream" },
      { id: "replay", label: "Replay" },
    ],
  },

};
