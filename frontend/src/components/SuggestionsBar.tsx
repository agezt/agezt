import { useCallback, useEffect, useState } from "react";
import {
  ArrowRight, BookOpen, Bot, Brain, CheckCircle, Copy, GitBranch, Link,
  List, Play, RefreshCw, Scale, Search, Shield, Star, Stethoscope,
  Wrench, Zap, type LucideIcon
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";

// IconMap maps icon names from the backend to Lucide components.
const iconMap: Record<string, LucideIcon> = {
  "search": Search,
  "stethoscope": Stethoscope,
  "git-branch": GitBranch,
  "book-open": BookOpen,
  "link": Link,
  "scale": Scale,
  "play": Play,
  "check-circle": CheckCircle,
  "wrench": Wrench,
  "refresh-cw": RefreshCw,
  "shield": Shield,
  "zap": Zap,
  "star": Star,
  "copy": Copy,
  "list": List,
  "arrow-right": ArrowRight,
  "bot": Bot,
  "brain": Brain,
};

// Suggestion is one clickable next-prompt chip.
export interface Suggestion {
  id: string;
  label: string;
  prompt: string;
  category: string;
  icon?: string;
}

// CategoryMeta styles each suggestion category consistently.
const categoryMeta: Record<string, { label: string; chipClass: string }> = {
  memory:  { label: "Memory",  chipClass: "bg-accent/10 text-accent border-accent/30 hover:bg-accent/20" },
  debug:   { label: "Debug",   chipClass: "bg-amber-500/10 text-amber-400 border-amber-500/30 hover:bg-amber-500/20" },
  explore: { label: "Explore", chipClass: "bg-blue-500/10 text-blue-400 border-blue-500/30 hover:bg-blue-500/20" },
  modify:  { label: "Modify",  chipClass: "bg-green-500/10 text-green-400 border-green-500/30 hover:bg-green-500/20" },
  review:  { label: "Review",  chipClass: "bg-purple-500/10 text-purple-400 border-purple-500/30 hover:bg-purple-500/20" },
  workflow:{ label: "Next",    chipClass: "bg-slate-500/10 text-slate-300 border-slate-500/30 hover:bg-slate-500/20" },
};

// SuggestionsBar fetches and displays context-aware suggested next prompts
// after a chat turn completes. Shown only when the run is idle (not busy).
// Clicking a suggestion inserts its prompt text into the chat input.
export function SuggestionsBar({
  sessionId,
  recentTools,
  busy,
  onPick,
}: {
  sessionId?: string;
  recentTools?: string[];
  busy: boolean;
  onPick: (prompt: string) => void;
}) {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [loading, setLoading] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  const fetch_ = useCallback(async () => {
    if (busy || dismissed) return;
    setLoading(true);
    try {
      // The read-args proxy forwards each query param as a single value, so
      // send recent tool names comma-joined (the backend splits on comma).
      const params = new URLSearchParams();
      if (sessionId) params.set("session_id", sessionId);
      if (recentTools && recentTools.length > 0) {
        params.set("tools", recentTools.join(","));
      }

      const qs = params.toString();
      const d = await getJSON<{ suggestions?: Suggestion[] }>(
        "/api/suggestions" + (qs ? `?${qs}` : ""),
      );
      const items = d.suggestions || [];
      setSuggestions(items);
      if (items.length > 0) setDismissed(false);
    } catch {
      // Non-critical: leave suggestions hidden on error.
    } finally {
      setLoading(false);
    }
  }, [busy, dismissed, sessionId, recentTools]);

  // Fetch when run transitions from busy→idle.
  useEffect(() => {
    if (!busy) fetch_();
  }, [busy, fetch_]);

  // Dismissed or no suggestions or still running → hide.
  if (dismissed || loading || busy || suggestions.length === 0) return null;

  return (
    <div
      className="mx-3 mb-2 flex flex-wrap items-center gap-2"
      role="group"
      aria-label="Suggested next prompts"
    >
      <span className="shrink-0 text-xs text-muted-foreground">
        Next:
      </span>
      {suggestions.map((s) => {
        const meta = categoryMeta[s.category] || categoryMeta.workflow;
        const Icon = s.icon ? iconMap[s.icon] : ArrowRight;
        return (
          <button
            key={s.id}
            onClick={() => {
              onPick(s.prompt);
              // Dismiss after picking so the bar doesn't persist.
              setDismissed(true);
            }}
            title={s.prompt}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1",
              "text-xs font-medium transition-colors cursor-pointer focus-glow",
              "animate-in fade-in slide-in-from-top-2 duration-300",
              meta.chipClass,
            )}
          >
            <Icon className="size-3 shrink-0" />
            {s.label}
            <ArrowRight className="size-2.5 shrink-0 opacity-60" />
          </button>
        );
      })}

      {/* Dismiss all */}
      <button
        onClick={() => setDismissed(true)}
        className="shrink-0 rounded-full px-1.5 py-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
        title="Dismiss suggestions"
      >
        ✕
      </button>
    </div>
  );
}
