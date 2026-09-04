import { lazy, type ComponentType, type LazyExoticComponent } from "react";
import {
  MessageSquare,
  Activity as ActivityIcon,
  Clapperboard,
  Waypoints,
  Scale,
  Telescope,
  Users,
  Eye,
  Radar,
  Waves,
  HeartPulse,
  Workflow,
  LayoutDashboard,
  BarChart3,
  ListTree,
  Wallet,
  Radio,
  Settings,
  Database,
  Cpu,
  Wrench,
  PackageOpen,
  Store,
  Boxes,
  Shield,
  Archive,
  CalendarClock,
  Network,
  Sparkles,
  Mic,
  Blocks,
  Bell,
  Anchor,
  Brain,
  Inbox as InboxIcon,
  MessagesSquare,
  CheckSquare,
  Target,
  Search,
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
  HardDrive,
  Shapes,
  Terminal,
  Lightbulb,
  type LucideIcon,
} from "lucide-react";
import { agentSlugFromHash } from "@/lib/agentnav";
import { incidentIdFromHash } from "@/lib/incidentnav";

type LazyView = LazyExoticComponent<ComponentType<any>>;
type NavRender = ComponentType<any> | LazyView;

function lazyNamed<T extends Record<string, unknown>>(loader: () => Promise<T>, key: keyof T): LazyView {
  return lazy(async () => ({ default: (await loader())[key] as ComponentType<any> }));
}

const EventFeed = lazyNamed(() => import("@/components/EventFeed"), "EventFeed");
const Chat = lazyNamed(() => import("@/views/Chat"), "Chat");
const Jarvis = lazyNamed(() => import("@/views/Jarvis"), "Jarvis");
const Voice = lazyNamed(() => import("@/views/Voice"), "Voice");
const ACPAgents = lazyNamed(() => import("@/views/ACPAgents"), "ACPAgents");
const Activity = lazyNamed(() => import("@/views/Activity"), "Activity");
const Mission = lazyNamed(() => import("@/views/Mission"), "Mission");
const Autonomy = lazyNamed(() => import("@/views/Autonomy"), "Autonomy");
const Health = lazyNamed(() => import("@/views/Health"), "Health");
const Analyst = lazyNamed(() => import("@/views/Analyst"), "Analyst");
const Alerts = lazyNamed(() => import("@/views/Alerts"), "Alerts");
const SearchView = lazyNamed(() => import("@/views/Search"), "Search");
const Replay = lazyNamed(() => import("@/views/Replay"), "Replay");
const Agents = lazyNamed(() => import("@/views/Agents"), "Agents");
const Roster = lazyNamed(() => import("@/views/Roster"), "Roster");
const Overseer = lazyNamed(() => import("@/views/Overseer"), "Overseer");
const Toolforge = lazyNamed(() => import("@/views/Toolforge"), "Toolforge");
const Mcp = lazyNamed(() => import("@/views/Mcp"), "Mcp");
const Workflows = lazyNamed(() => import("@/views/Workflows"), "Workflows");
const Workboard = lazyNamed(() => import("@/views/Workboard"), "Workboard");
const OKR = lazyNamed(() => import("@/views/OKR"), "OKR");
const Taste = lazyNamed(() => import("@/views/Taste"), "Taste");
const Seats = lazyNamed(() => import("@/views/Seats"), "Seats");
const Wizards = lazyNamed(() => import("@/views/Wizards"), "Wizards");
const Dashboard = lazyNamed(() => import("@/views/Dashboard"), "Dashboard");
const Insights = lazyNamed(() => import("@/views/Insights"), "Insights");
const Runs = lazyNamed(() => import("@/views/Runs"), "Runs");
const Budget = lazyNamed(() => import("@/views/Budget"), "Budget");
const FlowStudio = lazyNamed(() => import("@/views/FlowStudio"), "FlowStudio");
const ConfigCenter = lazyNamed(() => import("@/views/ConfigCenter"), "ConfigCenter");
const Cache = lazyNamed(() => import("@/views/Cache"), "Cache");
const Providers = lazyNamed(() => import("@/views/Providers"), "Providers");
const QuickConnect = lazyNamed(() => import("@/views/QuickConnect"), "QuickConnect");
const Connections = lazyNamed(() => import("@/views/Connections"), "Connections");
const Tools = lazyNamed(() => import("@/views/Tools"), "Tools");
const ExecutionProfiles = lazyNamed(() => import("@/views/ExecutionProfiles"), "ExecutionProfiles");
const Catalog = lazyNamed(() => import("@/views/Catalog"), "Catalog");
const Models = lazyNamed(() => import("@/views/Models"), "Models");
const Routing = lazyNamed(() => import("@/views/Routing"), "Routing");
const Chains = lazyNamed(() => import("@/views/Chains"), "Chains");
export const Setup = lazyNamed(() => import("@/views/Setup"), "Setup");
const Toolbox = lazyNamed(() => import("@/views/Toolbox"), "Toolbox");
const Market = lazyNamed(() => import("@/views/Market"), "Market");
const Channels = lazyNamed(() => import("@/views/Channels"), "Channels");
export const AgentPage = lazyNamed(() => import("@/views/AgentPage"), "AgentPage");
export const IncidentPage = lazyNamed(() => import("@/views/IncidentPage"), "IncidentPage");
const Data = lazyNamed(() => import("@/views/Data"), "Data");
const Council = lazyNamed(() => import("@/views/Council"), "Council");
const Conductor = lazyNamed(() => import("@/views/Conductor"), "Conductor");
const Research = lazyNamed(() => import("@/views/Research"), "Research");
const Persona = lazyNamed(() => import("@/views/Persona"), "Persona");
const Prompts = lazyNamed(() => import("@/views/Prompts"), "Prompts");
const Backup = lazyNamed(() => import("@/views/Backup"), "Backup");
const Policy = lazyNamed(() => import("@/views/Policy"), "Policy");
const Schedules = lazyNamed(() => import("@/views/Schedules"), "Schedules");
const World = lazyNamed(() => import("@/views/World"), "World");
const Skills = lazyNamed(() => import("@/views/Skills"), "Skills");
const Standing = lazyNamed(() => import("@/views/Standing"), "Standing");
const Memory = lazyNamed(() => import("@/views/Memory"), "Memory");
const Inbox = lazyNamed(() => import("@/views/Inbox"), "Inbox");
const Board = lazyNamed(() => import("@/views/Board"), "Board");
const Reflect = lazyNamed(() => import("@/views/Reflect"), "Reflect");
const Approvals = lazyNamed(() => import("@/views/Approvals"), "Approvals");
const Sandbox = lazyNamed(() => import("@/views/Sandbox"), "Sandbox");
const Storage = lazyNamed(() => import("@/views/Storage"), "Storage");
const Artifacts = lazyNamed(() => import("@/views/Artifacts"), "Artifacts");

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
      row("messages", "Messages", InboxIcon, [
        {
          id: "inbox",
          label: "Inbox",
          icon: InboxIcon,
          render: Inbox,
          keywords: "threads channel telegram slack discord email whatsapp conversations messages gelen",
        },
        {
          id: "board",
          label: "Agent Board",
          icon: MessagesSquare,
          render: Board,
          keywords: "agent to agent backchannel handoff coordination peer chatter notes help",
        },
      ]),
    ],
  },
  {
    id: "observe",
    label: "Observe",
    icon: Eye,
    rows: [
      row("overview", "Overview", LayoutDashboard, [
        {
          id: "overview",
          label: "Overview",
          icon: LayoutDashboard,
          render: Dashboard,
          keywords: "dashboard home summary at a glance now durum genel",
        },
        {
          id: "mission",
          label: "Mission Control",
          icon: Radar,
          render: Mission,
          keywords: "mission control realtime throughput events per second tokens spend sparkline",
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
          keywords: "history executions correlation cancel stop trace transcript koşu",
        },
        {
          id: "activity",
          label: "Activity",
          icon: ActivityIcon,
          render: Activity,
          keywords: "running now busy working in flight incidents in progress cancel",
        },
        {
          id: "insights",
          label: "Insights",
          icon: BarChart3,
          render: Insights,
          keywords: "analytics charts spend over time per model outcomes throughput stats",
        },
        {
          id: "replay",
          label: "Replay",
          icon: Clapperboard,
          render: Replay,
          keywords: "replay reconstruct step through past run timeline journal",
        },
      ]),
      row("health", "Health", HeartPulse, [
        {
          id: "health",
          label: "Health",
          icon: HeartPulse,
          render: Health,
          keywords:
            "health vitals uptime error rate gauges resilience diagnostics doctor system status daemon counters limits http surface credentials version sağlık",
        },
        {
          id: "cache",
          label: "Prompt cache",
          icon: Database,
          render: Cache,
          keywords: "cache savings prompt caching read write tokens cost saved",
        },
        {
          id: "tools",
          label: "Tool usage",
          icon: Wrench,
          render: Tools,
          keywords: "tool calls invocation log error rate latency monitor telemetry how often a tool is used",
        },
        {
          id: "providers",
          label: "Routing log",
          icon: Cpu,
          render: Providers,
          keywords: "fallback rate which provider served routed calls telemetry log reload",
        },
      ]),
      row("alerts", "Alerts", Bell, [
        {
          id: "alerts",
          label: "Alerts",
          icon: Bell,
          render: Alerts,
          keywords: "warnings critical attention notifications flagged uyarı",
        },
      ]),
      row("budget", "Budget", Wallet, [
        {
          id: "budget",
          label: "Budget",
          icon: Wallet,
          render: Budget,
          keywords: "cost spend limit cap quota money dollars ceiling bütçe",
        },
      ]),
    ],
  },
  {
    id: "automate",
    label: "Automate",
    icon: Workflow,
    rows: [
      row("wizards", "Wizards", Wand2, [
        {
          id: "wizards",
          label: "Wizards",
          icon: Wand2,
          render: Wizards,
          keywords: "guided flows step by step setup helper how do i getting started sihirbaz",
        },
      ]),
      row("workflows", "Workflows", GitFork, [
        {
          id: "workflows",
          label: "Workflows",
          icon: GitFork,
          render: Workflows,
          keywords: "automation nodes pipeline webhook trigger n8n retry test node dag",
        },
        {
          id: "flow",
          label: "Flow Studio",
          icon: Workflow,
          render: FlowStudio,
          keywords: "canvas plan generate refine visual editor graph copilot",
        },
      ]),
      row("work", "Work", CheckSquare, [
        {
          id: "workboard",
          label: "Workboard",
          icon: CheckSquare,
          render: Workboard,
          keywords: "tasks lanes kanban dispatch acceptance criteria proof blocked görev",
        },
        {
          id: "okr",
          label: "Objectives",
          icon: Target,
          render: OKR,
          keywords: "okr goals key results outcomes targets hedef",
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
        {
          id: "conductor",
          label: "Conductor",
          icon: Network,
          render: Conductor,
          keywords: "orchestration roles ensemble delegate coordination ask",
        },
      ]),
      row("seats", "Seats", Blocks, [
        {
          id: "seats",
          label: "Seats",
          icon: Blocks,
          render: Seats,
          keywords: "org chart positions roles assignment who does what koltuk",
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
          id: "catalog",
          label: "Tool registry",
          icon: Boxes,
          render: Catalog,
          keywords: "which tools exist registry catalog documentation search find a tool trust level capability enable disable",
        },
        {
          id: "toolbox",
          label: "Toolbox",
          icon: PackageOpen,
          render: Toolbox,
          keywords: "install cli binaries winget brew apt choco missing outdated machine host command line",
        },
        {
          id: "toolforge",
          label: "Tool Forge",
          icon: Hammer,
          render: Toolforge,
          keywords: "write a new tool script forge draft test promote quarantine custom code",
        },
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
        {
          id: "taste",
          label: "Taste",
          icon: Sparkles,
          render: Taste,
          keywords: "preferences style likes dislikes tone opinions zevk",
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
      row("thinking", "Thinking", Lightbulb, [
        {
          id: "research",
          label: "Research",
          icon: Telescope,
          render: Research,
          keywords: "deep research investigate sources report ask a question araştırma",
        },
        {
          id: "analyst",
          label: "Analyst",
          icon: Sparkles,
          render: Analyst,
          keywords: "ask about the system self analysis observability assistant explain diagnose",
        },
        {
          id: "reflect",
          label: "Reflection",
          icon: Lightbulb,
          render: Reflect,
          keywords: "retrospective lessons learned self review improve introspection",
        },
      ]),
      row("search", "Search", Search, [
        {
          id: "search",
          label: "Search",
          icon: Search,
          render: SearchView,
          keywords: "journal audit trail why did it do that trace cause verify integrity export ara",
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
            "outputs images html markdown pdf generated gallery preview çıktı file manager browse download delete attachments uploads tree dosya collect",
        },
        {
          id: "storage",
          label: "Storage",
          icon: HardDrive,
          render: Storage,
          keywords: "disk space usage reclaim collectors prune consolidate reaper cleanup gb",
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
          id: "quickconnect",
          label: "Quick Connect",
          icon: Plug,
          render: QuickConnect,
          keywords: "add provider paste api key connect openai anthropic deepseek openrouter custom byok bağlan",
        },
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
          id: "routing",
          label: "Routing",
          icon: RouteIcon,
          render: Routing,
          keywords: "which model for which task per agent routing rules default model yönlendirme",
        },
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
          id: "persona",
          label: "Default Identity",
          icon: Bot,
          render: Persona,
          keywords: "persona system prompt soul character tone name default agent kimlik",
        },
        {
          id: "prompts",
          label: "Prompts",
          icon: MessageSquarePlus,
          render: Prompts,
          keywords: "prompt library templates snippets reusable instructions",
        },
      ]),
      row("backup", "Backup", Archive, [
        {
          id: "backup",
          label: "Backup",
          icon: Archive,
          render: Backup,
          keywords: "export import snapshot restore migrate save everything yedek",
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
  system: "health", // 2026-09: the System vitals page merged into Health
  // Renames are aliases too: a bookmark to the old name must not fall through
  // to the chat fallback, which looks exactly like the app losing the page.
  dashboard: "overview", // 2026-09: "Dashboard" was renamed to "Overview"
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
  return VIEW_ALIASES[id] || "chat";
}
