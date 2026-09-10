// app/help/converse.ts — the Converse section of the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).
// Topics: jarvis, chat, voice, inbox, artifacts, data, board, approvals
//
// Each file exports a Record<string, HelpTopic> with just the
// topics that fall under the Converse section. The aggregator in
// help.ts spreads all six into the single HELP record. Topics
// remain plain data so they stay testable and tree-shakeable;
// <HelpDrawer> owns all presentation.

import type { HelpTopic } from "./types";

export const Converse: Record<string, HelpTopic> = {
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
      { id: "artifacts", label: "Artifacts & Files" },
      { id: "chat", label: "Chat" },
    ],
  },

  artifacts: {
    title: "Artifacts & Files",
    intro:
      "The showroom for everything your agents produce: reports, charts, generated pages, code, data files — bucketed by what each artifact IS, with a live preview per type and a fullscreen viewer for the big screen.",
    sections: [
      {
        heading: "Two modes",
        items: [
          {
            term: "Gallery",
            desc: "Everything the daemon stored, bucketed by what each artifact IS — image, svg, html, pdf, markdown, json, code, text — with a live preview per type. Search filters by name, caption, source or sender.",
          },
          {
            term: "File manager",
            desc: "The same store browsed by PATH instead of by kind: a folder tree, a listing, and a detail pane. This was the separate 'Files' view until the two were merged; #files still opens the console straight into this mode.",
          },
          {
            term: "Collect",
            desc: "Reaps stale artifacts older than 30 days. It dry-runs first and tells you the count and bytes, so the confirm is an informed one; the most recent files are always kept.",
          },
          {
            term: "Show run outputs",
            desc: "Large tool outputs are offloaded to the artifact store and would otherwise bury real files, so they are hidden until you ask. Nothing is lost — the bytes stay reachable from the run that produced them.",
          },
        ],
      },
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
      { id: "artifacts", label: "Artifacts & Files" },
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
      { id: "artifacts", label: "Artifacts & Files" },
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

};
