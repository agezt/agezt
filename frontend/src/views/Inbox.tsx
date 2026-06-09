import { useEffect, useState } from "react";
import { Inbox as InboxIcon, RefreshCw, ArrowDownLeft, ArrowUpRight, Send, Plus, X, Search } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

// COMMON_CHANNELS pre-fills the kind picker with the channels the daemon can carry;
// the field stays free-text so an unlisted kind still works.
const COMMON_CHANNELS = ["telegram", "slack", "discord", "webhook", "email", "sms", "whatsapp", "matrix", "teams"];

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

// threadMatches tests a thread against a lowercased query over its channel kind, channel
// id, and the sender + text of its messages — so you can find a conversation by who it's
// with, which channel it's on, or something that was said in it (M776).
export function threadMatches(th: Thread, q: string): boolean {
  if (!q) return true;
  const parts = [th.channel_kind, th.channel_id];
  for (const m of th.messages || []) {
    parts.push(m.sender, m.text);
  }
  return parts
    .filter((s): s is string => typeof s === "string")
    .join(" ")
    .toLowerCase()
    .includes(q);
}

// Inbox is the unified conversation view (SPEC-07): every channel thread —
// Telegram, Slack, Discord, email, … — folded from the journal's
// channel.inbound/outbound events into one place, newest activity first, with
// each message marked inbound (from the operator) or outbound (from the agent).
// Previously this view read the wrong payload key and never rendered.
export function Inbox() {
  const { events } = useEvents();
  const ui = useUI();
  const [threads, setThreads] = useState<Thread[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [showSend, setShowSend] = useState(false);
  const [q, setQ] = useState("");
  // Prefill the composer with a thread's channel+id when you click "reply".
  const [prefill, setPrefill] = useState<{ channel: string; to: string } | null>(null);

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
        <Button
          size="sm"
          className="ml-auto"
          onClick={() => {
            setPrefill(null);
            setShowSend((v) => !v);
          }}
          title="Send a message via a channel"
        >
          {showSend ? <X className="size-3.5" /> : <Send className="size-3.5" />} Send message
        </Button>
        <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {showSend && (
        <SendMessageForm
          key={prefill ? `${prefill.channel}:${prefill.to}` : "blank"}
          initial={prefill}
          onSent={(channel, to) => {
            setShowSend(false);
            setPrefill(null);
            ui.toast(`Message sent via ${channel} → ${to}`, "success");
            void reload();
          }}
          onError={(m) => ui.toast(m, "error")}
        />
      )}

      {/* Find a conversation by channel, contact, or something that was said. */}
      {threads && threads.length > 4 && (
        <div className="relative">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="filter conversations…"
            aria-label="Filter conversations"
            className="h-8 w-full rounded-md border border-border bg-panel pl-7 pr-12 text-xs text-foreground outline-none focus-visible:border-accent"
          />
          {q.trim() && (
            <span className="absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-muted">
              {threads.filter((th) => threadMatches(th, q.trim().toLowerCase())).length}/{threads.length}
            </span>
          )}
        </div>
      )}

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
          {(() => {
            const query = q.trim().toLowerCase();
            const shown = query ? threads.filter((th) => threadMatches(th, query)) : threads;
            if (shown.length === 0) return <p className="px-1 py-2 text-xs text-muted">no conversations match “{q.trim()}”</p>;
            return shown.map((th) => (
            <div key={th.correlation_id} className="rounded-lg border border-border bg-card p-3">
              <div className="mb-2 flex items-center gap-2">
                <Badge>{th.channel_kind || "?"}</Badge>
                {th.channel_id && <span className="truncate text-xs text-muted">{th.channel_id}</span>}
                {th.channel_kind && th.channel_id && (
                  <button
                    onClick={() => {
                      setPrefill({ channel: th.channel_kind!, to: th.channel_id! });
                      setShowSend(true);
                    }}
                    className="text-[11px] text-accent/80 transition-colors hover:text-accent"
                    title="Reply on this channel"
                  >
                    reply
                  </button>
                )}
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
            ));
          })()}
        </div>
      )}
    </div>
  );
}

// SendMessageForm sends an outbound message through a configured channel (M747) —
// reply to a Telegram/Slack/… conversation, or proactively ping a recipient, from
// the console. channel + to + text post to the send command; the daemon refuses if
// no channel of that kind is configured. `initial` prefills from a thread's "reply".
export function SendMessageForm({
  initial,
  onSent,
  onError,
}: {
  initial?: { channel: string; to: string } | null;
  onSent: (channel: string, to: string) => void;
  onError: (msg: string) => void;
}) {
  const [channel, setChannel] = useState(initial?.channel ?? "telegram");
  const [to, setTo] = useState(initial?.to ?? "");
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);

  const valid = channel.trim() !== "" && to.trim() !== "" && text.trim() !== "";

  async function send() {
    if (!valid) return;
    setSending(true);
    try {
      await postAction("/api/send", { channel: channel.trim().toLowerCase(), to: to.trim(), text: text.trim() });
      onSent(channel.trim().toLowerCase(), to.trim());
    } catch (e) {
      onError((e as Error).message);
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="rounded-lg border border-accent/30 bg-card p-3">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex flex-col gap-1 text-[11px] text-muted">
          Channel
          <input
            value={channel}
            onChange={(e) => setChannel(e.target.value)}
            list="send-channel-kinds"
            aria-label="Send channel"
            className="h-8 w-32 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
          />
          <datalist id="send-channel-kinds">
            {COMMON_CHANNELS.map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
        </label>
        <label className="flex min-w-0 flex-1 flex-col gap-1 text-[11px] text-muted">
          To (recipient / chat id)
          <input
            value={to}
            onChange={(e) => setTo(e.target.value)}
            placeholder="e.g. a chat id or @handle"
            aria-label="Send recipient"
            className="h-8 min-w-0 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Message to send…"
        aria-label="Send message text"
        className="mt-2 h-20 w-full resize-y rounded-md border border-border bg-panel p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
      />
      <div className="mt-2 flex items-center justify-between">
        <span className="text-[10px] text-muted">Sends via a configured channel; the daemon refuses if that channel isn’t set up.</span>
        <Button size="sm" onClick={send} disabled={!valid || sending}>
          {sending ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Send
        </Button>
      </div>
    </div>
  );
}
