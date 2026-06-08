import { useEffect, useState } from "react";
import { Anchor, RefreshCw, Pause, Play, Trash2, Clock, Zap } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Muted, ErrorText } from "@/components/JsonView";

interface Trigger {
  type?: string;
  schedule?: string;
  subject?: string;
}
interface Order {
  id: string;
  name?: string;
  enabled?: boolean;
  triggers?: Trigger[];
  initiative?: { mode?: string };
  plan?: string;
}

// Standing is the autonomy cockpit for Chronos standing orders: persistent goals
// that fire on a trigger (a cron schedule or a matching journal event) and act
// at their initiative level. Each order shows its triggers, autonomy mode and
// plan, with pause-resume / remove controls — so the operator can see and govern
// what the daemon does unprompted.
export function Standing() {
  const [orders, setOrders] = useState<Order[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ orders?: Order[] }>("/api/standing");
      setOrders(d.orders || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 8000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(id: string, path: string, params?: Record<string, string>) {
    setBusy(id);
    try {
      await postAction(path, { id, ...params });
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  const enabledCount = orders?.filter((o) => o.enabled).length ?? 0;

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Anchor className="size-4 text-accent" /> Standing orders
        </h2>
        <span className="text-xs text-muted">
          {orders ? `${orders.length} total` : ""}
          {orders && orders.length > 0 && <span className="text-good"> · {enabledCount} active</span>}
        </span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !orders ? (
        <Muted>loading…</Muted>
      ) : orders.length === 0 ? (
        <Muted>no standing orders — add one with `agt standing add --name N (--cron … | --event …)`</Muted>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-2">
            {orders.map((o) => (
              <li key={o.id} className="rounded-lg border border-border bg-card p-3">
                <div className="flex items-center gap-2">
                  <Badge variant={o.enabled ? "good" : "default"}>{o.enabled ? "active" : "paused"}</Badge>
                  <span className="text-sm font-semibold">{o.name || o.id}</span>
                  {o.initiative?.mode && (
                    <span className="rounded bg-panel px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
                      {o.initiative.mode}
                    </span>
                  )}
                  <div className="ml-auto flex items-center gap-1.5">
                    <button
                      onClick={() => act(o.id, "/api/standing/enable", { enabled: o.enabled ? "false" : "true" })}
                      disabled={busy === o.id}
                      title={o.enabled ? "Pause" : "Resume"}
                      className="text-muted transition-colors hover:text-foreground disabled:opacity-50"
                    >
                      {o.enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
                    </button>
                    <button
                      onClick={() => act(o.id, "/api/standing/remove")}
                      disabled={busy === o.id}
                      title="Remove"
                      className="text-muted transition-colors hover:text-bad disabled:opacity-50"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                </div>
                <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                  {(o.triggers || []).map((t, i) => (
                    <span
                      key={i}
                      className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-[10px]"
                      title={`trigger: ${t.type}`}
                    >
                      {t.type === "event" ? (
                        <Zap className="size-3 text-accent" />
                      ) : (
                        <Clock className="size-3 text-accent" />
                      )}
                      {t.type === "event" ? t.subject : t.schedule}
                    </span>
                  ))}
                </div>
                {o.plan && <p className="mt-1.5 line-clamp-2 text-xs text-foreground/80">{o.plan}</p>}
                <div className="mt-1 font-mono text-[10px] text-muted opacity-70">{o.id}</div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
