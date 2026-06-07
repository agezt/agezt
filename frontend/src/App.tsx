import { useState, type ComponentType } from "react";
import {
  Workflow,
  LayoutDashboard,
  ListTree,
  Wallet,
  Radio,
  Settings,
  Database,
  Cpu,
  Wrench,
  Shield,
  CalendarClock,
  Network,
  Sparkles,
  Anchor,
  Brain,
  Inbox as InboxIcon,
  CheckSquare,
  Pause,
  Play,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { ThemeToggle } from "@/components/ThemeToggle";
import { EventFeed } from "@/components/EventFeed";
import { Status } from "@/views/Status";
import { Runs } from "@/views/Runs";
import { Budget } from "@/views/Budget";
import { FlowStudio } from "@/views/FlowStudio";
import { GenericPanel } from "@/views/GenericPanel";

interface NavItem {
  id: string;
  label: string;
  icon: LucideIcon;
  render: ComponentType;
}

function panel(title: string, path: string): ComponentType {
  return () => <GenericPanel title={title} path={path} />;
}

const NAV: NavItem[] = [
  { id: "flow", label: "Flow Studio", icon: Workflow, render: FlowStudio },
  { id: "overview", label: "Overview", icon: LayoutDashboard, render: Status },
  { id: "runs", label: "Runs", icon: ListTree, render: Runs },
  { id: "budget", label: "Budget", icon: Wallet, render: Budget },
  { id: "feed", label: "Event Feed", icon: Radio, render: EventFeed },
  { id: "config", label: "Config", icon: Settings, render: panel("Config", "/api/config") },
  { id: "cache", label: "Cache", icon: Database, render: panel("Cache", "/api/cache") },
  { id: "providers", label: "Providers", icon: Cpu, render: panel("Providers", "/api/providers") },
  { id: "tools", label: "Tools", icon: Wrench, render: panel("Tools", "/api/tools") },
  { id: "policy", label: "Policy", icon: Shield, render: panel("Policy", "/api/policy") },
  { id: "schedules", label: "Schedules", icon: CalendarClock, render: panel("Schedules", "/api/schedules") },
  { id: "world", label: "World", icon: Network, render: panel("World", "/api/world") },
  { id: "skills", label: "Skills", icon: Sparkles, render: panel("Skills", "/api/skills") },
  { id: "standing", label: "Standing", icon: Anchor, render: panel("Standing", "/api/standing") },
  { id: "memory", label: "Memory", icon: Brain, render: panel("Memory", "/api/memory") },
  { id: "inbox", label: "Inbox", icon: InboxIcon, render: panel("Inbox", "/api/inbox") },
  { id: "reflect", label: "Reflection", icon: Brain, render: panel("Reflection", "/api/reflect") },
  { id: "approvals", label: "Approvals", icon: CheckSquare, render: panel("Approvals", "/api/approvals") },
];

export default function App() {
  const [active, setActive] = useState("flow");
  const { connected } = useEvents();
  const current = NAV.find((n) => n.id === active) || NAV[0];
  const View = current.render;

  return (
    <div className="flex h-full flex-col">
      <Header connected={connected} />
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

function Header({ connected }: { connected: boolean }) {
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
