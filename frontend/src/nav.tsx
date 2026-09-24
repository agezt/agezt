import { lazy, type ComponentType, type LazyExoticComponent } from "react";
import {
  MessageSquare,
  Activity as ActivityIcon,
  Clapperboard,
  Waypoints,
  Scale,
  Users,
  Eye,
  Radar,
  Waves,
  Workflow,
  LayoutDashboard,
  BarChart3,
  ListTree,
  Radio,
  BookOpen,
  Archive,
  Settings,
  Database,
  Cpu,
  Store,
  Shield,
  CalendarClock,
  Network,
  Sparkles,
  Mic,
  Blocks,
  Anchor,
  Brain,
  CheckSquare,
  FlaskConical,
  GitFork,
  Hammer,
  Plug,
  SlidersHorizontal,
  Layers,
  Route as RouteIcon,
  Link2,
  Bot,
  MessageSquarePlus,
  Wand2,
  Shapes,
  Terminal,
  type LucideIcon,
} from "lucide-react";
import { agentSlugFromHash } from "@/features/agents/lib/agentnav";
import { incidentIdFromHash } from "@/features/incidents/lib/incidentnav";

type LazyView = LazyExoticComponent<ComponentType<any>>;
type NavRender = ComponentType<any> | LazyView;

function lazyNamed<T extends Record<string, unknown>>(loader: () => Promise<T>, key: keyof T): LazyView {
  return lazy(async () => ({ default: (await loader())[key] as ComponentType<any> }));
}

// RemovedView is a placeholder for legacy @/views/ entries that were
// consolidated into features/* during the Day 1-18 sprint. Most nav slots
// below have been re-linked to a real replacement (see the `= SomeComponent`
// lines further down); the few that genuinely have no good map stay as
// `RemovedView` so their nav id (hash, ⌘K, help topic) keeps resolving instead
// of 404'ing — and the body now tells the operator what to do.
//
// Exported as `REMOVED_VIEW` (not just `RemovedView`) so nav.test.ts can
// reference-count how many nav rows still render this placeholder. If you
// add a new view that intentionally stays removed, update the
// REMOVED_VIEW_IDS set below AND bump the size assertion in nav.test.ts.
export const REMOVED_VIEW: NavRender = () => (
  <div className="m-6 rounded border border-dashed border-amber-500/40 bg-amber-500/5 p-4 text-sm text-amber-200">
    <p className="font-semibold">This view was retired during the Day 23 cleanup.</p>
    <p className="mt-1 text-amber-300/80">
      The original screen no longer exists — the Day 1-18 sprint consolidated its concerns into <code className="rounded bg-amber-500/10 px-1 font-mono text-xs">features/*</code>, and this slot has no perfect replacement.
    </p>
    <p className="mt-3 text-amber-300/80">
      <strong className="font-semibold text-amber-200">Press <kbd className="rounded border border-amber-500/40 bg-amber-500/10 px-1 font-mono text-xs">⌘K</kbd></strong> to open the command palette and search for what you need — that's the supported way to find any surface today.
    </p>
    <p className="mt-2 text-xs text-amber-300/60">
      The nav id is intentionally kept so existing bookmarks, help topics and command-palette entries still resolve here instead of 404'ing.
    </p>
  </div>
);

// Lazy imports come first — re-linked views below bind to these.
// Defining the lazy imports up top lets us point legacy @/views/* slots at
// their closest live replacement without forward-reference runtime errors.
const EventFeed = lazyNamed(() => import("@/components/EventFeed"), "EventFeed");
// Day 25: Chat and Jarvis came back as first-class features/* modules, so
// they sit next to Voice in the Talk section. Both export default, so we
// wrap them in `lazy()` directly (lazyNamed wants a named export).
const Chat = lazy(() => import("@/features/chat/components/Chat"));
const Jarvis = lazy(() => import("@/features/jarvis/components/Jarvis"));
const Voice = lazyNamed(() => import("@/features/voice/components/Voice"), "Voice");
const ACPAgents = lazyNamed(() => import("@/features/agents/components/ACPAgents"), "ACPAgents");
const Autonomy = lazyNamed(() => import("@/features/autonomy/components/Autonomy"), "Autonomy");
const MissionControl = lazy(() => import("@/features/observe/components/MissionControl"));
const Agents = lazyNamed(() => import("@/features/agents/components/Agents"), "Agents");
const Roster = lazyNamed(() => import("@/features/agents/components/Roster"), "Roster");
const Overseer = lazyNamed(() => import("@/features/overseer/components/Overseer"), "Overseer");
const Mcp = lazyNamed(() => import("@/features/mcp/components/Mcp"), "Mcp");
const Workflows = lazyNamed(() => import("@/features/workflows/components/Workflows"), "Workflows");
const Runs = lazyNamed(() => import("@/features/runs/components/Runs"), "Runs");
const ConfigCenter = lazyNamed(() => import("@/features/configcenter/components/ConfigCenter"), "ConfigCenter");
const Connections = lazyNamed(() => import("@/features/connections/components/Connections"), "Connections");
const ExecutionProfiles = lazyNamed(() => import("@/features/execution-profiles/components/ExecutionProfiles"), "ExecutionProfiles");
const Models = lazyNamed(() => import("@/features/models/components/Models"), "Models");
const Chains = lazyNamed(() => import("@/features/workflows/components/Chains"), "Chains");
const Market = lazyNamed(() => import("@/features/market/components/Market"), "Market");
const Channels = lazyNamed(() => import("@/features/channels/components/Channels"), "Channels");
const Data = lazyNamed(() => import("@/features/data/components/Data"), "Data");
const Council = lazyNamed(() => import("@/features/council/components/Council"), "Council");
const Policy = lazyNamed(() => import("@/features/policy/components/Policy"), "Policy");
const Approvals = lazy(() => import("@/features/govern/components/Approvals"));
const Schedules = lazyNamed(() => import("@/features/schedules/components/Schedules"), "Schedules");
const World = lazyNamed(() => import("@/features/world/components/World"), "World");
const Skills = lazyNamed(() => import("@/features/skills/components/Skills"), "Skills");
const Standing = lazyNamed(() => import("@/features/standing/components/Standing"), "Standing");
const Memory = lazyNamed(() => import("@/features/memory/components/Memory"), "Memory");
const Sandbox = lazyNamed(() => import("@/features/sandbox/components/Sandbox"), "Sandbox");
const Research = lazyNamed(() => import("@/features/knowledge/components/Research"), "Research");
const Analyst = lazyNamed(() => import("@/features/knowledge/components/Analyst"), "Analyst");
const Reflect = lazyNamed(() => import("@/features/knowledge/components/Reflect"), "Reflect");
const Backup = lazyNamed(() => import("@/features/admin/components/Backups"), "Backups");
export const Setup = lazyNamed(() => import("@/features/setup/components/Setup"), "Setup");
export const AgentPage = lazyNamed(() => import("@/features/agents/components/AgentPage"), "AgentPage");
export const IncidentPage = lazyNamed(() => import("@/features/incidents/components/IncidentPage"), "IncidentPage");

// Legacy @/views/* slots re-linked to their closest live surface in
// features/*. Only slots whose label genuinely describes what the target
// component renders are re-linked — every other slot stays as `REMOVED_VIEW`
// so the operator never lands on a page whose content doesn't match what the
// nav label promised. A wrong-feeling "Tool usage" tab that opens the MCP
// server page is worse than a clearly-marked placeholder that tells the
// operator to use ⌘K.
//
// The set of intentionally-removed ids is documented in REMOVED_VIEW_IDS
// below and asserted by nav.test.ts.
//
// Day 25: Chat and Jarvis came back as proper features/* modules — those
// two bindings are now lazy imports at the top of this file, not aliases
// pointing at REMOVED_VIEW.
const Activity = Runs;
const Mission = MissionControl;
const Replay = Runs;
const Prompts = Skills;

// Activity / Replay alias the same Runs component (different filters); Prompts
// alias is similar enough (Skills holds templates + the prompt library). These
// are kept because the row label AND the underlying view both promise the same
// thing to the operator — they differ only in framing, not in what renders.
//
// Every other alias that used to hide content behind a misleading label was
// removed on Day 28 (Health → Standing orders, Alerts → Standing orders,
// Search → Data Lake, Storage → Data Lake, Taste → Memory verbatim,
// Wizards → Schedules, Inbox → World, Overview/Dashboard → Standing orders).
// Those rows are gone from NAV; the legacy ids stay in REMOVED_VIEW_IDS as
// bookmark fallbacks.

/**
 * View ids that intentionally still render `REMOVED_VIEW`. Every other view in
 * NAV must render a real `features/*` component — see nav.test.ts. If you add a
 * new view id here, also document why it has no replacement.
 *
 * Each entry groups under its nav row (Talk / Observe / Automate / Govern /
 * Agents / Knowledge / Connect / Admin) so the operator who lands on the
 * placeholder can see at a glance which row the retired slot used to live in.
 */
export const REMOVED_VIEW_IDS: ReadonlySet<string> = new Set([
  // (Day 25 brought Mission Control back as features/observe/components/MissionControl,
  //  and Approvals back as features/govern/components/Approvals.)
  // ── Talk ───────────────────────────────────────────────────────────────
  // (Chat and Jarvis came back in Day 25 as features/chat/components/Chat
  // and features/jarvis/components/Jarvis; both render real surfaces now.)
  // Talk › Messages had an Agent Board tab (multi-agent messaging). Day 28
  // dropped the Messages row entirely; "messages" and "inbox" are bookmark
  // fallbacks, "board" stays here as a no-equivalent id.
  "board",
  "messages",
  "inbox",
  // ── Observe ────────────────────────────────────────────────────────────
  // Day 28 IA pass: the "Health" and "Alerts" rows lived as aliases to the
  // Standing component — clicking either tab stood up "Standing orders",
  // with no health metrics / no flagged-alerts surface of its own. Dropped
  // both rows; their ids are bookmark fallbacks now. The Monitor row also
  // dropped its alias "Overview" tab (was the same Standing alias); "overview"
  // stays as a no-equivalent id pointing at Monitor's first tab via the
  // VIEW_ALIASES table.
  "health",
  "alerts",
  "overview",
  // Observe › Runs › Insights — merged into the main Runs view (Insights
  // showed run analytics; the Runs page already shows them).
  "insights",
  // Observe › Health used to host a "Prompt cache" panel ("cache"), a "Tool
  // usage" stats panel ("tools") and a "Routing log" panel ("providers").
  // Standing covers health telemetry, MCP covers server config — neither is
  // the same content, so all three stay removed.
  "cache",
  "tools",
  "providers",
  // Observe › Budget (spend-tracking tab) had no live equivalent.
  "budget",
  // ── Automate ───────────────────────────────────────────────────────────
  // Automate › Workflows › Flow Studio — visual canvas retired; Workflows
  // list view is the live surface.
  "flow",
  // Automate › Work › Workboard + OKR — never re-implemented after Day 1-18.
  "workboard",
  "okr",
  // Day 28: "Wizards" row aliased to Schedules — same component, different
  // label, no distinct content. Dropped the row, "wizards" stays as a
  // bookmark fallback.
  "wizards",
  // ── Govern ─────────────────────────────────────────────────────────────
  // Govern › Seats (user/seat management) — Standing is related but is a
  // different surface (recurring tasks, not users).
  "seats",
  // Govern › Oversight › Conductor — Council is related (multi-model
  // deliberation) but renders a different shape.
  "conductor",
  // ── Knowledge ──────────────────────────────────────────────────────────
  // (Day 26 brought Thinking Partners back as features/knowledge/components:
  // Research, Analyst, Reflect — three real "thinking partner" surfaces.)
  // Day 28 dropped the "Search" row (alias to Data Lake) and the "Taste" tab
  // (verbatim Memory dup) and the "Storage" tab (alias to Data Lake). Their
  // ids land on the closest live surface via VIEW_ALIASES but stay retired
  // from NAV here so the rows don't grow back accidentally.
  "search",
  "taste",
  "storage",
  // ── Admin ──────────────────────────────────────────────────────────────
  // Admin › Identity › Persona (operator identity settings). Prompts is
  // re-linked to Skills; Persona has no equivalent.
  "persona",
  // (Day 26 brought Backups back as features/admin/components/Backups —
  // surfaces the daemon's rollback checkpoints.)
  // Connect › Routing — Chains is workflow-chains, not routing decisions.
  "routing",
  // ── Agents › Capabilities ──────────────────────────────────────────────
  // Capabilities used to host Catalog (tool registry), Toolbox (tool mgmt)
  // and Toolforge (tool authoring). The live Marketplace is a browse
  // surface, MCP is server config — neither matches what these used to be.
  "catalog",
  "toolbox",
  "toolforge",
]);
const Artifacts = lazyNamed(() => import("@/features/artifacts/components/Artifacts"), "Artifacts");

/**
 * NavItem is one VIEW — the addressable unit. Its `id` is the URL hash
 * (`#runs`), the help-topic key, and the command-palette target, and none of
 * those ever change when the console is reorganised.
 *
 * `keywords` is what the operator actually types into ⌘K. The palette searches
 * `label + group + keywords`, so a view without keywords is only findable by a
 * word already printed on the screen — which is useless when the question is
 * "where do I put my API key?". Every view carries the vocabulary that leads
 * to it, including the near-miss names it does NOT use.
 */

export interface NavItem {
  id: string;
  label: string;
  icon: LucideIcon;
  render: NavRender;
  keywords?: string;
}

/**
 * NavRow is one nav DESTINATION — a row in the sidebar. A row holds one or
 * more views: with a single view it navigates straight there, with several it
 * renders a tab strip and the views become facets of one place.
 *
 * Rows exist because 67 equally-weighted sidebar entries are not an
 * information architecture, they are a package index (see docs/CONSOLE-IA.md).
 * Tabs navigate by setting the view's own hash, so folding views into a row
 * costs no deep link, bookmark, help topic or ⌘K entry.
 */
export interface NavRow {
  id: string;
  label: string;
  icon: LucideIcon;
  views: NavItem[];
}

export interface NavGroup {
  id: string;
  label: string;
  icon: LucideIcon; // section icon for the two-level nav rail (M974)
  rows: NavRow[];
}

const row = (id: string, label: string, icon: LucideIcon, views: NavItem[]): NavRow => ({ id, label, icon, views });

// NAV_GROUPS organizes the console by operator job, not by backend package.
// A section is a job, a row is a noun, a tab is a facet of that noun. Stable
// view ids preserve hashes, deep links, help topics and command-palette
// actions while the eight sections answer "what am I here to do?" in at most
// six rows each.
export const NAV_GROUPS: NavGroup[] = [
  {
    id: "talk",
    label: "Talk",
    icon: MessageSquare,
    rows: [
      row("jarvis", "Jarvis", Sparkles, [
        {
          id: "jarvis",
          label: "Jarvis",
          icon: Sparkles,
          render: Jarvis,
          keywords: "presence companion assistant pillars hears acts knows profile",
        },
      ]),
      row("chat", "Chat", MessageSquare, [
        {
          id: "chat",
          label: "Chat",
          icon: MessageSquare,
          render: Chat,
          keywords: "conversation talk ask prompt sohbet message thread regenerate steer",
        },
      ]),
      row("voice", "Voice", Mic, [
        {
          id: "voice",
          label: "Voice",
          icon: Mic,
          render: Voice,
          keywords: "speak listen speech tts stt microphone hands-free wake word ses",
        },
      ]),
    ],
  },
  {
    id: "observe",
    label: "Observe",
    icon: Eye,
    rows: [
      // Day 28 IA pass: the legacy "Overview" tab pointed at Standing orders,
      // so the row was renamed to "Monitor" and the misleading alias tab was
      // dropped. The two surviving tabs are the real-time surfaces:
      // MissionControl (throughput / tokens / attention) and EventFeed
      // (live SSE firehose).
      row("overview", "Monitor", LayoutDashboard, [
        {
          id: "mission",
          label: "Mission Control",
          icon: Radar,
          render: Mission,
          keywords: "mission control realtime throughput events per second tokens spend sparkline attention",
        },
        {
          id: "feed",
          label: "Live Stream",
          icon: Radio,
          render: EventFeed,
          keywords: "live stream firehose events sse tail log raw",
        },
      ]),
      row("runs", "Runs", ListTree, [
        {
          id: "runs",
          label: "Runs",
          icon: ListTree,
          render: Runs,
          keywords:
            "history executions correlation cancel stop trace transcript koşu daemon recent activity log",
        },
        {
          id: "activity",
          label: "Activity",
          icon: ActivityIcon,
          render: Activity,
          keywords: "running now busy working in flight incidents in progress cancel",
        },
        {
          id: "replay",
          label: "Replay",
          icon: Clapperboard,
          render: Replay,
          keywords: "replay reconstruct step through past run timeline journal",
        },
      ]),
    ],
  },
  {
    id: "automate",
    label: "Automate",
    icon: Workflow,
    rows: [
      row("workflows", "Workflows", GitFork, [
        {
          id: "workflows",
          label: "Workflows",
          icon: GitFork,
          render: Workflows,
          keywords: "automation nodes pipeline webhook trigger n8n retry test node dag",
        },
      ]),
      row("triggers", "Triggers", CalendarClock, [
        {
          id: "schedules",
          label: "Schedules",
          icon: CalendarClock,
          render: Schedules,
          keywords: "cron timer recurring every day periodic job system tasks zamanlama",
        },
        {
          id: "standing",
          label: "Standing orders",
          icon: Anchor,
          render: Standing,
          keywords: "rules always when this happens do that reactive orders why fire",
        },
      ]),
      row("autonomy", "Autonomy", Waves, [
        {
          id: "autonomy",
          label: "Autonomy",
          icon: Waves,
          render: Autonomy,
          keywords: "pulse heartbeat proactive initiative cadence dial quiet chatty pause beat observers",
        },
      ]),
    ],
  },
  {
    id: "govern",
    label: "Govern",
    icon: Shield,
    rows: [
      row("approvals", "Approvals", CheckSquare, [
        {
          id: "approvals",
          label: "Approvals",
          icon: CheckSquare,
          render: Approvals,
          keywords: "human in the loop ask permission grant deny pending decision onay",
        },
      ]),
      row("policy", "Policy", Shield, [
        {
          id: "policy",
          label: "Policy",
          icon: Shield,
          render: Policy,
          keywords: "edict capability trust level allow ask deny hard deny rules redaction secrets test decision izin",
        },
      ]),
      row("oversight", "Oversight", Eye, [
        {
          id: "overseer",
          label: "Overseer",
          icon: Eye,
          render: Overseer,
          keywords: "supervision what is running who needs help live watch retire revive",
        },
        {
          id: "council",
          label: "Council",
          icon: Scale,
          render: Council,
          keywords: "deliberation debate multiple models vote second opinion members",
        },
      ]),
    ],
  },
  {
    id: "fleet",
    label: "Agents",
    icon: Users,
    rows: [
      // Agents and Roster stay SEPARATE rows. Folding them into one destination
      // was over-consolidation: seeing everything that runs and managing agent
      // identities are different jobs, and both words are how the operator
      // actually navigates — burying either one behind a tab loses it.
      row("agents", "Agents", Waypoints, [
        {
          id: "agents",
          label: "Agents",
          icon: Waypoints,
          render: Agents,
          keywords: "everything that runs triggers automations census fleet multi agent overview ajanlar",
        },
      ]),
      row("roster", "Roster", Users, [
        {
          id: "roster",
          label: "Roster",
          icon: Users,
          render: Roster,
          keywords: "create agent identity persona guardian system agents retire graveyard passport ajan kadro",
        },
      ]),
      row("skills", "Skills", Sparkles, [
        {
          id: "skills",
          label: "Skills",
          icon: Sparkles,
          render: Skills,
          keywords: "learned procedures bundles promote quarantine revert triggers hygiene yetenek",
        },
      ]),
      row("capabilities", "Capabilities", Hammer, [
        {
          id: "market",
          label: "Marketplace",
          icon: Store,
          render: Market,
          keywords: "install packs bundles browse download capability store publish sign sources mağaza",
        },
        {
          id: "execution-profiles",
          label: "Execution Profiles",
          icon: Terminal,
          render: ExecutionProfiles,
          keywords: "shell interpreter runtime environment command profile python node docker",
        },
      ]),
      row("sandbox", "Sandbox", FlaskConical, [
        {
          id: "sandbox",
          label: "Sandbox",
          icon: FlaskConical,
          render: Sandbox,
          keywords: "code exec projects what agents built files scratch workspace",
        },
      ]),
    ],
  },
  {
    id: "knowledge",
    label: "Knowledge",
    icon: Brain,
    rows: [
      row("memory", "Memory", Brain, [
        {
          id: "memory",
          label: "Memory",
          icon: Brain,
          render: Memory,
          keywords: "facts remember forget distill prune promote shared brain profile hafıza",
        },
      ]),
      row("world", "World", Network, [
        {
          id: "world",
          label: "World",
          icon: Network,
          render: World,
          keywords: "entities relations graph knowledge model people places things dünya",
        },
      ]),
      row("data", "Data & Files", Database, [
        {
          id: "data",
          label: "Data Lake",
          icon: Database,
          render: Data,
          keywords: "collections records tables rows insert query structured store veri",
        },
        {
          id: "artifacts",
          label: "Artifacts & Files",
          icon: Shapes,
          render: Artifacts,
          keywords:
            "outputs images html markdown pdf generated gallery preview çıktı file manager browse download delete attachments uploads tree dosya collect disk usage reclaim",
        },
      ]),
      row("thinking", "Thinking Partners", Sparkles, [
        {
          id: "research",
          label: "Research",
          icon: BookOpen,
          render: Research,
          keywords:
            "research plan sub-question source citation verify claim grounded multi-hop answer think partner araştır kaynak doğrula",
        },
        {
          id: "analyst",
          label: "Analyst",
          icon: BarChart3,
          render: Analyst,
          keywords:
            "analyst journal audit log breakdown distribution pattern frequency actor kind count top entries analiz ne kadar kim ne yapmış",
        },
        {
          id: "reflect",
          label: "Reflect",
          icon: Brain,
          render: Reflect,
          keywords:
            "reflect self-talk lesson correction supersede journal memory audit agent reflection düşün öğren kendini düzelt",
        },
      ]),
    ],
  },
  {
    id: "connect",
    label: "Connect",
    icon: Plug,
    rows: [
      row("providers-models", "Providers & Models", Cpu, [
        {
          id: "models",
          label: "Models & Keys",
          icon: Layers,
          render: Models,
          keywords: "api key token credential keyring secret models.dev catalog sync oauth sign in llm anahtar",
        },
      ]),
      row("routing", "Routing", RouteIcon, [
        {
          id: "chains",
          label: "Fallback Chains",
          icon: Link2,
          render: Chains,
          keywords: "fallback ladder retry another model chain health dots @name",
        },
      ]),
      row("channels", "Channels", Radio, [
        {
          id: "channels",
          label: "Channels",
          icon: Radio,
          render: Channels,
          keywords: "telegram slack discord email imap whatsapp sms irc accounts oauth qr connect kanal",
        },
      ]),
      row("integrations", "Integrations", Plug, [
        {
          id: "mcp",
          label: "MCP Servers",
          icon: Plug,
          render: Mcp,
          keywords: "model context protocol server stdio remote http attach catalog presets",
        },
        {
          id: "acp",
          label: "ACP Agents",
          icon: Blocks,
          render: ACPAgents,
          keywords: "agent client protocol external agents peers",
        },
        {
          id: "connections",
          label: "Connections",
          icon: Network,
          render: Connections,
          keywords: "connected am i connected status wired up providers channels mcp nodes health check bağlantı",
        },
      ]),
    ],
  },
  {
    id: "admin",
    label: "Admin",
    icon: Settings,
    rows: [
      row("setup", "Setup", Wand2, [
        {
          id: "setup",
          label: "Setup",
          icon: Wand2,
          render: Setup,
          keywords: "first run onboarding wizard get started install configure kurulum",
        },
      ]),
      row("configcenter", "Config Center", SlidersHorizontal, [
        {
          id: "configcenter",
          label: "Config Center",
          icon: SlidersHorizontal,
          render: ConfigCenter,
          keywords:
            "settings env vars options toggles password tunnel external access schema ayarlar raw dump effective configuration debug values",
        },
      ]),
      row("identity", "Identity", Bot, [
        {
          id: "prompts",
          label: "Prompts",
          icon: MessageSquarePlus,
          render: Prompts,
          keywords: "prompt library templates snippets reusable instructions",
        },
      ]),
      row("backups", "Backups", Archive, [
        {
          id: "backup",
          label: "Backups",
          icon: Archive,
          render: Backup,
          keywords:
            "rollback restore checkpoint snapshot revert undo file mutation backup yedek geri al",
        },
      ]),
    ],
  },
];

// ROWS is the flat list of nav destinations (sidebar rows) across all sections.
export const ROWS: NavRow[] = NAV_GROUPS.flatMap((g) => g.rows);

// NAV is the flat list of every VIEW, for view lookup, deep-link resolution and
// the command palette. Order follows the sidebar.
export const NAV: NavItem[] = ROWS.flatMap((r) => r.views);

// rowForView maps a view id to the nav row that renders it, so a deep link like
// `#models` highlights "Providers & Models" and opens on the Models tab.
export const rowForView: Record<string, NavRow> = Object.fromEntries(
  ROWS.flatMap((r) => r.views.map((v) => [v.id, r])),
);

// groupForView maps a view id to its containing group id (to auto-expand it).
export const groupForView: Record<string, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.rows.flatMap((r) => r.views.map((v) => [v.id, g.id]))),
);

// sectionForView maps a view id to its section LABEL, so the command palette
// groups views by the same sections as the sidebar.
export const sectionForView: Record<string, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.rows.flatMap((r) => r.views.map((v) => [v.id, g.label]))),
);

/**
 * VIEW_ALIASES keeps a retired view id addressable. When two views merge, the
 * loser's hash must not 404 into the fallback view — bookmarks, help `related`
 * chips, ⌘K history and other views' links all still carry it. The alias sends
 * it to the surface that absorbed it; the target view reads the original hash if
 * it needs to (Artifacts opens its file-manager mode for `#files`).
 */
export const VIEW_ALIASES: Record<string, string> = {
  files: "artifacts", // 2026-09: the file manager became a mode of Artifacts
  config: "configcenter", // 2026-09: the raw inventory became a fold in Config Center
  // Renames are aliases too: a bookmark to the old name must not fall through
  // to the chat fallback, which looks exactly like the app losing the page.
  dashboard: "mission", // 2026-09: "Dashboard" was renamed to "Overview"; 2026-09 (Day 28):
                       // "Overview" was rolled into the Monitor row whose first tab is
                       // Mission Control — old Dashboard bookmarks land there.
  // Day 28: the eight legacy ids whose rows had a misleading label are gone from
  // NAV entirely. Each alias sends bookmarks / help-chips / ⌘K history to the
  // closest live surface so the operator doesn't end up on a misleading page.
  health: "runs", // was Observe › Health (alias to Standing orders); Runs shows recent activity
  alerts: "runs", // was Observe › Alerts (alias to Standing orders); Runs shows what to look at
  search: "memory", // was Knowledge › Search (alias to Data Lake); Memory shows the journal search substitute
  inbox: "channels", // was Talk › Messages → Inbox (alias to World graph); Channels owns message routing
  messages: "channels", // was Talk › Messages row label
  wizards: "setup", // was Automate › Wizards (alias to Schedules); Setup is the guided-flow home
  overview: "mission", // was Observe › Overview's first tab (alias to Standing orders); Monitor › Mission Control
  taste: "memory", // was Knowledge › Memory › Taste (verbatim Memory dup); Memory's the home
  storage: "artifacts", // was Knowledge › Data & Files › Storage (alias to Data Lake); Artifacts owns file usage
};

// viewFromHash reads a valid view id from the URL hash (#agents → "agents"),
// falling back to chat so a stale/empty hash never blanks the app. The
// `#agent/<slug>` detail route (M960) isn't a nav view of its own — it renders
// the full-page AgentPage — so it resolves to "agents" here, keeping that nav
// item highlighted while you're on one of its agents.
export function viewFromHash(): string {
  if (agentSlugFromHash(location.hash)) return "agents";
  if (incidentIdFromHash(location.hash)) return "autonomy";
  const id = location.hash.replace(/^#\/?/, "").split("?")[0];
  if (NAV.some((n) => n.id === id)) return id;
  return VIEW_ALIASES[id] || "mission";
}
