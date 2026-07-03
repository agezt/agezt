// Small helper layer for the app's hash router. The router in App.tsx (see
// `viewFromHash`) listens for `hashchange` and reflects the hash back into the
// active view; setting `location.hash` is therefore the canonical "navigate"
// verb. Centralising it here keeps callers from sprinkling raw
// `location.hash = …` statements (which are easy to typo: missing the leading
// `#`, double-encoding, etc.) and gives tests a single seam to mock.
//
// Until this layer existed, several call sites wrote `window.location.hash =
// \`files?path=...\`` directly. They were functional but inconsistent: some
// used `location`, others `window.location`, and some stripped the leading
// `#` from the value (which is a no-op) while others didn't. `goToHash`
// normalises all of that.
//
// `goToView` is a tiny convenience for the common "switch to <nav id>"
// pattern, used by FeatureCard etc. It also accepts an opaque query string
// (e.g. `?path=notes/README.md`) so callers can deep-link without re-deriving
// the URL.

/**
 * Normalise a hash target so it begins with a single `#`. An empty input is
 * left empty — callers decide whether to navigate to "" or to skip.
 */
export function normaliseHash(target: string): string {
  if (!target) return "";
  // Trim first so whitespace-only input reads as empty rather than "#   ".
  const trimmed = target.trim();
  if (!trimmed) return "";
  // Strip a single leading "#" if present, then re-add it. We accept either
  // form because typing "#agents" in a console is muscle memory for users
  // coming from apps where the hash is always displayed.
  const stripped = trimmed.replace(/^#?/, "");
  return stripped ? `#${stripped}` : "";
}

/**
 * Navigate to a hash target. Refuses to do anything when the target is empty,
 * and avoids emitting a `hashchange` event when the destination already
 * matches the current hash (which would otherwise re-render the active view).
 *
 * Safe to call from anywhere; the App hashchange listener picks it up on the
 * next tick. Falls back silently on non-browser environments (server-side
 * rendering, unit tests).
 */
export function goToHash(target: string): void {
  if (typeof window === "undefined" || typeof location === "undefined") return;
  const next = normaliseHash(target);
  if (!next) return;
  if (location.hash === next) return;
  location.hash = next;
}

/**
 * Navigate to a known nav view id, optionally with a query string. The view
 * id is the un-hashed identifier the router already understands (e.g. "agents",
 * "files", "chat"). The query is appended verbatim after "?".
 *
 * @example goToView("files", `path=${encodeURIComponent("notes/README.md")}`)
 */
export function goToView(view: string, query: string = ""): void {
  if (!view) return;
  const q = query ? (query.startsWith("?") ? query : `?${query}`) : "";
  goToHash(`${view}${q}`);
}
