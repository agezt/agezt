// ChatGPTSignInCard.tsx — the OAuth-backed ChatGPT subscription login card.
// Re-used by Models & Keys and the Setup wizard. Handles the full flow: status
// poll on mount, "sign in" button (gated behind a confirm), browser authorize
// tab, status polling until done, a "cancel" affordance while polling, and a
// "disconnect" affordance when active.
//
// Independent of any surrounding wizard — caller wires the success via
// `onChanged()` and is free to advance/reset its own UI.

import { useCallback, useEffect, useRef, useState } from "react";
import { KeyRound, RefreshCw, X } from "lucide-react";
import { postJSON } from "@/app/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useUI } from "@/components/ui/feedback";

export interface ChatGPTSignInCardProps {
  /**
   * Called whenever the connected state changes (connect, disconnect, or
   * initial mount with an existing connection). Caller usually reloads the
   * catalog / picks the ChatGPT provider as default.
   */
  onChanged?: () => void;
  /** Compact presentation — drops the bottom status line. */
  compact?: boolean;
  /** Override the polling timeout in seconds (default 180s). */
  pollTimeoutSec?: number;
}

/**
 * Connect a ChatGPT subscription (Plus/Pro) as a provider via OAuth — no API
 * key. Backed by the same login Codex CLI uses; gated behind a one-time
 * acknowledgement. Returns nothing — `onChanged` is fired when status changes.
 */
export function ChatGPTSignInCard({ onChanged, compact = false, pollTimeoutSec = 180 }: ChatGPTSignInCardProps) {
  const { toast, confirm } = useUI();
  const [connected, setConnected] = useState(false);
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("");

  // `connectedRef` mirrors `connected` so callbacks fired inside async loops
  // see the latest value. Without this, `refresh()` would always read the
  // `connected` from the render that scheduled it — a stale-closure trap that
  // could fire `onChanged` on every status poll instead of only on edges.
  const connectedRef = useRef(false);
  useEffect(() => {
    connectedRef.current = connected;
  }, [connected]);

  // In-flight OAuth flow. `abortRef.current` becomes true when the user clicks
  // Cancel or the component unmounts; the polling loop in signIn() exits on
  // the next tick instead of running out the full timeout.
  const abortRef = useRef(false);

  // Latest onChanged ref so the polling loop doesn't capture a stale one.
  const onChangedRef = useRef(onChanged);
  useEffect(() => {
    onChangedRef.current = onChanged;
  }, [onChanged]);

  const refresh = useCallback(async () => {
    try {
      const r = await postJSON<{ connected?: boolean; email?: string }>("/api/provider/oauth/status", { state: "" });
      const wasConnected = connectedRef.current;
      const nowConnected = !!r.connected;
      setConnected(nowConnected);
      setEmail(r.email || "");
      if (nowConnected && !wasConnected) onChangedRef.current?.();
    } catch {
      /* unauthenticated console or daemon down — leave as disconnected */
    }
  }, []);

  // Initial mount + whenever the daemon URL changes (rare).
  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Cancel any in-flight polling on unmount so navigating away mid-OAuth
  // doesn't keep the loop running in the background.
  useEffect(() => {
    return () => {
      abortRef.current = true;
    };
  }, []);

  function cancel() {
    abortRef.current = true;
    setStatus("");
    setBusy(false);
  }

  async function signIn() {
    const ok = await confirm({
      title: "Sign in with ChatGPT?",
      message:
        "This connects your ChatGPT subscription via the same login Codex CLI uses. It relies on an unofficial OpenAI backend — it may stop working or violate OpenAI's terms, and only ever uses your own account. Continue?",
      confirmLabel: "Continue",
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    setStatus("Opening ChatGPT…");
    abortRef.current = false;
    try {
      const r = await postJSON<{ authorize_url?: string; state?: string; error?: string }>(
        "/api/provider/oauth/start",
        { provider: "chatgpt" },
      );
      if (!r.authorize_url || !r.state) throw new Error(r.error || "could not start sign-in");
      window.open(r.authorize_url, "_blank", "noopener,noreferrer");
      setStatus("Waiting for you to authorize in the new tab…");
      const iters = Math.max(1, Math.floor(pollTimeoutSec / 2));
      for (let i = 0; i < iters; i += 1) {
        if (abortRef.current) return;
        await new Promise((res) => setTimeout(res, 2000));
        if (abortRef.current) return;
        const st = await postJSON<{ status?: string; error?: string }>("/api/provider/oauth/status", { state: r.state });
        if (st.status === "done") {
          toast("Connected ChatGPT", "success");
          setStatus("");
          await refresh();
          onChangedRef.current?.();
          return;
        }
        if (st.status === "error") throw new Error(st.error || "authorization failed");
      }
      throw new Error("timed out waiting for authorization");
    } catch (e) {
      if (!abortRef.current) {
        setStatus("");
        toast((e as Error).message, "error");
      }
    } finally {
      if (!abortRef.current) setBusy(false);
    }
  }

  async function importCodex() {
    setBusy(true);
    try {
      const r = await postJSON<{ connected?: boolean; email?: string; error?: string }>(
        "/api/provider/oauth/import",
        {},
      );
      if (!r.connected) throw new Error(r.error || "no Codex CLI login found");
      toast("Imported ChatGPT login from Codex CLI", "success");
      await refresh();
      onChangedRef.current?.();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function disconnect() {
    setBusy(true);
    try {
      await postJSON("/api/provider/oauth/logout", {});
      toast("Disconnected ChatGPT", "info");
      setConnected(false);
      setEmail("");
      onChangedRef.current?.();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="glass rounded-xl p-3">
      <div className="flex flex-wrap items-center gap-2">
        <KeyRound className="size-4 text-accent" />
        <span className="text-sm font-medium text-foreground">Sign in with ChatGPT</span>
        {connected ? (
          <Badge variant="good">connected{email ? ` · ${email}` : ""}</Badge>
        ) : (
          <Badge variant="default">not connected</Badge>
        )}
        <div className="ml-auto flex items-center gap-2">
          {connected ? (
            <Button variant="ghost" size="sm" onClick={disconnect} disabled={busy} aria-label="Disconnect ChatGPT">
              Disconnect
            </Button>
          ) : (
            <>
              <Button
                variant="ghost"
                size="sm"
                onClick={importCodex}
                disabled={busy}
                title="Use a local `codex login` session"
              >
                Import from Codex CLI
              </Button>
              {busy ? (
                <Button variant="ghost" size="sm" onClick={cancel} aria-label="Cancel ChatGPT sign-in">
                  <X className="size-3.5" /> Cancel
                </Button>
              ) : null}
              <Button size="sm" onClick={signIn} disabled={busy} aria-label="Sign in with ChatGPT">
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <KeyRound className="size-3.5" />} Sign in with ChatGPT
              </Button>
            </>
          )}
        </div>
      </div>
      {!compact && status && <p className="mt-1.5 text-[11px] text-muted" aria-live="polite">{status}</p>}
    </div>
  );
}
