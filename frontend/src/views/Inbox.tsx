import { useEffect, useState } from "react";
import { Inbox as InboxIcon, RefreshCw, ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

interface Message {
  direction?: string; // "in" | "out"
  sender?: string;
  text?: string;
  ts_unix_ms?: number;
}
interface Thread {
  correlation_id: string;
  channel_kind?: string;
  channel_id?: string;
  messages?: Message[];
  last_ts_unix_ms?: number;
}

// Inbox is the unified conversation view (SPEC-07): every channel thread —
// Telegram, Slack, Discord, email, … — folded from the journal's
// channel.inbound/outbound events into one place, newest activity first, with
// each message marked inbound (from the operator) or outbound (from the agent).
// Previously this view read the wrong payload key and never rendered.
export function Inbox() {
  const { events } = useEvents();
  const [threads, setThreads] = useState<Thread[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<{ threads?: Thread[] }>("/api/inbox", { limit: "50" });
      setThreads(d.threads || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Nudge on channel traffic.
  const head = events[0]?.kind || "";
  useEffect(() => {
    if (head.startsWith("channel.")) reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <InboxIcon className="size-4 text-accent" /> Inbox
        </h2>
        <span className="text-xs text-muted">{threads ? `${threads.length} conversation(s)` : ""}</span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !threads ? (
        <SkeletonList count={4} lines={1} />
      ) : threads.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted">
          <InboxIcon className="size-8 opacity-40" />
          <span className="text-sm">no conversations — connect a channel (Telegram, Slack, …) to chat with the agent</span>
        </div>
      ) : (
        <div className="min-h-0 flex-1 space-y-3 overflow-auto">
          {threads.map((th) => (
            <div key={th.correlation_id} className="rounded-lg border border-border bg-card p-3">
              <div className="mb-2 flex items-center gap-2">
                <Badge>{th.channel_kind || "?"}</Badge>
                {th.channel_id && <span className="truncate text-xs text-muted">{th.channel_id}</span>}
                <span className="ml-auto text-[10px] tabular-nums text-muted">
                  {th.last_ts_unix_ms ? fmtTime(th.last_ts_unix_ms) : ""}
                </span>
              </div>
              <ul className="space-y-1.5">
                {(th.messages || []).map((m, i) => {
                  const out = m.direction === "out";
                  return (
                    <li key={i} className={cn("flex gap-2", out && "flex-row-reverse")}>
                      <span className={cn("mt-0.5 shrink-0", out ? "text-accent" : "text-good")}>
                        {out ? <ArrowUpRight className="size-3.5" /> : <ArrowDownLeft className="size-3.5" />}
                      </span>
                      <div
                        className={cn(
                          "max-w-[80%] rounded-lg px-2.5 py-1.5 text-xs",
                          out ? "bg-accent/10 text-foreground" : "bg-panel text-foreground/90",
                        )}
                      >
                        {m.sender && !out && <div className="text-[10px] font-semibold text-muted">{m.sender}</div>}
                        <div className="whitespace-pre-wrap break-words">{m.text}</div>
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
