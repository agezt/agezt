import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronUp, FileText, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { languageFor } from "@/lib/language";
import { PINNED_MONACO_VERSION } from "@/lib/monaco";

// MonacoView is the project's thin wrapper over @monaco-editor/react so callers
// don't need to know about loader configuration. Lazy-loaded so the editor
// code (and its CDN fetch) only happens when a code block actually becomes
// visible — keeping chat scroll light when no code is on screen.
//
// Props:
//   value         — the buffer to render (string)
//   path          — used to pick the language id; "" → plaintext
//   readOnly      — default true: chat blocks never allow editing. The
//                   standalone Files editor lifts this to false.
//   collapsed     — start with a small height + "Expand" affordance.
//   height        — full editor height in px (only used when not collapsed).
//   onChange      — called on every keystroke when readOnly is false.
//
// The Monaco bundle (~3 MB) is loaded from the pinned jsdelivr CDN configured
// in lib/monaco.ts. We deliberately do NOT bundle the editor into the SPA
// because the embedded `kernel/webui/dist` gets go:embed'd into the daemon
// binary.

const Editor = lazy(async () => {
  // Re-exported under a stable alias so a vitest mock can replace it.
  const mod = await import("@monaco-editor/react");
  return { default: (mod as { Editor: React.ComponentType<Record<string, unknown>> }).Editor };
});

export function MonacoView({
  value,
  path = "",
  readOnly = true,
  collapsed = false,
  height = 360,
  onChange,
  className,
  "data-testid": dataTestId,
}: {
  value: string;
  path?: string;
  readOnly?: boolean;
  collapsed?: boolean;
  height?: number;
  onChange?: (next: string | undefined) => void;
  className?: string;
  /** Pass-through for tests that want to assert on the rendered editor root. */
  "data-testid"?: string;
}) {
  const lang = useMemo(() => languageFor(path), [path]);
  const [expanded, setExpanded] = useState(!collapsed);
  const [copyOk, setCopyOk] = useState(false);
  // `mounted` flips true once the lazy Editor's onMount fires. Until then we
  // render the value as a plain <pre> so the buffer is visible immediately —
  // important for tests, but also for users: chat scroll doesn't blank while
  // the CDN bundle downloads.
  const [mounted, setMounted] = useState(false);
  // Editor instance handle, captured in onMount so we can dispose it on
  // unmount. Without this, switching `path` (or remounting the whole panel)
  // accumulates Monaco models in its internal URI registry until the page
  // slogs down — Monaco leaks unless explicitly torn down.
  // typed as `unknown` because @monaco-editor/react's editor type is the
  // third-party package's own `editor.IEditor`; we only need `dispose()`.
  const editorRef = useRef<{ dispose: () => void } | null>(null);

  // Editor height: collapse mode shows the first ~12 lines (a chat-sized
  // preview), expand mode shows a full editor at `height`. We don't fight
  // Monaco's content-sized mode because it gets twitchy on long pastes.
  const lineCount = useMemo(() => (value ? value.split("\n").length : 1), [value]);
  const collapsedLines = Math.min(12, lineCount);
  const editorHeight = expanded ? Math.max(height, 120) : Math.min(220, collapsedLines * 18 + 36);

  // Empty value: don't mount Monaco at all. The previous guard (`!value &&
  // value !== ""`) was a bug — empty string is falsy so the condition was
  // always false, and a zero-byte buffer paid for a Monaco instance and an
  // eager <pre> with minHeight:editorHeight. Render a tiny placeholder
  // instead.
  if (value === "") {
    return (
      <div className={cn("flex items-center gap-2 rounded-md border border-border bg-card/40 px-3 py-2 text-xs text-muted", className)} data-testid={dataTestId}>
        <FileText className="size-3.5" />
        <span>empty {path || "buffer"}</span>
      </div>
    );
  }

  // Dispose the Monaco editor instance when this component unmounts.
  // Without this the next mount creates a fresh editor AND the old model
  // stays registered with Monaco, leaking memory over many viewings of
  // long chat sessions.
  useEffect(() => {
    return () => {
      editorRef.current?.dispose();
      editorRef.current = null;
    };
  }, []);

  return (
    <div className={cn("group relative overflow-hidden rounded-md border border-border", className)} data-testid={dataTestId}>
      <div className="flex items-center gap-2 border-b border-border bg-panel/60 px-2 py-1 text-[11px] text-muted">
        <span className="font-mono">{lang}</span>
        {path && (
          <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-foreground/70" title={path}>
            {path}
          </span>
        )}
        <span className="ml-auto inline-flex items-center gap-1">
          <button
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(value);
                setCopyOk(true);
                window.setTimeout(() => setCopyOk(false), 1200);
              } catch {
                /* clipboard unavailable — silently no-op */
              }
            }}
            className="rounded border border-border bg-card px-1.5 py-0.5 text-[11px] text-muted hover:text-foreground"
            title={copyOk ? "Copied" : "Copy"}
          >
            {copyOk ? "copied" : "copy"}
          </button>
          <button
            onClick={() => setExpanded((v) => !v)}
            className="inline-flex items-center gap-1 rounded border border-border bg-card px-1.5 py-0.5 text-[11px] text-muted hover:text-foreground"
            title={expanded ? "Collapse" : "Expand"}
            aria-label={expanded ? "Collapse code block" : "Expand code block"}
          >
            {expanded ? <ChevronUp className="size-3" /> : <ChevronDown className="size-3" />}
            {expanded ? "collapse" : "expand"}
          </button>
        </span>
      </div>
      {/*
        Until the lazy Editor mounts, render a plain <pre> with the value so
        the buffer is visible immediately — tests don't need to wait, and in
        production the user sees the code the instant the surface renders
        instead of seeing a "loading…" spinner while the CDN bundle downloads.
      */}
      {!mounted && (
        <pre
          data-testid="monaco-fallback"
          className="overflow-auto whitespace-pre rounded-md bg-card p-3 font-mono text-[12px] leading-relaxed text-foreground/90"
          style={{ minHeight: editorHeight }}
        >
          {value}
        </pre>
      )}
      <Suspense
        fallback={
          <div className="flex items-center gap-2 p-3 text-xs text-muted">
            <Loader2 className="size-3 animate-spin" /> loading editor (v{PINNED_MONACO_VERSION})…
          </div>
        }
      >
        <Editor
          height={`${editorHeight}px`}
          language={lang}
          value={value}
          theme={useMonacoTheme()}
          options={{
            readOnly,
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            fontFamily: "JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, monospace",
            fontSize: 12,
            lineNumbers: readOnly ? "off" : "on",
            wordWrap: "on",
            renderLineHighlight: readOnly ? "none" : "all",
            scrollbar: { vertical: "auto", horizontal: "auto" },
            domReadOnly: readOnly,
            renderWhitespace: "none",
            automaticLayout: true,
          }}
          onChange={onChange as unknown as (v: string | undefined) => void}
          onMount={(editor: unknown) => {
            // Hide the eager <pre> now that the editor is alive.
            setMounted(true);
            // Capture the editor so the cleanup effect above can dispose it.
            editorRef.current = editor as { dispose: () => void };
          }}
        />
      </Suspense>
    </div>
  );
}

// useMonacoTheme picks Monaco's theme id from the document's `data-theme`
// attribute (set by lib/theme). Falls back to "vs-dark" when unknown — Monaco
// always needs *some* theme, and vs-dark plays nicely with the console's
// default dark mode.
function useMonacoTheme(): string {
  const [theme, setTheme] = useState<string>(() => readTheme());
  useEffect(() => {
    const obs = new MutationObserver(() => setTheme(readTheme()));
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "class"] });
    return () => obs.disconnect();
  }, []);
  return theme === "light" ? "vs" : "vs-dark";
}

function readTheme(): string {
  if (typeof document === "undefined") return "dark";
  const dt = document.documentElement.getAttribute("data-theme");
  if (dt === "light") return "light";
  if (dt === "dark" || dt === "system") return "dark";
  // Some apps toggle via class=light / class=dark on <html>.
  const cl = document.documentElement.className || "";
  if (/\bdark\b/.test(cl)) return "dark";
  if (/\blight\b/.test(cl)) return "light";
  return "dark";
}

// MonacoOnlyExports intentionally limited — keep the surface narrow so
// callers don't import the heavyweight @monaco-editor/react directly. Bump
// these names when adding new wrappers; the lazy import above keeps the
// bundle lean.
/** @public Debug introspection for the Monaco loader. */
export const __monacoRefs = { Editor };

// Expose an `editorRef` factory so tests / call sites can call setValue etc.
// without an explicit ref handle. Returns nothing in this preview build —
// left intentionally minimal until the standalone Files editor (Slice 4)
// needs imperative access.
/** @public Exposes the Monaco editor ref for parent components. */
export function useEditorRef() {
  return useRef(null);
}
