import { cn } from "@/lib/utils";
import { agentHue, initials } from "@/lib/agent";

// AgentAvatar (M948) is the shared agent identity chip: a gradient monogram in
// the agent's deterministic hue (so it's recognisable at a glance everywhere),
// with an optional live status. "running" gives it a breathing halo + a pulsing
// good-dot; "sleeping"/"paused" keep a small state marker; "retired" desaturates
// it. Used by the roster, the fleet strip, and the cockpit so an agent looks the
// same wherever it appears.
export function AgentAvatar({
  slug,
  name,
  size = 32,
  status,
  className,
}: {
  slug: string;
  name?: string;
  size?: number;
  status?: "running" | "sleeping" | "paused" | "retired";
  className?: string;
}) {
  const hue = agentHue(slug);
  const running = status === "running";
  const sleeping = status === "sleeping";
  const paused = status === "paused";
  const retired = status === "retired";
  return (
    <span
      aria-hidden
      className={cn(
        "relative flex shrink-0 items-center justify-center rounded-md font-semibold text-white",
        running && "breathe",
        retired && "opacity-40 grayscale",
        className,
      )}
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.38),
        // A two-stop diagonal gradient (hue → a neighbour hue) reads richer and
        // more "alive" than the old flat fill, while staying the agent's colour.
        background: `linear-gradient(135deg, hsl(${hue} 62% 50%), hsl(${(hue + 42) % 360} 60% 38%))`,
      }}
    >
      {initials(name, slug)}
      {running && (
        <span className="work-pulse absolute -right-0.5 -top-0.5 size-2 rounded-full bg-good ring-2 ring-card" />
      )}
      {sleeping && (
        <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-muted ring-2 ring-card" />
      )}
      {paused && (
        <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-warn ring-2 ring-card" />
      )}
    </span>
  );
}
