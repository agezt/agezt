import { useEffect, useMemo, useState } from "react";
import { Radio, ChevronDown, ChevronRight, X, MessageCircle } from "lucide-react";
import { getJSON } from "@/lib/api";
import { useEvents } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/Markdown";
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
  const [open, setOpen] = useState(true);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  async function reload() {
    try {
      const d = await getJSON<{ threads?: InboxThread[] }>("/api/inbox", { limit: "100" });
      setSessions(sessionsFromInboxThreads(d.threads ?? []));
    } catch {
      // Non-fatal: the section just stays empty if the inbox isn't reachable.
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
        className="flex w-full items-center gap-1.5 px-1 text-[11px] font-semibold uppercase tracking-wide text-muted hover:text-foreground"
      >
        {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        <Radio className="size-3" /> Channels
        <span className="ml-auto rounded-full bg-panel px-1.5 text-[10px]">{sessions.length}</span>
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
              <span className="w-full truncate pl-4 text-[10px] text-muted">{lastSnippet(s)}</span>
            </button>
          ))}
        </div>
      )}

      {selected && <SessionPane session={selected} onClose={() => setSelectedKey(null)} />}
    </div>
  );
}

function SessionPane({ session, onClose }: { session: ChannelSession; onClose: () => void }) {
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
                    "max-w-[80%] rounded-2xl px-3 py-1.5 text-sm",
                    out ? "bg-accent/15 text-foreground" : "bg-card border border-border",
                  )}
                >
                  {!out && m.sender && <div className="mb-0.5 text-[10px] font-semibold text-muted">{m.sender}</div>}
                  <Markdown source={m.text || ""} className="text-sm" />
                  <div className="mt-0.5 text-right text-[9px] text-muted opacity-70">{fmtTime(m.ts_unix_ms)}</div>
                </div>
              </div>
            );
          })}
        </div>

        <div className="border-t border-border px-4 py-2 text-[11px] text-muted">
          Read-only — replies go out on {session.channelKind}. This view follows the conversation live.
        </div>
      </div>
    </div>
  );
}
