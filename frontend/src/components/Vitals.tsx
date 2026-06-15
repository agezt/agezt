import { useEffect, useState } from "react";
import { Activity, Wallet, CalendarClock, Sparkles, CheckSquare, Pause } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { getJSON } from "@/lib/api";
import { money } from "@/lib/format";
import { cn } from "@/lib/utils";
import { AnimatedNumber } from "@/components/AnimatedNumber";

interface Status {
  halted?: boolean;
  active_runs?: number;
  active_skills?: number;
  pending_approvals?: number;
  schedules?: { total?: number; enabled?: number };
}
interface Budget {
  spent_mc?: number;
}

// Vitals is the always-visible monitoring strip under the header: the system's
// live pulse — in-flight runs, today's spend, active schedules and skills, and
// anything that needs attention (pending approvals, a halt) — glanceable from
// EVERY view, not just Mission Control. Each chip deep-links to the relevant view.
export function Vitals({ onNavigate }: { onNavigate: (id: string) => void }) {
  const [st, setSt] = useState<Status | null>(null);
  const [bg, setBg] = useState<Budget | null>(null);

  useEffect(() => {
    let live = true;
    async function tick() {
      try {
        const [s, b] = await Promise.all([
          getJSON<Status>("/api/status"),
          getJSON<Budget>("/api/budget").catch(() => ({}) as Budget),
        ]);
        if (live) {
          setSt(s);
          setBg(b);
        }
      } catch {
        /* a transient fetch failure just leaves the last values */
      }
    }
    tick();
    const id = setInterval(tick, 5000);
    return () => {
      live = false;
      clearInterval(id);
    };
  }, []);

  const runs = st?.active_runs ?? 0;
  const pending = st?.pending_approvals ?? 0;
  const schedEnabled = st?.schedules?.enabled ?? 0;

  return (
    <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border bg-background px-3 py-1.5 text-xs">
      {st?.halted && (
        <span className="mr-1 inline-flex items-center gap-1 rounded-full bg-bad/15 px-2 py-0.5 font-semibold text-bad">
          <Pause className="size-3" /> HALTED
        </span>
      )}
      <Vital icon={Activity} label="runs" value={runs} live={runs > 0} onClick={() => onNavigate("activity")} />
      <Vital icon={Wallet} label="today" value={money(bg?.spent_mc ?? 0)} onClick={() => onNavigate("budget")} />
      <Vital
        icon={CalendarClock}
        label="schedules"
        value={schedEnabled}
        onClick={() => onNavigate("schedules")}
      />
      <Vital icon={Sparkles} label="skills" value={st?.active_skills ?? 0} onClick={() => onNavigate("skills")} />
      {pending > 0 && (
        <Vital
          icon={CheckSquare}
          label="approvals"
          value={pending}
          attention
          onClick={() => onNavigate("approvals")}
        />
      )}
    </div>
  );
}

function Vital({
  icon: Icon,
  label,
  value,
  live,
  attention,
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  value: string | number;
  live?: boolean;
  attention?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      title={`Go to ${label}`}
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-0.5 transition-colors hover:bg-panel",
        attention ? "text-amber-500" : "text-muted hover:text-foreground",
      )}
    >
      <Icon className={cn("size-3.5", live && "animate-pulse text-good", attention && "text-amber-500")} />
      {typeof value === "number" ? (
        <AnimatedNumber value={value} className="tabular-nums font-medium text-foreground" />
      ) : (
        <span className="tabular-nums font-medium text-foreground">{value}</span>
      )}
      <span className="hidden text-muted sm:inline">{label}</span>
    </button>
  );
}
