import { useEffect, useState, type ReactNode } from "react";
import { Inbox as InboxIcon, RefreshCw, ArrowDownLeft, ArrowUpRight, Send, Plus, X, Search, ListTree, AtSign } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { useInboxPager } from "@/lib/cursorPager";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { useUI } from "@/components/ui/feedback";
import { Page } from "@/components/ui/page";
import { BlobArtifact, type ArtifactEntry } from "@/views/Files";
import { focusRun } from "@/lib/runfocus";

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
  // Cursor-paginated thread list (SPEC-07 → /api/inbox limit+cursor): the first
  // page loads on mount and "Load 50 more" walks older conversations, so thread
  // #51+ is reachable instead of the list silently stopping at 50.
  const {
    paged,
    error: err,
    loading,
    loadMore,
    loadingMore,
    moreError,
    hasMore,
    reload: reloadThreads,
  } = useInboxPager(undefined, 50);
  const threads = paged as unknown as Thread[];
  // Inbound images persisted as artifacts (M822/M828), grouped by the run
  // correlation that brought them — the SAME key the inbox threads are grouped by
  // (the channel handler runs under that corr), so a thread's images render inline.
  const [imagesByCorr, setImagesByCorr] = useState<Record<string, ArtifactEntry[]>>({});
  const [showSend, setShowSend] = useState(false);
  const [q, setQ] = useState("");
  // Prefill the composer with a thread's channel+id when you click "reply".
  const [prefill, setPrefill] = useState<{ channel: string; to: string } | null>(null);

  // Pull the inbound images and bucket them by correlation for inline display.
  async function reloadImages() {
    try {
      const imgs = await getJSON<{ entries?: ArtifactEntry[] }>("/api/artifacts", { kind: "image" });
      const byCorr: Record<string, ArtifactEntry[]> = {};
      for (const e of imgs.entries || []) {
        if (!e.corr) continue;
        (byCorr[e.corr] ||= []).push(e);
      }
      setImagesByCorr(byCorr);
    } catch {
      /* images are a nicety — a failure here never breaks the inbox */
    }
  }

  function reload() {
    reloadThreads();
    void reloadImages();
  }
  useEffect(() => {
    void reloadImages();
  }, []);

  // Nudge on channel traffic.
  const head = events[0]?.kind || "";
  useEffect(() => {
    if (head.startsWith("channel.")) reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  return (
    <Page
      icon={InboxIcon}
      title="Inbox"
      description={threads.length > 0 ? `${threads.length}${hasMore ? "+" : ""} conversation(s)` : "Unified conversation view across every channel."}
      mode="fill"
      width="wide"
      actions={
        <>
          <Button
            size="sm"
            onClick={() => {
              setPrefill(null);
              setShowSend(true);
            }}
            title="Send a message via a channel"
          >
            <Send className="size-3.5" /> Send message
          </Button>
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        </>
      }
    >
      {showSend && (
        <InboxModal title={prefill ? `Reply via ${prefill.channel}` : "Send message"} onClose={() => setShowSend(false)}>
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
        </InboxModal>
      )}

      {/* Find a conversation by channel, contact, or something that was said. */}
      {threads.length > 4 && (
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
            <span className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted">
              {threads.filter((th) => threadMatches(th, q.trim().toLowerCase())).length}/{threads.length}
            </span>
          )}
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : loading && threads.length === 0 ? (
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
            <div key={th.correlation_id} className="glass rounded-xl p-3">
              <div className="mb-2 flex items-center gap-2">
                <Badge>{th.channel_kind || "?"}</Badge>
                {th.channel_id && <span className="truncate text-xs text-muted">{th.channel_id}</span>}
                {th.channel_kind && th.channel_id && (
                  <button
                    onClick={() => {
                      setPrefill({ channel: th.channel_kind!, to: th.channel_id! });
                      setShowSend(true);
                    }}
                    className="text-xs text-accent/80 transition-colors hover:text-accent"
                    title="Reply on this channel"
                  >
                    reply
                  </button>
                )}
                {th.correlation_id && (
                  <button
                    onClick={() => {
                      focusRun(th.correlation_id);
                      location.hash = "runs";
                    }}
                    className="inline-flex items-center gap-1 text-xs text-muted transition-colors hover:text-accent"
                    title="Open the governed run created for this channel message"
                  >
                    <ListTree className="size-3" /> run
                  </button>
                )}
                <span className="ml-auto text-xs tabular-nums text-muted">
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
                        {m.sender && !out && <div className="text-xs font-semibold text-muted">{m.sender}</div>}
                        <div className="whitespace-pre-wrap break-words">{m.text}</div>
                      </div>
                    </li>
                  );
                })}
              </ul>
              {(imagesByCorr[th.correlation_id]?.length ?? 0) > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5 border-t border-border pt-2">
                  {imagesByCorr[th.correlation_id].map((e) => (
                    <div key={e.id} title={e.caption || e.name || "image"} className="block size-16 overflow-hidden rounded-md border border-border bg-panel">
                      <BlobArtifact entry={e} kind="image" alt={e.caption || "image"} className="size-full object-cover" />
                    </div>
                  ))}
                </div>
              )}
            </div>
            ));
          })()}
          <LoadMoreFooter
            hasMore={hasMore}
            loadingMore={loadingMore}
            moreError={moreError}
            onLoadMore={loadMore}
            pageSize={50}
            label="conversations"
          />
        </div>
      )}
    </Page>
  );
}

function InboxModal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Send className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Use a configured channel without leaving the conversation list.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close inbox modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 overflow-auto p-4">{children}</div>
      </div>
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
    <div className="space-y-3 rounded-lg border border-border/70 bg-panel/70 p-3">
      <div>
        <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">
          <Send className="size-3" /> Channel
        </div>
        <div className="flex flex-wrap gap-1.5">
          {COMMON_CHANNELS.slice(0, 7).map((kind) => (
            <button
              key={kind}
              type="button"
              onClick={() => setChannel(kind)}
              className={cn(
                "inline-flex h-7 items-center gap-1 rounded-md border px-2 text-xs transition-colors",
                channel.trim().toLowerCase() === kind
                  ? "border-accent bg-accent/15 text-accent"
                  : "border-border bg-card text-muted hover:border-accent/50 hover:text-foreground",
              )}
              aria-pressed={channel.trim().toLowerCase() === kind}
            >
              <AtSign className="size-3" /> {kind}
            </button>
          ))}
          <input
            value={channel}
            onChange={(e) => setChannel(e.target.value)}
            list="send-channel-kinds"
            aria-label="Send channel"
            className="h-7 w-28 rounded-md border border-border bg-card px-2 text-xs outline-none focus-visible:border-accent"
            title="Custom channel kind"
          />
          <datalist id="send-channel-kinds">
            {COMMON_CHANNELS.map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
        </div>
      </div>
      <div className="grid gap-2">
        <label className="grid gap-1 text-xs text-muted">
          Recipient / chat id
          <input
            value={to}
            onChange={(e) => setTo(e.target.value)}
            placeholder="e.g. a chat id or @handle"
            aria-label="Send recipient"
            className="h-8 min-w-0 rounded-md border border-border bg-card px-2 text-sm outline-none focus-visible:border-accent"
          />
        </label>
      </div>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Message to send…"
        aria-label="Send message text"
        className="h-20 w-full resize-y rounded-md border border-border bg-card p-2 text-sm outline-none placeholder:text-muted/60 focus-visible:border-accent"
      />
      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-xs text-muted">{channel.trim().toLowerCase() || "channel"} → {to.trim() || "recipient"}</span>
        <Button size="sm" onClick={send} disabled={!valid || sending}>
          {sending ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Send
        </Button>
      </div>
    </div>
  );
}
