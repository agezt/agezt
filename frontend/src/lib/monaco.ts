import { loader } from "@monaco-editor/react";

// Monaco loader config (M1017). The editor itself is loaded on demand from a
// pinned CDN — NOT bundled with the SPA — so the embedded `kernel/webui/dist`
// stays small (~3 MB of editor code never ships in the binary). To self-host
// later, point `paths.vs` at the vendored `monaco-editor/min/vs` directory.
//
// The pinned version is the only knob that affects build behaviour; bump it
// deliberately, not on a floating latest.

export const PINNED_MONACO_VERSION = "0.52.2";
export const MONACO_CDN_BASE = `https://cdn.jsdelivr.net/npm/monaco-editor@${PINNED_MONACO_VERSION}/min/vs`;

// ensureLoader is idempotent: calling it more than once is harmless. It MUST
// run before any `<Editor />` mounts so the first paint doesn't try to fetch
// from the network unconfigured.
export function ensureLoader(): void {
  if ((loader as { _configured?: boolean })._configured) return;
  try {
    loader.config({ paths: { vs: MONACO_CDN_BASE } });
  } catch {
    // The loader throws if it's already been configured (e.g. from a hot
    // reload during dev). That's fine — the prior config wins.
  }
  (loader as { _configured?: boolean })._configured = true;
}

ensureLoader();
