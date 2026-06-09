import { useEffect, useMemo, useState, type ComponentType } from "react";
import {
  MessageSquare,
  Activity as ActivityIcon,
  Clapperboard,
  Waypoints,
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
  Boxes,
  Shield,
  CalendarClock,
  Network,
  Sparkles,
  Bell,
  Anchor,
  Brain,
  Inbox as InboxIcon,
  MessagesSquare,
  CheckSquare,
  Pause,
  Play,
  Search,
  ChevronDown,
  FlaskConical,
  SlidersHorizontal,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { CommandPalette } from "@/components/CommandPalette";
import { MiniChat } from "@/components/MiniChat";
import { AlertBell } from "@/components/AlertBell";
import { Vitals } from "@/components/Vitals";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";

type ConfirmRequest = ConfirmOptions;
import type { CommandItem } from "@/lib/commands";
import { ThemeToggle } from "@/components/ThemeToggle";
import { EventFeed } from "@/components/EventFeed";
import { Chat } from "@/views/Chat";
import { Activity } from "@/views/Activity";
import { Mission } from "@/views/Mission";
import { Autonomy } from "@/views/Autonomy";
import { Health } from "@/views/Health";
import { Analyst } from "@/views/Analyst";
import { Alerts } from "@/views/Alerts";
import { Search as SearchView } from "@/views/Search";
import { Replay } from "@/views/Replay";
import { Agents } from "@/views/Agents";
import { Dashboard } from "@/views/Dashboard";
import { Insights } from "@/views/Insights";
import { Status } from "@/views/Status";
import { Runs } from "@/views/Runs";
import { Budget } from "@/views/Budget";
import { FlowStudio } from "@/views/FlowStudio";
import { Config } from "@/views/Config";
import { ConfigCenter } from "@/views/ConfigCenter";
import { Cache } from "@/views/Cache";
import { Providers } from "@/views/Providers";
import { Tools } from "@/views/Tools";
import { Catalog } from "@/views/Catalog";
import { Policy } from "@/views/Policy";
import { Schedules } from "@/views/Schedules";
import { World } from "@/views/World";
import { Skills } from "@/views/Skills";
import { Standing } from "@/views/Standing";
import { Memory } from "@/views/Memory";
import { Inbox } from "@/views/Inbox";
import { Board } from "@/views/Board";
import { Reflect } from "@/views/Reflect";
import { Approvals } from "@/views/Approvals";
import { Sandbox } from "@/views/Sandbox";

interface NavItem {
  id: string;
  label: string;
  icon: LucideIcon;
  render: ComponentType;
}

interface NavGroup {
  id: string;
  label: string;
  items: NavItem[];
}

// NAV_GROUPS organises the ~30 views into labelled sections so the sidebar reads
// as a map of the system rather than a flat wall of links: Converse (talk to /
// between agents), Monitor (live observability), Agents (introspect their work),
// Automation (unattended behaviour), Knowledge (what it knows & has learned), and
// System (configuration & plumbing).
const NAV_GROUPS: NavGroup[] = [
  {
    id: "converse",
    label: "Converse",
    items: [
      { id: "chat", label: "Chat", icon: MessageSquare, render: Chat },
      { id: "inbox", label: "Inbox", icon: InboxIcon, render: Inbox },
      { id: "board", label: "Agent Board", icon: MessagesSquare, render: Board },
      { id: "approvals", label: "Approvals", icon: CheckSquare, render: Approvals },
    ],
  },
  {
    id: "monitor",
    label: "Monitor",
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
    items: [
      { id: "agents", label: "Agents", icon: Waypoints, render: Agents },
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
    items: [
      { id: "schedules", label: "Schedules", icon: CalendarClock, render: Schedules },
      { id: "standing", label: "Standing", icon: Anchor, render: Standing },
    ],
  },
  {
    id: "knowledge",
    label: "Knowledge",
    items: [
      { id: "memory", label: "Memory", icon: Brain, render: Memory },
      { id: "world", label: "World", icon: Network, render: World },
      { id: "skills", label: "Skills", icon: Sparkles, render: Skills },
      { id: "reflect", label: "Reflection", icon: Brain, render: Reflect },
    ],
  },
  {
    id: "system",
    label: "System",
    items: [
      { id: "overview", label: "Overview", icon: LayoutDashboard, render: Dashboard },
      { id: "system", label: "System", icon: Settings, render: Status },
      { id: "configcenter", label: "Config Center", icon: SlidersHorizontal, render: ConfigCenter },
      { id: "config", label: "Config", icon: Settings, render: Config },
      { id: "providers", label: "Providers", icon: Cpu, render: Providers },
      { id: "tools", label: "Tools", icon: Wrench, render: Tools },
      { id: "catalog", label: "Catalog", icon: Boxes, render: Catalog },
      { id: "policy", label: "Policy", icon: Shield, render: Policy },
      { id: "cache", label: "Cache", icon: Database, render: Cache },
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
// falling back to chat so a stale/empty hash never blanks the app.
function viewFromHash(): string {
  const id = location.hash.replace(/^#\/?/, "");
  return NAV.some((n) => n.id === id) ? id : "chat";
}

// COLLAPSE_KEY persists which sidebar groups the user has collapsed.
const COLLAPSE_KEY = "agezt.nav.collapsed";

function loadCollapsed(): Record<string, boolean> {
  try {
    return JSON.parse(localStorage.getItem(COLLAPSE_KEY) || "{}");
  } catch {
    return {};
  }
}

export default function App() {
  const [active, setActiveRaw] = useState(viewFromHash);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(loadCollapsed);
  const { connected } = useEvents();
  const ui = useUI();
  const current = NAV.find((n) => n.id === active) || NAV[0];
  const View = current.render;

  const toggleGroup = (id: string) => {
    setCollapsed((c) => {
      const next = { ...c, [id]: !c[id] };
      try {
        localStorage.setItem(COLLAPSE_KEY, JSON.stringify(next));
      } catch {
        /* ignore quota/availability errors */
      }
      return next;
    });
  };

  // Deep-linkable views: setActive also reflects into the URL hash, so views are
  // bookmarkable and the browser back/forward buttons move between them.
  const setActive = (id: string) => {
    setActiveRaw(id);
    if (location.hash.replace(/^#\/?/, "") !== id) location.hash = id;
  };
  // Sync when the hash changes externally (back/forward, manual edit).
  useEffect(() => {
    function onHash() {
      setActiveRaw(viewFromHash());
    }
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  // ⌘K / Ctrl+K opens the command palette from anywhere.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const commands = useMemo<CommandItem[]>(() => {
    const views: CommandItem[] = NAV.map((n) => ({
      id: `view-${n.id}`,
      label: n.label,
      group: sectionForView[n.id] || "Go to",
      run: () => setActive(n.id),
    }));
    const actions: CommandItem[] = [
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
        id: "act-theme",
        label: "Toggle theme",
        group: "Action",
        keywords: "dark light appearance",
        run: () => document.documentElement.classList.toggle("dark"),
      },
    ];
    return [...views, ...actions];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ui]);

  return (
    <div className="flex h-full flex-col">
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} items={commands} />
      <MiniChat hidden={active === "chat"} onExpand={() => setActive("chat")} />
      <Header connected={connected} onOpenPalette={() => setPaletteOpen(true)} />
      <Vitals onNavigate={setActive} />
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* Nav: horizontal scroll on small screens, grouped sidebar on lg+. */}
        <nav className="flex shrink-0 gap-1 overflow-x-auto border-b border-border p-2 lg:w-56 lg:flex-col lg:gap-0.5 lg:overflow-y-auto lg:border-b-0 lg:border-r">
          {NAV_GROUPS.map((g) => {
            // A group is open unless explicitly collapsed — but the group holding
            // the active view is always shown so the current page is never hidden.
            const hasActive = groupForView[active] === g.id;
            const isCollapsed = !!collapsed[g.id] && !hasActive;
            return (
              <div key={g.id} className="contents lg:block">
                <button
                  onClick={() => toggleGroup(g.id)}
                  className="hidden w-full items-center gap-1.5 rounded px-2 pb-1 pt-3 text-left text-[10px] font-semibold uppercase tracking-wider text-muted/70 transition-colors hover:text-muted lg:flex"
                  title={isCollapsed ? "Expand" : "Collapse"}
                >
                  <ChevronDown className={cn("size-3 transition-transform", isCollapsed && "-rotate-90")} />
                  {g.label}
                </button>
                {g.items.map((n) => (
                  <button
                    key={n.id}
                    onClick={() => setActive(n.id)}
                    className={cn(
                      "flex shrink-0 items-center gap-2 rounded-md px-3 py-1.5 text-left text-sm transition-colors lg:ml-1",
                      isCollapsed && "lg:hidden",
                      n.id === active
                        ? "bg-accent/15 font-medium text-accent"
                        : "text-muted hover:bg-panel hover:text-foreground",
                    )}
                  >
                    <n.icon className="size-4 shrink-0" />
                    <span>{n.label}</span>
                  </button>
                ))}
              </div>
            );
          })}
        </nav>
        <main className="min-h-0 flex-1 overflow-auto p-3">
          {/* Keyed remount so each view fades + rises in on navigation. */}
          <div key={active} className="view-enter h-full">
            <View />
          </div>
        </main>
      </div>
    </div>
  );
}

function Header({ connected, onOpenPalette }: { connected: boolean; onOpenPalette: () => void }) {
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
    <header className="flex items-center gap-3 border-b border-border bg-panel px-4 py-2">
      <h1 className="text-sm font-semibold tracking-wide">
        <span className="text-accent">agezt</span> · console
      </h1>
      <span
        className={cn(
          "ml-1 inline-flex items-center gap-1 text-xs",
          connected ? "text-good" : "text-bad",
        )}
      >
        ● {connected ? "live" : "disconnected"}
      </span>
      <div className="ml-auto flex items-center gap-2">
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
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-bad px-3 text-sm text-bad transition-colors hover:bg-bad hover:text-white disabled:opacity-50"
        >
          <Pause className="size-4" /> Halt
        </button>
        <button
          onClick={() => act("/api/resume", { success: "Resumed" })}
          disabled={busy}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-3 text-sm transition-colors hover:border-accent disabled:opacity-50"
        >
          <Play className="size-4" /> Resume
        </button>
        <ThemeToggle />
      </div>
    </header>
  );
}
