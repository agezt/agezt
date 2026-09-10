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
      { id: "tools", label: "Tool usage" },
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
      { id: "configcenter", label: "Config Center" },
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

  persona: {
    title: "Default Identity",
    intro:
      "The daemon's default identity instructions — used by runs that are not bound to a roster agent. Edit it here and the very next default-identity run uses it; no restart.",
    sections: [
      {
        heading: "Effective configuration",
        items: [
          {
            term: "What it shows",
            desc: "The read-only inventory folded in at the bottom of the page: every AGEZT_* value the daemon actually sees, grouped by area, plus its resolved paths and routing keys. This was the separate 'Config' view; #config still lands here.",
          },
          {
            term: "When to open it",
            desc: "When a setting you exported does not seem to apply. The editors above write the config store; this pane answers the different question of whether the environment reached the daemon at all.",
          },
        ],
      },
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
      { id: "configcenter", label: "Config Center" },
      { id: "providers", label: "Providers" },
      { id: "backup", label: "Backup" },
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
    title: "Tool usage",
    intro:
      "The tool-usage monitor: how much each tool is actually being called, how often it errors, how slow it is, and a live invocation log. It does not list the tools themselves — the Tool registry does.",
    sections: [
      {
        heading: "The monitor",
        items: [
          {
            term: "Error ring & tiles",
            desc: "Overall error rate (drawn once there are calls to rate), total calls, errored calls, and how many distinct tools were used.",
          },
          {
            term: "Usage by tool",
            desc: "One bar per tool that has been called, longest first, with its call count, error share and average latency. Tools that have never been called are not here — look in the Tool registry.",
          },
          {
            term: "Invocation log",
            desc: "Recent calls with success/error coloring, latency, and an input → output preview.",
          },
        ],
      },
    ],
    tips: [
      "This page answers \"what is being called\". The Tool registry answers \"what exists, and under what policy\" — that is where the full list, the search and the trust levels live.",
    ],
    related: [
      { id: "catalog", label: "Tool registry" },
      { id: "mcp", label: "MCP Servers" },
      { id: "toolforge", label: "Tool Forge" },
    ],
  },

  catalog: {
    title: "Tool registry",
    intro:
      "The agent's capability surface: every tool it can call, what each does, which capability governs it, the current trust level, and how much it has been used. Search it by name, description or capability; Tool usage charts the calls.",
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
      { id: "tools", label: "Tool usage" },
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
      { id: "catalog", label: "Tool registry" },
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
      { id: "artifacts", label: "Artifacts & Files" },
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
  workboard: {
    title: "Workboard",
    intro:
      "The task board the fleet actually works from: lanes of work items, each dispatched to a seat or agent, each carrying the acceptance criteria that decide whether it is genuinely done.",
    sections: [
      {
        heading: "Lanes and items",
        items: [
          {
            term: "Lanes",
            desc: "Work moves left to right through the lanes. An item shows who holds it, what it is blocked on, and how long it has been sitting there.",
          },
          {
            term: "Dispatch",
            desc: "Hands an item to a seat or an agent. Dispatching is what turns a written task into a governed run — the agent picks it up with the item's context attached.",
          },
          {
            term: "Block / unblock",
            desc: "Records why work stopped, in the open, rather than letting an item quietly rot in a lane. The reason travels with the item.",
          },
        ],
      },
      {
        heading: "Proof",
        paragraphs: [
          "An item can carry acceptance criteria, and completing it can be gated on a proof verdict rather than on someone clicking done. That is the difference between a board that tracks intent and one that tracks outcomes.",
        ],
      },
    ],
    tips: [
      "Comments on an item are part of the record — the agent working it can read them.",
      "Objectives (the OKR tab) link down to workboard items, so a key result can show the work actually moving it.",
    ],
    related: [
      { id: "okr", label: "Objectives" },
      { id: "seats", label: "Seats" },
      { id: "roster", label: "Roster" },
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
      { id: "toolbox", label: "Host CLI tools" },
      { id: "sandbox", label: "Sandbox" },
      { id: "toolforge", label: "Tool Forge" },
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
