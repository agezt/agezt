import { useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

// ActivityChip is the always-visible "something is working" signal in the global
// header ([[product-layer-priority]]). When a run is in flight ANYWHERE — a chat
// reply still streaming, a tool call, an autonomous agent, a background job — it
// lights up with a spinner + count so the operator never mistakes "busy in the
// background" for "frozen". It reflects the same live active-run count the
// Overseer badge uses (folded from the SSE event buffer), so it is honest and
// needs no extra wiring per view.
//
// Visible from every screen including Chat (the header is part of the app shell,
// not the routed view), which is exactly the moment the user reported feeling
// stuck: you send a message, the agent thinks/uses tools for a while, and the
// chat pane is quiet — now the header chip is visibly alive.

// useActiveAfterglow returns true while count > 0, and stays true for `holdMs`
// after it drops back to 0, so a very short operation still flashes the chip for
// a beat instead of flickering past below the eye's notice.
function useActiveAfterglow(count: number, holdMs = 1500): boolean {
  const [on, setOn] = useState(count > 0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    if (count > 0) {
      if (timer.current) clearTimeout(timer.current);
      timer.current = undefined;
      setOn(true);
      return;
    }
    timer.current = setTimeout(() => setOn(false), holdMs);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [count, holdMs]);
  return on;
}

export function ActivityChip({ count, onClick }: { count: number; onClick?: () => void }) {
  const active = useActiveAfterglow(count);
  if (!active) return null;
  const n = Math.max(count, 0);
  const countLabel = n > 99 ? "99+" : String(n);
  return (
    <button
      type="button"
      onClick={onClick}
      aria-live="polite"
      aria-label={n > 0 ? `${countLabel} işlem arka planda çalışıyor` : "arka planda çalışıyor"}
      title={n > 0 ? `${n} run${n === 1 ? "" : "s"} in flight — open the Overseer` : "working in the background — open the Overseer"}
      className={cn(
        "now-in glow-accent ml-1 inline-flex shrink-0 items-center gap-1.5 rounded-full",
        "border border-accent/40 bg-accent/10 px-2 py-0.5 text-xs font-medium text-accent",
        "transition-shadow hover:shadow-e2",
      )}
    >
      <Loader2 className="size-3 animate-spin" aria-hidden="true" />
      <span>working</span>
      {n > 0 && (
        <span className="rounded-full bg-accent/20 px-1 text-xs font-semibold tabular-nums">{countLabel}</span>
      )}
      <span className="work-pulse size-1.5 rounded-full bg-accent" aria-hidden="true" />
    </button>
  );
}
