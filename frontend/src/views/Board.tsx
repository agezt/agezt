import { useEffect, useMemo, useState } from "react";
import { MessagesSquare, RefreshCw, Hash, User, ArrowRight, CornerDownRight } from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";

interface Msg {
  topic: string;
  from?: string;
  text: string;
  ts_unix_ms?: number;
  // Addressed messaging (M788): direct agent-to-agent messages and replies.
  id?: string;
  to?: string;
  reply_to?: string;
}

// awaitingReply reports which ADDRESSED messages have no reply on the board
// yet — the "someone asked and nobody answered" set the operator should see at
// a glance. Pure + unit-tested.
export function awaitingReply(messages: Msg[]): Set<string> {
  const answered = new Set<string>();
  for (const m of messages) if (m.reply_to) answered.add(m.reply_to);
  const out = new Set<string>();
  for (const m of messages) {
    // A reply is an ANSWER, not a question — it never awaits one itself.
    if (m.to && m.id && !m.reply_to && !answered.has(m.id)) out.add(m.id);
  }
  return out;
}
interface BoardData {
  messages?: Msg[];
  topics?: Record<string, number>;
  count?: number;
}

// Board is the inter-agent conversation view: the shared, persistent message
// board (kernel/board) every agent on the daemon posts to and reads from to
// coordinate and talk to each other. The operator watches the back-channel here
// — handoffs, notes an agent left for its next cycle, peer chatter — filtered by
// topic. Read-only: the board is written by agents via the `board` tool.
export function Board() {
  const [data, setData] = useState<BoardData | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [topic, setTopic] = useState<string | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<BoardData>("/api/board", { limit: "200" });
      setData(d);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const topics = useMemo(
    () => Object.entries(data?.topics || {}).sort((a, b) => b[1] - a[1]),
    [data],
  );
  const messages = useMemo(
    () => (data?.messages || []).filter((m) => !topic || m.topic === topic),
    [data, topic],
  );
  // Unanswered DMs are computed over the WHOLE board (a reply may live under
  // another topic filter), so the badge never lies because of the filter.
  const waiting = useMemo(() => awaitingReply(data?.messages || []), [data]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <MessagesSquare className="size-4 text-accent" /> Agent board
        </h2>
        <span className="text-xs text-muted">
          {data ? `${data.count ?? 0} message${data.count === 1 ? "" : "s"}` : ""}
          {topics.length > 0 && <span className="text-muted"> · {topics.length} topic{topics.length === 1 ? "" : "s"}</span>}
        </span>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      </div>

      {topics.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <button
            onClick={() => setTopic(null)}
            className={cn(
              "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
              topic === null ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted hover:text-foreground",
            )}
          >
            all
          </button>
          {topics.map(([name, n]) => (
            <button
              key={name}
              onClick={() => setTopic(name === topic ? null : name)}
              className={cn(
                "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
                topic === name ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted hover:text-foreground",
              )}
            >
              <Hash className="size-3 opacity-70" />
              {name}
              <span className="opacity-60">{n}</span>
            </button>
          ))}
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !data ? (
        <SkeletonList count={4} lines={2} />
      ) : messages.length === 0 ? (
        <Muted>
          no messages yet — agents talk here with the `board` tool (post a note on a topic, read each
          other's). Try: <span className="font-mono">agt run "post 'hello' to the board topic 'general'"</span>
        </Muted>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-2">
            {messages.map((m, i) => (
              <li key={i} className="rounded-lg border border-border bg-card p-3">
                <div className="flex items-center gap-2">
                  <span className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-[10px] text-accent">
                    <Hash className="size-3 opacity-70" />
                    {m.topic}
                  </span>
                  {m.from && (
                    <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-foreground/80">
                      <User className="size-3 text-muted" />
                      {m.from}
                    </span>
                  )}
                  {m.to && (
                    <span className="inline-flex items-center gap-1 text-[11px] text-accent">
                      <ArrowRight className="size-3" />
                      {m.to}
                    </span>
                  )}
                  {m.reply_to && (
                    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-[10px] text-muted" title={`reply to ${m.reply_to}`}>
                      <CornerDownRight className="size-3" />
                      reply
                    </span>
                  )}
                  {m.id && waiting.has(m.id) && (
                    <span className="rounded-full border border-warn/40 bg-warn/10 px-2 py-0.5 text-[10px] text-warn">
                      awaiting reply
                    </span>
                  )}
                  <span className="ml-auto font-mono text-[10px] text-muted opacity-70">{fmtTime(m.ts_unix_ms)}</span>
                </div>
                <p className="mt-1.5 whitespace-pre-wrap break-words text-sm text-foreground/90">{m.text}</p>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
