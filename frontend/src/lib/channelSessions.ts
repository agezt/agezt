// channelSessions regroups the Inbox's per-correlation channel threads (each a
// single inbound message + its reply) into continuous per-user SESSIONS so the
// Chat view can present a Telegram (or Slack/Discord) conversation as one
// followable thread (M841). The daemon mints a fresh correlation per inbound
// message, so /api/inbox returns many short threads for one chat; we merge them
// by (channel_kind, channel_id, sender) into a session whose messages are the
// union, time-ordered. Pure + unit-tested.
//
// The thread and message shapes are the daemon's own rows: the browser names the
// fields it reads, the daemon names what they are.
import type {
  InboxResponseThreadsItem,
  InboxResponseThreadsItemMessagesItem,
} from "@/app/api/types.generated";

type ChannelMessage = {
  direction?: InboxResponseThreadsItemMessagesItem["direction"];
  sender?: InboxResponseThreadsItemMessagesItem["sender"];
  text?: InboxResponseThreadsItemMessagesItem["text"];
  ts_unix_ms?: InboxResponseThreadsItemMessagesItem["ts_unix_ms"];
};

export type InboxThread = {
  correlation_id: InboxResponseThreadsItem["correlation_id"];
  channel_kind?: InboxResponseThreadsItem["channel_kind"];
  channel_id?: InboxResponseThreadsItem["channel_id"];
  messages?: ChannelMessage[] | null;
  last_ts_unix_ms?: InboxResponseThreadsItem["last_ts_unix_ms"];
};

export interface ChannelSession {
  key: string; // stable id: channelKind|channelId|sender
  channelKind: string;
  channelId: string;
  sender: string;
  title: string;
  messages: ChannelMessage[];
  lastTs: number;
  correlationIds: string[];
}

// sessionKey identifies the participant: a 1:1 chat is (kind, channelId, sender).
// An outbound-only message may lack a sender, so it falls back to the channelId so
// replies still land in the right session.
function sessionKey(kind: string, channelId: string, sender: string): string {
  return `${kind}|${channelId}|${sender || channelId}`;
}

// sessionsFromInboxThreads merges threads into sessions, newest-active first.
export function sessionsFromInboxThreads(threads: InboxThread[]): ChannelSession[] {
  const byKey = new Map<string, ChannelSession>();

  for (const th of threads) {
    const kind = th.channel_kind || "?";
    const channelId = th.channel_id || "";
    // A thread's participant is the sender of its inbound message (out-only
    // threads inherit the channelId). Group every message of the thread under it.
    const inbound = (th.messages || []).find((m) => m.direction === "in");
    const sender = inbound?.sender || "";
    const key = sessionKey(kind, channelId, sender);

    let s = byKey.get(key);
    if (!s) {
      s = {
        key,
        channelKind: kind,
        channelId,
        sender,
        title: sender || channelId || kind,
        messages: [],
        lastTs: 0,
        correlationIds: [],
      };
      byKey.set(key, s);
    }
    s.correlationIds.push(th.correlation_id);
    for (const m of th.messages || []) s.messages.push(m);
    const last = th.last_ts_unix_ms || 0;
    if (last > s.lastTs) s.lastTs = last;
  }

  const sessions = [...byKey.values()];
  for (const s of sessions) {
    s.messages.sort((a, b) => (a.ts_unix_ms || 0) - (b.ts_unix_ms || 0));
    if (s.messages.length > 0) {
      s.lastTs = Math.max(s.lastTs, s.messages[s.messages.length - 1].ts_unix_ms || 0);
    }
  }
  sessions.sort((a, b) => b.lastTs - a.lastTs);
  return sessions;
}

// lastSnippet returns a short preview of a session's most recent message.
export function lastSnippet(s: ChannelSession): string {
  const last = s.messages[s.messages.length - 1];
  if (!last) return "";
  const who = last.direction === "out" ? "↩ " : "";
  return who + (last.text || "").replace(/\s+/g, " ").slice(0, 48);
}
