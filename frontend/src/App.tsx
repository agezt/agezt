import { useEffect, useMemo, useState, type ComponentType } from "react";
import {
  MessageSquare,
  Activity as ActivityIcon,
  Clapperboard,
  Waypoints,
  Radar,
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
  CheckSquare,
  Pause,
  Play,
  Search,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { CommandPalette } from "@/components/CommandPalette";
import { AlertBell } from "@/components/AlertBell";
import type { CommandItem } from "@/lib/commands";
import { ThemeToggle } from "@/components/ThemeToggle";
import { EventFeed } from "@/components/EventFeed";
import { Chat } from "@/views/Chat";
import { Activity } from "@/views/Activity";
import { Mission } from "@/views/Mission";
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
import { Reflect } from "@/views/Reflect";
import { Approvals } from "@/views/Approvals";

interface NavItem {
  id: string;
  label: string;
  icon: LucideIcon;
  render: ComponentType;
}

const NAV: NavItem[] = [
  { id: "chat", label: "Chat", icon: MessageSquare, render: Chat },
  { id: "activity", label: "Activity", icon: ActivityIcon, render: Activity },
  { id: "mission", label: "Mission Control", icon: Radar, render: Mission },
  { id: "analyst", label: "Analyst", icon: Sparkles, render: Analyst },
  { id: "alerts", label: "Alerts", icon: Bell, render: Alerts },
  { id: "replay", label: "Replay", icon: Clapperboard, render: Replay },
  { id: "agents", label: "Agents", icon: Waypoints, render: Agents },
  { id: "flow", label: "Flow Studio", icon: Workflow, render: FlowStudio },
  { id: "overview", label: "Overview", icon: LayoutDashboard, render: Dashboard },
  { id: "insights", label: "Insights", icon: BarChart3, render: Insights },
  { id: "runs", label: "Runs", icon: ListTree, render: Runs },
  { id: "system", label: "System", icon: Settings, render: Status },
  { id: "budget", label: "Budget", icon: Wallet, render: Budget },
  { id: "feed", label: "Live Stream", icon: Radio, render: EventFeed },
  { id: "search", label: "Search", icon: Search, render: SearchView },
  { id: "config", label: "Config", icon: Settings, render: Config },
  { id: "cache", label: "Cache", icon: Database, render: Cache },
  { id: "providers", label: "Providers", icon: Cpu, render: Providers },
  { id: "tools", label: "Tools", icon: Wrench, render: Tools },
  { id: "catalog", label: "Catalog", icon: Boxes, render: Catalog },
  { id: "policy", label: "Policy", icon: Shield, render: Policy },
  { id: "schedules", label: "Schedules", icon: CalendarClock, render: Schedules },
  { id: "world", label: "World", icon: Network, render: World },
  { id: "skills", label: "Skills", icon: Sparkles, render: Skills },
  { id: "standing", label: "Standing", icon: Anchor, render: Standing },
  { id: "memory", label: "Memory", icon: Brain, render: Memory },
  { id: "inbox", label: "Inbox", icon: InboxIcon, render: Inbox },
  { id: "reflect", label: "Reflection", icon: Brain, render: Reflect },
  { id: "approvals", label: "Approvals", icon: CheckSquare, render: Approvals },
];

// viewFromHash reads a valid view id from the URL hash (#agents → "agents"),
// falling back to chat so a stale/empty hash never blanks the app.
function viewFromHash(): string {
  const id = location.hash.replace(/^#\/?/, "");
  return NAV.some((n) => n.id === id) ? id : "chat";
}

export default function App() {
  const [active, setActiveRaw] = useState(viewFromHash);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const { connected } = useEvents();
  const current = NAV.find((n) => n.id === active) || NAV[0];
  const View = current.render;

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
      group: "Go to",
      run: () => setActive(n.id),
    }));
    const actions: CommandItem[] = [
      {
        id: "act-halt",
        label: "Halt all runs",
        group: "Action",
        keywords: "freeze stop emergency",
        run: () => {
          if (window.confirm("Freeze ALL in-flight runs?")) postAction("/api/halt").catch(() => {});
        },
      },
      {
        id: "act-resume",
        label: "Resume",
        group: "Action",
        keywords: "unpause continue",
        run: () => postAction("/api/resume").catch(() => {}),
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
  }, []);

  return (
    <div className="flex h-full flex-col">
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} items={commands} />
      <Header connected={connected} onOpenPalette={() => setPaletteOpen(true)} />
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* Nav: horizontal scroll on small screens, sidebar on lg+. */}
        <nav className="flex shrink-0 gap-1 overflow-x-auto border-b border-border p-2 lg:w-52 lg:flex-col lg:overflow-y-auto lg:border-b-0 lg:border-r">
          {NAV.map((n) => (
            <button
              key={n.id}
              onClick={() => setActive(n.id)}
              className={cn(
                "flex shrink-0 items-center gap-2 rounded-md px-3 py-1.5 text-left text-sm transition-colors",
                n.id === active ? "bg-accent/15 text-accent" : "text-muted hover:bg-panel hover:text-foreground",
              )}
            >
              <n.icon className="size-4 shrink-0" />
              <span>{n.label}</span>
            </button>
          ))}
        </nav>
        <main className="min-h-0 flex-1 overflow-auto p-3">
          <View />
        </main>
      </div>
    </div>
  );
}

function Header({ connected, onOpenPalette }: { connected: boolean; onOpenPalette: () => void }) {
  const [busy, setBusy] = useState(false);
  async function act(path: string, confirmMsg?: string) {
    if (confirmMsg && !window.confirm(confirmMsg)) return;
    setBusy(true);
    try {
      await postAction(path);
    } catch (e) {
      window.alert(`${path}: ${(e as Error).message}`);
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
          onClick={() => act("/api/halt", "Freeze ALL in-flight runs?")}
          disabled={busy}
          className="inline-flex h-8 items-center gap-1.5 rounded-md border border-bad px-3 text-sm text-bad transition-colors hover:bg-bad hover:text-white disabled:opacity-50"
        >
          <Pause className="size-4" /> Halt
        </button>
        <button
          onClick={() => act("/api/resume")}
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
