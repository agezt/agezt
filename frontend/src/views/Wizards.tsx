import { useState, type ReactNode } from "react";
import { Wand2, X, Bot, CalendarClock, KeyRound, Check, ArrowRight, type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { Setup } from "@/views/Setup";
import { NewAgentForm } from "@/views/Roster";
import { NewScheduleForm } from "@/views/Schedules";

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
    return <DoneState message="Schedule created — it will run on its cadence." onAnother={() => setDone(false)} onClose={onClose} />;
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
    desc: "Have the daemon run an instruction on a cadence — every N minutes, daily, or once.",
    icon: CalendarClock,
    hue: "#34d399",
    render: (close) => <ScheduleWizard onClose={close} />,
  },
];

export function Wizards() {
  const [active, setActive] = useState<WizardDef | null>(null);
  const close = () => setActive(null);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Wand2 className="size-4 text-accent" />
        <h2 className="text-sm font-semibold">Wizards</h2>
        <span className="text-xs text-muted">guided flows — finish a task without hunting through menus</span>
      </div>

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
