import { lazy, Suspense, useEffect, useMemo, useRef, useState, type ComponentType, type LazyExoticComponent } from "react";
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
  Pause,
  Play,
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
  HelpCircle,
  HardDrive,
  Shapes,
  RefreshCw,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { postAction, getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { ingestCouncilEvent } from "@/lib/councilStore";
import { ingestConductorEvent } from "@/lib/conductorStore";
import { attentionAlertCount } from "@/lib/alerts";
import { foldActivityEvent, summarize, type ActivityState } from "@/lib/activity";
import { CommandPalette } from "@/components/CommandPalette";
import { HelpDrawer } from "@/components/HelpDrawer";
import { MiniChat } from "@/components/MiniChat";
import { AlertBell } from "@/components/AlertBell";
import { ApprovalsBell } from "@/components/ApprovalsBell";
import { NotifyToggle } from "@/components/NotifyToggle";
import { Vitals } from "@/components/Vitals";
import { FleetNowBar } from "@/components/FleetNowBar";
import { ActivityChip } from "@/components/ActivityChip";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";
import { TooltipProvider } from "@/components/ui/tooltip";

type ConfirmRequest = ConfirmOptions;
import type { CommandItem } from "@/lib/commands";
import { ThemeToggle } from "@/components/ThemeToggle";
import { AdvancedToggle } from "@/components/AdvancedToggle";
import { toggleTheme } from "@/lib/theme";
import { toggleAdvanced } from "@/lib/advanced";
import { useChat } from "@/lib/chatStore";
import { focusRun } from "@/lib/runfocus";
import { agentSlugFromHash, openAgent } from "@/lib/agentnav";
import { incidentIdFromHash } from "@/lib/incidentnav";
import { exportAppearance, parseAppearanceJSON, applyAppearanceBundle } from "@/lib/appearance";
import { parseConfigBundle, fetchConfigBundle, applyConfigBundle } from "@/lib/configbackup";
import { downloadText } from "@/lib/export";
import { AccentPicker } from "@/components/AccentPicker";
import { ConsoleName } from "@/components/ConsoleName";
import { anyCredentialed, type SetupCatalog } from "@/lib/setup";

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
const Catalog = lazyNamed(() => import("@/views/Catalog"), "Catalog");
const Models = lazyNamed(() => import("@/views/Models"), "Models");
const Routing = lazyNamed(() => import("@/views/Routing"), "Routing");
const Chains = lazyNamed(() => import("@/views/Chains"), "Chains");
const Setup = lazyNamed(() => import("@/views/Setup"), "Setup");
const Toolbox = lazyNamed(() => import("@/views/Toolbox"), "Toolbox");
const Market = lazyNamed(() => import("@/views/Market"), "Market");
const Channels = lazyNamed(() => import("@/views/Channels"), "Channels");
const AgentPage = lazyNamed(() => import("@/views/AgentPage"), "AgentPage");
const IncidentPage = lazyNamed(() => import("@/views/IncidentPage"), "IncidentPage");
const Files = lazyNamed(() => import("@/views/Files"), "Files");
const Data = lazyNamed(() => import("@/views/Data"), "Data");
const Council = lazyNamed(() => import("@/views/Council"), "Council");
const Conductor = lazyNamed(() => import("@/views/Conductor"), "Conductor");
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

interface NavGroup {
  id: string;
  label: string;
  icon: LucideIcon; // section icon for the two-level nav rail (M974)
  items: NavItem[];
}

// Per-section accent hue (M979) — each section gets its own colour so the nav
// reads as a vivid, navigable map rather than one flat grey list. Used for the
// rail icon tint, the active section pill, and the item active state.
const SECTION_HUE: Record<string, number> = {
  converse: 255, // blue
  monitor: 150, // green
  agents: 290, // violet
  automation: 55, // amber
  knowledge: 195, // cyan
  provision: 18, // coral
  system: 230, // indigo
};
const sectionHue = (id: string) => SECTION_HUE[id] ?? 255;

// NAV_GROUPS organises the ~30 views into labelled sections so the sidebar reads
// as a map of the system rather than a flat wall of links: Converse (talk to /
// between agents), Monitor (live observability), Agents (introspect their work),
// Automation (unattended behaviour), Knowledge (what it knows & has learned), and
// System (configuration & plumbing).
const NAV_GROUPS: NavGroup[] = [
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
      { id: "world", label: "World", icon: Network, render: World },
      { id: "skills", label: "Skills", icon: Sparkles, render: Skills },
      { id: "reflect", label: "Reflection", icon: Brain, render: Reflect },
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
      { id: "config", label: "Config", icon: Settings, render: Config },
      { id: "connections", label: "Connections", icon: Network, render: Connections },
      { id: "quickconnect", label: "Quick Connect", icon: Plug, render: QuickConnect },
      { id: "providers", label: "Providers", icon: Cpu, render: Providers },
      { id: "models", label: "Models", icon: Layers, render: Models },
      { id: "routing", label: "Routing", icon: RouteIcon, render: Routing },
      { id: "chains", label: "Fallback Chains", icon: Link2, render: Chains },
      { id: "tools", label: "Tools", icon: Wrench, render: Tools },
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
const NAV: NavItem[] = NAV_GROUPS.flatMap((g) => g.items);

// groupForView maps a view id to its containing group id (to auto-expand it).
const groupForView: Record<string, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.items.map((it) => [it.id, g.id])),
);

// sectionForView maps a view id to its section LABEL, so the command palette
// groups views by the same sections as the sidebar.
const sectionForView: Record<string, string> = Object.fromEntries(
  NAV_GROUPS.flatMap((g) => g.items.map((it) => [it.id, g.label])),
);

// viewFromHash reads a valid view id from the URL hash (#agents → "agents"),
// falling back to chat so a stale/empty hash never blanks the app. The
// `#agent/<slug>` detail route (M960) isn't a nav view of its own — it renders
// the full-page AgentPage — so it resolves to "agents" here, keeping that nav
// item highlighted while you're on one of its agents.
function viewFromHash(): string {
  if (agentSlugFromHash(location.hash)) return "agents";
  if (incidentIdFromHash(location.hash)) return "autonomy";
  const id = location.hash.replace(/^#\/?/, "").split("?")[0];
  return NAV.some((n) => n.id === id) ? id : "chat";
}

export default function App() {
  const [active, setActiveRaw] = useState(viewFromHash);
  const [hashKey, setHashKey] = useState(() => location.hash);
  // The agent currently addressed by `#agent/<slug>` (M960), or null on a normal
  // view. When set, the main area renders the full-page AgentPage instead of the
  // active nav view.
  const [agentSlug, setAgentSlug] = useState<string | null>(() => agentSlugFromHash(location.hash));
  const [incidentId, setIncidentId] = useState<string | null>(() => incidentIdFromHash(location.hash));
  const { newChat } = useChat();
  const [paletteOpen, setPaletteOpen] = useState(false);
  // Page-aware help drawer (M920): one global toggle, content follows `active`.
  const [helpOpen, setHelpOpen] = useState(false);
  // Recent runs offered as ⌘K "Open run" commands (fulfils the palette's promise).
  // Refreshed whenever the palette opens so the list is current without polling.
  const [recentRuns, setRecentRuns] = useState<{ correlation_id?: string; intent?: string; status?: string }[]>([]);
  // Build provenance (M971): the daemon's semver + git revision, shown in the
  // sidebar footer so it's unambiguous which build is actually running (the
  // semver alone can't distinguish two dev builds).
  const [build, setBuild] = useState<{ version?: string; revision?: string; built?: string; build_modified?: boolean } | null>(null);
  // Every roster agent offered as a ⌘K "Open agent" command, so any created
  // agent's identity page is one keystroke away from anywhere (M967).
  const [paletteAgents, setPaletteAgents] = useState<{ slug: string; name?: string; system?: boolean; retired?: boolean }[]>([]);
  // Two-level nav (M974): the section whose items the secondary list shows. It
  // follows the active view's section, but a rail click can browse another
  // section without navigating yet.
  const [navSection, setNavSection] = useState<string>(() => groupForView[viewFromHash()] || NAV_GROUPS[0].id);
  const { connected, events, subscribe } = useEvents();
  const ui = useUI();

  // Feed council.* events into the module-level council store (M987) from the app
  // level, so a deliberation keeps assembling even when the Council view isn't
  // mounted — letting the operator navigate away and return mid-run.
  useEffect(() => subscribe(ingestCouncilEvent), [subscribe]);
  useEffect(() => subscribe(ingestConductorEvent), [subscribe]);

  // Fetch the daemon's build provenance once for the sidebar footer (M971).
  useEffect(() => {
    getJSON<{ version?: string; revision?: string; built?: string; build_modified?: boolean }>("/api/version")
      .then(setBuild)
      .catch(() => {});
  }, []);

  // Keep the secondary nav list pointed at the active view's section (M974), so
  // navigating (incl. via ⌘K or a deep link) reveals the right item list.
  const activeGroupId = groupForView[active] || NAV_GROUPS[0].id;
  useEffect(() => {
    setNavSection(activeGroupId);
  }, [activeGroupId]);
  const shownGroup = NAV_GROUPS.find((g) => g.id === navSection) || NAV_GROUPS[0];

  // Unseen-alert badge on the Alerts nav item (M779): count the critical/warning alerts
  // in the live buffer so the cockpit flags "something needs attention" from anywhere —
  // not only when you happen to open the Alerts tab. Opening that tab marks them seen.
  const liveAlertCount = useMemo(() => attentionAlertCount(events, { nowMs: Date.now() }), [events]);
  const [seenAlerts, setSeenAlerts] = useState(0);
  useEffect(() => {
    if (active === "alerts") setSeenAlerts(liveAlertCount);
  }, [active, liveAlertCount]);
  const unseenAlerts = Math.max(0, liveAlertCount - seenAlerts);

  // Live "active runs" badge on the Overseer nav item (M868): fold the live event
  // buffer into run state and count those still running, so the operator sees how
  // many runs are in flight from ANY view — ambient monitoring, like the alert
  // badge. Events are newest-first, so fold in reverse (chronological) order.
  // Same cold-start semantics as the alert badge: it reflects the live buffer.
  const activeRunCount = useMemo(() => {
    let state: ActivityState = {};
    for (let i = events.length - 1; i >= 0; i--) state = foldActivityEvent(state, events[i]);
    return summarize(state).running;
  }, [events]);

  const current = NAV.find((n) => n.id === active) || NAV[0];
  const View = current.render;

  // First-run setup (M816): auto-open the full-screen wizard until the catalog
  // reports at least one usable provider. API-key providers become usable when a
  // key exists; keyless/local providers become usable when they are runnable.
  // Once dismissed for this install, it stays quiet until opened from nav.
  const [needsSetup, setNeedsSetup] = useState(false);
  useEffect(() => {
    if (localStorage.getItem("agezt.setup.skipped") === "1") return;
    getJSON<SetupCatalog>("/api/catalog")
      .then((c) => setNeedsSetup(!anyCredentialed(c)))
      .catch(() => {});
  }, []);
  // Hidden inputs behind the ⌘K "Import appearance" / "Import configuration" commands.
  const appearanceFileRef = useRef<HTMLInputElement>(null);
  const configFileRef = useRef<HTMLInputElement>(null);

  async function importAppearanceFile(file: File) {
    try {
      const bundle = parseAppearanceJSON(await file.text());
      applyAppearanceBundle(bundle);
      ui.toast(`Appearance imported (${Object.keys(bundle).join(", ")})`, "success");
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  // Export the daemon-side config (default identity + prompt templates + routing) as one bundle.
  async function exportConfig() {
    try {
      const bundle = await fetchConfigBundle();
      downloadText("agezt-config.json", JSON.stringify(bundle, null, 2), "application/json");
    } catch (e) {
      ui.toast(`Export failed: ${(e as Error).message}`, "error");
    }
  }

  // Restore a daemon-config bundle: apply each section it carries to the daemon.
  async function importConfigFile(file: File) {
    try {
      const applied = await applyConfigBundle(parseConfigBundle(await file.text()));
      ui.toast(`Config imported: ${applied.join(", ")}`, "success");
    } catch (e) {
      ui.toast(`Import failed: ${(e as Error).message}`, "error");
    }
  }

  // Deep-linkable views: setActive also reflects into the URL hash, so views are
  // bookmarkable and the browser back/forward buttons move between them.
  const setActive = (id: string) => {
    setActiveRaw(id);
    setAgentSlug(null); // leaving any agent detail route for a normal nav view
    setIncidentId(null); // leaving any incident detail route for a normal nav view
    if (location.hash.replace(/^#\/?/, "") !== id) location.hash = id;
  };
  // Sync when the hash changes externally (back/forward, manual edit, openAgent).
  useEffect(() => {
    function onHash() {
      setActiveRaw(viewFromHash());
      setAgentSlug(agentSlugFromHash(location.hash));
      setIncidentId(incidentIdFromHash(location.hash));
      setHashKey(location.hash);
    }
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // ⌘K / Ctrl+K opens the command palette from anywhere; "?" toggles the help
  // drawer — but never while the operator is typing in a field.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
        return;
      }
      if (e.key === "?" && !e.metaKey && !e.ctrlKey && !e.altKey) {
        const t = e.target as HTMLElement | null;
        const typing =
          t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement || !!t?.isContentEditable;
        if (!typing) {
          e.preventDefault();
          setHelpOpen((o) => !o);
        }
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Refresh the recent-runs list each time the palette opens, so "Open run …"
  // commands reflect what just happened. Best-effort — a fetch failure just leaves
  // the previous list (or none).
  useEffect(() => {
    if (!paletteOpen) return;
    let live = true;
    getJSON<{ runs?: { correlation_id?: string; intent?: string; status?: string }[] }>("/api/runs")
      .then((d) => {
        if (live) setRecentRuns((d.runs || []).filter((r) => r.correlation_id).slice(0, 8));
      })
      .catch(() => {});
    getJSON<{ profiles?: { slug: string; name?: string; system?: boolean; retired?: boolean }[] }>("/api/agents")
      .then((d) => {
        if (live) setPaletteAgents((d.profiles || []).filter((p) => p.slug));
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [paletteOpen]);

  const commands = useMemo<CommandItem[]>(() => {
    const views: CommandItem[] = NAV.map((n) => ({
      id: `view-${n.id}`,
      label: n.label,
      group: sectionForView[n.id] || "Go to",
      run: () => setActive(n.id),
    }));
    const actions: CommandItem[] = [
      {
        id: "act-new-chat",
        label: "New chat",
        group: "Action",
        keywords: "conversation thread compose ask message",
        run: () => {
          newChat();
          setActive("chat");
        },
      },
      {
        id: "act-halt",
        label: "Halt all runs",
        group: "Action",
        keywords: "freeze stop emergency",
        run: async () => {
          const ok = await ui.confirm({
            title: "Freeze all in-flight runs?",
            message: "Every running and queued run is paused until you resume.",
            confirmLabel: "Halt",
            danger: true,
          });
          if (ok) {
            try {
              await postAction("/api/halt");
              ui.toast("All runs halted", "success");
            } catch (e) {
              ui.toast((e as Error).message, "error");
            }
          }
        },
      },
      {
        id: "act-resume",
        label: "Resume",
        group: "Action",
        keywords: "unpause continue",
        run: () =>
          postAction("/api/resume")
            .then(() => ui.toast("Resumed", "success"))
            .catch((e) => ui.toast((e as Error).message, "error")),
      },
      {
        id: "act-help",
        label: "Help for this page",
        group: "Action",
        keywords: "guide manual docs documentation how to explain",
        run: () => setHelpOpen(true),
      },
      {
        id: "act-theme",
        label: "Toggle theme",
        group: "Action",
        keywords: "dark light appearance",
        run: () => toggleTheme(),
      },
      {
        id: "act-advanced",
        label: "Toggle Advanced mode",
        group: "Action",
        keywords: "calm detail diagnostics expert verbose simple",
        run: () => toggleAdvanced(),
      },
      {
        id: "act-appearance-export",
        label: "Export appearance settings",
        group: "Action",
        keywords: "backup theme accent console name download settings",
        run: () => downloadText("agezt-appearance.json", JSON.stringify(exportAppearance(), null, 2), "application/json"),
      },
      {
        id: "act-appearance-import",
        label: "Import appearance settings",
        group: "Action",
        keywords: "restore theme accent console name upload settings",
        run: () => appearanceFileRef.current?.click(),
      },
      {
        id: "act-config-export",
        label: "Export configuration (default identity, prompt templates, routing)",
        group: "Action",
        keywords: "backup config identity prompt templates routing download daemon profile",
        run: () => void exportConfig(),
      },
      {
        id: "act-config-import",
        label: "Import configuration (default identity, prompt templates, routing)",
        group: "Action",
        keywords: "restore config identity prompt templates routing upload daemon profile",
        run: () => configFileRef.current?.click(),
      },
    ];
    // Recent runs → "Open run …" — jump straight to a run's detail from ⌘K.
    const runCmds: CommandItem[] = recentRuns.map((r) => ({
      id: `run-${r.correlation_id}`,
      label: r.intent?.trim() || r.correlation_id || "run",
      group: "Open run",
      keywords: `run ${r.status || ""} ${r.correlation_id || ""}`,
      run: () => {
        focusRun(r.correlation_id!);
        setActive("runs");
      },
    }));
    // Roster agents → "Open agent …" — jump straight to any agent's identity
    // page from ⌘K (M967). Guardians and graveyard agents are tagged in keywords
    // so they're searchable too.
    const agentCmds: CommandItem[] = paletteAgents.map((a) => ({
      id: `agent-${a.slug}`,
      label: a.name && a.name !== a.slug ? `${a.name} (${a.slug})` : a.slug,
      group: "Open agent",
      keywords: `agent ${a.slug} ${a.name || ""} ${a.system ? "guardian system" : ""} ${a.retired ? "retired graveyard" : ""}`,
      run: () => openAgent(a.slug),
    }));
    return [...views, ...actions, ...agentCmds, ...runCmds];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ui, newChat, recentRuns, paletteAgents]);

  return (
    <TooltipProvider delayDuration={200}>
    <div className="flex h-full flex-col">
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} items={commands} />
      <HelpDrawer
        open={helpOpen}
        viewId={agentSlug ? "agent" : active}
        group={agentSlug ? "Agents" : sectionForView[active]}
        icon={agentSlug ? Bot : current.icon}
        onClose={() => setHelpOpen(false)}
        onNavigate={setActive}
      />
      {needsSetup && (
        <Suspense fallback={<RouteLoading label="Setup" />}>
          <Setup
            overlay
            onDone={() => {
              setNeedsSetup(false);
              setActive("chat");
            }}
            onSkip={() => {
              localStorage.setItem("agezt.setup.skipped", "1");
              setNeedsSetup(false);
            }}
          />
        </Suspense>
      )}
      <input
        ref={appearanceFileRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        aria-hidden="true"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void importAppearanceFile(f);
          e.target.value = "";
        }}
      />
      <input
        ref={configFileRef}
        type="file"
        accept="application/json,.json"
        className="hidden"
        aria-hidden="true"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void importConfigFile(f);
          e.target.value = "";
        }}
      />
      <MiniChat hidden={active === "chat"} onExpand={() => setActive("chat")} />
      <Header
        connected={connected}
        chatActive={active === "chat" && !agentSlug}
        activeRunCount={activeRunCount}
        onNavigate={setActive}
        onOpenChat={() => setActive("chat")}
        onOpenPalette={() => setPaletteOpen(true)}
        onOpenHelp={() => setHelpOpen(true)}
      />
      <Vitals onNavigate={setActive} />
      <FleetNowBar onNavigate={setActive} />
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* Two-level nav (M974): a big-icon section RAIL on the far left, then a
            secondary LIST of that section's views. Far fewer items on screen at
            once than the old long single list. On small screens both rows scroll
            horizontally. */}
        <nav className="flex min-w-0 shrink-0 overflow-hidden lg:border-r lg:border-border">
          {/* Section rail — colourful, glassy, one accent per section. */}
          <div className="flex max-w-full shrink-0 gap-1.5 overflow-x-auto p-2 lg:flex-col lg:gap-2 lg:overflow-visible lg:border-r lg:border-border">
            {NAV_GROUPS.map((g) => {
              const on = navSection === g.id;
              const isActiveSection = activeGroupId === g.id;
              const hue = sectionHue(g.id);
              const sectionBadge =
                (g.id === "monitor" ? unseenAlerts : 0) + (g.id === "agents" ? activeRunCount : 0);
              return (
                <button
                  key={g.id}
                  onClick={() => setNavSection(g.id)}
                  title={g.label}
                  aria-label={g.label}
                  className={cn(
                    "relative flex size-12 shrink-0 flex-col items-center justify-center gap-0.5 rounded-2xl transition-all duration-150",
                    on ? "scale-[1.04] shadow-e2 ring-1 ring-inset" : "hover:scale-[1.03] hover:bg-panel",
                  )}
                  style={
                    on
                      ? {
                          background: `linear-gradient(155deg, oklch(0.62 0.17 ${hue} / 0.3), oklch(0.6 0.16 ${hue} / 0.08))`,
                          // Mid-lightness so the label reads on BOTH the dark gradient
                          // tint and a light background (M982 light-mode fix).
                          color: `oklch(0.58 0.17 ${hue})`,
                          boxShadow: `inset 0 0 0 1px oklch(0.62 0.16 ${hue} / 0.5), 0 6px 18px -8px oklch(0.6 0.16 ${hue} / 0.5)`,
                        }
                      : { color: `oklch(0.6 0.14 ${hue})` }
                  }
                >
                  <g.icon className="size-5" />
                  <span className="text-[8px] font-semibold leading-none tracking-tight">{g.label}</span>
                  {isActiveSection && !on && (
                    <span className="absolute right-1.5 top-1.5 size-1.5 rounded-full" style={{ background: `oklch(0.62 0.16 ${hue})` }} />
                  )}
                  {sectionBadge > 0 && (
                    <span className="absolute -right-0.5 -top-0.5 inline-flex min-w-3.5 items-center justify-center rounded-full bg-bad px-0.5 text-[8px] font-bold leading-3.5 text-white">
                      {sectionBadge > 99 ? "99+" : sectionBadge}
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* Secondary item list for the selected section */}
          <div className="flex min-w-0 max-w-full flex-1 gap-1 overflow-x-auto p-2 lg:w-44 lg:flex-none lg:flex-col lg:gap-0.5 lg:overflow-y-auto">
            <div
              className="hidden items-center gap-1.5 px-2 pb-1.5 pt-1 text-[11px] font-bold uppercase tracking-widest lg:flex"
              style={{ color: `oklch(0.6 0.14 ${sectionHue(shownGroup.id)})` }}
            >
              <shownGroup.icon className="size-3.5" />
              {shownGroup.label}
            </div>
            {shownGroup.items.map((n) => {
              const hue = sectionHue(shownGroup.id);
              const isOn = n.id === active;
              return (
              <button
                key={n.id}
                onClick={() => setActive(n.id)}
                className={cn(
                  "relative flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition-all duration-150",
                  isOn
                    ? "font-semibold before:absolute before:left-0 before:top-1/2 before:hidden before:h-5 before:w-[3px] before:-translate-y-1/2 before:rounded-r-full before:content-[''] lg:before:block"
                    : "text-muted hover:bg-panel hover:text-foreground",
                )}
                style={
                  isOn
                    ? {
                        background: `linear-gradient(90deg, oklch(0.6 0.16 ${hue} / 0.2), oklch(0.6 0.16 ${hue} / 0.03))`,
                        color: `oklch(0.56 0.16 ${hue})`,
                      }
                    : undefined
                }
              >
                {isOn && <span className="absolute left-0 top-1/2 hidden h-5 w-[3px] -translate-y-1/2 rounded-r-full lg:block" style={{ background: `oklch(0.62 0.16 ${hue})` }} />}
                <n.icon className="size-4 shrink-0" />
                <span>{n.label}</span>
                {n.id === "alerts" && unseenAlerts > 0 && (
                  <span
                    className="ml-auto inline-flex min-w-4 items-center justify-center rounded-full bg-bad px-1 text-[10px] font-semibold leading-4 text-white"
                    title={`${unseenAlerts} new alert${unseenAlerts === 1 ? "" : "s"} — the agent flagged something`}
                    aria-label={`${unseenAlerts} unseen alerts`}
                  >
                    {unseenAlerts > 99 ? "99+" : unseenAlerts}
                  </span>
                )}
                {n.id === "overseer" && activeRunCount > 0 && (
                  <span
                    className="ml-auto inline-flex min-w-4 items-center justify-center rounded-full bg-accent/20 px-1 text-[10px] font-semibold leading-4 text-accent"
                    title={`${activeRunCount} run${activeRunCount === 1 ? "" : "s"} in flight`}
                    aria-label={`${activeRunCount} active runs`}
                  >
                    {activeRunCount > 99 ? "99+" : activeRunCount}
                  </span>
                )}
              </button>
              );
            })}
            {build && (
              <div
                className="mt-auto hidden px-3 pt-3 text-[10px] leading-tight text-muted/60 lg:block"
                title={
                  build.revision
                    ? `Daemon build ${build.revision}${build.build_modified ? " (modified working tree)" : ""}${build.built ? ` · built ${build.built}` : ""}`
                    : build.built
                      ? `Daemon built ${build.built}`
                      : "Build revision unavailable (binary built without VCS info)"
                }
              >
                v{build.version || "?"}
                {build.revision ? (
                  <> · {build.revision.slice(0, 7)}{build.build_modified ? "+" : ""}</>
                ) : null}
              </div>
            )}
          </div>
        </nav>
        <main className="min-h-0 flex-1 overflow-auto p-3 sm:p-4">
          {/* Keyed remount so each view fades + rises in on navigation. The
              `#agent/<slug>` detail route (M960) takes over the main area. */}
          <div
            key={incidentId ? `incident/${incidentId}` : agentSlug ? `agent/${agentSlug}` : hashKey || active}
            className="view-enter h-full"
          >
            <Suspense fallback={<RouteLoading label={incidentId ? "Incident" : agentSlug ? "Agent" : current.label} />}>
              {incidentId ? (
                <IncidentPage incidentId={incidentId} onNavigate={setActive} />
              ) : agentSlug ? (
                <AgentPage slug={agentSlug} onNavigate={setActive} />
              ) : (
                <View />
              )}
            </Suspense>
          </div>
        </main>
      </div>
    </div>
    </TooltipProvider>
  );
}

function RouteLoading({ label }: { label: string }) {
  return (
    <div className="flex h-full min-h-[240px] items-center justify-center">
      <div className="inline-flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-sm text-muted shadow-e1">
        <RefreshCw className="size-4 animate-spin" />
        Loading {label}...
      </div>
    </div>
  );
}

function Header({
  connected,
  chatActive,
  activeRunCount,
  onNavigate,
  onOpenChat,
  onOpenPalette,
  onOpenHelp,
}: {
  connected: boolean;
  chatActive: boolean;
  activeRunCount: number;
  onNavigate: (id: string) => void;
  onOpenChat: () => void;
  onOpenPalette: () => void;
  onOpenHelp: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const ui = useUI();
  async function act(path: string, opts?: { confirm?: ConfirmRequest; success?: string }) {
    if (opts?.confirm && !(await ui.confirm(opts.confirm))) return;
    setBusy(true);
    try {
      await postAction(path);
      if (opts?.success) ui.toast(opts.success, "success");
    } catch (e) {
      ui.toast(`${path}: ${(e as Error).message}`, "error");
    } finally {
      setBusy(false);
    }
  }
  return (
    <header className="relative z-10 flex flex-wrap items-center gap-2 border-b border-border bg-panel px-3 py-2 shadow-e1 sm:gap-3 sm:px-4">
      {/* Lit accent edge under the header (M977 command-center). */}
      <div className="accent-rule pointer-events-none absolute inset-x-0 bottom-0 h-px" />
      <ConsoleName />
      <span
        className={cn(
          "ml-1 inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium",
          connected ? "border-good/30 bg-good/10 text-good" : "border-bad/30 bg-bad/10 text-bad",
        )}
      >
        ● {connected ? "live" : "disconnected"}
      </span>
      {/* Always-visible "something is working" chip: lights up whenever a run is
          in flight anywhere (chat reply, tool call, autonomous agent), so the
          background never reads as a frozen screen. */}
      <ActivityChip count={activeRunCount} onClick={() => onNavigate("overseer")} />
      <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5 sm:gap-2">
        {/* Always-on Chat button (M985): the chat surface is the product's core
            ([[product-layer-priority]]), but the two-level nav buries it two
            clicks deep from any other section. This jumps there from anywhere. */}
        <button
          onClick={onOpenChat}
          aria-current={chatActive ? "page" : undefined}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-sm font-medium transition-colors",
            chatActive
              ? "bg-accent text-white shadow-e1"
              : "border border-accent/40 text-accent hover:bg-accent hover:text-white",
          )}
          title="Go to Chat"
        >
          <MessageSquare className="size-4" />
          <span className="hidden sm:inline">Chat</span>
        </button>
        <NotifyToggle />
        <ApprovalsBell />
        <AlertBell />
        <button
          onClick={onOpenPalette}
          className="hidden h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-xs text-muted transition-colors hover:border-accent hover:text-foreground sm:inline-flex"
          title="Command palette"
        >
          <Search className="size-3.5" />
          <kbd className="rounded border border-border px-1 text-[10px]">⌘K</kbd>
        </button>
        <button
          onClick={onOpenHelp}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-xs text-muted transition-colors hover:border-accent hover:text-foreground"
          title="Help for this page (?)"
          aria-label="Help for this page"
        >
          <HelpCircle className="size-3.5" />
          <span className="hidden sm:inline">Help</span>
        </button>
        <button
          onClick={() =>
            act("/api/halt", {
              confirm: {
                title: "Freeze all in-flight runs?",
                message: "Every running and queued run is paused until you resume. Use this to stop the daemon fast.",
                confirmLabel: "Halt",
                danger: true,
              },
              success: "All runs halted",
            })
          }
          disabled={busy}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-bad px-2.5 text-sm text-bad transition-colors hover:bg-bad hover:text-white disabled:opacity-50 sm:px-3"
          aria-label="Halt all runs"
        >
          <Pause className="size-4" /> <span className="hidden sm:inline">Halt</span>
        </button>
        <button
          onClick={() => act("/api/resume", { success: "Resumed" })}
          disabled={busy}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-sm transition-colors hover:border-accent disabled:opacity-50 sm:px-3"
          aria-label="Resume all runs"
        >
          <Play className="size-4" /> <span className="hidden sm:inline">Resume</span>
        </button>
        <AccentPicker />
        <AdvancedToggle />
        <ThemeToggle />
      </div>
    </header>
  );
}
