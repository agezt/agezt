import { getTheme, setTheme, type Theme } from "@/lib/theme";
import { getAccentHue, saveAccentHue } from "@/lib/accent";
import { getConsoleName, saveConsoleName } from "@/lib/brand";

// Appearance backup (M735): the per-device console look — theme, accent hue, console
// name — bundled into one portable file so you can carry "your" console to another
// browser or machine, mirroring the prompts (M724) / routing (M727) import-export.
// These prefs live in localStorage, not the daemon, so the bundle is pure frontend.

export interface AppearanceBundle {
  theme?: Theme;
  accentHue?: number;
  consoleName?: string;
}

// exportAppearance snapshots the current look as a versioned, wrapped object — the
// shape downloadText writes and parseAppearanceJSON reads back.
export function exportAppearance(): { version: number; appearance: Required<AppearanceBundle> } {
  return {
    version: 1,
    appearance: {
      theme: getTheme(),
      accentHue: getAccentHue(),
      consoleName: getConsoleName(),
    },
  };
}

// parseAppearanceJSON normalises an imported file into a partial bundle, keeping only
// recognised, valid fields (so a foreign/garbage file can't poison the look). Accepts
// either a bare object or a `{appearance:{…}}` wrapper. Throws on bad JSON or a file
// with no usable appearance fields.
export function parseAppearanceJSON(text: string): AppearanceBundle {
  const data = JSON.parse(text);
  const src = data && typeof data.appearance === "object" && data.appearance ? data.appearance : data;
  if (!src || typeof src !== "object" || Array.isArray(src)) {
    throw new Error("expected an appearance object (or a {appearance:{…}} wrapper)");
  }
  const out: AppearanceBundle = {};
  if (src.theme === "dark" || src.theme === "light") out.theme = src.theme;
  if (typeof src.accentHue === "number" && Number.isFinite(src.accentHue)) {
    // Normalise to [0,360) so an out-of-range hue still maps onto the wheel.
    out.accentHue = ((src.accentHue % 360) + 360) % 360;
  }
  if (typeof src.consoleName === "string" && src.consoleName.trim()) {
    out.consoleName = src.consoleName.trim().slice(0, 32);
  }
  if (Object.keys(out).length === 0) {
    throw new Error("no valid appearance fields (theme / accentHue / consoleName) found");
  }
  return out;
}

// applyAppearanceBundle applies each present field through its shared-store setter, so
// the change persists AND every consumer (header picker, rename control, theme button)
// re-renders in lockstep.
export function applyAppearanceBundle(b: AppearanceBundle): void {
  if (b.theme) setTheme(b.theme);
  if (b.accentHue != null) saveAccentHue(b.accentHue);
  if (b.consoleName != null) saveConsoleName(b.consoleName);
}
