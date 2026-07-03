import { useEffect, useRef, type ReactNode } from "react";
import { cn } from "@/lib/utils";

// Modal — a small, opinionated, accessible modal primitive that consolidates the
// three slightly-different inline modals the app accumulated before this layer
// landed (nav drawer in App.tsx, file-detail viewer in FileMention.tsx,
// artifact viewer in Artifacts.tsx, preview modal in Files.tsx).
//
// Behaviour every modal needs, provided here so callers don't reinvent them:
//   • Escape closes (when dismissable).
//   • Clicking the backdrop closes; clicking inside the panel does not.
//   • Tab / Shift+Tab cycles focus inside the modal — focus never escapes into
//     the underlying page.
//   • On open, focus moves to either `initialFocusRef.current` or the first
//     focusable element inside the panel; on close, focus returns to whatever
//     owned the trigger.
//   • While open, body scroll is locked so background scrolling doesn't fight
//     touch / wheel events on the modal.
//   • role="dialog" + aria-modal="true" + aria-label are applied to the panel.
//
// `zIndex` lets callers stack multiple modals — confirm dialogs that open from
// inside an existing modal, for example — without each component hard-coding a
// number that could collide with the design's token grid. The default 200
// matches the existing z-[200] inline modals.

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]):not([disabled])';

function focusables(root: HTMLElement | null): HTMLElement[] {
  if (!root) return [];
  // Note: we deliberately don't filter on offsetParent / getBoundingClientRect
  // because jsdom (our test env) does not compute layout, and viewport-less
  // rendering would otherwise drop every focusable. The focusables inside an
  // opened modal are always rendered in the live UI; any with display:none or
  // visibility:hidden still respect the `disabled` attribute and the
  // tabindex=-1 escape hatch above. If an app-level style hides children, that
  // component is responsible for setting `disabled` or `tabIndex={-1}`.
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
}

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  /** Accessible label for screen readers when there's no visible header. */
  ariaLabel: string;
  /**
   * When true (the default), Escape and backdrop clicks call onClose.
   * Set false for blocking flows where the user must explicitly choose.
   */
  dismissable?: boolean;
  /** Z-index of the modal layer (panel + backdrop). Default 200. */
  zIndex?: number;
  /** Optional ref to the element that should receive focus on open. */
  initialFocusRef?: React.RefObject<HTMLElement | null>;
  /**
   * Optional ref attached to the panel. Useful so a parent component can
   * measure, focus, or query the panel.
   */
  panelRef?: React.RefObject<HTMLElement | null>;
  /** The panel contents. */
  children: ReactNode;
  /** Additional classes merged onto the panel element. */
  panelClassName?: string;
  /**
   * Additional classes merged onto the overlay (the dimmed backdrop behind
   * the panel). Defaults to `bg-black/70` so callers get the same backdrop
   * colour the inline modals used; pass an empty string to opt out (e.g. if
   * you want a transparent overlay because the panel itself fills the screen,
   * like the nav drawer).
   */
  overlayClassName?: string;
}

/**
 * Modal is a controlled component: the caller owns `open`. While open it
 * renders a fixed-position backdrop + a centered content panel and runs the
 * focus/escape/scroll behaviours above. Once `open` flips to false, all
 * listeners are torn down and focus is restored to the previous owner.
 */
export function Modal({
  open,
  onClose,
  ariaLabel,
  dismissable = true,
  zIndex = 200,
  initialFocusRef,
  panelRef,
  children,
  panelClassName,
  overlayClassName = "bg-black/70",
}: ModalProps) {
  const innerPanelRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    // Snapshot whatever was focused when the modal opened; restore on close.
    // On render-heavy pages the focusable list is computed lazily so an
    // async-mounted panel still resolves on the next tick.
    const prevFocused = document.activeElement as HTMLElement | null;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    let raf = 0;
    const focusInitial = () => {
      const panel = innerPanelRef.current;
      if (!panel) return;
      if (initialFocusRef?.current) {
        initialFocusRef.current.focus();
        return;
      }
      const items = focusables(panel);
      if (items.length > 0) items[0].focus();
      else panel.focus();
    };
    raf = window.requestAnimationFrame(focusInitial);

    const onKey = (e: KeyboardEvent) => {
      if (!dismissable && e.key === "Escape") return;
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key === "Tab") {
        const panel = innerPanelRef.current;
        if (!panel) return;
        const items = focusables(panel);
        if (items.length === 0) {
          // Sole empty modal: stay put, but consume the key so the page
          // underneath doesn't move.
          e.preventDefault();
          return;
        }
        const first = items[0];
        const last = items[items.length - 1];
        const active = document.activeElement as HTMLElement | null;
        const insidePanel = active ? panel.contains(active) : false;
        if (!insidePanel) {
          // Focus had already escaped (programmatic focus, drag, etc.) —
          // pull it back to whichever end the user was heading.
          e.preventDefault();
          (e.shiftKey ? last : first).focus();
          return;
        }
        if (!e.shiftKey && active === last) {
          e.preventDefault();
          first.focus();
        } else if (e.shiftKey && active === first) {
          e.preventDefault();
          last.focus();
        }
        // Otherwise let the browser advance focus naturally.
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      window.cancelAnimationFrame(raf);
      window.removeEventListener("keydown", onKey, true);
      document.body.style.overflow = prevOverflow;
      // Restore focus to the element that opened the modal. Skip when the
      // opener was already blurred / no-longer-focusable (e.g. unmounted).
      if (prevFocused && document.body.contains(prevFocused)) {
        prevFocused.focus?.();
      }
    };
  }, [open, dismissable, onClose, initialFocusRef]);

  if (!open) return null;

  // Split refs into the caller's optional external panelRef and our internal
  // local one. Setting both keeps both observers happy without forcing the
  // caller to forward refs manually.
  const setRefs = (node: HTMLDivElement | null) => {
    innerPanelRef.current = node;
    if (panelRef) (panelRef as React.MutableRefObject<HTMLElement | null>).current = node;
  };

  return (
    <div
      className={cn("fixed inset-0 flex items-center justify-center p-4 backdrop-blur-sm", overlayClassName)}
      style={{ zIndex }}
      role="presentation"
      onClick={(e) => {
        if (!dismissable) return;
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={setRefs}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        tabIndex={-1}
        // Stop click propagation so clicks inside the panel don't dismiss the
        // modal. Callers can attach their own onClick — it composes.
        onClick={(e) => e.stopPropagation()}
        className={cn("glass relative flex flex-col overflow-hidden rounded-xl shadow-2xl", panelClassName)}
      >
        {children}
      </div>
    </div>
  );
}
