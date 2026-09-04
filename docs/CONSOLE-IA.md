# Console Information Architecture

> Status: **implemented** (Phases A, B and C, 2026-09-04). This document is
> both the record of what was wrong and the law the console is now held to by
> `frontend/src/nav.test.ts` and `frontend/src/consoledoc.test.ts`.
>
> Written 2026-09-04 after a sweep of
> `frontend/src` (67 nav views, 82 components, 140 lib modules) and the
> control-plane surface behind it (298 `/api/*` routes, ~53k LOC in
> `kernel/controlplane`).
>
> It answers "what is where, and why couldn't I find it?" — sections 1 and 2
> describe the console as it was, section 3 the shape it now has.

## 1. What the console was

```
frontend/src
├── nav.tsx            8 sections → 67 nav items → 67 lazy views   ← the entire IA
├── App.tsx            shell: hash router, ⌘K palette, help drawer, inspector
├── views/             67 views + 2 non-nav (Login, VoiceSetup) + Chat/ roster/ schedules/
├── components/        82 shared components (+ ui/ design system, agentdetail/)
└── lib/               140 modules: one per domain (api, events, chatStore, fleet, …)
```

Routing was — and still is — hash-based and flat: `#<viewId>`, plus two
full-page detail routes (`#agent/<slug>` → `AgentPage`, `#incident/<id>` →
`IncidentPage`).

Every view was reachable; there were **no orphans**. The problem was not
missing structure, it was *too many equally-weighted destinations*.

### Section sizes before

| Section | Items |
|---|---|
| Talk | 5 |
| Observe | 10 |
| Automate | 8 |
| Govern | 6 |
| Knowledge | 9 |
| Connect | 9 |
| Build | 10 |
| Admin | 10 |

Eight sections, none of which fit in one glance, several of which mixed
"watch this" with "change this".

## 2. Root causes of "I can't find anything"

### 2.1 The nav mirrors the backend op namespace, not operator jobs

`kernel/controlplane` exposes ~298 flat protocol ops through a single dispatch
registry. Historically every new kernel package earned a nav item of the same
name, so the console reads as a *package index* rather than a set of jobs.
`kernel/toolbox`, `kernel/toolforge`, `kernel/toolreg`, `kernel/market` and
`kernel/skill` are five packages — and five nav items — for what the operator
experiences as one question: "what can my agents do?"

### 2.2 Twelve views answer "how is it going?"

`Overview`, `Mission Control`, `Health`, `System`, `Activity`, `Insights`,
`Analyst`, `Runs`, `Budget`, `Cache`, `Tools`, `Providers` are all read-only
telemetry, split across three different sections. The overlap is measurable —
these views hit the same endpoints:

| Endpoint | Views hitting it |
|---|---|
| `/api/runs` | AgentPage, Agents, Analyst, Dashboard, Insights, Mission, Overseer, Replay, Runs |
| `/api/agents` | AgentPage, Board, Dashboard, IncidentPage, Overseer, Roster, Schedules, Standing, Voice |
| `/api/journal` | Alerts, Council, Dashboard, IncidentPage, Jarvis, Replay |
| `/api/catalog` | Chains, Connections, Models, QuickConnect, Routing, Setup |
| `/api/config/set` | ConfigCenter, ExecutionProfiles, QuickConnect, Setup, VoiceSetup |
| `/api/artifacts` | Artifacts, ChannelSessions, Files, Inbox |

### 2.3 Monitor and manager shared a label — under the wrong one

The single biggest findability trap in the console:

| Nav label | What it actually is | Where the *management* lives |
|---|---|---|
| **Providers** | a routing/fallback **telemetry log** | Models (keys), Quick Connect, Setup |
| **Tools** | a tool-call **usage monitor** | Catalog, Toolbox, Tool Forge, Marketplace |
| **Catalog** | the tool **registry + trust levels** | — (but named like a model catalog) |
| **Models** | model catalog **and** provider API keys | — (keys are not discoverable from the label) |

"Where do I add an API key?" had no obviously right answer: it was in Models,
Quick Connect and Setup — and *not* in Providers.

### 2.4 Near-duplicate view pairs

| Pair | Overlap |
|---|---|
| `Files` (M823 file manager) vs `Artifacts` (M931 gallery) | same `/api/artifacts` data, two presentations |
| `Config` vs `Config Center` | Config is a raw `/api/config` dump; Config Center is the schema-driven editor |
| `Health` vs `System` (Status) | two vitals dashboards, in two different sections |
| `Agents` vs `Roster` vs `Overseer` | census vs identity CRUD vs live supervision, all agent-shaped |
| `Setup` vs `Quick Connect` vs `Models` | three ways to credential a provider |
| `Inbox` vs `Agent Board` | human threads vs agent threads, presented identically |
| `Workflows` vs `Flow Studio` | saved-workflow CRUD vs the generate/refine canvas |
| `Tool registry` vs `Tool usage` | both drew the whole tool catalogue — name, capability, description — one under call-volume charts |

### 2.5 ⌘K could not find anything by its real-world name

`App.tsx` built view commands with **no `keywords`**:

```ts
const views: CommandItem[] = NAV.map((n) => ({
  id: `view:${n.id}`, label: n.label, group: sectionForView[n.id] || "Go to", run: …
}));
```

`filterCommands` searches `label + group + keywords`. With keywords empty the
palette only matches the label. So "api key", "token", "cron", "webhook",
"deny", "quota", "guardian", "tts" — the words an operator actually types —
return **no matches**, even though every one of them has a home.

Actions and agents carried keywords. Views, the largest group, did not.

### 2.6 The written map was stale

`docs/CONSOLE.md` still documented the pre-M974 six sections
(*Converse / Monitor / Agents / Automation / Knowledge / System*) long after the
code had eight. The only prose map of the console was wrong, and nothing in the
build noticed.

## 3. The architecture

Three principles:

1. **A section is a job, an item is a noun, a tab is a facet.** If two views
   answer the same operator question with different framing, they are tabs of
   one destination — not two nav rows.
2. **Monitor lives with its manager.** Telemetry is a tab on the thing it
   measures, never a separate top-level peer with a colliding name.
3. **No id is ever lost.** Every view id stays a working deep link, a help
   topic and a ⌘K target. Consolidation changes *where a view renders*, never
   *how it is addressed* — and a view that is genuinely merged away keeps its
   hash through `VIEW_ALIASES`.

### 3.1 The nav as shipped (35 destinations, 64 views, 0 lost addresses)

| Section | Destination | Tabs (existing view ids) |
|---|---|---|
| **Talk** | Jarvis | `jarvis` |
| | Chat | `chat` |
| | Voice | `voice` |
| | Messages | `inbox` · `board` |
| **Watch** | Overview | `overview` · `mission` · `feed` |
| | Runs | `runs` · `activity` · `insights` · `replay` |
| | Health | `health` · `cache` · `tools` (usage) · `providers` (routing log) — `system` merged in |
| | Alerts | `alerts` |
| | Budget | `budget` |
| **Automate** | Wizards | `wizards` |
| | Workflows | `workflows` · `flow` |
| | Work | `workboard` · `okr` |
| | Triggers | `schedules` · `standing` |
| | Autonomy | `autonomy` |
| **Govern** | Approvals | `approvals` |
| | Policy | `policy` |
| | Oversight | `overseer` · `council` · `conductor` |
| | Seats | `seats` |
| **Agents** | Fleet | `agents` · `roster` |
| | Skills | `skills` |
| | Capabilities | `catalog` · `toolbox` · `toolforge` · `market` · `execution-profiles` |
| | Sandbox | `sandbox` |
| **Knowledge** | Memory | `memory` · `taste` |
| | World | `world` |
| | Thinking | `research` · `analyst` · `reflect` |
| | Search | `search` |
| | Data & Files | `data` · `artifacts` (gallery + file manager) · `storage` — `files` merged in |
| **Connect** | Providers & Models | `quickconnect` · `models` |
| | Routing | `routing` · `chains` |
| | Channels | `channels` |
| | Integrations | `mcp` · `acp` · `connections` |
| **Admin** | Setup | `setup` |
| | Config Center | `configcenter` — `config` merged in as a fold |
| | Identity | `persona` · `prompts` |
| | Backup | `backup` |

`providers` (the routing telemetry log) moved to Watch → Health, resolving
§2.3: the Connect section holds only provider *management*.

No section exceeds six rows — pinned by `nav.test.ts`. The sidebar is
glanceable.

### 3.2 Renames that removed collisions

| Was | Is | Why |
|---|---|---|
| Providers | **Routing log** (a Health tab) | it is telemetry, not a manager |
| Tools | **Tool usage** (a Health tab) | same |
| Catalog | **Tool registry** | it registers tools + trust levels, not models |
| Models | **Models & Keys**, under *Providers & Models* | it is where API keys live; say so |
| System | deleted, merged into **Health** | one vitals surface |
| Config | deleted, a fold inside **Config Center** | it is a debug dump, not a destination |
| Files | deleted, a mode of **Artifacts & Files** | one artifact surface, two ways to look at it |

### 3.2b The tool split

`Tool registry` and `Tool usage` both rendered the full catalogue. They are not
the same question, so neither was deleted — the content moved to the page that
owns the question:

| Question | Page | What it holds |
|---|---|---|
| What can this agent do, and under what policy? | **Tool registry** (Agents › Capabilities) | every tool, its description, its Edict capability, its editable trust level — plus the search and capability filter, which used to live on the monitor |
| What is actually being called, how often, how badly? | **Tool usage** (Observe › Health) | error rate, call volume, per-tool bars, the live invocation log — and nothing that has never been called |

`Tool usage` links to the registry when there is nothing to chart.

### 3.3 Mechanics (no router rewrite)

- `ViewTabs` (`components/AppNav.tsx`) renders a row's facets; each tab simply
  navigates to that view's own `#hash`. No `?tab=` parameter, no second source
  of truth — the hash router stays the only one.
- The sidebar highlights a row when `rowForView[active]` matches it, so a deep
  link into any facet (`#models`) lights up its destination.
- Every bookmark, ⌘K entry, help `related` chip and `location.hash = "…"` call
  site keeps working untouched.
- `help.ts` keys stay per-view and the drawer follows the active *tab*, so
  page-aware help got sharper rather than blunter. The drawer titles itself
  from the NAV label, so a renamed view cannot show a stale header.

## 4. What shipped

**Phase A — findability.** Every view carries `keywords` (§2.5), the colliding
names are fixed (§3.2), and `docs/CONSOLE.md`'s map is rewritten and pinned to
`nav.tsx` by `consoledoc.test.ts` (§2.6).

**Phase B — the row/tab IA (§3.1).** `NAV_GROUPS` → `NavRow` → `NavItem`. Rows
are the sidebar; a row with more than one view renders `ViewTabs`, whose tabs
navigate to each view's own `#hash`. 67 sidebar entries became 35 destinations
without changing a single address.

**Phase C — retire the duplicates.** Three merges, each deleting a page rather
than re-parenting it:

| Merged | Into | Mechanism |
|---|---|---|
| `views/Files.tsx` | `views/Artifacts.tsx` | a "file manager" mode beside the gallery |
| `views/Config.tsx` | `views/ConfigCenter.tsx` | `ConfigInventory` folded under a Disclosure |
| `views/Status.tsx` | `views/Health.tsx` | its unique tiles + an Advanced block |

Three things fell out of Phase C that were not on the plan:

- **The artifact domain lived inside a view.** `views/Files.tsx` exported the
  types, classifiers, raw-URL builder and `BlobArtifact` that the Artifacts
  gallery, Inbox, ChannelSessions and the file-manager workspace all imported —
  four modules depending on a *screen*. It is now `lib/artifacts.tsx`, which is
  what made the merge possible at all.
- **A deep link that never worked.** The Artifacts viewer's "file manager"
  button navigated to `#files?path=…`, and `FileManagerWorkspace` read no query
  at all — it always landed on the root. It now takes `initialPath` and opens on
  the file.
- **Two views had no help topic and nothing noticed.** `help.test.ts` checked
  coverage against a hand-kept id list that had drifted, hiding that `workboard`
  and `execution-profiles` (and the `incident` route) were undocumented. The
  guard now derives from `NAV`; the three topics are written.

Retired ids stay addressable through `VIEW_ALIASES` in `nav.tsx` — `#files`
opens the file-manager mode, `#config` the Config Center, `#system` Health.

## 5. Invariants to hold

- Every view id in `nav.tsx` keeps a `HELP` topic (`help.test.ts` guards this).
- Every view id remains resolvable from a bare `#id` hash.
- The command palette must find every destination by at least one word that is
  **not** in its label.
- `docs/CONSOLE.md` is guarded against `nav.tsx` so the prose map cannot drift.
- A retired view id gets a `VIEW_ALIASES` entry; it is never simply deleted.
- Domain logic lives in `lib/`. A view is a screen, not a module other screens
  import from.
