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
  FolderOpen,
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
  FileCog,
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
const Status = lazyNamed(() => import("@/views/Status"), "Status");
const Runs = lazyNamed(() => import("@/views/Runs"), "Runs");
const Budget = lazyNamed(() => import("@/views/Budget"), "Budget");
const FlowStudio = lazyNamed(() => import("@/views/FlowStudio"), "FlowStudio");
const Config = lazyNamed(() => import("@/views/Config"), "Config");
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
const Files = lazyNamed(() => import("@/views/Files"), "Files");
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

interface NavItem {
  id: string;
  label: string;
  icon: LucideIcon;
  render: NavRender;
}

export interface NavGroup {
  id: string;
  label: string;
  icon: LucideIcon; // section icon for the two-level nav rail (M974)
  items: NavItem[];
}

// NAV_GROUPS organises the ~30 views into labelled sections so the sidebar reads
// as a map of the system rather than a flat wall of links: Converse (talk to /
// between agents), Monitor (live observability), Agents (introspect their work),
// Automation (unattended behaviour), Knowledge (what it knows & has learned), and
// System (configuration & plumbing).
export const NAV_GROUPS: NavGroup[] = [
  {
    id: "converse",
    label: "Converse",
    icon: MessageSquare,
    items: [
      { id: "jarvis", label: "Jarvis", icon: Sparkles, render: Jarvis },
      { id: "chat", label: "Chat", icon: MessageSquare, render: Chat },
      { id: "voice", label: "Voice", icon: Mic, render: Voice },
      { id: "inbox", label: "Inbox", icon: InboxIcon, render: Inbox },
      { id: "files", label: "Files", icon: FolderOpen, render: Files },
      { id: "artifacts", label: "Artifacts", icon: Shapes, render: Artifacts },
      { id: "data", label: "Data Lake", icon: Database, render: Data },
      { id: "board", label: "Agent Board", icon: MessagesSquare, render: Board },
      { id: "approvals", label: "Approvals", icon: CheckSquare, render: Approvals },
    ],
  },
  {
    id: "monitor",
    label: "Monitor",
    icon: ActivityIcon,
    items: [
      { id: "mission", label: "Mission Control", icon: Radar, render: Mission },
      { id: "health", label: "Health", icon: HeartPulse, render: Health },
      { id: "activity", label: "Activity", icon: ActivityIcon, render: Activity },
      { id: "autonomy", label: "Autonomy", icon: Waves, render: Autonomy },
      { id: "alerts", label: "Alerts", icon: Bell, render: Alerts },
      { id: "feed", label: "Live Stream", icon: Radio, render: EventFeed },
      { id: "insights", label: "Insights", icon: BarChart3, render: Insights },
      { id: "runs", label: "Runs", icon: ListTree, render: Runs },
      { id: "budget", label: "Budget", icon: Wallet, render: Budget },
    ],
  },
  {
    id: "agents",
    label: "Agents",
    icon: Bot,
    items: [
      { id: "agents", label: "Agents", icon: Waypoints, render: Agents },
      { id: "roster", label: "Roster", icon: Users, render: Roster },
      { id: "overseer", label: "Overseer", icon: Eye, render: Overseer },
      { id: "council", label: "Council", icon: Scale, render: Council },
      { id: "conductor", label: "Conductor", icon: Network, render: Conductor },
      { id: "research", label: "Research", icon: Telescope, render: Research },
      { id: "toolforge", label: "Tool Forge", icon: Hammer, render: Toolforge },
      { id: "mcp", label: "MCP Servers", icon: Plug, render: Mcp },
      { id: "acp", label: "ACP Agents", icon: Blocks, render: ACPAgents },
      { id: "sandbox", label: "Sandbox", icon: FlaskConical, render: Sandbox },
      { id: "flow", label: "Flow Studio", icon: Workflow, render: FlowStudio },
      { id: "replay", label: "Replay", icon: Clapperboard, render: Replay },
      { id: "analyst", label: "Analyst", icon: Sparkles, render: Analyst },
      { id: "search", label: "Search", icon: Search, render: SearchView },
    ],
  },
  {
    id: "automation",
    label: "Automation",
    icon: Workflow,
    items: [
      { id: "wizards", label: "Wizards", icon: Wand2, render: Wizards },
      { id: "workflows", label: "Workflows", icon: GitFork, render: Workflows },
      { id: "workboard", label: "Workboard", icon: CheckSquare, render: Workboard },
      { id: "okr", label: "Objectives", icon: Target, render: OKR },
      { id: "seats", label: "Seats", icon: Blocks, render: Seats },
      { id: "schedules", label: "Schedules", icon: CalendarClock, render: Schedules },
      { id: "standing", label: "Standing", icon: Anchor, render: Standing },
    ],
  },
  {
    id: "knowledge",
    label: "Knowledge",
    icon: Brain,
    items: [
      { id: "memory", label: "Memory", icon: Brain, render: Memory },
      { id: "taste", label: "Taste", icon: Sparkles, render: Taste },
      { id: "world", label: "World", icon: Network, render: World },
      { id: "skills", label: "Skills", icon: Sparkles, render: Skills },
      { id: "reflect", label: "Reflection", icon: Lightbulb, render: Reflect },
    ],
  },
  {
    id: "provision",
    label: "Setup",
    icon: PackageOpen,
    items: [
      { id: "setup", label: "Setup", icon: Wand2, render: Setup },
      { id: "toolbox", label: "Toolbox", icon: PackageOpen, render: Toolbox },
      { id: "market", label: "Marketplace", icon: Store, render: Market },
      { id: "channels", label: "Channels", icon: Radio, render: Channels },
    ],
  },
  {
    id: "system",
    label: "System",
    icon: SlidersHorizontal,
    items: [
      { id: "overview", label: "Overview", icon: LayoutDashboard, render: Dashboard },
      { id: "system", label: "System", icon: Settings, render: Status },
      { id: "persona", label: "Default Identity", icon: Bot, render: Persona },
      { id: "prompts", label: "Prompts", icon: MessageSquarePlus, render: Prompts },
      { id: "configcenter", label: "Config Center", icon: SlidersHorizontal, render: ConfigCenter },
      { id: "config", label: "Config", icon: FileCog, render: Config },
      { id: "connections", label: "Connections", icon: Network, render: Connections },
      { id: "quickconnect", label: "Quick Connect", icon: Plug, render: QuickConnect },
      { id: "providers", label: "Providers", icon: Cpu, render: Providers },
      { id: "models", label: "Models", icon: Layers, render: Models },
      { id: "routing", label: "Routing", icon: RouteIcon, render: Routing },
      { id: "chains", label: "Fallback Chains", icon: Link2, render: Chains },
      { id: "tools", label: "Tools", icon: Wrench, render: Tools },
      { id: "execution-profiles", label: "Execution Profiles", icon: Terminal, render: ExecutionProfiles },
      { id: "catalog", label: "Catalog", icon: Boxes, render: Catalog },
      { id: "policy", label: "Policy", icon: Shield, render: Policy },
      { id: "cache", label: "Cache", icon: Database, render: Cache },
      { id: "storage", label: "Storage", icon: HardDrive, render: Storage },
      { id: "backup", label: "Backup", icon: Archive, render: Backup },
    ],
  },
];

// NAV is the flat list derived from the groups, for view lookup, deep-link
// resolution, and the command palette.
export const NAV: NavItem[] = NAV_GROUPS.flatMap((g) => g.items);

// groupForView maps a view id to its containing group id (to auto-expand it).
export const groupForView: Record<string, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.items.map((it) => [it.id, g.id])),
);

// sectionForView maps a view id to its section LABEL, so the command palette
// groups views by the same sections as the sidebar.
export const sectionForView: Record<string, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.items.map((it) => [it.id, g.label])),
);

// viewFromHash reads a valid view id from the URL hash (#agents → "agents"),
// falling back to chat so a stale/empty hash never blanks the app. The
// `#agent/<slug>` detail route (M960) isn't a nav view of its own — it renders
// the full-page AgentPage — so it resolves to "agents" here, keeping that nav
// item highlighted while you're on one of its agents.
export function viewFromHash(): string {
  if (agentSlugFromHash(location.hash)) return "agents";
  if (incidentIdFromHash(location.hash)) return "autonomy";
  const id = location.hash.replace(/^#\/?/, "").split("?")[0];
  return NAV.some((n) => n.id === id) ? id : "chat";
}
