import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import { CheckCircle2, XCircle, Info, AlertTriangle, X } from "lucide-react";
import { cn } from "@/lib/utils";

// A small, self-contained feedback layer so the app never falls back to the
// browser's alert()/confirm()/prompt() — every transient message is a toast,
// every "are you sure?" is a styled modal, and every one-line text ask is a
// styled input modal. One provider exposes all three via useUI().

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

export interface PromptOptions {
  title: string;
  message?: string;
  placeholder?: string;
  initial?: string;
  confirmLabel?: string;
  cancelLabel?: string;
}

interface UI {
  /** Show a transient toast (auto-dismisses). */
  toast: (text: string, kind?: ToastKind) => void;
  /** Ask the user to confirm; resolves true if they confirm, false otherwise. */
  confirm: (opts: ConfirmOptions) => Promise<boolean>;
  /** Ask the user for one line of text; resolves the value, or null if cancelled. */
  prompt: (opts: PromptOptions) => Promise<string | null>;
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
  const [pendingPrompt, setPendingPrompt] = useState<{ opts: PromptOptions; resolve: (v: string | null) => void } | null>(null);
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

  const prompt = useCallback(
    (opts: PromptOptions) => new Promise<string | null>((resolve) => setPendingPrompt({ opts, resolve })),
    [],
  );

  const settlePrompt = useCallback(
    (v: string | null) => {
      pendingPrompt?.resolve(v);
      setPendingPrompt(null);
    },
    [pendingPrompt],
  );

  return (
    <UICtx.Provider value={{ toast, confirm, prompt }}>
      {children}
      <Toaster toasts={toasts} onClose={dismiss} />
      {pending && <ConfirmModal opts={pending.opts} onResult={settle} />}
      {pendingPrompt && <PromptModal opts={pendingPrompt.opts} onResult={settlePrompt} />}
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

function PromptModal({ opts, onResult }: { opts: PromptOptions; onResult: (v: string | null) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(opts.initial ?? "");

  // Focus (and select) the input on open; Escape cancels. Enter submits via
  // the input's own key handler so it never fights the global listener.
  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onResult(null);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onResult]);

  return (
    <div
      className="modal-overlay fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-4"
      onClick={() => onResult(null)}
    >
      <div
        className="modal-in w-full max-w-sm rounded-xl bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <h3 className="text-sm font-semibold text-foreground">{opts.title}</h3>
        {opts.message && <p className="mt-1 text-sm text-muted">{opts.message}</p>}
        <input
          ref={inputRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") onResult(value);
          }}
          placeholder={opts.placeholder}
          className="mt-3 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm text-foreground outline-none transition-colors focus:border-accent"
        />
        <div className="mt-4 flex justify-end gap-2">
          <button
            onClick={() => onResult(null)}
            className="rounded-md border border-border px-3 py-1.5 text-sm text-muted transition-colors hover:text-foreground"
          >
            {opts.cancelLabel || "Cancel"}
          </button>
          <button
            onClick={() => onResult(value)}
            className="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-accent/85"
          >
            {opts.confirmLabel || "OK"}
          </button>
        </div>
      </div>
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
        className="modal-in w-full max-w-sm rounded-xl bg-card p-4 shadow-xl shadow-black/30"
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
