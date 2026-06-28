import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Eye, EyeOff, KeyRound, Lock, RefreshCw, ShieldCheck } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { Button } from "@/components/ui/button";

// Password second factor (M817). The daemon's URL token gets the SPA shell to
// load; when AGEZT_WEB_PASSWORD is set, every DATA route also needs a session
// cookie minted by POST /api/login. AuthGate probes /api/authmeta and, when a
// password is required and this browser hasn't logged in, renders the lock
// screen INSTEAD of the app — so no data call fires (and 401s) before login.
// With no password configured the gate is a transparent pass-through.

interface AuthMeta {
  password_required: boolean;
  authed: boolean;
}

export function AuthGate({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<"unknown" | "ok" | "need">("unknown");

  const probe = useCallback(() => {
    getJSON<AuthMeta>("/api/authmeta")
      .then((m) => setState(m.password_required && !m.authed ? "need" : "ok"))
      // A failing probe (no/!valid token, older daemon without the route) must
      // not lock the user out of an otherwise-usable console — fall through to
      // the app and let its own requests surface any real auth error.
      .catch(() => setState("ok"));
  }, []);

  useEffect(() => {
    probe();
  }, [probe]);

  if (state === "unknown") return <AuthProbeScreen />;
  if (state === "need") return <Login onAuthed={() => setState("ok")} />;
  return <>{children}</>;
}

export function Login({ onAuthed }: { onAuthed: () => void }) {
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [showPw, setShowPw] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!pw || busy) return;
    setBusy(true);
    setErr("");
    try {
      await postJSON("/api/login", { password: pw });
      setPw("");
      // Verify the session is actually accepted by the DATA routes before
      // revealing the app. In strict mode (token AND session) — or against an
      // older daemon — a token-less password login mints a cookie but every
      // /api/* still 401s; surface that here instead of dropping the user into
      // an app that fails on every request. A failed probe is treated as
      // success (optimistic) so a transient blip never locks a usable console.
      try {
        const m = await getJSON<AuthMeta>("/api/authmeta");
        if (m.password_required && !m.authed) {
          setErr(
            "Signed in, but this browser still can't reach the data routes — the daemon also requires the URL token (strict mode). Open the tokened link from the daemon banner.",
          );
          return;
        }
      } catch {
        /* probe failed — be optimistic; the app's own requests surface real errors */
      }
      onAuthed();
    } catch (e) {
      setErr((e as Error).message || "login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="fixed inset-0 z-[300] flex items-center justify-center overflow-y-auto bg-background/95 p-4 backdrop-blur-sm">
      <div className="mx-auto flex w-full max-w-sm flex-col items-center gap-6 py-8">
        {/* Brand — the aurora + gradient wordmark matches the command-center aesthetic. */}
        <div className="flex flex-col items-center gap-2 text-center">
          <div className="inline-flex size-12 items-center justify-center rounded-xl bg-gradient-to-br from-accent/25 to-accent2/20 text-accent ring-1 ring-inset ring-accent/30">
            <ShieldCheck className="size-6" />
          </div>
          <h1 className="text-gradient text-2xl font-bold leading-tight tracking-normal">AGEZT</h1>
          <p className="max-w-xs text-sm text-muted">Multi-agent orchestration console</p>
        </div>

        <form
          onSubmit={submit}
          className="w-full rounded-xl border border-border bg-card p-6 shadow-e2"
          aria-label="Console login"
        >
          <h2 className="mb-1 text-base font-semibold">Console locked</h2>
          <p className="mb-4 text-sm text-muted">
            Enter the password to unlock this session.
          </p>
          <div className="flex items-center rounded-lg border border-border bg-panel transition-colors focus-within:border-accent">
            <Lock className="ml-3 size-4 shrink-0 text-muted" />
            <input
              type={showPw ? "text" : "password"}
              value={pw}
              autoFocus
              autoComplete="current-password"
              onChange={(e) => {
                setPw(e.target.value);
                if (err) setErr("");
              }}
              placeholder="Password"
              aria-label="Console password"
              className="min-w-0 flex-1 bg-transparent px-2 py-2.5 text-sm outline-none placeholder:text-muted"
            />
            <button
              type="button"
              onClick={() => setShowPw((v) => !v)}
              className="mr-1.5 inline-flex size-7 items-center justify-center rounded-md text-muted transition-colors hover:bg-card hover:text-foreground"
              title={showPw ? "Hide password" : "Show password"}
              aria-label={showPw ? "Hide password" : "Show password"}
            >
              {showPw ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            </button>
          </div>
          {err && (
            <div className="mt-3 flex items-start gap-2 rounded-lg border border-bad/30 bg-bad/10 px-3 py-2 text-sm text-bad" role="alert">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{err}</span>
            </div>
          )}
          <Button type="submit" variant="accent" className="mt-5 w-full" disabled={busy || !pw} aria-label="Unlock console">
            {busy ? "Checking…" : "Unlock"}
          </Button>
        </form>

        <p className="text-xs text-muted/60">
          Password access is enabled for this browser session.
        </p>
      </div>
    </div>
  );
}

function AuthProbeScreen() {
  return (
    <div className="fixed inset-0 z-[300] flex items-center justify-center bg-background/95 p-4">
      <div className="inline-flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-sm text-muted shadow-e1">
        <RefreshCw className="size-4 animate-spin text-accent" />
        Checking console session
      </div>
    </div>
  );
}
