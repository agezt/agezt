import { useEffect, useState } from "react";
import { Sparkles, ArrowRight, Ear, Zap, UserRound } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";

// JarvisPresenceCard — a compact entry point to the Jarvis presence page, dropped on
// the Dashboard so the triad (hear / act / know) is discoverable from the landing
// view. Cheap: a single /api/pulse read for the live initiative mode; the full page
// does the rest.
export function JarvisPresenceCard() {
  const [mode, setMode] = useState<string>("");

  useEffect(() => {
    let alive = true;
    getJSON<{ initiative?: string; running?: boolean; paused?: boolean }>("/api/pulse")
      .then((p) => {
        if (!alive) return;
        const init = (p?.initiative || "").toLowerCase();
        const running = !!p?.running && !p?.paused;
        setMode(!running ? "resting" : init === "act" ? "acting on its own" : init === "ask" ? "asking first" : init === "off" ? "observing" : "");
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  return (
    <a
      href="#jarvis"
      className="glass group flex items-center justify-between gap-3 rounded-xl p-3 ring-1 ring-inset ring-transparent transition-all hover:ring-accent/40"
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-accent/25 to-accent2/20 text-accent ring-1 ring-inset ring-accent/30">
          <Sparkles className="size-4" />
        </span>
        <div className="min-w-0">
          <div className="font-semibold">
            Jarvis <span className="text-xs font-normal text-muted">presence</span>
          </div>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted">
            <span className="inline-flex items-center gap-1">
              <Ear className="size-3" /> hears you
            </span>
            <span className="opacity-40">·</span>
            <span className="inline-flex items-center gap-1">
              <Zap className="size-3" /> {mode || "acts for you"}
            </span>
            <span className="opacity-40">·</span>
            <span className="inline-flex items-center gap-1">
              <UserRound className="size-3" /> knows you
            </span>
          </div>
        </div>
      </div>
      <ArrowRight className={cn("size-4 shrink-0 text-muted transition-transform group-hover:translate-x-0.5 group-hover:text-accent")} />
    </a>
  );
}
