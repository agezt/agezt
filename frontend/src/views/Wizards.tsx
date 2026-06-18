import { useState, type ReactNode } from "react";
import { Wand2, X, Bot, CalendarClock, KeyRound, Check, ArrowRight, Plug, Anchor, Wallet, type LucideIcon } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { postAction } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Setup } from "@/views/Setup";
import { QuickConnect } from "@/views/QuickConnect";
import { NewAgentForm, usdToMc } from "@/views/Roster";
import { NewScheduleForm } from "@/views/Schedules";
import { NewServerForm } from "@/views/Mcp";
import { NewOrderForm } from "@/views/Standing";

// Wizards (M949) is the "get things done without hunting through menus" hub:
// guided, step-by-step flows that complete a whole task in a focused overlay.
// Each flow reuses the EXISTING forms/endpoints (the first-run Setup for
// provider+model, NewAgentForm, NewScheduleForm) — this is sequencing + a
// launcher, not new behaviour — so adding a wizard later is one registry entry.

interface WizardDef {
  id: string;
  title: string;
  desc: string;
  icon: LucideIcon;
  hue: string; // accent colour for the card
  render: (close: () => void) => ReactNode;
}

// DoneState caps a wizard with a success screen + a path to repeat or leave,
// so a flow finishes inside the wizard instead of dumping you back into a view.
function DoneState({ message, onAnother, onClose }: { message: string; onAnother?: () => void; onClose: () => void }) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-good/40 bg-good/5 p-6 text-center">
      <span className="flex size-12 items-center justify-center rounded-full bg-good/15 text-good">
        <Check className="size-6" />
      </span>
      <div className="text-sm font-medium">{message}</div>
      <div className="flex gap-2">
        {onAnother && (
          <Button variant="ghost" size="sm" onClick={onAnother}>
            Do another
          </Button>
        )}
        <Button size="sm" onClick={onClose}>
          Done
        </Button>
      </div>
    </div>
  );
}

function AgentWizard({ onClose }: { onClose: () => void }) {
  const ui = useUI();
  const [created, setCreated] = useState<string | null>(null);
  if (created) {
    return <DoneState message={`Agent “${created}” created — run it from Chat or delegate by name.`} onAnother={() => setCreated(null)} onClose={onClose} />;
  }
  return (
    <NewAgentForm
      onCreated={(slug) => {
        ui.toast(`agent ${slug} created`, "success");
        setCreated(slug);
      }}
      onError={(m) => ui.toast(m, "error")}
    />
  );
}

function ScheduleWizard({ onClose }: { onClose: () => void }) {
  const ui = useUI();
  const [done, setDone] = useState(false);
  if (done) {
    return <DoneState message="Schedule created — cron will trigger its selected target." onAnother={() => setDone(false)} onClose={onClose} />;
  }
  return (
    <NewScheduleForm
      onCreated={() => {
        ui.toast("schedule created", "success");
        setDone(true);
      }}
      onError={(m) => ui.toast(m, "error")}
    />
  );
}

function McpWizard({ onClose }: { onClose: () => void }) {
  const ui = useUI();
  const [created, setCreated] = useState<string | null>(null);
  if (created) {
    return <DoneState message={`MCP server “${created}” attached — its tools are now available to agents.`} onAnother={() => setCreated(null)} onClose={onClose} />;
  }
  return (
    <NewServerForm
      onCreated={(name) => {
        ui.toast(`MCP server ${name} added`, "success");
        setCreated(name);
      }}
      onError={(m) => ui.toast(m, "error")}
    />
  );
}

function StandingWizard({ onClose }: { onClose: () => void }) {
  const ui = useUI();
  const [created, setCreated] = useState<string | null>(null);
  if (created) {
    return <DoneState message={`Standing order “${created}” created — it fires on its trigger.`} onAnother={() => setCreated(null)} onClose={onClose} />;
  }
  return (
    <NewOrderForm
      onCreated={(name) => {
        ui.toast(`standing order ${name} created`, "success");
        setCreated(name);
      }}
      onError={(m) => ui.toast(m, "error")}
    />
  );
}

function BudgetWizard({ onClose }: { onClose: () => void }) {
  const ui = useUI();
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  async function apply(unlimited: boolean) {
    const mc = unlimited ? 0 : usdToMc(draft);
    if (mc === null) {
      ui.toast("enter a dollar amount like 25 or 1.50", "error");
      return;
    }
    setBusy(true);
    try {
      await postAction("/api/budget_set", { ceiling_mc: String(mc) });
      ui.toast(unlimited ? "daily ceiling removed" : `daily ceiling set to $${(mc / 1e9).toFixed(2)}`, "success");
      setDone(unlimited ? "Unlimited — no daily ceiling." : `Daily ceiling set to $${(mc / 1e9).toFixed(2)}.`);
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }
  if (done) return <DoneState message={done} onClose={onClose} />;
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <p className="text-xs text-muted">
        Set the daily spend ceiling. The daemon halts spend before it exceeds this — leave it unlimited to remove the cap.
      </p>
      <label className="flex flex-col gap-1 text-[11px] text-muted">
        Daily ceiling (USD)
        <input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && apply(false)}
          placeholder="25.00"
          aria-label="Daily ceiling in USD"
          className="rounded-md border border-border bg-panel px-2 py-1 text-sm text-foreground outline-none focus-visible:border-accent"
        />
      </label>
      <div className="flex gap-2">
        <Button size="sm" onClick={() => apply(false)} disabled={busy || !draft.trim()}>
          Set ceiling
        </Button>
        <Button size="sm" variant="ghost" onClick={() => apply(true)} disabled={busy}>
          Unlimited
        </Button>
      </div>
    </div>
  );
}

const WIZARDS: WizardDef[] = [
  {
    id: "provider",
    title: "Connect a provider",
    desc: "Add an API key and pick a default model — the three-step path from zero to a thinking daemon.",
    icon: KeyRound,
    hue: "var(--accent)",
    render: (close) => <Setup onDone={close} />,
  },
  {
    id: "quickconnect",
    title: "Quick Connect a coding plan",
    desc: "Paste a key for Z.ai/GLM, MiniMax, Kimi, DeepSeek, opencode and more — one click, no endpoint setup.",
    icon: Plug,
    hue: "#2563eb",
    render: () => <QuickConnect />,
  },
  {
    id: "agent",
    title: "Create an agent",
    desc: "Give a new roster agent its soul, model, and daily budget — then run it by name.",
    icon: Bot,
    hue: "#a78bfa",
    render: (close) => <AgentWizard onClose={close} />,
  },
  {
    id: "schedule",
    title: "Schedule a task",
    desc: "Create a cron trigger for an agent wake, workflow, system task, or tool call.",
    icon: CalendarClock,
    hue: "#34d399",
    render: (close) => <ScheduleWizard onClose={close} />,
  },
  {
    id: "mcp",
    title: "Add an MCP server",
    desc: "Attach an external Model-Context-Protocol server so its tools become available to your agents.",
    icon: Plug,
    hue: "#60a5fa",
    render: (close) => <McpWizard onClose={close} />,
  },
  {
    id: "standing",
    title: "Add a standing order",
    desc: "Create a durable standing order that can wake an agent from a schedule, event, or channel trigger.",
    icon: Anchor,
    hue: "#fbbf24",
    render: (close) => <StandingWizard onClose={close} />,
  },
  {
    id: "budget",
    title: "Set the daily budget",
    desc: "Cap daily spend — the daemon halts before it exceeds your ceiling.",
    icon: Wallet,
    hue: "#f472b6",
    render: (close) => <BudgetWizard onClose={close} />,
  },
];

export function Wizards() {
  const [active, setActive] = useState<WizardDef | null>(null);
  const close = () => setActive(null);

  return (
    <div className="space-y-4">
      <PageHeader
        icon={Wand2}
        title="Wizards"
        description="Guided flows — finish a task without hunting through menus."
      />

      <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {WIZARDS.map((w) => {
          const Icon = w.icon;
          return (
            <li key={w.id}>
              <button
                onClick={() => setActive(w)}
                className="group flex h-full w-full flex-col gap-2 rounded-xl border border-border bg-card p-4 text-left transition-all hover:-translate-y-0.5 hover:border-accent hover:shadow-lg"
              >
                <span
                  className="flex size-10 items-center justify-center rounded-lg"
                  style={{ color: w.hue, backgroundColor: `color-mix(in oklch, ${w.hue} 16%, transparent)` }}
                >
                  <Icon className="size-5" />
                </span>
                <div className="text-sm font-semibold">{w.title}</div>
                <div className="text-xs leading-relaxed text-muted">{w.desc}</div>
                <span className="mt-auto inline-flex items-center gap-1 pt-1 text-xs font-medium text-accent opacity-0 transition-opacity group-hover:opacity-100">
                  Start <ArrowRight className="size-3" />
                </span>
              </button>
            </li>
          );
        })}
      </ul>

      {active && (
        <div className="fixed inset-0 z-[150] flex items-start justify-center overflow-y-auto bg-background/95 backdrop-blur-sm">
          <div className="mx-auto flex w-full max-w-2xl flex-col gap-3 p-4">
            <div className="flex items-center gap-2">
              <active.icon className="size-5" style={{ color: active.hue }} />
              <h2 className="text-base font-semibold">{active.title}</h2>
              <button className="ml-auto text-muted transition-colors hover:text-foreground" onClick={close} aria-label="Close wizard">
                <X className="size-4" />
              </button>
            </div>
            {active.render(close)}
          </div>
        </div>
      )}
    </div>
  );
}
