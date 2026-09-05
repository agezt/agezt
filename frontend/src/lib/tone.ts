// The console's colour language, in one place.
//
// A tone is a MEANING, not a colour: `good` is "this is working", `warn` is
// "look at this", `bad` is "this is broken", `accent` is "this is live / the
// primary thing", `muted` is "context". Views name the meaning; this file
// decides what it looks like, so the same status reads the same on every page.
//
// It exists because ten different views had each declared their own
// tone→class lookup — Health, Storage, Policy, Providers, Insights, Memory,
// Skills, Dashboard, the roster cards, the metric widget — with slightly
// different opacities and borders. Same idea, ten looks: that is what made the
// console feel like ten products.

export type Tone = "accent" | "good" | "warn" | "bad" | "muted";

/** Foreground colour only — for text and icons sitting on a normal surface. */
export const toneText: Record<Tone, string> = {
  accent: "text-accent",
  good: "text-good",
  warn: "text-warn",
  bad: "text-bad",
  muted: "text-muted",
};

/** A tinted chip/plate: soft background + matching foreground. */
export const toneChip: Record<Tone, string> = {
  accent: "bg-accent/10 text-accent",
  good: "bg-good/10 text-good",
  warn: "bg-warn/10 text-warn",
  bad: "bg-bad/10 text-bad",
  muted: "bg-panel text-muted",
};

/** Background tint only — pair it with `toneText` when you need them separate. */
export const toneBg: Record<Tone, string> = {
  accent: "bg-accent/10",
  good: "bg-good/10",
  warn: "bg-warn/10",
  bad: "bg-bad/10",
  muted: "bg-panel",
};

/** A bordered plate — the icon square at the head of a panel. */
export const tonePlate: Record<Tone, string> = {
  accent: "border-accent/40 bg-accent/5 text-accent",
  good: "border-good/40 bg-good/5 text-good",
  warn: "border-warn/40 bg-warn/5 text-warn",
  bad: "border-bad/40 bg-bad/5 text-bad",
  muted: "border-border bg-panel text-muted",
};

/** A whole surface tinted by its state — callout cards, diagnostic banners. */
export const toneSurface: Record<Tone, string> = {
  accent: "border-accent/40 bg-accent/5",
  good: "border-good/40 bg-good/5",
  warn: "border-warn/40 bg-warn/5",
  bad: "border-bad/40 bg-bad/5",
  muted: "border-border bg-panel/40",
};

/** Border tint only — for a tile that carries its state in its edge. */
export const toneBorder: Record<Tone, string> = {
  accent: "border-accent/30",
  good: "border-good/30",
  warn: "border-warn/30",
  bad: "border-bad/30",
  muted: "border-border",
};

/** A solid bar segment — distribution bars, gauges. */
export const toneBar: Record<Tone, string> = {
  accent: "bg-accent",
  good: "bg-good",
  warn: "bg-warn",
  bad: "bg-bad",
  muted: "bg-muted/40",
};

/**
 * toneForRate maps a 0..100 percentage to a tone using the console's standard
 * thresholds, so "90% is green" means the same thing on every page.
 * `invert` is for rates where LOWER is better (error rate, fallback rate).
 */
export function toneForRate(pct: number, invert = false): Tone {
  if (invert) return pct === 0 ? "good" : pct < 10 ? "warn" : "bad";
  return pct >= 90 ? "good" : pct >= 70 ? "warn" : "bad";
}

// STATUS_TONE is the console's shared status vocabulary. The same word must mean
// the same colour wherever it appears: "failed" was text-bad on one page and
// border-bad/40 on another, "done" was border-good here and text-good there, and
// the fleet, the agent detail tabs, the inspector and the insights charts each
// decided independently.
const STATUS_TONE: Record<string, Tone> = {
  // Healthy / finished well.
  ok: "good",
  good: "good",
  done: "good",
  completed: "good",
  complete: "good",
  passed: "good",
  pass: "good",
  ready: "good",
  armed: "good",
  enabled: "good",
  installed: "good",
  supported: "good",
  // In progress — the live tone.
  running: "accent",
  active: "accent",
  live: "accent",
  working: "accent",
  routed: "accent",
  streaming: "accent",
  // Needs a look.
  warn: "warn",
  warning: "warn",
  degraded: "warn",
  pending: "warn",
  stalled: "warn",
  blocked: "warn",
  quarantined: "warn",
  unsupported: "warn",
  // Broken.
  bad: "bad",
  failed: "bad",
  fail: "bad",
  error: "bad",
  cancelled: "bad",
  canceled: "bad",
  denied: "bad",
  // Inert.
  idle: "muted",
  retired: "muted",
  disabled: "muted",
  archived: "muted",
  unknown: "muted",
  "": "muted",
};

/**
 * toneForStatus maps a status word to its meaning. Unknown words read as
 * `muted` — a status nobody has classified is context, not an alarm.
 */
export function toneForStatus(status?: string | null): Tone {
  return STATUS_TONE[(status || "").toLowerCase().trim()] ?? "muted";
}
