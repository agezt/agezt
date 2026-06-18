import { useEffect, useMemo, useState } from "react";
import { MessagesSquare, RefreshCw, Hash, User, ArrowRight, CornerDownRight, Search, LifeBuoy, Megaphone, Send, X, CheckCheck } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Markdown } from "@/components/Markdown";
import { PageHeader } from "@/components/ui/page-header";

interface Msg {
  topic: string;
  from?: string;
  text: string;
  ts_unix_ms?: number;
  // Addressed messaging (M788): direct agent-to-agent messages and replies.
  id?: string;
  to?: string;
  reply_to?: string;
  acked_by?: string[];
  // Mailbox (M849): a help request flagged for assistance; a broadcast is To "*".
  help?: boolean;
}

interface HelpData {
  open_help?: Msg[];
  count?: number;
}

export type BoardMessageFilter = "all" | "awaiting" | "dm" | "broadcast" | "acked" | "help";

interface BoardAgent {
  slug: string;
  name?: string;
  enabled?: boolean;
  retired?: boolean;
  managed?: boolean;
  direct_callable?: boolean;
  parent_agent?: string;
  owner_agent?: string;
  kind?: string;
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
    if (m.to && m.to !== "*" && m.id && !m.reply_to && !answered.has(m.id) && (m.acked_by || []).length === 0) out.add(m.id);
  }
  return out;
}

export function messageAckedBy(m: Pick<Msg, "acked_by">): string {
  return (m.acked_by || []).map((by) => by.trim()).filter(Boolean).join(", ");
}

export function filterBoardMessages(messages: Msg[], filter: BoardMessageFilter, waiting: Set<string> = awaitingReply(messages)): Msg[] {
  if (filter === "all") return messages;
  return messages.filter((m) => {
    if (filter === "awaiting") return !!m.id && waiting.has(m.id);
    if (filter === "dm") return !!m.to && m.to !== "*" && !m.help;
    if (filter === "broadcast") return m.to === "*";
    if (filter === "acked") return (m.acked_by || []).length > 0;
    if (filter === "help") return !!m.help;
    return true;
  });
}

export function boardMessageFilterCounts(messages: Msg[], waiting: Set<string> = awaitingReply(messages)): Record<BoardMessageFilter, number> {
  return {
    all: messages.length,
    awaiting: filterBoardMessages(messages, "awaiting", waiting).length,
    dm: filterBoardMessages(messages, "dm", waiting).length,
    broadcast: filterBoardMessages(messages, "broadcast", waiting).length,
    acked: filterBoardMessages(messages, "acked", waiting).length,
    help: filterBoardMessages(messages, "help", waiting).length,
  };
}

export function boardMessageInvolvesAgent(m: Pick<Msg, "from" | "to" | "acked_by">, agent: string): boolean {
  const ref = agent.trim().toLowerCase();
  if (!ref) return true;
  const from = (m.from || "").trim().toLowerCase();
  const to = (m.to || "").trim().toLowerCase();
  const acked = (m.acked_by || []).some((by) => by.trim().toLowerCase() === ref);
  return from === ref || to === ref || m.to === "*" || acked;
}

export function filterBoardMessagesByAgent(messages: Msg[], agent: string): Msg[] {
  const ref = agent.trim();
  if (!ref) return messages;
  return messages.filter((m) => boardMessageInvolvesAgent(m, ref));
}

export function boardAgentFilterFromHash(hash: string): string {
  const raw = hash.replace(/^#\/?/, "");
  const [, query = ""] = raw.split("?");
  if (!query) return "";
  return new URLSearchParams(query).get("agent")?.trim() || "";
}

export function boardHashForAgentFilter(agent: string): string {
  const ref = agent.trim();
  return ref ? `#board?agent=${encodeURIComponent(ref)}` : "#board";
}
interface BoardData {
  messages?: Msg[];
  topics?: Record<string, number>;
  count?: number;
}

export interface BoardCounts {
  messages: number;
  topics: number;
  awaiting: number;
  help: number;
}

export function boardCounts(data: BoardData | null | undefined, help: Msg[] = []): BoardCounts {
  const messages = data?.messages || [];
  return {
    messages: data?.count ?? messages.length,
    topics: Object.keys(data?.topics || {}).length,
    awaiting: awaitingReply(messages).size,
    help: help.length,
  };
}

export function boardAgentWakeIssue(agent?: BoardAgent): string {
  if (!agent) return "";
  if (agent.retired) return "revive this agent before waking it";
  if (agent.enabled === false) return "resume this agent before waking it";
  if (boardAgentManaged(agent)) {
    const owner = agent.parent_agent || agent.owner_agent;
    return owner ? `managed sub-agent; wake ${owner} instead` : "managed sub-agent; only its parent/owner can wake it";
  }
  return "";
}

export interface BoardWakePlan {
  ref: string;
  label: string;
  issue: string;
}

export function boardAgentWakePlan(agent?: BoardAgent, roster: BoardAgent[] = []): BoardWakePlan {
  if (!agent) return { ref: "", label: "Wake recipient", issue: "" };
  if (boardAgentManaged(agent)) {
    const manager = agent.parent_agent || agent.owner_agent || "";
    if (!manager) {
      return { ref: "", label: "Wake manager", issue: "managed sub-agent; only its parent/owner can wake it" };
    }
    const managerAgent = roster.find((p) => p.slug === manager);
    const managerIssue = boardAgentWakeIssue(managerAgent);
    if (managerIssue) {
      return { ref: "", label: `Wake ${manager}`, issue: `manager ${manager}: ${managerIssue}` };
    }
    return { ref: manager, label: `Wake ${manager}`, issue: "" };
  }
  const issue = boardAgentWakeIssue(agent);
  return { ref: issue ? "" : agent.slug, label: "Wake recipient", issue };
}

export interface BoardAgentMailboxSummary {
  value: string;
  detail: string;
  tone: "good" | "warn" | "muted";
  waiting: number;
  received: number;
  sent: number;
  acked: number;
  wake: string;
}

export function boardAgentMailboxSummary(messages: Msg[], agent: string, roster: BoardAgent[] = []): BoardAgentMailboxSummary {
  const ref = agent.trim();
  if (!ref) {
    return {
      value: "all mailboxes",
      detail: `${messages.length} visible board message${messages.length === 1 ? "" : "s"}`,
      tone: "muted",
      waiting: awaitingReply(messages).size,
      received: messages.length,
      sent: 0,
      acked: 0,
      wake: "select an agent",
    };
  }
  const lower = ref.toLowerCase();
  const involved = filterBoardMessagesByAgent(messages, ref);
  const waiting = filterBoardMessages(involved, "awaiting", awaitingReply(messages)).filter((m) => boardMessageInvolvesAgent(m, ref)).length;
  const received = involved.filter((m) => {
    const to = (m.to || "").trim().toLowerCase();
    return to === lower || m.to === "*";
  }).length;
  const sent = involved.filter((m) => (m.from || "").trim().toLowerCase() === lower).length;
  const acked = involved.filter((m) => (m.acked_by || []).some((by) => by.trim().toLowerCase() === lower)).length;
  const agentRow = roster.find((a) => a.slug.trim().toLowerCase() === lower);
  const wakePlan = boardAgentWakePlan(agentRow, roster);
  const wake = wakePlan.issue || (wakePlan.ref ? `${wakePlan.label.toLowerCase()} -> ${wakePlan.ref}` : "wake recipient");
  return {
    value: waiting > 0 ? `${ref} mailbox: ${waiting} waiting` : `${ref} mailbox clear`,
    detail: `${received} received · ${sent} sent · ${acked} acked · ${wake}`,
    tone: waiting > 0 ? "warn" : received > 0 || sent > 0 || acked > 0 ? "good" : "muted",
    waiting,
    received,
    sent,
    acked,
    wake,
  };
}

function boardAgentManaged(agent: BoardAgent): boolean {
  return agent.kind === "subagent" || !!agent.managed || agent.direct_callable === false;
}

// Board is the inter-agent conversation view: the shared, persistent message
// board (kernel/board) every agent on the daemon posts to and reads from to
// coordinate and talk to each other. The operator watches the back-channel here
// — handoffs, notes an agent left for its next cycle, peer chatter — filtered by
// topic. Operators can also drop DM/help/broadcast/topic messages into the same
// mailbox path agents use through the `board` tool.
export function Board() {
  const [data, setData] = useState<BoardData | null>(null);
  const [help, setHelp] = useState<Msg[]>([]);
  const [agents, setAgents] = useState<BoardAgent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [sending, setSending] = useState(false);
  const [showCompose, setShowCompose] = useState(false);
  const [sendMode, setSendMode] = useState<"dm" | "help" | "broadcast" | "topic">("dm");
  const [sendFrom, setSendFrom] = useState("operator");
  const [sendTo, setSendTo] = useState("");
  const [sendTopic, setSendTopic] = useState("dm");
  const [sendText, setSendText] = useState("");
  const [replyTo, setReplyTo] = useState("");
  const [replyText, setReplyText] = useState("");
  const [wakeAfterSend, setWakeAfterSend] = useState(false);
  const [topic, setTopic] = useState<string | null>(null);
  const [topicQuery, setTopicQuery] = useState("");
  const [messageFilter, setMessageFilter] = useState<BoardMessageFilter>("all");
  const [agentFilter, setAgentFilter] = useState(() => boardAgentFilterFromHash(location.hash));
  const [showAllTopics, setShowAllTopics] = useState(false);

  async function reload() {
    setLoading(true);
    try {
      const [d, h, a] = await Promise.all([
        getJSON<BoardData>("/api/board", { limit: "200" }),
        getJSON<HelpData>("/api/board/help").catch(() => ({ open_help: [] }) as HelpData),
        getJSON<{ profiles?: BoardAgent[] }>("/api/agents").catch(() => ({ profiles: [] })),
      ]);
      setData(d);
      setHelp(h.open_help || []);
      setAgents(a.profiles || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function sendBoardMessage() {
    const text = sendText.trim();
    const recipient = sendTo.trim();
    const selectedRecipient = agents.find((a) => a.slug === recipient);
    const wakePlan = boardAgentWakePlan(selectedRecipient, agents);
    if (!text) {
      setErr("message text is required");
      return;
    }
    const body: Record<string, unknown> = { from: sendFrom.trim() || "operator", text };
    if (sendMode === "dm") {
      if (!recipient) {
        setErr("recipient is required");
        return;
      }
      body.to = recipient;
      body.topic = sendTopic.trim() || "dm";
    } else if (sendMode === "help") {
      body.help = true;
      if (recipient) body.to = recipient;
    } else if (sendMode === "broadcast") {
      body.to = "*";
    } else {
      if (!sendTopic.trim()) {
        setErr("topic is required");
        return;
      }
      body.topic = sendTopic.trim();
    }
    setSending(true);
    try {
      await postJSON("/api/board/send", body);
      if (wakeAfterSend && (sendMode === "dm" || sendMode === "help") && wakePlan.ref) {
        await postAction("/api/agents/wake", { ref: wakePlan.ref, reason: "mailbox message" });
      }
      setSendText("");
      setErr(null);
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setSending(false);
    }
  }

  async function ackBoardMessage(id: string) {
    if (!agentFilter.trim()) return;
    setLoading(true);
    try {
      await postJSON("/api/board/ack", { id, by: agentFilter.trim() });
      setErr(null);
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function replyBoardMessage(id: string) {
    const from = agentFilter.trim();
    const text = replyText.trim();
    if (!from || !text) return;
    setLoading(true);
    try {
      await postJSON("/api/board/send", { from, reply_to: id, text });
      setReplyTo("");
      setReplyText("");
      setErr(null);
      await reload();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  function changeAgentFilter(next: string) {
    setAgentFilter(next);
    setReplyTo("");
    setReplyText("");
    const nextHash = boardHashForAgentFilter(next);
    if (location.hash !== nextHash) history.replaceState(null, "", nextHash);
  }

  useEffect(() => {
    reload();
    const id = setInterval(reload, 6000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => {
    if (!sendTo && agents.length > 0 && (sendMode === "dm" || sendMode === "help")) {
      setSendTo(agents[0].slug);
    }
  }, [agents, sendMode, sendTo]);

  const topics = useMemo(
    () => Object.entries(data?.topics || {}).sort((a, b) => b[1] - a[1]),
    [data],
  );
  // With many agents the board grows a long tail of topics; a search + a visible
  // cap keep the chip row from swallowing the view (M829). The selected topic is
  // always kept visible even when filtered/capped out, so the filter never hides
  // what you're looking at.
  const filteredTopics = useMemo(() => {
    const q = topicQuery.trim().toLowerCase();
    return q ? topics.filter(([name]) => name.toLowerCase().includes(q)) : topics;
  }, [topics, topicQuery]);
  const TOPIC_CAP = 24;
  const visibleTopics = useMemo(() => {
    const base = showAllTopics ? filteredTopics : filteredTopics.slice(0, TOPIC_CAP);
    if (topic && !base.some(([n]) => n === topic)) {
      const sel = filteredTopics.find(([n]) => n === topic) ?? topics.find(([n]) => n === topic);
      if (sel) return [sel, ...base];
    }
    return base;
  }, [filteredTopics, showAllTopics, topic, topics]);
  const hiddenCount = filteredTopics.length - Math.min(filteredTopics.length, TOPIC_CAP);
  const topicMessages = useMemo(
    () => (data?.messages || []).filter((m) => !topic || m.topic === topic),
    [data, topic],
  );
  const agentMessages = useMemo(() => filterBoardMessagesByAgent(topicMessages, agentFilter), [agentFilter, topicMessages]);
  // Unanswered DMs are computed over the WHOLE board (a reply may live under
  // another topic filter), so the badge never lies because of the filter.
  const waiting = useMemo(() => awaitingReply(data?.messages || []), [data]);
  const filterCounts = useMemo(() => boardMessageFilterCounts(agentMessages, waiting), [agentMessages, waiting]);
  const messages = useMemo(() => filterBoardMessages(agentMessages, messageFilter, waiting), [agentMessages, messageFilter, waiting]);
  const counts = useMemo(() => boardCounts(data, help), [data, help]);
  const selectedRecipient = useMemo(() => agents.find((a) => a.slug === sendTo.trim()), [agents, sendTo]);
  const wakePlan = useMemo(() => boardAgentWakePlan(selectedRecipient, agents), [agents, selectedRecipient]);
  const agentMailbox = useMemo(
    () => boardAgentMailboxSummary(topicMessages, agentFilter, agents),
    [agentFilter, agents, topicMessages],
  );

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={MessagesSquare}
        title="Agent board"
        description={
          <>
            {data ? `${data.count ?? 0} message${data.count === 1 ? "" : "s"}` : "Shared inter-agent message board"}
            {topics.length > 0 && <span className="text-muted"> · {topics.length} topic{topics.length === 1 ? "" : "s"}</span>}
          </>
        }
        actions={
          <>
            <Button size="sm" onClick={() => setShowCompose((v) => !v)} title="New message">
              {showCompose ? <X className="size-3.5" /> : <Send className="size-3.5" />}
              {showCompose ? "Close" : "New message"}
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {showCompose && (
        <div className="rounded-lg border border-border bg-card p-3">
          <div className="grid gap-2 md:grid-cols-[120px_1fr_1fr]">
            <select
              value={sendMode}
              onChange={(e) => setSendMode(e.target.value as "dm" | "help" | "broadcast" | "topic")}
              aria-label="Message mode"
              className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent"
            >
              <option value="dm">DM</option>
              <option value="help">Help</option>
              <option value="broadcast">Broadcast</option>
              <option value="topic">Topic</option>
            </select>
            <input
              value={sendFrom}
              onChange={(e) => setSendFrom(e.target.value)}
              aria-label="From"
              placeholder="from"
              className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent"
            />
            {(sendMode === "dm" || sendMode === "help") ? (
              agents.length > 0 ? (
                <select
                  value={sendTo}
                  onChange={(e) => setSendTo(e.target.value)}
                  aria-label="To"
                  className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent"
                >
                  {agents.map((a) => (
                    <option key={a.slug} value={a.slug}>
                      {agentLabel(a)}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  value={sendTo}
                  onChange={(e) => setSendTo(e.target.value)}
                  aria-label="To"
                  placeholder={sendMode === "help" ? "to (optional)" : "to"}
                  className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent"
                />
              )
            ) : (
              <input
                value={sendTopic}
                onChange={(e) => setSendTopic(e.target.value)}
                aria-label="Topic"
                placeholder="topic"
                disabled={sendMode === "broadcast"}
                className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent disabled:opacity-60"
              />
            )}
          </div>
          {sendMode === "dm" && (
            <input
              value={sendTopic}
              onChange={(e) => setSendTopic(e.target.value)}
              aria-label="Topic"
              placeholder="topic"
              className="mt-2 w-full rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent"
            />
          )}
          <textarea
            value={sendText}
            onChange={(e) => setSendText(e.target.value)}
            aria-label="Message text"
            placeholder="message"
            rows={3}
            className="mt-2 w-full resize-none rounded-md border border-border bg-panel px-2 py-1.5 text-sm outline-none focus-visible:border-accent"
          />
          <div className="mt-2 flex items-center justify-between gap-2">
            {(sendMode === "dm" || sendMode === "help") && (
              <div className="flex min-w-0 flex-col gap-0.5">
                <label className="inline-flex items-center gap-2 text-xs text-muted">
                  <input
                    type="checkbox"
                    checked={wakeAfterSend && !!wakePlan.ref}
                    onChange={(e) => setWakeAfterSend(e.target.checked)}
                    disabled={!sendTo.trim() || !wakePlan.ref}
                    className="size-3.5 accent-accent"
                    aria-label="Wake recipient"
                  />
                  {wakePlan.label}
                </label>
                {wakePlan.issue && <span className="text-[10px] text-warn">{wakePlan.issue}</span>}
              </div>
            )}
            <Button size="sm" onClick={sendBoardMessage} disabled={sending || !sendText.trim() || (sendMode === "dm" && !sendTo.trim())}>
              <Send className="size-3.5" />
              Send
            </Button>
          </div>
        </div>
      )}

      {help.length > 0 && (
        <div className="rounded-lg border border-warn/40 bg-warn/10 p-2.5">
          <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold text-warn">
            <LifeBuoy className="size-3.5" />
            {help.length} open help request{help.length === 1 ? "" : "s"} — waiting for an answer
          </div>
          <ul className="space-y-1">
            {help.map((h, i) => (
              <li key={h.id ?? i} className="flex items-start gap-2 text-xs">
                {h.from && <span className="shrink-0 font-semibold text-foreground/80">{h.from}</span>}
                {h.to && h.to !== "*" && (
                  <span className="inline-flex shrink-0 items-center gap-0.5 text-accent">
                    <ArrowRight className="size-3" />
                    {h.to}
                  </span>
                )}
                <span className="text-foreground/90">{h.text}</span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted opacity-70">{fmtTime(h.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {data && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <BoardStat label="messages" value={counts.messages} />
          <BoardStat label="topics" value={counts.topics} accent={counts.topics > 0} />
          <BoardStat label="awaiting" value={counts.awaiting} accent={counts.awaiting > 0} warn={counts.awaiting > 0} />
          <BoardStat label="help" value={counts.help} accent={counts.help > 0} warn={counts.help > 0} />
        </div>
      )}

      {topics.length > 0 && (
        <div className="flex flex-col gap-1.5">
          {topics.length > 12 && (
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <input
                value={topicQuery}
                onChange={(e) => setTopicQuery(e.target.value)}
                placeholder={`filter ${topics.length} topics…`}
                aria-label="Filter topics"
                className="w-full rounded-md border border-border bg-panel py-1 pl-7 pr-2 text-xs outline-none focus-visible:border-accent"
              />
            </div>
          )}
          <div className="flex max-h-24 flex-wrap items-center gap-1.5 overflow-y-auto">
            <button
              onClick={() => setTopic(null)}
              className={cn(
                "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
                topic === null ? "border-accent bg-accent/10 text-accent" : "border-border bg-panel text-muted hover:text-foreground",
              )}
            >
              all
            </button>
            {visibleTopics.map(([name, n]) => (
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
            {!showAllTopics && hiddenCount > 0 && !topicQuery && (
              <button
                onClick={() => setShowAllTopics(true)}
                className="rounded-full border border-dashed border-border px-2.5 py-0.5 text-[11px] text-muted hover:text-accent"
              >
                +{hiddenCount} more
              </button>
            )}
            {showAllTopics && filteredTopics.length > TOPIC_CAP && (
              <button
                onClick={() => setShowAllTopics(false)}
                className="rounded-full border border-dashed border-border px-2.5 py-0.5 text-[11px] text-muted hover:text-accent"
              >
                show fewer
              </button>
            )}
            {topicQuery && filteredTopics.length === 0 && (
              <span className="text-[11px] text-muted">no topics match “{topicQuery}”</span>
            )}
          </div>
        </div>
      )}

      {data && (
        <div className="flex flex-col gap-2">
          {agentFilter.trim() && (
            <div
              className={cn(
                "flex flex-wrap items-center gap-2 rounded-lg border bg-card px-3 py-2 text-xs",
                agentMailbox.tone === "warn"
                  ? "border-warn/50"
                  : agentMailbox.tone === "good"
                    ? "border-good/40"
                    : "border-border",
              )}
            >
              <span
                className={cn(
                  "font-semibold",
                  agentMailbox.tone === "warn" ? "text-warn" : agentMailbox.tone === "good" ? "text-good" : "text-muted",
                )}
              >
                {agentMailbox.value}
              </span>
              <span className="text-muted">{agentMailbox.detail}</span>
            </div>
          )}
          <div className="flex flex-wrap items-center gap-2">
          {agents.length > 0 && (
            <label className="inline-flex items-center gap-1.5 text-xs text-muted">
              <User className="size-3.5" />
              <select
                value={agentFilter}
                onChange={(e) => changeAgentFilter(e.target.value)}
                aria-label="Filter by agent"
                className="rounded-full border border-border bg-panel px-2.5 py-1 text-xs text-foreground outline-none focus-visible:border-accent"
              >
                <option value="">All agents</option>
                {agents.map((a) => (
                  <option key={a.slug} value={a.slug}>
                    {agentLabel(a)}
                  </option>
                ))}
              </select>
            </label>
          )}
          <div className="flex flex-wrap items-center gap-1.5">
            {([
              ["all", "All"],
              ["awaiting", "Awaiting"],
              ["dm", "DM"],
              ["broadcast", "Broadcast"],
              ["acked", "Seen"],
              ["help", "Help"],
            ] as [BoardMessageFilter, string][]).map(([id, label]) => (
              <button
                key={id}
                onClick={() => setMessageFilter(id)}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors",
                  messageFilter === id ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
                )}
              >
                {label}
                <span className="rounded-full bg-card px-1.5 text-[10px] tabular-nums">{filterCounts[id]}</span>
              </button>
            ))}
          </div>
          </div>
        </div>
      )}

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !data ? (
        <SkeletonList count={4} lines={2} />
      ) : messages.length === 0 ? (
        messageFilter === "all" ? (
          <Muted>
            no messages yet — agents talk here with the `board` tool (post a note on a topic, read each
            other's). Try: <span className="font-mono">agt run "post 'hello' to the board topic 'general'"</span>
          </Muted>
        ) : (
          <Muted>no {messageFilter} messages match this board view{agentFilter ? ` for ${agentFilter}` : ""}</Muted>
        )
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <ul className="space-y-2">
            {messages.map((m, i) => {
              const ackedBy = messageAckedBy(m);
              const selectedAgent = agentFilter.trim().toLowerCase();
              const selectedAcked = !!selectedAgent && (m.acked_by || []).some((by) => by.trim().toLowerCase() === selectedAgent);
              const canAckForSelected = !!agentFilter.trim() && !!m.id && !selectedAcked;
              return (
              <li key={i} className="glass rounded-xl p-3">
                <div className="flex items-center gap-2">
                  {ackedBy ? (
                    <span className="inline-flex items-center gap-1 rounded-full border border-good/40 bg-good/10 px-2 py-0.5 text-[10px] text-good" title={`acknowledged by ${ackedBy}`}>
                      <CheckCheck className="size-3" />
                      seen by {ackedBy}
                    </span>
                  ) : null}
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
                  {m.help && (
                    <span className="inline-flex items-center gap-1 rounded-full border border-warn/40 bg-warn/10 px-2 py-0.5 text-[10px] text-warn" title="help request">
                      <LifeBuoy className="size-3" />
                      help
                    </span>
                  )}
                  {m.to === "*" ? (
                    <span className="inline-flex items-center gap-1 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-[10px] text-accent" title="broadcast to every agent">
                      <Megaphone className="size-3" />
                      all
                    </span>
                  ) : (
                    m.to && (
                      <span className="inline-flex items-center gap-1 text-[11px] text-accent">
                        <ArrowRight className="size-3" />
                        {m.to}
                      </span>
                    )
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
                  {canAckForSelected && (
                    <span className="ml-auto flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={loading}
                        onClick={() => {
                          setReplyTo(replyTo === m.id ? "" : m.id || "");
                          setReplyText("");
                        }}
                        title={`Reply to this board message as ${agentFilter}`}
                      >
                        <CornerDownRight className="size-3.5" /> Reply as {agentFilter}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={loading}
                        onClick={() => m.id && ackBoardMessage(m.id)}
                        title={`Acknowledge this board message as ${agentFilter}`}
                      >
                        <CheckCheck className="size-3.5" /> Ack as {agentFilter}
                      </Button>
                    </span>
                  )}
                  <span className="ml-auto font-mono text-[10px] text-muted opacity-70">{fmtTime(m.ts_unix_ms)}</span>
                </div>
                <Markdown source={m.text} className="mt-1.5 text-sm text-foreground/90" />
                {m.id && replyTo === m.id && (
                  <div className="mt-2 flex flex-col gap-1.5 rounded-md border border-border bg-card/60 p-2 sm:flex-row">
                    <input
                      value={replyText}
                      onChange={(e) => setReplyText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && !e.shiftKey) void replyBoardMessage(m.id || "");
                      }}
                      aria-label={`Reply to ${m.id}`}
                      placeholder={`reply as ${agentFilter}`}
                      className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
                    />
                    <Button size="sm" disabled={loading || !replyText.trim()} onClick={() => replyBoardMessage(m.id || "")}>
                      <CornerDownRight className="size-3.5" /> Send reply
                    </Button>
                  </div>
                )}
              </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}

function BoardStat({ label, value, accent, warn }: { label: string; value: number | string; accent?: boolean; warn?: boolean }) {
  return (
    <div className={cn("rounded-lg border bg-card p-2.5", warn ? "border-warn/50" : accent ? "border-accent/50" : "border-border")}>
      <div className="text-[10px] font-semibold uppercase tracking-wider text-muted">{label}</div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", warn ? "text-warn" : accent && "text-accent")}>{value}</div>
    </div>
  );
}

function agentLabel(agent: BoardAgent): string {
  const flags = [
    agent.retired ? "retired" : "",
    agent.enabled === false ? "paused" : "",
    agent.kind === "system" ? "system" : "",
  ].filter(Boolean);
  const base = agent.name ? `${agent.name} (${agent.slug})` : agent.slug;
  return flags.length ? `${base} (${flags.join(", ")})` : base;
}
