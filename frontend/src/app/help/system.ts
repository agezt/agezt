// app/help/system.ts — the System section of the in-app manual.
// Day 5b split (see scripts/dev/split-help-topics.py).
// Topics: overview, setup, toolbox, channels, market, persona, prompts, configcenter, connections, providers, models, routing, chains, tools, catalog, policy, cache, storage, backup, wizards, workboard, incident
//
// Each file exports a Record<string, HelpTopic> with just the
// topics that fall under the System section. The aggregator in
// help.ts spreads all six into the single HELP record. Topics
// remain plain data so they stay testable and tree-shakeable;
// <HelpDrawer> owns all presentation.

import type { HelpTopic } from "./types";

export const System: Record<string, HelpTopic> = {
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
      { id: "models", label: "Models" },
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
{ id: "configcenter", label: "Config Center" },
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
      { id: "skills", label: "Skills" },
    ],
  },

  backup: {
    title: "Backups",
    intro:
      "The rollback surface. Before any file-mutating action the daemon captures a checkpoint listing the affected paths and a snapshot of their current contents — you can roll any of them back from here.",
    sections: [
      {
        heading: "Reading the list",
        items: [
          {
            term: "When / ID / Run / Kind / Description",
            desc: "One row per checkpoint. The kind tells you what kind of mutation created it (config, skill, channel, world, …); the run column links to the originating correlation id.",
          },
          {
            term: "Affected paths",
            desc: "The files that will be reverted when you restore this checkpoint. Up to three are shown inline; the rest are a '+N more' hint.",
          },
          {
            term: "Size",
            desc: "How big the snapshot is. Config writes are tiny; skill imports and world edits are larger.",
          },
        ],
      },
      {
        heading: "Restoring",
        items: [
          {
            term: "Restore",
            desc: "Opens a confirm dialog listing every affected path. Confirming calls /api/rollback/apply and reloads the list.",
          },
          {
            term: "Irreversible checkpoints",
            desc: "If the snapshot is missing or the path is read-only, the Restore button disables itself. The daemon will not apply a half-broken rollback.",
          },
          {
            term: "Filter by run id",
            desc: "Paste a correlation id into the Run ID box and click Refresh to narrow the list to one run's worth of mutations — useful when investigating an incident.",
          },
        ],
      },
    ],
    tips: [
      "Restoring a checkpoint does NOT undo the journal entry that described the original mutation — the journal is append-only. The rollback is a new event of its own.",
    ],
    related: [
      { id: "configcenter", label: "Config Center" },
      { id: "analyst", label: "Analyst" },
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
      { id: "configcenter", label: "Config Center" },
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
            desc: "How many providers are keyed (have a usable credential) out of the catalog total, with the connected ones listed. 'Add provider' switches to the Provider Keys tab in this view, where the catalog is the source of truth for env var names.",
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
      { id: "models", label: "Models & Keys" },
      { id: "channels", label: "Channels" },
      { id: "mcp", label: "MCP Servers" },
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
      { id: "setup", label: "Setup" },
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
      { id: "models", label: "Models" },
      { id: "agents", label: "Agents" },
    ],
  },

  approvals: {
    title: "Approvals",
    intro:
      "Human-in-the-loop queue. Capabilities the policy refuses to auto-decide land here for the operator to grant or deny. The header bell is the count; this page is the long form, with filters and history.",
    sections: [
      {
        heading: "Pending",
        items: [
          { term: "Each row", desc: "Capability name + the agent's reason for asking. Click Grant to allow once or Deny to refuse. Decisions are immediate — the agent resumes on the next tick." },
          { term: "Live updates", desc: "New requests appear here without a refresh; the daemon publishes approval.* events the page subscribes to. The header bell and the page count stay in lock-step." },
        ],
      },
      {
        heading: "History",
        items: [
          { term: "Granted / Denied", desc: "Filter chips toggle between granted and denied decisions. The list is read-only — once decided, the row stays for audit but the daemon won't re-ask the same question." },
          { term: "Empty state", desc: "On a fresh install the history is empty until at least one decision is made. That's normal — the policy engine only escalates when it's uncertain." },
        ],
      },
    ],
    tips: [
      "Grant once doesn't grant forever: the next time the agent hits a denied capability, the question returns here.",
      "Trust web content (in Chat) and auto-approve forge (in Chat) both bypass this queue — that's by design, the operator accepts a broader blast radius in exchange for less interruption.",
    ],
    related: [
      { id: "policy", label: "Policy" },
      { id: "chat", label: "Chat" },
      { id: "runs", label: "Runs" },
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
    ],
  },

  "execution-profiles": {
    title: "Execution Profiles",
    intro:
      "Named runtimes for anything the agents execute: which interpreter or shell runs a command, with which working directory and environment. A profile is picked by name instead of hard-coding a path in every tool.",
    sections: [
      {
        heading: "What a profile defines",
        items: [
          {
            term: "Interpreter / shell",
            desc: "The binary that actually runs the code — a python, a node, a shell — plus the arguments it needs. This is what makes the same script portable between machines.",
          },
          {
            term: "Environment",
            desc: "The variables and working directory the command sees. Keeping them in a profile means a change lands everywhere that profile is used, at once.",
          },
          {
            term: "Check",
            desc: "Verifies the profile against this host before you rely on it: is the interpreter present, does it run, does it report the version you expect.",
          },
        ],
      },
    ],
    tips: [
      "Run the check after changing a machine — a profile that pointed at a since-removed interpreter fails at the worst moment otherwise.",
    ],
    related: [
      { id: "sandbox", label: "Sandbox" },
    ],
  },

  incident: {
    title: "Incident",
    intro:
      "One incident's full page: what the autonomy layer flagged, the agent it concerns, the evidence from the journal, and the actions that resolve it. Reached from Autonomy or from any incident badge.",
    sections: [
      {
        heading: "Reading the page",
        items: [
          {
            term: "Timeline",
            desc: "The journal events that led to the incident, newest first, so the sequence that produced the failure is readable rather than inferred.",
          },
          {
            term: "The agent",
            desc: "Who the incident is about, with its live state. Escalations raised by that agent appear alongside, since they are usually the same story.",
          },
        ],
      },
      {
        heading: "Acting on it",
        items: [
          {
            term: "Repair / wake / retire",
            desc: "The lifecycle controls, applied in context: attempt a self-repair, wake the agent to try again, or retire it if it should stop running at all.",
          },
          {
            term: "Resolve",
            desc: "Closes the incident. Resolving is recorded in the journal like any other decision, so the audit trail shows who called it done.",
          },
        ],
      },
    ],
    related: [
      { id: "autonomy", label: "Autonomy" },
      { id: "roster", label: "Roster" },
      { id: "runs", label: "Runs" },
    ],
  },
};
