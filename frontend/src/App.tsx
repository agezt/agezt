import { useEffect, useMemo, useRef, useState, type ComponentType } from "react";
import {
  MessageSquare,
  Activity as ActivityIcon,
  Clapperboard,
  Waypoints,
  Users,
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
  Archive,
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
  Layers,
  Route as RouteIcon,
  Bot,
  MessageSquarePlus,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { postAction, getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { attentionAlertCount } from "@/lib/alerts";
import { CommandPalette } from "@/components/CommandPalette";
import { MiniChat } from "@/components/MiniChat";
import { AlertBell } from "@/components/AlertBell";
import { Vitals } from "@/components/Vitals";
import { useUI, type ConfirmOptions } from "@/components/ui/feedback";

type ConfirmRequest = ConfirmOptions;
import type { CommandItem } from "@/lib/commands";
import { ThemeToggle } from "@/components/ThemeToggle";
import { toggleTheme } from "@/lib/theme";
import { useChat } from "@/lib/chatStore";
import { focusRun } from "@/lib/runfocus";
import { exportAppearance, parseAppearanceJSON, applyAppearanceBundle } from "@/lib/appearance";
import { parseConfigBundle, fetchConfigBundle, applyConfigBundle } from "@/lib/configbackup";
import { downloadText } from "@/lib/export";
import { AccentPicker } from "@/components/AccentPicker";
import { ConsoleName } from "@/components/ConsoleName";
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
import { Roster } from "@/views/Roster";
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
import { Models } from "@/views/Models";
import { Routing } from "@/views/Routing";
import { Persona } from "@/views/Persona";
import { Prompts } from "@/views/Prompts";
import { Backup } from "@/views/Backup";
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
      { id: "roster", label: "Roster", icon: Users, render: Roster },
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
      { id: "persona", label: "Persona", icon: Bot, render: Persona },
      { id: "prompts", label: "Prompts", icon: MessageSquarePlus, render: Prompts },
      { id: "configcenter", label: "Config Center", icon: SlidersHorizontal, render: ConfigCenter },
      { id: "config", label: "Config", icon: Settings, render: Config },
      { id: "providers", label: "Providers", icon: Cpu, render: Providers },
      { id: "models", label: "Models", icon: Layers, render: Models },
      { id: "routing", label: "Routing", icon: RouteIcon, render: Routing },
      { id: "tools", label: "Tools", icon: Wrench, render: Tools },
      { id: "catalog", label: "Catalog", icon: Boxes, render: Catalog },
      { id: "policy", label: "Policy", icon: Shield, render: Policy },
      { id: "cache", label: "Cache", icon: Database, render: Cache },
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
  const { newChat } = useChat();
  const [paletteOpen, setPaletteOpen] = useState(false);
  // Recent runs offered as ⌘K "Open run" commands (fulfils the palette's promise).
  // Refreshed whenever the palette opens so the list is current without polling.
  const [recentRuns, setRecentRuns] = useState<{ correlation_id?: string; intent?: string; status?: string }[]>([]);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(loadCollapsed);
  const { connected, events } = useEvents();
  const ui = useUI();

  // Unseen-alert badge on the Alerts nav item (M779): count the critical/warning alerts
  // in the live buffer so the cockpit flags "something needs attention" from anywhere —
  // not only when you happen to open the Alerts tab. Opening that tab marks them seen.
  const liveAlertCount = useMemo(() => attentionAlertCount(events), [events]);
  const [seenAlerts, setSeenAlerts] = useState(0);
  useEffect(() => {
    if (active === "alerts") setSeenAlerts(liveAlertCount);
  }, [active, liveAlertCount]);
  const unseenAlerts = Math.max(0, liveAlertCount - seenAlerts);

  const current = NAV.find((n) => n.id === active) || NAV[0];
  const View = current.render;
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

  // Export the daemon-side config (persona + prompts + routing) as one bundle.
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
        id: "act-theme",
        label: "Toggle theme",
        group: "Action",
        keywords: "dark light appearance",
        run: () => toggleTheme(),
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
        label: "Export configuration (persona, prompts, routing)",
        group: "Action",
        keywords: "backup config persona prompts routing download daemon profile",
        run: () => void exportConfig(),
      },
      {
        id: "act-config-import",
        label: "Import configuration (persona, prompts, routing)",
        group: "Action",
        keywords: "restore config persona prompts routing upload daemon profile",
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
    return [...views, ...actions, ...runCmds];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ui, newChat, recentRuns]);

  return (
    <div className="flex h-full flex-col">
      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} items={commands} />
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
                    {n.id === "alerts" && unseenAlerts > 0 && (
                      <span
                        className="ml-auto inline-flex min-w-4 items-center justify-center rounded-full bg-bad px-1 text-[10px] font-semibold leading-4 text-white"
                        title={`${unseenAlerts} new alert${unseenAlerts === 1 ? "" : "s"} — the agent flagged something`}
                        aria-label={`${unseenAlerts} unseen alerts`}
                      >
                        {unseenAlerts > 99 ? "99+" : unseenAlerts}
                      </span>
                    )}
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
      <ConsoleName />
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
        <AccentPicker />
        <ThemeToggle />
      </div>
    </header>
  );
}
