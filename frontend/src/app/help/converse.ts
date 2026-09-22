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
      { id: "agents", label: "Agents" },
      { id: "connections", label: "Connections" },
    ],
  },

  jarvis: {
    title: "Jarvis",
    intro:
      "Your operator-side companion. Where Chat is the long-form conversation, Jarvis is the 'what's my agent doing right now?' glance — readiness, recent runs, and one-click nudges.",
    sections: [
      {
        heading: "What it answers",
        items: [
          { term: "Who's online", desc: "The roster, with each agent's readiness chip — green for ready, dim for offline or paused." },
          { term: "What's running", desc: "The most recent runs with their status and the active correlation id, refreshed every ten seconds." },
          { term: "Quick prompts", desc: "Four one-click starters (summarize, diagnose, blockers, stand-up note) so a small nudge doesn't need a fresh sentence." },
        ],
      },
      {
        heading: "How it talks to Chat",
        items: [
          { term: "Quick prompts", desc: "Send to the same ChatEngine the floating MiniChat uses — there's only one thread per conversation across Jarvis, Chat, and MiniChat." },
          { term: "Open chat", desc: "Switches the active conversation thread and jumps you to Talk › Chat to read the stream." },
        ],
      },
    ],
    tips: [
      "Jarvis pulls from /api/agents and /api/runs every 30 / 10 seconds; the numbers will lag a real-time change by at most that long.",
    ],
    related: [
      { id: "chat", label: "Chat" },
      { id: "runs", label: "Runs" },
      { id: "roster", label: "Roster" },
    ],
  },

  chat: {
    title: "Chat",
    intro:
      "Full-page conversation surface bound to the same ChatEngine that powers the floating MiniChat. One active thread across the two — switch in Chat, the MiniChat keeps the same conversation.",
    sections: [
      {
        heading: "Left rail",
        items: [
          { term: "Threads", desc: "Pinned conversations first, then by recency. Pin / rename / delete via the hover buttons." },
          { term: "Active thread", desc: "Highlighted in the rail. Its title is auto-derived from the first user message; rename via the + button." },
        ],
      },
      {
        heading: "Top bar",
        items: [
          { term: "Model / Agent / Profile / Persona", desc: "Per-conversation overrides — change once, that thread runs with the new identity until you reset it." },
          { term: "Auto-approve forge / Trust web content", desc: "Session-scoped trust grants. Auto-approve skips HITL prompts when the agent forges new tools; trust web skips the prompt-injection guard for operator-driven research loops." },
          { term: "Stop / New", desc: "Stop halts the active run; New starts a fresh thread (the previous one is preserved)." },
        ],
      },
      {
        heading: "Composer",
        items: [
          { term: "Enter to send, Shift+Enter for newline", desc: "Standard. While a run streams, Enter queues the message — M962 — and it sends as the run finishes." },
          { term: "Queue indicator", desc: "Shows the front queued message and whether it auto-sends or waits. Open Chat to manage the queue manually." },
        ],
      },
    ],
    tips: [
      "State persists per browser: same store as MiniChat, so switching between the two never loses your place.",
      "History briefing (M925) keeps long threads from silently losing their start — the daemon folds older turns into one summary.",
    ],
    related: [
      { id: "jarvis", label: "Jarvis" },
      { id: "runs", label: "Runs" },
      { id: "council", label: "Council" },
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
      { id: "data", label: "Data Lake" },
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

};
