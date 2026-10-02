import { useEffect, useState } from "react";
import { Activity, Wallet, CalendarClock, Sparkles, CheckSquare, Pause } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { getJSON } from "@/app/api";
import { money } from "@/app/format";
import { cn } from "@/app/utils";
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
    <div>
      <div className="flex shrink-0 items-center gap-1 overflow-x-auto bg-background px-3 py-1.5 text-xs">
      {st?.halted && (
        <span className="mr-1 inline-flex items-center gap-1 rounded-full bg-bad/15 px-2 py-0.5 font-semibold text-bad">
          <Pause className="size-3" /> HALTED
        </span>
      )}
      {/* This counts active_runs, but read "0 runs" beside "$0.00 today" and it
          says no run has happened — which is false the moment one finishes.
          Name what is actually counted. */}
      <Vital icon={Activity} label="running" value={runs} live={runs > 0} onClick={() => onNavigate("activity")} />
      {/* No onClick: "budget" was retired with no live equivalent, so there is
          nothing honest to link to. The number still reads out; it just stopped
          promising a page. */}
      <Vital icon={Wallet} label="today" value={money(bg?.spent_mc ?? 0)} />
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
      <div className="gradient-rule" />
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
  onClick?: () => void;
}) {
  const className = cn(
    "inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-0.5 transition-colors",
    onClick && "focus-glow",
    attention ? "bg-amber-500/10 text-amber-500" : "text-muted hover:bg-panel hover:text-foreground",
  );
  const body = (
    <>
      <Icon className={cn("size-3.5", live && "animate-pulse text-good", attention && "text-amber-500")} />
      {typeof value === "number" ? (
        <AnimatedNumber value={value} className="tabular-nums font-medium text-foreground" />
      ) : (
        <span className="tabular-nums font-medium text-foreground">{value}</span>
      )}
      <span className="hidden text-muted sm:inline">{label}</span>
    </>
  );
  // A vital with no destination renders as a readout, not a link. The budget
  // vital used to pass onClick={() => onNavigate("budget")} for a view that was
  // retired with "no live equivalent" — so the button promised "Go to today" and
  // delivered Mission Control via the hash fallback. A control that looks
  // clickable and is not is worse than one that never claimed to be.
  if (!onClick) {
    return <span className={className}>{body}</span>;
  }
  return (
    <button onClick={onClick} title={`Go to ${label}`} className={className}>
      {body}
    </button>
  );
}
