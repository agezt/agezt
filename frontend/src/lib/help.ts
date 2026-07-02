// help.ts — the in-app manual. One HelpTopic per view id (App.tsx NAV), rendered
// by <HelpDrawer>. Content is plain data so it stays testable and tree-shakeable;
// the drawer owns all presentation. When a new view is added to NAV, add a topic
// here — help.test.ts guards that every id is covered.

interface HelpItem {
  /** Short bold lead-in — a control, concept, or column on the page. */
  term: string;
  /** What it does / how to use it. */
  desc: string;
}

interface HelpSection {
  heading: string;
  paragraphs?: string[];
  items?: HelpItem[];
}

export interface HelpTopic {
  title: string;
  /** One- or two-sentence orientation: what this page is for. */
  intro: string;
  sections: HelpSection[];
  /** Practical "did you know" pointers, rendered as callouts. */
  tips?: string[];
  /** Other views that complete the story; chips navigate there. */
  related?: { id: string; label: string }[];
}

export const HELP: Record<string, HelpTopic> = {
  // ───────────────────────────── Converse ─────────────────────────────
  jarvis: {
    title: "Jarvis",
    intro:
      "The presence surface — the three pillars that turn AGEZT from a tool into a companion, shown together as one live status: it hears you, acts for you, and knows you. Every number on this page is live.",
    sections: [
      {
        heading: "The three pillars",
        items: [
          {
            term: "It hears you (Voice)",
            desc: "Hands-free conversation. Shows whether server text-to-speech is wired for a natural voice, or whether it falls back to the browser voice. Jumps to the Voice console to start talking.",
          },
          {
            term: "It acts for you (Initiative)",
            desc: "The Pulse heartbeat and its autonomy level — acting on its own, asking first, or just observing — plus live beats and how many observers are watching. Jumps to Autonomy to tune the dial.",
          },
          {
            term: "It knows you (Profile)",
            desc: "How many facets AGEZT has distilled about you as the operator, with a preview. Rebuild on demand, or jump to Memory to manage them.",
          },
        ],
      },
      {
        heading: "Presence meter",
        items: [
          {
            term: "X of 3 live",
            desc: "A pillar is 'live' when it is actually doing its job: voice can speak, the heartbeat is running above 'observe only', and at least one profile facet exists. Three of three means fully present.",
          },
        ],
      },
    ],
    related: [
      { id: "voice", label: "Voice" },
      { id: "autonomy", label: "Autonomy" },
      { id: "memory", label: "Memory" },
    ],
  },

  chat: {
    title: "Chat",
    intro:
      "The front door to your agent. Type an intent and watch the governed loop answer live — streaming text, tool calls with their policy verdicts, and the final cost of the run.",
    sections: [
      {
        heading: "The conversation",
        items: [
          {
            term: "Streaming answers",
            desc: "Assistant replies stream in as they are generated. A pulsing indicator shows the agent is still working; tool calls appear inline, in chronological order, each with the capability it used and the policy decision it received.",
          },
          {
            term: "Reasoning block",
            desc: "When the model emits reasoning, it shows as a collapsible block above the answer. It auto-expands while streaming and collapses once the answer is done.",
          },
          {
            term: "Edit & re-run",
            desc: "Hover any of your own messages and click the pencil to refine it. The conversation re-runs from that point with your revised wording.",
          },
          {
            term: "Regenerate",
            desc: "Re-sends the last user message for a fresh answer — useful when a reply came from a fallback model or just missed the mark.",
          },
          {
            term: "Fallback note",
            desc: "If the primary model failed and a fallback answered, the message is annotated with the model path (a → b → c) so you always know who actually replied.",
          },
        ],
      },
      {
        heading: "The composer",
        items: [
          {
            term: "Attachments",
            desc: "Attach files to prepend them as context for the next message. They are cleared automatically after sending.",
          },
          {
            term: "Mic input",
            desc: "Dictate your message with the microphone button; speech is transcribed into the composer.",
          },
          {
            term: "Model / agent / identity pickers",
            desc: "Override which model answers, run as a specific roster agent, or apply a per-thread identity override — all without touching global config.",
          },
          {
            term: "Auto-speak",
            desc: "When enabled, finished answers are read aloud. It triggers on completion only, so reloading the page never re-reads an old answer.",
          },
        ],
      },
      {
        heading: "Threads",
        items: [
          {
            term: "Conversation sidebar",
            desc: "Search past conversations, start a new chat, or pin a thread. Pinned threads auto-scroll with new messages; scroll up to unpin and read history undisturbed.",
          },
          {
            term: "Saved prompts",
            desc: "The empty state offers your saved prompt library as one-click starters — manage them on the Prompts page.",
          },
        ],
      },
    ],
    tips: [
      "Press ⌘K / Ctrl+K and run \"New chat\" from anywhere in the console.",
      "Learned-memory chips under an answer show what the agent chose to remember from the exchange.",
    ],
    related: [
      { id: "prompts", label: "Prompts" },
      { id: "runs", label: "Runs" },
      { id: "persona", label: "Default Identity" },
    ],
  },

  voice: {
    title: "Voice",
    intro:
      "Hands-free conversation — the \"talk to AGEZT\" surface. Start it and just speak: it listens until you pause, runs your agent, and speaks the answer back sentence-by-sentence. Talk over it any time and it stops to listen.",
    sections: [
      {
        heading: "The loop",
        items: [
          {
            term: "Listening (VAD)",
            desc: "After you start, the mic stays open and voice-activity detection waits for you to speak, then for a short trailing silence that marks the end of your turn — no push-to-talk.",
          },
          {
            term: "Streaming speech",
            desc: "The answer is spoken as it streams, one sentence at a time, instead of waiting for the whole reply — so it starts talking back almost immediately.",
          },
          {
            term: "Barge-in",
            desc: "Start talking while it's speaking and it stops instantly and listens — just like interrupting a person.",
          },
          {
            term: "The orb",
            desc: "The central orb reflects the current phase (idle, listening, thinking, speaking) and pulses with your mic level so you can see it's hearing you.",
          },
        ],
      },
      {
        heading: "Controls",
        items: [
          {
            term: "Wake word",
            desc: "Toggle on to require saying \"agezt\" (or \"jarvis\") before a turn — fully hands-free. Off by default; turns start as soon as you speak after Start.",
          },
          {
            term: "Agent",
            desc: "Pick which roster agent answers, or leave on Default routing to let the daemon decide. Locked while a session is running.",
          },
          {
            term: "High-quality voice",
            desc: "When the daemon has a TTS backend configured (AGEZT_TTS_*), replies use that natural voice; otherwise it falls back to the browser's built-in speech.",
          },
        ],
      },
    ],
    tips: [
      "Voice input needs an STT backend (AGEZT_STT_*) for transcription; without it the browser's own recognition is used where available.",
      "The same conversation runs through the normal governed agent loop — tools, policy, and memory all apply.",
    ],
    related: [
      { id: "chat", label: "Chat" },
      { id: "agents", label: "Agents" },
      { id: "connections", label: "Connections" },
    ],
  },

  inbox: {
    title: "Inbox",
    intro:
      "Every channel conversation — Telegram, Slack, Discord, email and more — folded into one unified view, newest activity first.",
    sections: [
      {
        heading: "Reading threads",
        items: [
          {
            term: "Thread cards",
            desc: "Each thread shows its channel badge, the contact/channel id, and the latest messages inline. Blue down-arrows are inbound (from the person), orange up-arrows are outbound (from the agent).",
          },
          {
            term: "Search",
            desc: "Filters across channel names, contact ids, and full message content — not just titles.",
          },
          {
            term: "Inbound images",
            desc: "Pictures received on a channel render as gallery thumbnails inside the thread and link to the Files page for full preview.",
          },
        ],
      },
      {
        heading: "Sending messages",
        items: [
          {
            term: "Send form",
            desc: "Pick a channel, enter the recipient, type your text, and press Ctrl+Enter (or click Send). The daemon refuses if that channel isn't configured.",
          },
          {
            term: "Reply button",
            desc: "On any thread, Reply pre-fills the send form with the right channel and recipient so you can answer in two keystrokes.",
          },
        ],
      },
    ],
    tips: [
      "Threads are reconstructed live from the journal's channel events — nothing here is a separate database that can drift.",
    ],
    related: [
      { id: "files", label: "Files" },
      { id: "chat", label: "Chat" },
    ],
  },

  files: {
    title: "Files",
    intro:
      "The artifact store: every file the daemon has indexed — images received on channels, uploads, and other artifacts — browsable, previewable, and downloadable.",
    sections: [
      {
        heading: "Browsing",
        items: [
          {
            term: "Filter chips",
            desc: "Switch between All, Images (rendered as a lazy-loading gallery with timestamps), and Files (a list with name, kind, size, and time).",
          },
          {
            term: "Preview modal",
            desc: "Click anything to preview it inline: images, PDFs, code, markdown, plain text, and JSON all render in place. Binary formats offer a download instead.",
          },
          {
            term: "Download / delete",
            desc: "Every file row has a direct download link and a delete button.",
          },
        ],
      },
      {
        heading: "Housekeeping",
        items: [
          {
            term: "Collect",
            desc: "Reaps artifacts older than 30 days. It always dry-runs first — you see exactly how many files and bytes would go before confirming the real deletion.",
          },
        ],
      },
    ],
    tips: [
      "Text previews are capped at 2 MB so a huge log can't lock up the browser.",
      "Images are grouped by run correlation id — the same grouping the Inbox uses for its threads.",
    ],
    related: [
      { id: "inbox", label: "Inbox" },
      { id: "artifacts", label: "Artifacts" },
      { id: "data", label: "Data Lake" },
    ],
  },

  artifacts: {
    title: "Artifacts",
    intro:
      "The showroom for everything your agents produce: reports, charts, generated pages, code, data files — bucketed by what each artifact IS, with a live preview per type and a fullscreen viewer for the big screen.",
    sections: [
      {
        heading: "The gallery",
        items: [
          {
            term: "Category sections",
            desc: "Artifacts are bucketed by type — Images, SVG, HTML, Markdown, JSON, Code, PDF, Text, Other. Pictures show themselves as thumbnails; everything else shows a type icon over its name, so a wall of outputs reads at a glance.",
          },
          {
            term: "Category chips",
            desc: "Each chip carries a live count; click to focus one category, click again to go back to all. Counts follow the search box, so 'report' + HTML shows exactly the generated report pages.",
          },
          {
            term: "Search",
            desc: "Matches name, caption, source channel, and sender — the fields a human remembers an artifact by.",
          },
        ],
      },
      {
        heading: "The viewer",
        items: [
          {
            term: "Live previews",
            desc: "Markdown renders formatted; JSON is pretty-printed; code and text show monospaced; PDFs embed. HTML runs live inside a sandboxed frame — scripts may execute, but the frame has no same-origin access, so it can never reach the console's token or API.",
          },
          {
            term: "Fullscreen",
            desc: "The expand button grows the viewer to fill the monitor — a generated dashboard or chart at its intended size. Esc closes.",
          },
          {
            term: "Download & delete",
            desc: "Every artifact downloads with its original name. Delete removes the index entry; the underlying bytes are garbage-collected once nothing else references them.",
          },
        ],
      },
    ],
    tips: [
      "Files is the flat manager (everything in arrival order); Artifacts is the same store re-cut by type — use whichever matches the question in your head.",
      "Text previews are capped at 2 MB; bigger artifacts offer a download instead.",
    ],
    related: [
      { id: "files", label: "Files" },
      { id: "runs", label: "Runs" },
      { id: "storage", label: "Storage" },
    ],
  },

  data: {
    title: "Data Lake",
    intro:
      "Your personal structured-data store. Agents create and fill collections with the db tool; this page is where you browse, edit, and search them.",
    sections: [
      {
        heading: "Collections",
        items: [
          {
            term: "Sidebar",
            desc: "Lists every collection with a record count. Built-in collections (seeded at startup) carry a lock icon; agents can create more at any time.",
          },
          {
            term: "Bespoke views",
            desc: "Known schemas get purpose-built layouts — expenses, calendar, tasks, habits, notes, bookmarks, contacts. Everything else falls back to a generic table with columns inferred from the records.",
          },
        ],
      },
      {
        heading: "Editing records",
        items: [
          {
            term: "Add / edit / delete",
            desc: "Use the Add button or the pencil on any row to open the record editor; the trash icon deletes. The editor coerces values sensibly — numbers, booleans (\"true\"/\"1\"/\"yes\" all work), and comma-separated tags.",
          },
          {
            term: "Search",
            desc: "Filters the current collection's records as you type.",
          },
        ],
      },
    ],
    tips: [
      "Ask the agent in Chat to log something (\"track this expense…\") and watch the matching collection update here.",
    ],
    related: [
      { id: "files", label: "Files" },
      { id: "memory", label: "Memory" },
    ],
  },

  board: {
    title: "Agent Board",
    intro:
      "The shared message board agents use to coordinate with each other — handoffs, questions, status notes. You're reading their internal radio traffic.",
    sections: [
      {
        heading: "Reading the board",
        items: [
          {
            term: "Topic chips",
            desc: "Filter by topic. With many topics the chip row becomes searchable and caps at 24 visible with a \"show all\" toggle.",
          },
          {
            term: "Message anatomy",
            desc: "Each post shows the topic, the sender, who it was addressed to (an arrow for direct messages, a megaphone for broadcasts to *), any reply-to link, and the timestamp. Bodies render as markdown.",
          },
          {
            term: "Awaiting-reply badge",
            desc: "Direct messages that never got an answer are flagged. The check runs over the whole board — not just the current topic filter — so the badge never lies.",
          },
          {
            term: "Help requests",
            desc: "Posts flagged as help requests surface in a banner at the top so calls for assistance don't drown in traffic.",
          },
        ],
      },
    ],
    tips: [
      "The board is read-only from here — agents write to it via their board tool. To generate traffic, run a multi-agent task.",
    ],
    related: [
      { id: "agents", label: "Agents" },
      { id: "overseer", label: "Overseer" },
    ],
  },

  approvals: {
    title: "Approvals",
    intro:
      "Human-in-the-loop gating. When the agent hits a capability set to \"ask\", the request lands here and waits for your verdict.",
    sections: [
      {
        heading: "Acting on requests",
        items: [
          {
            term: "Pending panel",
            desc: "Each waiting request shows the capability, the input, and why it was gated — with Approve and Deny buttons. The run is paused until you decide (or the request times out).",
          },
          {
            term: "Decision history",
            desc: "Below the pending list: an audit trail of past rulings — granted, denied, or timed out — with the capability, reason, resolver, and timestamp.",
          },
        ],
      },
      {
        heading: "Where the gates come from",
        paragraphs: [
          "Which capabilities require approval is governed on the Policy page (trust levels and ask-mode). The bell in the header mirrors this page's pending count from anywhere in the console.",
        ],
      },
    ],
    tips: [
      "A request that times out is recorded as such — silence is never treated as consent.",
    ],
    related: [
      { id: "policy", label: "Policy" },
      { id: "runs", label: "Runs" },
    ],
  },

  // ───────────────────────────── Monitor ─────────────────────────────
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
      { id: "system", label: "System" },
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

  // ───────────────────────────── Agents ─────────────────────────────
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
      { id: "tools", label: "Tools" },
      { id: "sandbox", label: "Sandbox" },
      { id: "catalog", label: "Catalog" },
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
      { id: "tools", label: "Tools" },
      { id: "catalog", label: "Catalog" },
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
      { id: "tools", label: "Tools" },
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
      { id: "files", label: "Files" },
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

  // ───────────────────────────── Automation ─────────────────────────────
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

  // ───────────────────────────── Knowledge ─────────────────────────────
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

  // ───────────────────────────── System ─────────────────────────────
  overview: {
    title: "Overview",
    intro:
      "The cockpit: every key gauge on one screen — throughput rings, spend, live activity, active agents, and anything that needs your attention.",
    sections: [
      {
        heading: "The panels",
        items: [
          {
            term: "Needs attention",
            desc: "Recent critical and warning alerts, with one-click jumps to the affected run. Resolved halts clear themselves.",
          },
          {
            term: "Active agents",
            desc: "A mini-gallery of up to six currently-running lead runs; click through to the full Agents view.",
          },
          {
            term: "Rings & tiles",
            desc: "Success rate, budget consumption, schedule status, and activity rate, alongside running/completed/failed/skills counts.",
          },
          {
            term: "Spend by model",
            desc: "The top five models by cost.",
          },
          {
            term: "Live ticker",
            desc: "The 40 most recent events, newest first.",
          },
        ],
      },
    ],
    tips: [
      "Everything here is a doorway — click any panel to land on the page that owns the detail.",
    ],
    related: [
      { id: "mission", label: "Mission Control" },
      { id: "health", label: "Health" },
      { id: "insights", label: "Insights" },
    ],
  },

  setup: {
    title: "Setup",
    intro:
      "The guided first-run wizard: sync the model catalog, add a provider key, and pick a model — three steps from zero to a working agent.",
    sections: [
      {
        heading: "The steps",
        items: [
          {
            term: "1 — Catalog",
            desc: "Sync the provider/model catalog (one-time; works offline thereafter).",
          },
          {
            term: "2 — Provider",
            desc: "Search and pick a provider — credentialed ones sort first — then paste an API key. Keyless providers say so.",
          },
          {
            term: "3 — Model",
            desc: "Choose the default model from the selected provider's list.",
          },
        ],
      },
      {
        heading: "When it appears",
        paragraphs: [
          "The wizard auto-opens full-screen on first run while no provider has credentials, and never auto-opens again once any key exists (or after you skip it). You can always return here to redo any step — completed steps are pre-filled and skipped.",
        ],
      },
    ],
    related: [
      { id: "providers", label: "Providers" },
      { id: "models", label: "Models" },
      { id: "toolbox", label: "Toolbox" },
    ],
  },

  toolbox: {
    title: "Toolbox",
    intro:
      "The host's CLI tool library: see which command-line tools are installed, missing, or out of date on the machine agezt runs on — and install the missing ones from here.",
    sections: [
      {
        heading: "What you see",
        items: [
          {
            term: "Census band",
            desc: "Catalog size, plus how many tools are installed, missing, outdated, and installable on this host. The OS and the detected package managers (winget/choco/brew/apt…) are shown in the header.",
          },
          {
            term: "Status badges",
            desc: "Each tool card reads installed (with its version), missing, or update (when 'Check updates' found a newer release). Cards with no recipe for this OS say so.",
          },
          {
            term: "Filters & search",
            desc: "Narrow by installed / missing or by category (search, data, media, build, cloud…), or type to search names, descriptions, and managers.",
          },
        ],
      },
      {
        heading: "Installing",
        items: [
          {
            term: "One tool",
            desc: "Click Install on a card to run the shown package-manager command. The exact command appears under every missing tool before you run it.",
          },
          {
            term: "Bulk install",
            desc: "Install all missing, or all missing in a category — each asks for confirmation (it changes the host) and then streams per-tool progress live in the output panel.",
          },
          {
            term: "Check updates",
            desc: "Asks the host package managers what's upgradable and flags those tools with an Update button.",
          },
        ],
      },
    ],
    tips: [
      "Installs run the real package manager on the machine agezt runs on, as the daemon's user — some packages may need elevation; failures show the command so you can run it yourself.",
    ],
    related: [
      { id: "setup", label: "Setup" },
      { id: "tools", label: "Tools" },
      { id: "sandbox", label: "Sandbox" },
    ],
  },

  channels: {
    title: "Channels",
    intro:
      "Connect AGEZT to the messaging platforms you already use — Telegram, WhatsApp, Slack, Discord, Matrix, SMS, and more. Each channel is set up separately from its own card; once connected, conversations flow in as the agent's inbox and it can reply and send notifications out.",
    sections: [
      {
        heading: "Connecting a channel",
        items: [
          {
            term: "Connect / Edit",
            desc: "Open a channel's card to enter its account details (bot token, allowed chats, webhook address…). Required fields are marked; secrets are stored encrypted in the vault and never shown back.",
          },
          {
            term: "Connected vs needs setup",
            desc: "A green 'connected' badge means the required credentials are present. 'Needs setup' means it's listed but not yet configured.",
          },
          {
            term: "Restart to apply",
            desc: "Saved settings take effect when the daemon restarts. Fields already set in the environment are shown read-only (the environment wins).",
          },
        ],
      },
      {
        heading: "Two-way vs outbound",
        items: [
          {
            term: "Two-way",
            desc: "Telegram, WhatsApp, Slack, Discord, Matrix, SMS, Signal, and the generic webhook can receive messages that drive the agent (within their allowlist) and reply.",
          },
          {
            term: "Outbound only",
            desc: "Email, Teams, and Home Assistant deliver notifications and agent messages out, but don't take inbound commands.",
          },
        ],
      },
    ],
    tips: [
      "An empty allowlist is fail-closed: the channel can still send out, but won't act on inbound messages until you list who's allowed.",
    ],
    related: [
      { id: "inbox", label: "Inbox" },
      { id: "config", label: "Config Center" },
      { id: "chat", label: "Chat" },
    ],
  },

  market: {
    title: "Marketplace",
    intro:
      "Install ready-made capability packs in one click. A pack bundles skills, MCP servers, and the CLI tools they need — installing it wires those into the systems that already run them, so your agents can use them immediately.",
    sections: [
      {
        heading: "What you see",
        items: [
          {
            term: "Pack cards",
            desc: "Each card shows the pack name, version, a one-line description, and its contents at a glance — how many skills, MCP servers, and CLI tools it carries.",
          },
          {
            term: "Signed badge",
            desc: "A shield marks whether the pack is cryptographically signed. Unsigned packs still install (you'll see a note), signed ones are verified first.",
          },
          {
            term: "Search & categories",
            desc: "Filter by category or type to search names, descriptions, and tags. The built-in Official marketplace works fully offline.",
          },
        ],
      },
      {
        heading: "Installing",
        items: [
          {
            term: "Install",
            desc: "Materializes the pack: its skills enter the Forge (active and retrievable), its MCP servers are registered, and any CLI tools it needs are reported so you can add them in the Toolbox.",
          },
          {
            term: "Uninstall",
            desc: "Reverses exactly what the pack added — its skills are quarantined and its MCP servers removed. Shared or other-agent resources are never touched.",
          },
        ],
      },
    ],
    tips: [
      "A pack only describes the CLI tools it needs — it never installs them on the host silently. Install those from the Toolbox once.",
    ],
    related: [
      { id: "skills", label: "Skills" },
      { id: "mcp", label: "MCP Servers" },
      { id: "toolbox", label: "Toolbox" },
    ],
  },

  system: {
    title: "System",
    intro:
      "The daemon's status board: operational state, live counters, delegation limits, HTTP surface, credentials, and provider routing — refreshed every few seconds.",
    sections: [
      {
        heading: "What's shown",
        items: [
          {
            term: "Operational banner",
            desc: "Green when healthy — with model, uptime, and version. A red pulsing badge means the kernel is halted.",
          },
          {
            term: "Live counters",
            desc: "Active runs, pending approvals, journal head, tools, memory records, world entities, active skills, and schedules.",
          },
          {
            term: "Delegation card",
            desc: "Whether delegation is enabled and its guardrails: max depth, fan-out, and spend per delegation tree.",
          },
          {
            term: "HTTP surface",
            desc: "Every address the daemon listens on, with a loopback badge when it's local-only.",
          },
          {
            term: "Credentials & routing",
            desc: "The credential chain in use, plus provider fallback count and the most recent fallback reason.",
          },
        ],
      },
    ],
    related: [
      { id: "health", label: "Health" },
      { id: "config", label: "Config" },
      { id: "policy", label: "Policy" },
    ],
  },

  persona: {
    title: "Default Identity",
    intro:
      "The daemon's default identity instructions — used by runs that are not bound to a roster agent. Edit it here and the very next default-identity run uses it; no restart.",
    sections: [
      {
        heading: "Editing",
        items: [
          {
            term: "The editor",
            desc: "One large textarea with a character count, unsaved-changes warning, and Save / Discard / Clear buttons.",
          },
          {
            term: "Presets",
            desc: "Three starters — Terse & proactive, Careful & explicit, Friendly concierge — that replace the editor content wholesale as a starting point.",
          },
          {
            term: "Status line",
            desc: "Shows whether custom default identity instructions are active or the built-in default is in effect.",
          },
        ],
      },
    ],
    tips: [
      "Saving empty instructions reverts to the built-in default — that's the intended way to reset.",
      "For a one-off identity change, use the per-thread identity override in Chat instead of editing the daemon default.",
    ],
    related: [
      { id: "prompts", label: "Prompts" },
      { id: "roster", label: "Roster" },
      { id: "chat", label: "Chat" },
    ],
  },

  prompts: {
    title: "Prompts",
    intro:
      "Your saved prompt library — reusable starters offered on Chat's empty state, editable and reorderable here.",
    sections: [
      {
        heading: "Managing the library",
        items: [
          {
            term: "Editor rows",
            desc: "Each prompt is a title + text pair, with add, remove, and up/down reorder controls. Blank rows are dropped on save.",
          },
          {
            term: "Save / discard",
            desc: "Changes are staged in the editor until you save; an unsaved-changes warning keeps you honest.",
          },
          {
            term: "Import / export",
            desc: "Round-trip as JSON. Import merges into the editor and dedupes on title + text, so re-importing is safe.",
          },
        ],
      },
    ],
    related: [
      { id: "chat", label: "Chat" },
      { id: "persona", label: "Default Identity" },
    ],
  },

  configcenter: {
    title: "Config Center",
    intro:
      "The editable side of configuration: schema-driven forms over the daemon's config store and vault, covering built-in sections and any sections plugins have registered.",
    sections: [
      {
        heading: "Editing",
        items: [
          {
            term: "Sections & search",
            desc: "Settings are grouped into section cards with a sticky nav; the search box filters fields by label or env name across all sections.",
          },
          {
            term: "Field types",
            desc: "Text, password, number, boolean, CSV, and select inputs — each rendered appropriately.",
          },
          {
            term: "Live vs restart",
            desc: "Every field is badged: live fields apply immediately, restart fields take effect on the next daemon start.",
          },
        ],
      },
      {
        heading: "Provenance & protection",
        items: [
          {
            term: "Env-pinned fields",
            desc: "Values forced by environment variables are read-only here — the env always wins.",
          },
          {
            term: "Secrets",
            desc: "Write-only: the UI shows only \"set\" / \"not set\", never the value itself.",
          },
          {
            term: "Locked fields",
            desc: "Can be changed but not cleared.",
          },
        ],
      },
    ],
    tips: [
      "The read-only Config page shows the effective merged result of everything — useful to verify what actually took effect.",
    ],
    related: [
      { id: "config", label: "Config" },
      { id: "providers", label: "Providers" },
      { id: "backup", label: "Backup" },
    ],
  },

  config: {
    title: "Config",
    intro:
      "A read-only snapshot of the daemon's effective configuration: environment variables grouped by area, path mappings, and routing.",
    sections: [
      {
        heading: "What's shown",
        items: [
          {
            term: "Summary stats",
            desc: "Active model, whether default identity instructions are set, tool and plugin counts, and the ask policy.",
          },
          {
            term: "Grouped settings",
            desc: "Variables bucketed into Provider, Channels, Interfaces, Autonomy, Security, Tools, and Other — each section badged with its key count.",
          },
          {
            term: "Paths & routing",
            desc: "Filesystem path mappings and the routing configuration as raw JSON.",
          },
        ],
      },
    ],
    tips: [
      "Nothing here is editable by design — change values in Config Center and verify the result here.",
    ],
    related: [
      { id: "configcenter", label: "Config Center" },
      { id: "system", label: "System" },
    ],
  },

  connections: {
    title: "Connections",
    intro:
      "One cockpit for everything you've wired up — AI providers, communication channels, and MCP servers — and what still needs connecting. Read-only; each section links to the place you manage it.",
    sections: [
      {
        heading: "What it shows",
        items: [
          {
            term: "AI Providers",
            desc: "How many providers are keyed (have a usable credential) out of the catalog total, with the connected ones listed. 'Connect a provider' jumps to Quick Connect.",
          },
          {
            term: "Channels",
            desc: "Channels that are live (running) and those configured-but-not-yet-started (restart to start). 'Manage channels' opens the Channels wizard.",
          },
          {
            term: "MCP Servers",
            desc: "MCP servers attached to the agent (and those enabled but not attached). 'Manage MCP' opens the MCP view.",
          },
        ],
      },
    ],
    tips: ["Green = connected/live/attached; amber = configured but needs a restart or attach."],
    related: [
      { id: "quickconnect", label: "Quick Connect" },
      { id: "channels", label: "Channels" },
      { id: "mcp", label: "MCP Servers" },
    ],
  },
  quickconnect: {
    title: "Quick Connect",
    intro:
      "A branded gallery to connect an AI provider in one step: pick a card, paste an API key, and the provider is registered and keyed — no manual catalog or endpoint setup.",
    sections: [
      {
        heading: "Connecting a provider",
        items: [
          {
            term: "Pick a card, paste a key",
            desc: "Each card targets a coding/token-plan provider (Z.ai/GLM, MiniMax, Kimi/Moonshot, DeepSeek, opencode and more) or a popular OpenAI-compatible endpoint. Paste the key and press Connect — the provider is written to the catalog's custom layer and the key stored in the keyring, live, no restart.",
          },
          {
            term: "Endpoint & model are editable",
            desc: "Each card pre-fills the base URL and a default model, but both are editable under the 'Endpoint & model' disclosure — so a provider with a non-standard endpoint still works.",
          },
          {
            term: "Custom provider",
            desc: "The Custom card connects any OpenAI- or Anthropic-compatible API: give it a name, base URL, key env var, model, and key.",
          },
          {
            term: "Get a key",
            desc: "Every card links to the provider's API-key page so you can grab a key without leaving the flow.",
          },
        ],
      },
    ],
    tips: [
      "Connecting only stores the provider and key — set it as the default or wire it into routing afterwards from Models/Routing.",
    ],
    related: [
      { id: "models", label: "Models" },
      { id: "routing", label: "Routing" },
      { id: "providers", label: "Providers" },
    ],
  },
  providers: {
    title: "Providers",
    intro:
      "The routing monitor: how many LLM calls were routed, how often fallbacks kicked in, and which providers actually served the traffic.",
    sections: [
      {
        heading: "Reading it",
        items: [
          {
            term: "Fallback ring",
            desc: "The fallback rate as a color-coded gauge — green when primaries hold, red when they don't.",
          },
          {
            term: "Tiles & bars",
            desc: "Routed call count, fallback count, provider count, and a routes-by-provider bar chart.",
          },
          {
            term: "Routing activity log",
            desc: "The last 50 routing events — normal decisions and fallbacks color-coded, with timestamps and truncated failure reasons.",
          },
        ],
      },
      {
        heading: "Actions",
        items: [
          {
            term: "Reload",
            desc: "Re-reads credentials and the catalog without restarting the daemon — use after adding keys outside the UI.",
          },
          {
            term: "Refresh",
            desc: "Just re-fetches the stats — light, and also happens automatically every few seconds.",
          },
        ],
      },
    ],
    related: [
      { id: "models", label: "Models" },
      { id: "routing", label: "Routing" },
      { id: "health", label: "Health" },
    ],
  },

  models: {
    title: "Models",
    intro:
      "The model catalog — every provider and model synced from models.dev — plus per-provider API-key management.",
    sections: [
      {
        heading: "The catalog",
        items: [
          {
            term: "Sync",
            desc: "Pulls the latest catalog (same source as `agt catalog sync`) and reports what changed. Timestamp and source URL are shown.",
          },
          {
            term: "Provider cards",
            desc: "Expandable; each shows a keyed / no-key badge and model count. Expanded, you get the full model table: context window, input/output price per million tokens, and capability badges (tool-calling, reasoning).",
          },
          {
            term: "Search",
            desc: "Filters across provider and model names at once.",
          },
        ],
      },
      {
        heading: "Keys",
        items: [
          {
            term: "Key manager",
            desc: "Store several keys per provider and pick which one is active. Keys are write-only — only a last-4 fingerprint is ever shown back.",
          },
        ],
      },
    ],
    tips: [
      "Model pickers across the console only offer models from keyed providers — if a model is missing, add a key here first.",
    ],
    related: [
      { id: "providers", label: "Providers" },
      { id: "routing", label: "Routing" },
      { id: "setup", label: "Setup" },
    ],
  },

  routing: {
    title: "Routing",
    intro:
      "Per-task model chains: for each task type, an ordered list — primary first, then fallbacks — that the governor walks when a model fails.",
    sections: [
      {
        heading: "Editing chains",
        items: [
          {
            term: "Task types",
            desc: "Chat, plan, code, delegate, and the rest — each gets its own chain. Known types are listed with help text; custom types sort after.",
          },
          {
            term: "Chain rows",
            desc: "The primary wears a badge; fallbacks are numbered. Reorder with the arrows, remove with the ×, add models via the picker.",
          },
          {
            term: "Fallback activity",
            desc: "Each chain shows how often it actually fell back, including the last failed→next transition and the reason.",
          },
          {
            term: "Save / discard",
            desc: "Changes stage in the editor until saved.",
          },
        ],
      },
    ],
    tips: [
      "An empty chain means \"daemon default\" — deleting all rows is how you hand a task type back to the default model.",
      "Import merges and overrides per task type; export gives you the whole table as JSON.",
    ],
    related: [
      { id: "models", label: "Models" },
      { id: "providers", label: "Providers" },
      { id: "chains", label: "Fallback Chains" },
    ],
  },

  chains: {
    title: "Fallback Chains",
    intro:
      "Named, reusable model ladders. A chain is an ordered list of models tried in turn; pick a chain anywhere you pick a model — agents, routing, chat — and the governor expands it at run time. Edit a chain in one place and every reference updates.",
    sections: [
      {
        heading: "Managing chains",
        items: [
          {
            term: "Create & name",
            desc: "New chain makes an empty ladder; rename to a slug (lower-case, digits, dashes). The name is how it's referenced as @name elsewhere.",
          },
          {
            term: "Ordering models",
            desc: "The primary wears a badge; fallbacks are numbered. Reorder with the arrows, remove with the ×, add models via the picker. A chain may not reference another chain.",
          },
          {
            term: "Default chain",
            desc: "Star one chain as the default — any run that resolves to no chain of its own (no agent, task, or explicit model) uses it, so even a bare agent gets a fallback ladder.",
          },
          {
            term: "Save",
            desc: "Changes stage in the editor until saved; saving applies live and persists. Unknown model ids are flagged but not rejected.",
          },
        ],
      },
    ],
    tips: [
      "Selecting @chain for an agent replaces its model and per-agent fallbacks — the chain is self-contained.",
      "Deleting a chain makes dangling @name references fall through to the default chain (or the daemon model).",
    ],
    related: [
      { id: "routing", label: "Routing" },
      { id: "models", label: "Models" },
      { id: "agents", label: "Agents" },
    ],
  },

  tools: {
    title: "Tools",
    intro:
      "The tool-usage monitor: call volume, error rate, per-tool latency, and a live invocation log across built-in, MCP, forged, and skill tools.",
    sections: [
      {
        heading: "The gallery",
        items: [
          {
            term: "Error ring & tiles",
            desc: "Overall error rate, total calls, errored calls, and how many distinct tools were used.",
          },
          {
            term: "Tool rows",
            desc: "Name, a source badge (mcp / forged / skill / built-in), call count (or \"idle\"), the governing capability, error count, and average latency. Used tools sort first.",
          },
          {
            term: "Search & capability chips",
            desc: "Filter by name or click a capability chip to see only tools under that capability.",
          },
          {
            term: "Invocation log",
            desc: "Recent calls with success/error coloring, latency, and an input → output preview.",
          },
        ],
      },
    ],
    tips: [
      "This page is usage; the Catalog page is permissions — same tools, different question.",
    ],
    related: [
      { id: "catalog", label: "Catalog" },
      { id: "mcp", label: "MCP Servers" },
      { id: "toolforge", label: "Tool Forge" },
    ],
  },

  catalog: {
    title: "Catalog",
    intro:
      "The agent's capability surface: every tool it can call, what each does, which capability governs it, the current trust level, and usage stats.",
    sections: [
      {
        heading: "The grid",
        items: [
          {
            term: "Tool cards",
            desc: "Name, description, governing capability, call count and errors (or \"unused\").",
          },
          {
            term: "Trust level dropdown",
            desc: "Grant or restrict each tool live by setting its level from L0 (blocked) to L4 (fully trusted) — the same levels the Policy page manages in bulk.",
          },
        ],
      },
    ],
    tips: [
      "Level colors encode confidence at a glance — red is restricted, green is trusted.",
    ],
    related: [
      { id: "policy", label: "Policy" },
      { id: "tools", label: "Tools" },
    ],
  },

  policy: {
    title: "Policy",
    intro:
      "The capability control center: trust levels per capability, the ask-mode, hard-deny rules, and tools to test decisions and the secret redactor — all live at runtime.",
    sections: [
      {
        heading: "Trust & gating",
        items: [
          {
            term: "Trust-level bar",
            desc: "The distribution of capabilities across L0–L4, color-coded from red (blocked) to green (trusted).",
          },
          {
            term: "Capability grid",
            desc: "A dropdown per capability to move it between levels live.",
          },
          {
            term: "Ask mode",
            desc: "Global allow / prompt / deny behavior for capabilities that gate on asking.",
          },
          {
            term: "Hard-deny rules",
            desc: "Substring rules that block matching inputs outright. Add new ones with an optional scope; only runtime-added rules are removable from the UI.",
          },
        ],
      },
      {
        heading: "Testing",
        items: [
          {
            term: "Policy test",
            desc: "Dry-run a decision for a capability + input and see the verdict — read-only, nothing is mutated.",
          },
          {
            term: "Redaction check",
            desc: "Paste text and see what the secret redactor would scrub.",
          },
          {
            term: "Decision log",
            desc: "Recent policy decisions with the overall denial rate.",
          },
        ],
      },
    ],
    tips: [
      "Approvals is where \"ask\" verdicts land — this page decides what gets asked in the first place.",
    ],
    related: [
      { id: "approvals", label: "Approvals" },
      { id: "catalog", label: "Catalog" },
    ],
  },

  cache: {
    title: "Cache",
    intro:
      "The prompt-cache savings monitor: how much money caching saved, the read/write token split, and how many priced calls were covered.",
    sections: [
      {
        heading: "What's shown",
        items: [
          {
            term: "Savings hero",
            desc: "Total dollars saved by prompt caching.",
          },
          {
            term: "Token tiles",
            desc: "Cache-read tokens, cache-write tokens, and covered call count (priced calls only).",
          },
          {
            term: "Read/write split",
            desc: "A ring gauge plus breakdown bar — heavy reads relative to writes means the cache is earning its keep.",
          },
        ],
      },
    ],
    tips: [
      "An empty page just means no cache-priced calls have happened yet — it fills in as traffic flows.",
    ],
    related: [
      { id: "budget", label: "Budget" },
      { id: "insights", label: "Insights" },
    ],
  },

  storage: {
    title: "Storage",
    intro:
      "What under the daemon's home directory (~/.agezt) is taking the space, and the collectors that reclaim it. Every subsystem owns one subdirectory, so the breakdown is a faithful inventory of where the bytes live.",
    sections: [
      {
        heading: "The breakdown",
        items: [
          {
            term: "Summary band",
            desc: "Total bytes and file count under the home dir, filesystem free space (red below 10%), and the largest subsystem at a glance.",
          },
          {
            term: "Per-directory bars",
            desc: "Each top-level subdirectory with what lives there, its file count, size, and share of the total. The journal is append-only and full-retention, so it growing forever is by design — everything else is reclaimable.",
          },
        ],
      },
      {
        heading: "Collectors",
        paragraphs: [
          "Every destructive collector is dry-run first: it reports the candidates and asks for confirmation before deleting anything.",
        ],
        items: [
          {
            term: "Artifact collect",
            desc: "Reaps stored files (inbound images, tool outputs) older than the threshold. Content blobs are kept while any other entry still references the same bytes.",
          },
          {
            term: "Memory prune",
            desc: "Hard-removes soft-deleted memory records (tombstoned or superseded) past their recovery window. Active memories are never touched.",
          },
          {
            term: "Memory consolidate",
            desc: "Compacts the brain: clusters near-duplicate memories and merges each cluster into one richer record. Originals are superseded, not deleted — recoverable until the next prune.",
          },
          {
            term: "Reaper scan",
            desc: "Read-only detection of roster agents idle for 30+ days and the stale artifact pile. Nothing is deleted from here — retire agents in the Roster, collect artifacts with the collector above.",
          },
        ],
      },
    ],
    tips: [
      "Run the dry-run freely — nothing is ever deleted without the confirm dialog.",
      "A full disk is the classic silent outage: the journal can no longer write and the daemon stops recording. Watch the free-space card.",
    ],
    related: [
      { id: "files", label: "Files" },
      { id: "memory", label: "Memory" },
      { id: "roster", label: "Roster" },
      { id: "health", label: "Health" },
    ],
  },

  backup: {
    title: "Backup",
    intro:
      "Export and restore in three scopes: this browser's appearance, the daemon's config bundle, and a full snapshot of everything customizable.",
    sections: [
      {
        heading: "The three scopes",
        items: [
          {
            term: "Appearance",
            desc: "Theme, accent, console name — browser-local settings that live on this device only.",
          },
          {
            term: "Daemon config",
            desc: "Default identity, prompt templates, and routing chains — the bundle shows its current contents before you export.",
          },
          {
            term: "Full snapshot",
            desc: "Everything customizable in one file — best for seeding a fresh daemon. Restore shows counts of what's inside and requires confirmation.",
          },
        ],
      },
      {
        heading: "Restore semantics",
        paragraphs: [
          "Memory and the world model deduplicate on import (content-addressed), so re-importing is safe. Standing orders and schedules are additive — importing the same snapshot twice can create duplicates there, and the confirm dialog spells that out.",
        ],
      },
    ],
    related: [
      { id: "configcenter", label: "Config Center" },
      { id: "memory", label: "Memory" },
      { id: "schedules", label: "Schedules" },
    ],
  },

  wizards: {
    title: "Wizards",
    intro:
      "Guided, step-by-step flows that complete a whole task in a focused overlay — so you don't have to hunt through menus. Each wizard reuses the same forms and endpoints the dedicated views do; it just sequences them for you.",
    sections: [
      {
        heading: "Available flows",
        items: [
          { term: "Connect a provider", desc: "Sync the catalog, add an API key, and pick a default model — the first-run Setup flow, reachable any time." },
          { term: "Create an agent", desc: "Give a new roster agent its soul, model, and daily budget, then run it by name." },
          { term: "Schedule a task", desc: "Have the daemon run a typed target on a cadence — every N minutes, daily, or once." },
        ],
      },
      {
        heading: "How it works",
        paragraphs: [
          "Pick a card to open the wizard as a focused overlay; finish or close to return. Nothing here is new daemon behaviour — wizards are a launcher over the existing actions, so more will be added over time.",
        ],
      },
    ],
    related: [
      { id: "setup", label: "Setup" },
      { id: "roster", label: "Roster" },
      { id: "schedules", label: "Schedules" },
    ],
  },
};

/** Fallback topic for a view id with no entry (should be caught by tests). */
export const FALLBACK_TOPIC: HelpTopic = {
  title: "Help",
  intro: "No detailed guide has been written for this page yet.",
  sections: [
    {
      heading: "General navigation",
      paragraphs: [
        "Use the sidebar to move between views, or press ⌘K / Ctrl+K for the command palette. Most pages update live from the daemon's event stream — no manual refreshing needed.",
      ],
    },
  ],
};

export function helpTopicFor(viewId: string): HelpTopic {
  return HELP[viewId] ?? FALLBACK_TOPIC;
}
