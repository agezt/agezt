import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { CheckCircle2, XCircle, Info, AlertTriangle, X } from "lucide-react";
import { cn } from "@/lib/utils";

// A small, self-contained feedback layer so the app never falls back to the
// browser's alert()/confirm() — every transient message is a toast and every
// "are you sure?" is a styled modal. One provider exposes both via useUI().

type ToastKind = "success" | "error" | "info";
interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
}

export interface ConfirmOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

interface UI {
  /** Show a transient toast (auto-dismisses). */
  toast: (text: string, kind?: ToastKind) => void;
  /** Ask the user to confirm; resolves true if they confirm, false otherwise. */
  confirm: (opts: ConfirmOptions) => Promise<boolean>;
}

const UICtx = createContext<UI | null>(null);

// useUI returns the toast + confirm helpers. Components must be rendered inside
// <UIProvider> (wired at the app root).
export function useUI(): UI {
  const c = useContext(UICtx);
  if (!c) throw new Error("useUI must be used within <UIProvider>");
  return c;
}

const TOAST_MS = 4200;

export function UIProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [pending, setPending] = useState<{ opts: ConfirmOptions; resolve: (v: boolean) => void } | null>(null);
  const seq = useRef(0);

  const dismiss = useCallback((id: number) => setToasts((t) => t.filter((x) => x.id !== id)), []);

  const toast = useCallback(
    (text: string, kind: ToastKind = "info") => {
      const id = ++seq.current;
      setToasts((t) => [...t, { id, kind, text }]);
      window.setTimeout(() => dismiss(id), TOAST_MS);
    },
    [dismiss],
  );

  const confirm = useCallback(
    (opts: ConfirmOptions) => new Promise<boolean>((resolve) => setPending({ opts, resolve })),
    [],
  );

  const settle = useCallback(
    (v: boolean) => {
      pending?.resolve(v);
      setPending(null);
    },
    [pending],
  );

  return (
    <UICtx.Provider value={{ toast, confirm }}>
      {children}
      <Toaster toasts={toasts} onClose={dismiss} />
      {pending && <ConfirmModal opts={pending.opts} onResult={settle} />}
    </UICtx.Provider>
  );
}

const TOAST_STYLE: Record<ToastKind, { icon: typeof Info; ring: string; tint: string }> = {
  success: { icon: CheckCircle2, ring: "border-good/40", tint: "text-good" },
  error: { icon: XCircle, ring: "border-bad/40", tint: "text-bad" },
  info: { icon: Info, ring: "border-accent/40", tint: "text-accent" },
};

function Toaster({ toasts, onClose }: { toasts: Toast[]; onClose: (id: number) => void }) {
  if (toasts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-[min(92vw,22rem)] flex-col gap-2">
      {toasts.map((t) => {
        const s = TOAST_STYLE[t.kind];
        const Icon = s.icon;
        return (
          <div
            key={t.id}
            className={cn(
              "toast-in pointer-events-auto flex items-start gap-2.5 rounded-lg border bg-card px-3 py-2.5 text-sm shadow-lg shadow-black/20",
              s.ring,
            )}
            role="status"
          >
            <Icon className={cn("mt-0.5 size-4 shrink-0", s.tint)} />
            <span className="min-w-0 flex-1 break-words text-foreground">{t.text}</span>
            <button
              onClick={() => onClose(t.id)}
              className="shrink-0 text-muted transition-colors hover:text-foreground"
              title="Dismiss"
            >
              <X className="size-3.5" />
            </button>
          </div>
        );
      })}
    </div>
  );
}

function ConfirmModal({ opts, onResult }: { opts: ConfirmOptions; onResult: (v: boolean) => void }) {
  const confirmRef = useRef<HTMLButtonElement>(null);

  // Focus the confirm button on open; Escape cancels, Enter confirms.
  useEffect(() => {
    confirmRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onResult(false);
      else if (e.key === "Enter") onResult(true);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onResult]);

  return (
    <div
      className="modal-overlay fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-4"
      onClick={() => onResult(false)}
    >
      <div
        className="modal-in w-full max-w-sm rounded-xl border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <div className="flex items-start gap-2.5">
          {opts.danger && <AlertTriangle className="mt-0.5 size-5 shrink-0 text-bad" />}
          <div className="min-w-0">
            <h3 className="text-sm font-semibold text-foreground">{opts.title}</h3>
            {opts.message && <p className="mt-1 text-sm text-muted">{opts.message}</p>}
          </div>
        </div>
        <div className="mt-4 flex justify-end gap-2">
          <button
            onClick={() => onResult(false)}
            className="rounded-md border border-border px-3 py-1.5 text-sm text-muted transition-colors hover:text-foreground"
          >
            {opts.cancelLabel || "Cancel"}
          </button>
          <button
            ref={confirmRef}
            onClick={() => onResult(true)}
            className={cn(
              "rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors",
              opts.danger ? "bg-bad hover:bg-bad/85" : "bg-accent hover:bg-accent/85",
            )}
          >
            {opts.confirmLabel || "Confirm"}
          </button>
        </div>
      </div>
    </div>
  );
}
