import { useCallback, useEffect, useState } from "react";
import { Lock } from "lucide-react";
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

  if (state === "unknown") return null; // brief: avoids a flash of the app behind the lock
  if (state === "need") return <Login onAuthed={() => setState("ok")} />;
  return <>{children}</>;
}

export function Login({ onAuthed }: { onAuthed: () => void }) {
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

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
    <div className="fixed inset-0 z-[300] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <form
        onSubmit={submit}
        className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-2xl"
        aria-label="Console login"
      >
        <div className="mb-4 flex items-center gap-2">
          <Lock className="h-5 w-5 text-accent" />
          <h2 className="text-lg font-semibold">Console locked</h2>
        </div>
        <p className="mb-3 text-sm text-muted">
          This console is password-protected. Enter the password to continue.
        </p>
        <input
          type="password"
          value={pw}
          autoFocus
          onChange={(e) => setPw(e.target.value)}
          placeholder="password"
          aria-label="Console password"
          className="w-full rounded-md border border-border bg-panel px-3 py-2 text-sm outline-none focus-visible:border-accent"
        />
        {err && (
          <p className="mt-2 text-xs text-bad" role="alert">
            {err}
          </p>
        )}
        <Button type="submit" className="mt-4 w-full" disabled={busy || !pw} aria-label="Unlock console">
          {busy ? "Checking…" : "Unlock"}
        </Button>
      </form>
    </div>
  );
}
