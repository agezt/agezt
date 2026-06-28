import React, { useEffect, useMemo, useState } from "react";
import type { ComponentType } from "react";
import { MessagesSquare, RefreshCw, Hash, User, ArrowRight, CornerDownRight, LifeBuoy, Megaphone, Send, X, CheckCheck, Zap, Inbox, MessageSquare, Terminal, AtSign, BellRing, type LucideIcon } from "lucide-react";
import { getJSON, postAction, postJSON } from "@/lib/api";
import { cn, fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Muted, ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { Markdown } from "@/components/Markdown";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { Badge } from "@/components/ui/badge";
import { Disclosure } from "@/components/ui/disclosure";

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
      setShowCompose(false);
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
        description="Per-skill signal bus."
        actions={
          <>
            <Button size="sm" onClick={() => setShowCompose(true)} title="New message">
              <Send className="size-3.5" />
              New message
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {showCompose && (
        <BoardModal title="New board message" onClose={() => setShowCompose(false)}>
          <div className="space-y-2">
            <div className="rounded-lg border border-accent/25 bg-accent/10 p-2.5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <div className="grid size-8 place-items-center rounded-lg border border-accent/25 bg-background/70 text-accent">
                    {sendMode === "help" ? <LifeBuoy className="size-4" /> : sendMode === "broadcast" ? <Megaphone className="size-4" /> : sendMode === "topic" ? <Hash className="size-4" /> : <Send className="size-4" />}
                  </div>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold text-foreground">
                      {sendMode === "dm" ? "Direct mailbox" : sendMode === "help" ? "Help request" : sendMode === "broadcast" ? "Broadcast" : "Topic note"}
                    </div>
                    <div className="truncate text-[11px] text-muted">
                      {sendMode === "broadcast" ? "to everyone" : sendMode === "topic" ? (sendTopic.trim() || "topic required") : (sendTo.trim() || "recipient required")}
                    </div>
                  </div>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  <Badge variant={sendText.trim() ? "good" : "warn"}>{sendText.trim() ? "message ready" : "needs text"}</Badge>
                  {(sendMode === "dm" || sendMode === "help") && wakePlan.ref ? <Badge variant="accent">wake armed</Badge> : null}
                </div>
              </div>
            </div>

            <ComposeSection icon={MessageSquare} title="Message" meta={sendText.trim() ? `${sendText.trim().slice(0, 46)}${sendText.trim().length > 46 ? "..." : ""}` : "empty"} defaultOpen>
              <textarea
                value={sendText}
                onChange={(e) => setSendText(e.target.value)}
                aria-label="Message text"
                placeholder="message"
                rows={3}
                className="w-full resize-none rounded-md border border-border bg-panel px-2 py-1.5 text-sm outline-none focus-visible:border-accent"
              />
            </ComposeSection>

            <ComposeSection icon={AtSign} title="Route" meta={sendMode === "broadcast" ? "all agents" : sendMode === "topic" ? (sendTopic.trim() || "topic") : (sendTo.trim() || "recipient")} defaultOpen>
              <div className="grid gap-2 md:grid-cols-[1fr_1fr]">
                <MessageModePicker value={sendMode} onChange={setSendMode} />
                <input
                  value={sendFrom}
                  onChange={(e) => setSendFrom(e.target.value)}
                  aria-label="From"
                  placeholder="from"
                  className="rounded-md border border-border bg-panel px-2 py-1.5 text-xs outline-none focus-visible:border-accent"
                />
                {(sendMode === "dm" || sendMode === "help") ? (
                  agents.length > 0 ? (
                    <AgentChipPicker value={sendTo} onChange={setSendTo} agents={agents} label="To" />
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
            </ComposeSection>

            {(sendMode === "dm" || sendMode === "help") && (
              <ComposeSection icon={BellRing} title="Delivery" meta={wakePlan.issue ? "wake blocked" : wakePlan.ref ? "wake available" : "message only"}>
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
                {wakePlan.issue && <div className="mt-1 text-xs text-warn">{wakePlan.issue}</div>}
              </ComposeSection>
            )}

            <div className="flex justify-end">
              <Button size="sm" onClick={sendBoardMessage} disabled={sending || !sendText.trim() || (sendMode === "dm" && !sendTo.trim())}>
                <Send className="size-3.5" />
                Send
              </Button>
            </div>
          </div>
        </BoardModal>
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
                <span className="ml-auto shrink-0 font-mono text-xs text-muted opacity-70">{fmtTime(h.ts_unix_ms)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {data && messages.length > 0 && (
        <TabNav
          value={topic ?? "all"}
          onValueChange={(v) => setTopic(v === "all" ? null : v)}
          tabs={[
            {
              id: "all",
              label: "All",
              icon: Inbox,
              count: messages.length,
              content: (
                <BoardMessageGroups messages={messages} agentFilter={agentFilter} waiting={waiting} replyTo={replyTo} />
              ),
            },
            ...(["decisions", "briefings", "board", "tool_calls"] as const).map((t) => {
              const tabMessages = messages.filter((m) => m.topic === t);
              if (tabMessages.length === 0) return null;
              return {
                id: t,
                label: t.charAt(0).toUpperCase() + t.slice(1).replace("_", " "),
                icon: t === "decisions" ? Zap : t === "briefings" ? MessageSquare : t === "board" ? Inbox : Terminal,
                count: tabMessages.length,
                content: (
                  <BoardMessageGroups messages={tabMessages} agentFilter={agentFilter} waiting={waiting} replyTo={replyTo} />
                ),
              };
            }).filter(Boolean) as { id: string; label: string; icon: ComponentType<{ className?: string }>; count: number; content: React.ReactNode }[],
          ]}
        />
      )}

      {data && agents.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex items-center gap-1.5 text-xs text-muted">
            <User className="size-3.5" />
            <AgentChipPicker
              value={agentFilter}
              onChange={changeAgentFilter}
              agents={agents}
              label="Filter by agent"
              includeAll
              compact
            />
          </div>
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
                  "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors",
                  messageFilter === id ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
                )}
              >
                {label}
                <span className="rounded-full bg-panel px-1 text-xs tabular-nums">{filterCounts[id]}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {data && agentFilter.trim() && (
        <div
          className={cn(
            "rounded-lg border px-3 py-2",
            agentMailbox.tone === "warn"
              ? "border-warn/40 bg-warn/10"
              : agentMailbox.tone === "good"
                ? "border-good/30 bg-good/5"
                : "border-border bg-card",
          )}
        >
          <div
            className={cn(
              "text-xs font-semibold",
              agentMailbox.tone === "warn" ? "text-warn" : agentMailbox.tone === "good" ? "text-good" : "text-foreground/90",
            )}
          >
            {agentMailbox.value}
          </div>
          <div className="mt-0.5 text-[11px] text-muted">{agentMailbox.detail}</div>
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
        <div className="min-h-0 flex-1 overflow-auto" data-testid="board-message-list">
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
                    <span className="inline-flex items-center gap-1 rounded-full border border-good/40 bg-good/10 px-2 py-0.5 text-xs text-good" title={`acknowledged by ${ackedBy}`}>
                      <CheckCheck className="size-3" />
                      seen by {ackedBy}
                    </span>
                  ) : null}
                  <span className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-xs text-accent">
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
                    <span className="inline-flex items-center gap-1 rounded-full border border-warn/40 bg-warn/10 px-2 py-0.5 text-xs text-warn" title="help request">
                      <LifeBuoy className="size-3" />
                      help
                    </span>
                  )}
                  {m.to === "*" ? (
                    <span className="inline-flex items-center gap-1 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-xs text-accent" title="broadcast to every agent">
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
                    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-panel px-2 py-0.5 text-xs text-muted" title={`reply to ${m.reply_to}`}>
                      <CornerDownRight className="size-3" />
                      reply
                    </span>
                  )}
                  {m.id && waiting.has(m.id) && (
                    <span className="rounded-full border border-warn/40 bg-warn/10 px-2 py-0.5 text-xs text-warn">
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
                  <span className="ml-auto font-mono text-xs text-muted opacity-70">{fmtTime(m.ts_unix_ms)}</span>
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

function BoardModal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: React.ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Send className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Drop a DM, help request, topic note, or broadcast into the agent mailbox.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close board modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}

type BoardSendMode = "dm" | "help" | "broadcast" | "topic";

function ComposeSection({
  icon: Icon,
  title,
  meta,
  children,
  defaultOpen = false,
}: {
  icon: LucideIcon;
  title: string;
  meta: string;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <Disclosure
      defaultOpen={defaultOpen}
      className="rounded-lg border border-border bg-panel/45"
      summaryClassName="px-2.5 py-2"
      contentClassName="px-2.5 pb-2"
      summary={
        <span className="flex min-w-0 items-center gap-2">
          <span className="grid size-7 shrink-0 place-items-center rounded-md border border-border bg-background/70 text-accent">
            <Icon className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">{title}</span>
            <span className="block truncate text-[11px] font-normal text-muted">{meta}</span>
          </span>
        </span>
      }
    >
      {children}
    </Disclosure>
  );
}

function MessageModePicker({ value, onChange }: { value: BoardSendMode; onChange: (value: BoardSendMode) => void }) {
  const modes: Array<{ id: BoardSendMode; label: string; icon: ComponentType<{ className?: string }> }> = [
    { id: "dm", label: "DM", icon: Send },
    { id: "help", label: "Help", icon: LifeBuoy },
    { id: "broadcast", label: "Broadcast", icon: Megaphone },
    { id: "topic", label: "Topic", icon: Hash },
  ];
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Message mode">
      {modes.map(({ id, label, icon: Icon }) => (
        <button
          key={id}
          type="button"
          aria-pressed={value === id}
          onClick={() => onChange(id)}
          className={cn(
            "inline-flex h-8 items-center gap-1.5 rounded-md border px-2 text-xs font-medium transition-colors",
            value === id
              ? "border-accent bg-accent/15 text-accent"
              : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
          )}
        >
          <Icon className="size-3.5" />
          {label}
        </button>
      ))}
    </div>
  );
}

function AgentChipPicker({
  value,
  onChange,
  agents,
  label,
  includeAll = false,
  compact = false,
}: {
  value: string;
  onChange: (value: string) => void;
  agents: BoardAgent[];
  label: string;
  includeAll?: boolean;
  compact?: boolean;
}) {
  const items = includeAll ? [{ slug: "", name: "All agents" } as BoardAgent, ...agents] : agents;
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={label}>
      {items.map((agent) => {
        const selected = value === agent.slug;
        return (
          <button
            key={agent.slug || "all"}
            type="button"
            aria-pressed={selected}
            onClick={() => onChange(agent.slug)}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-md border font-medium transition-colors",
              compact ? "h-7 px-2 text-[11px]" : "h-8 px-2 text-xs",
              selected
                ? "border-accent bg-accent/15 text-accent"
                : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
            )}
          >
            <User className="size-3.5" />
            {agentLabel(agent)}
          </button>
        );
      })}
    </div>
  );
}

function BoardMessageGroups({ messages, agentFilter, waiting, replyTo }: {
  messages: Msg[];
  agentFilter: string;
  waiting: Set<string>;
  replyTo: string;
}) {
  const groups = useMemo(() => {
    const map = new Map<string, Msg[]>();
    for (const m of messages) {
      const key = m.topic || "(no topic)";
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(m);
    }
    return Array.from(map.entries()).sort((a, b) => b[1].length - a[1].length);
  }, [messages]);

  if (groups.length === 0) {
    return <Muted>no messages yet</Muted>;
  }

  if (groups.length === 1) {
    return (
      <div className="space-y-2">
        {renderMessageList(groups[0][1], agentFilter, waiting, replyTo)}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {groups.map(([topic, msgs]) => (
        <BoardTopicPanel
          key={topic}
          title={topic}
          icon={Hash}
          status={`${msgs.length} message${msgs.length === 1 ? "" : "s"}`}
        >
          {renderMessageList(msgs, agentFilter, waiting, replyTo)}
        </BoardTopicPanel>
      ))}
    </div>
  );
}

function BoardTopicPanel({
  icon: Icon,
  title,
  status,
  children,
}: {
  icon: ComponentType<{ className?: string }>;
  title: string;
  status: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-accent/35 bg-accent/5 text-accent">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-semibold">{title}</h3>
          <div className="truncate text-xs text-muted">{status}</div>
        </div>
      </div>
      {children}
    </section>
  );
}

function renderMessageList(msgs: Msg[], agentFilter: string, waiting: Set<string>, replyTo: string) {
  return (
    <ul className="space-y-2">
      {msgs.map((m, i) => {
        const ackedBy = messageAckedBy(m);
        const selectedAgent = agentFilter.trim().toLowerCase();
        const selectedAcked = !!selectedAgent && (m.acked_by || []).some((by) => by.trim().toLowerCase() === selectedAgent);
        const canAckForSelected = !!agentFilter.trim() && !!m.id && !selectedAcked;
        return (
          <li key={i} className="glass rounded-xl p-3">
            <div className="flex flex-wrap items-center gap-1.5">
              {m.help && (
                <Badge variant="warn">
                  <LifeBuoy className="size-3 mr-1" />
                  help
                </Badge>
              )}
              {m.to === "*" && (
                <Badge variant="accent">
                  <Megaphone className="size-3 mr-1" />
                  all
                </Badge>
              )}
              {m.reply_to && (
                <Badge>
                  <CornerDownRight className="size-3 mr-1" />
                  reply
                </Badge>
              )}
              {ackedBy ? (
                <Badge variant="good">
                  <CheckCheck className="size-3 mr-1" />
                  seen
                </Badge>
              ) : null}
              {m.id && waiting.has(m.id) && (
                <Badge variant="warn">awaiting</Badge>
              )}
              {m.from && (
                <span className="ml-auto flex items-center gap-1 text-[11px] font-semibold text-foreground/80">
                  <User className="size-3 text-muted" />
                  {m.from}
                </span>
              )}
              {m.to && m.to !== "*" && (
                <span className="flex items-center gap-0.5 text-[11px] text-accent">
                  <ArrowRight className="size-3" />
                  {m.to}
                </span>
              )}
              <span className="ml-auto font-mono text-xs text-muted">{fmtTime(m.ts_unix_ms)}</span>
            </div>
            <Markdown source={m.text} className="mt-1.5 text-sm text-foreground/90" />
            {canAckForSelected && (
              <div className="mt-2 flex items-center gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {}}
                  title={`Reply as ${agentFilter}`}
                >
                  <CornerDownRight className="size-3.5" /> Reply
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {}}
                  title={`Ack as ${agentFilter}`}
                >
                  <CheckCheck className="size-3.5" /> Ack
                </Button>
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function BoardStat({ label, value, accent, warn }: { label: string; value: number | string; accent?: boolean; warn?: boolean }) {
  return (
    <div className={cn("rounded-lg border bg-card p-2.5", warn ? "border-warn/50" : accent ? "border-accent/50" : "border-border")}>
      <div className="text-xs font-semibold uppercase tracking-normal text-muted">{label}</div>
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
