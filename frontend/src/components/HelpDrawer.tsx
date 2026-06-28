import { useEffect } from "react";
import { X, Lightbulb, ArrowUpRight, type LucideIcon } from "lucide-react";
import { helpTopicFor } from "@/lib/help";

// HelpDrawer — the in-app manual (M920). A right-hand sheet that slides in over
// the current view (never squeezing it) and explains the page the operator is
// on: what it shows, every control, and where to go next. Content lives in
// lib/help.ts keyed by view id; this component owns all presentation.
//
// Width: full on phones, a comfortable 30rem column on tablets, and 40% of the
// viewport on desktop — the agreed maximum so the page underneath stays legible.

export interface HelpDrawerProps {
  open: boolean;
  /** Active view id — selects the topic from lib/help. */
  viewId: string;
  /** Sidebar section label ("Monitor", "System", …) shown as a breadcrumb chip. */
  group?: string;
  /** The view's nav icon, echoed in the drawer header. */
  icon?: LucideIcon;
  onClose: () => void;
  /** Navigate to a related view (drawer stays open and re-renders its topic). */
  onNavigate?: (viewId: string) => void;
}

export function HelpDrawer({ open, viewId, group, icon: Icon, onClose, onNavigate }: HelpDrawerProps) {
  // Escape closes from anywhere while open.
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;
  const topic = helpTopicFor(viewId);

  return (
    <div
      className="modal-overlay fixed inset-0 z-[140] bg-black/40 backdrop-blur-[2px]"
      onClick={onClose}
    >
      <aside
        role="dialog"
        aria-modal="true"
        aria-label={`Help: ${topic.title}`}
        onClick={(e) => e.stopPropagation()}
        className="help-drawer-in fixed inset-y-0 right-0 flex w-full flex-col border-l border-border bg-background shadow-2xl shadow-black/40 sm:w-[30rem] xl:w-[40vw]"
      >
        {/* Header band: accent-tinted gradient with a giant ghost icon for depth. */}
        <header className="relative shrink-0 overflow-hidden border-b border-border bg-gradient-to-br from-accent/15 via-panel to-panel">
          {Icon && (
            <Icon
              aria-hidden
              className="pointer-events-none absolute -bottom-10 -right-6 size-44 -rotate-12 text-accent opacity-[0.06]"
            />
          )}
          <div className="relative p-5 pr-12">
            <div className="flex items-center gap-2">
              {group && (
                <span className="rounded-full border border-accent/30 bg-accent/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-normal text-accent">
                  {group}
                </span>
              )}
              <span className="text-[10px] font-semibold uppercase tracking-normal text-muted/70">
                Page guide
              </span>
            </div>
            <div className="mt-2.5 flex items-center gap-3">
              {Icon && (
                <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-accent/15 text-accent">
                  <Icon className="size-5" />
                </span>
              )}
              <h2 className="text-xl font-semibold tracking-normal text-foreground">{topic.title}</h2>
            </div>
            <p className="mt-2.5 max-w-prose text-sm leading-relaxed text-muted">{topic.intro}</p>
          </div>
          <button
            onClick={onClose}
            aria-label="Close help"
            title="Close (Esc)"
            className="absolute right-3 top-3 rounded-md p-1.5 text-muted transition-colors hover:bg-panel hover:text-foreground"
          >
            <X className="size-4" />
          </button>
        </header>

        {/* Body: numbered sections, definition rows, tip callouts, related chips.
            Keyed on the view id so switching pages re-runs the entrance fade. */}
        <div key={viewId} className="view-enter min-h-0 flex-1 overflow-y-auto">
          <div className="space-y-7 p-5">
            {topic.sections.map((s, i) => (
              <section key={s.heading}>
                <div className="mb-3 flex items-baseline gap-2.5">
                  <span className="font-mono text-xs font-semibold text-accent">
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <h3 className="text-sm font-semibold uppercase tracking-normal text-foreground">
                    {s.heading}
                  </h3>
                  <span className="h-px flex-1 self-center bg-border" />
                </div>
                {s.paragraphs?.map((p) => (
                  <p key={p.slice(0, 32)} className="mb-2.5 text-sm leading-relaxed text-muted">
                    {p}
                  </p>
                ))}
                {s.items && (
                  <dl className="space-y-3">
                    {s.items.map((it) => (
                      <div key={it.term} className="border-l-2 border-accent/30 pl-3">
                        <dt className="text-sm font-medium text-foreground">{it.term}</dt>
                        <dd className="mt-0.5 text-sm leading-relaxed text-muted">{it.desc}</dd>
                      </div>
                    ))}
                  </dl>
                )}
              </section>
            ))}

            {topic.tips && topic.tips.length > 0 && (
              <section className="space-y-2">
                {topic.tips.map((tip) => (
                  <div
                    key={tip.slice(0, 32)}
                    className="flex items-start gap-2.5 rounded-lg border border-accent/25 bg-accent/10 p-3"
                  >
                    <Lightbulb className="mt-0.5 size-4 shrink-0 text-accent" />
                    <p className="text-sm leading-relaxed text-foreground/90">{tip}</p>
                  </div>
                ))}
              </section>
            )}

            {topic.related && topic.related.length > 0 && (
              <section>
                <div className="mb-2 text-[10px] font-semibold uppercase tracking-normal text-muted/70">
                  Keep exploring
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {topic.related.map((r) => (
                    <button
                      key={r.id}
                      onClick={() => onNavigate?.(r.id)}
                      className="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-accent"
                    >
                      {r.label}
                      <ArrowUpRight className="size-3" />
                    </button>
                  ))}
                </div>
              </section>
            )}
          </div>
        </div>

        {/* Footer: keyboard affordances. */}
        <footer className="flex shrink-0 items-center gap-3 border-t border-border bg-panel px-5 py-2 text-[11px] text-muted">
          <span>
            <kbd className="rounded border border-border bg-background px-1 font-mono">?</kbd> toggle help
          </span>
          <span>
            <kbd className="rounded border border-border bg-background px-1 font-mono">Esc</kbd> close
          </span>
          <span className="ml-auto text-muted/60">Guide follows the page you're on</span>
        </footer>
      </aside>
    </div>
  );
}
