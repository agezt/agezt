import { useEffect, useMemo, useState } from "react";
import { Radio, ChevronDown, ChevronRight, X, MessageCircle, Send } from "lucide-react";
import { getJSON, postAction } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
import { useUI } from "@/components/ui/feedback";
import { BlobArtifact, type ArtifactEntry } from "@/views/Files";
import {
  sessionsFromInboxThreads,
  lastSnippet,
  type ChannelSession,
  type InboxThread,
} from "@/lib/channelSessions";

// ChannelSessions (M841): a "Channels" section for the Chat sidebar. Each inbound
// channel message (Telegram/Slack/…) is grouped into a per-user SESSION the owner
// can open and FOLLOW LIVE from the chat surface — the incoming messages and the
// agent's replies, updating as new traffic arrives. Read-only here (replies happen
// on the channel itself); a fully self-contained widget so the Chat view only has
// to drop it into the sidebar.
export function ChannelSessions() {
  const { events } = useEvents();
  const [sessions, setSessions] = useState<ChannelSession[]>([]);
  const [imagesByCorr, setImagesByCorr] = useState<Record<string, ArtifactEntry[]>>({});
  const [open, setOpen] = useState(true);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  async function reload() {
    try {
      const d = await getJSON<{ threads?: InboxThread[] }>("/api/inbox", { limit: "100" });
      setSessions(sessionsFromInboxThreads(d.threads ?? []));
    } catch {
      // Non-fatal: the section just stays empty if the inbox isn't reachable.
    }
    try {
      // Inbound images, bucketed by correlation, so a session can show them inline.
      const imgs = await getJSON<{ entries?: ArtifactEntry[] }>("/api/artifacts", { kind: "image" });
      const byCorr: Record<string, ArtifactEntry[]> = {};
      for (const e of imgs.entries || []) {
        if (e.corr) (byCorr[e.corr] ||= []).push(e);
      }
      setImagesByCorr(byCorr);
    } catch {
      /* images are a nicety */
    }
  }

  useEffect(() => {
    reload();
  }, []);

  // Live-refresh when channel traffic arrives (same nudge the Inbox uses).
  const head = events[0]?.kind || "";
  useEffect(() => {
    if (head.startsWith("channel.")) reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [head, events[0]?.id]);

  const selected = useMemo(
    () => sessions.find((s) => s.key === selectedKey) ?? null,
    [sessions, selectedKey],
  );

  if (sessions.length === 0) return null;

  return (
    <div className="mt-2 shrink-0 border-t border-border pt-2">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-1.5 px-1 text-[11px] font-semibold uppercase tracking-normal text-muted hover:text-foreground"
      >
        {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        <Radio className="size-3" /> Channels
        <span className="ml-auto rounded-full bg-panel px-1.5 text-xs">{sessions.length}</span>
      </button>
      {open && (
        <div className="mt-1 max-h-44 space-y-0.5 overflow-auto">
          {sessions.map((s) => (
            <button
              key={s.key}
              onClick={() => setSelectedKey(s.key)}
              className="flex w-full flex-col items-start gap-0.5 rounded-md border border-transparent px-2 py-1 text-left hover:border-border hover:bg-panel/60"
              title={`${s.channelKind} · ${s.sender || s.channelId}`}
            >
              <span className="flex w-full items-center gap-1.5">
                <MessageCircle className="size-3 shrink-0 text-accent" />
                <span className="min-w-0 flex-1 truncate text-xs font-medium">{s.title}</span>
                <span className="shrink-0 text-[9px] uppercase text-muted">{s.channelKind}</span>
              </span>
              <span className="w-full truncate pl-4 text-xs text-muted">{lastSnippet(s)}</span>
            </button>
          ))}
        </div>
      )}

      {selected && (
        <SessionPane
          session={selected}
          images={selected.correlationIds.flatMap((c) => imagesByCorr[c] || [])}
          onClose={() => setSelectedKey(null)}
          onSent={reload}
        />
      )}
    </div>
  );
}

function SessionPane({
  session,
  images,
  onClose,
  onSent,
}: {
  session: ChannelSession;
  images: ArtifactEntry[];
  onClose: () => void;
  onSent: () => void;
}) {
  const ui = useUI();
  const [reply, setReply] = useState("");
  const [sending, setSending] = useState(false);
  // Reply target: the chat/room id, falling back to the sender for 1:1 channels.
  const to = session.channelId || session.sender;

  async function send() {
    const text = reply.trim();
    if (!text || sending) return;
    setSending(true);
    try {
      await postAction("/api/send", { channel: session.channelKind, to, text });
      setReply("");
      ui.toast(`Sent to ${session.title} on ${session.channelKind}`, "success");
      onSent();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-2.5">
          <Radio className="size-4 text-accent" />
          <span className="text-sm font-semibold">{session.title}</span>
          <Badge variant="default">{session.channelKind}</Badge>
          <span className="inline-flex items-center gap-1 text-[11px] text-good">
            <span className="size-1.5 animate-pulse rounded-full bg-good" /> following live
          </span>
          <button onClick={onClose} className="ml-auto text-muted hover:text-foreground" aria-label="Close session">
            <X className="size-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-2 overflow-auto bg-panel/30 p-3">
          {session.messages.map((m, i) => {
            const out = m.direction === "out";
            return (
              <div key={i} className={cn("flex", out ? "justify-end" : "justify-start")}>
                <div
                  className={cn(
                    "max-w-[80%] rounded-xl px-3 py-1.5 text-sm",
                    out ? "bg-accent/15 text-foreground" : "bg-card border border-border",
                  )}
                >
                  {!out && m.sender && <div className="mb-0.5 text-xs font-semibold text-muted">{m.sender}</div>}
                  <Markdown source={m.text || ""} className="text-sm" />
                  <div className="mt-0.5 text-right text-[9px] text-muted opacity-70">{fmtTime(m.ts_unix_ms)}</div>
                </div>
              </div>
            );
          })}
          {images.length > 0 && (
            <div className="flex flex-wrap gap-1.5 border-t border-border/60 pt-2">
              {images.map((e) => (
                <div key={e.id} title={e.caption || e.name || "image"} className="block size-16 overflow-hidden rounded-md border border-border bg-panel">
                  <BlobArtifact entry={e} kind="image" alt={e.caption || "image"} className="size-full object-cover" />
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="flex items-end gap-2 border-t border-border px-3 py-2">
          <textarea
            value={reply}
            onChange={(e) => setReply(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send();
              }
            }}
            rows={1}
            placeholder={`Reply on ${session.channelKind} to ${to}…`}
            aria-label="Reply"
            className="min-h-8 flex-1 resize-none rounded-md border border-border bg-panel px-2 py-1.5 text-sm outline-none focus-visible:border-accent"
          />
          <button
            onClick={() => void send()}
            disabled={sending || reply.trim() === ""}
            className="inline-flex h-8 items-center gap-1 rounded-md bg-accent px-3 text-sm text-accent-foreground disabled:opacity-50"
          >
            <Send className="size-3.5" /> Send
          </button>
        </div>
      </div>
    </div>
  );
}
