import { useMemo, useState } from "react";
import { ArrowUpRight, Share2, Zap, Mail, ArrowRight, Send, CheckCheck, CornerDownRight, LifeBuoy, Megaphone } from "lucide-react";
import { postJSON } from "@/lib/api";
import { cn, fmtAgo, clip } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { useUI } from "@/components/ui/feedback";
import { openIncident } from "@/lib/incidentnav";
import { type AgentProfile } from "@/views/Roster";
import { type ApiOrder } from "@/lib/fleet";
import { escalationCausalityLineage, mailboxWakeFor, type MailboxWakeRef, type EscalationCausalityLineage, type AgentEscalation } from "@/lib/agentdetail";
import { AgentWakeAccess, BoardMessage, Stat } from "@/components/agentdetail/shared";
import { agentManagedSubagent } from "@/components/agentdetail/capability";

export function agentMailboxSubjects(slug: string): {
  kind: "dm" | "help" | "broadcast";
  label: string;
  subject: string;
}[] {
  const s = slug.trim();
  if (!s) return [];
  return [
    { kind: "dm", label: "DM", subject: `board.dm.${s}` },
    { kind: "help", label: "Help", subject: `board.help.${s}` },
    { kind: "broadcast", label: "Broadcast", subject: "board.broadcast" },
  ];
}

export function mailboxSubjectBinding(
  orders: ApiOrder[],
  subject: string,
): ApiOrder | undefined {
  return orders.find((o) =>
    (o.triggers || []).some((t) => t.type === "event" && t.subject === subject),
  );
}


export function mailboxWakeArmIssue(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">,
  access?: Pick<AgentWakeAccess, "channel_allowed" | "manager" | "reason">,
): string {
  if (profile.retired) return "revive this agent before arming mailbox wake";
  if (profile.enabled === false) return "resume this agent before arming mailbox wake";
  if (access && access.channel_allowed === false) {
    const owner = access.manager || profile.parent_agent || profile.owner_agent;
    return owner
      ? `channel wake blocked; arm mailbox wake on ${owner}`
      : access.reason || "channel wake is blocked for this agent";
  }
  if (agentManagedSubagent(profile)) {
    const owner = profile.parent_agent || profile.owner_agent;
    return owner
      ? `managed sub-agent; arm mailbox wake on ${owner}`
      : "managed sub-agent; only its parent/owner can wake it";
  }
  return "";
}

export function operatorWakeIssue(
  profile: Pick<AgentProfile, "enabled" | "retired" | "kind" | "managed" | "direct_callable" | "parent_agent" | "owner_agent">,
): string {
  if (profile.retired) return "revive this agent before waking it";
  if (profile.enabled === false) return "resume this agent before waking it";
  if (agentManagedSubagent(profile)) {
    const owner = profile.parent_agent || profile.owner_agent;
    return owner
      ? `managed sub-agent; wake ${owner} instead`
      : "managed sub-agent; only its parent/owner can wake it";
  }
  return "";
}


// CommsTab is the agent's mailbox: the board messages it sent, was addressed, or
// received as a broadcast/ack — its communication trail with the rest of the
// fleet.
export function agentBoardMessages(
  messages: BoardMessage[],
  slug: string,
): BoardMessage[] {
  const s = slug.trim().toLowerCase();
  return messages
    .filter((m) => {
      const from = (m.from || "").trim().toLowerCase();
      const to = (m.to || "").trim().toLowerCase();
      const acked = (m.acked_by || []).some(
        (a) => a.trim().toLowerCase() === s,
      );
      return from === s || to === s || m.to === "*" || acked;
    })
    .sort((a, b) => (b.ts_unix_ms || 0) - (a.ts_unix_ms || 0));
}

export function messageAckedBy(m: BoardMessage, slug: string): boolean {
  const s = slug.trim().toLowerCase();
  return (m.acked_by || []).some((a) => a.trim().toLowerCase() === s);
}

export function messageAckedByLabel(m: Pick<BoardMessage, "acked_by">): string {
  return (m.acked_by || []).map((a) => a.trim()).filter(Boolean).join(", ");
}

export function waitingForAgent(
  messages: BoardMessage[],
  slug: string,
): BoardMessage[] {
  const s = slug.trim().toLowerCase();
  const answered = new Set(
    messages.filter((m) => m.reply_to).map((m) => m.reply_to as string),
  );
  return messages.filter((m) => {
    if (!m.id || m.reply_to || answered.has(m.id) || messageAckedBy(m, slug))
      return false;
    const from = (m.from || "").trim().toLowerCase();
    const to = (m.to || "").trim().toLowerCase();
    return to === s || (m.to === "*" && from !== s);
  });
}

export interface AgentInboxPrioritySummary {
  direct: number;
  broadcast: number;
  help: number;
  replied: number;
  stale: number;
  waiting: number;
  label: string;
  detail: string;
  tone: "good" | "warn" | "muted";
}

export function agentInboxPrioritySummary(
  messages: BoardMessage[],
  slug: string,
  nowMs = Date.now(),
): AgentInboxPrioritySummary {
  const s = slug.trim().toLowerCase();
  const waiting = waitingForAgent(messages, slug);
  const direct = waiting.filter((m) => (m.to || "").trim().toLowerCase() === s).length;
  const broadcast = waiting.filter((m) => m.to === "*").length;
  const help = waiting.filter((m) => m.help).length;
  const replied = messages.filter((m) => {
    if (!m.reply_to) return false;
    const from = (m.from || "").trim().toLowerCase();
    const to = (m.to || "").trim().toLowerCase();
    return from === s || to === s;
  }).length;
  const staleCutoff = nowMs - 24 * 60 * 60 * 1000;
  const stale = waiting.filter((m) => (m.ts_unix_ms || nowMs) < staleCutoff).length;
  const parts = [
    direct ? `${direct} direct` : "",
    broadcast ? `${broadcast} broadcast` : "",
    help ? `${help} help` : "",
    replied ? `${replied} replied` : "",
    stale ? `${stale} stale` : "",
  ].filter(Boolean);
  return {
    direct,
    broadcast,
    help,
    replied,
    stale,
    waiting: waiting.length,
    label: waiting.length > 0 ? `${waiting.length} waiting` : "inbox clear",
    detail: parts.length > 0 ? parts.join(" · ") : "no waiting direct, broadcast, help, replied, or stale messages",
    tone: stale > 0 || help > 0 ? "warn" : waiting.length > 0 ? "warn" : replied > 0 ? "muted" : "good",
  };
}


// CommsCausalityLineage renders the delegated/doctor comms causality lineage for
// one escalation message: the wake run this agent's run that the message caused
// (deep-links into the Activity tab) and the incident chain (root → parent →
// current, each deep-linkable to the Incident page). This is the delegated/doctor
// analogue of the mailbox "woke" badge — it proves the message-to-wake causal
// link and lets the operator follow it downstream.
function CommsCausalityLineage({
  slug,
  lineage,
  onFocusRun,
}: {
  slug: string;
  lineage: EscalationCausalityLineage;
  onFocusRun: (correlationId: string | undefined) => void;
}) {
  if (!lineage.origin) return null;
  const tone = lineage.origin === "delegated" ? "text-accent" : "text-muted";
  return (
    <span className="inline-flex flex-wrap items-center gap-1 rounded bg-card px-1.5 py-0.5 text-xs">
      <span
        className={cn("inline-flex items-center gap-1", tone)}
        title={`This escalation message woke ${slug}`}
      >
        <Zap className="size-3" />
        {lineage.label}
      </span>
      {lineage.wakeCorrelationId && (
        <button
          onClick={() => onFocusRun(lineage.wakeCorrelationId)}
          className="font-mono text-accent transition-colors hover:text-accent2"
          title="Open the run this escalation woke in Activity"
        >
          run {clip(lineage.wakeCorrelationId, 24)}
        </button>
      )}
      {lineage.rootIncidentId && (
        <button
          onClick={() => openIncident(lineage.rootIncidentId!)}
          className="font-mono text-muted transition-colors hover:text-accent"
          title={
            lineage.rootAgent
              ? `Open the root incident (root ${lineage.rootAgent})`
              : "Open the root incident"
          }
        >
          root {clip(lineage.rootIncidentId, 18)}
        </button>
      )}
      {lineage.parentIncidentId &&
        lineage.parentIncidentId !== lineage.rootIncidentId &&
        lineage.parentIncidentId !== lineage.incidentId && (
          <button
            onClick={() => openIncident(lineage.parentIncidentId!)}
            className="font-mono text-muted transition-colors hover:text-accent"
            title="Open the parent (delegation hop) incident"
          >
            parent {clip(lineage.parentIncidentId, 18)}
          </button>
        )}
      {lineage.incidentId && lineage.incidentId !== lineage.rootIncidentId && (
        <button
          onClick={() => openIncident(lineage.incidentId!)}
          className="font-mono text-muted transition-colors hover:text-accent"
          title="Open this agent's incident"
        >
          incident {clip(lineage.incidentId, 18)}
        </button>
      )}
      {lineage.nextOwner && (
        <span className="font-mono text-muted">→ {lineage.nextOwner}</span>
      )}
    </span>
  );
}

export function CommsTab({
  slug,
  messages,
  escalations,
  wokeMessages,
  onFocusRun,
  onManage,
  onChanged,
}: {
  slug: string;
  messages: BoardMessage[] | null;
  escalations: AgentEscalation[] | null;
  wokeMessages?: Record<string, MailboxWakeRef>;
  onFocusRun: (correlationId: string | undefined) => void;
  onManage: (view: string) => void;
  onChanged: () => void;
}) {
  const ui = useUI();
  const [topic, setTopic] = useState("dm");
  const [text, setText] = useState("");
  const [outTo, setOutTo] = useState("");
  const [outTopic, setOutTopic] = useState("dm");
  const [outText, setOutText] = useState("");
  const [replyTo, setReplyTo] = useState("");
  const [replyText, setReplyText] = useState("");
  const [busy, setBusy] = useState(false);
  const loadedMessages = messages || [];
  const waiting = waitingForAgent(loadedMessages, slug);
  const inboxPriority = agentInboxPrioritySummary(loadedMessages, slug);
  const escalationByMessage = useMemo(
    () =>
      new Map(
        (escalations || [])
          .filter((row) => row.message_id)
          .map((row) => [String(row.message_id), row] as const),
      ),
    [escalations],
  );
  if (!messages) return <SkeletonList count={3} lines={2} />;
  const sent = loadedMessages.filter((m) => m.from === slug).length;
  const received = loadedMessages.filter(
    (m) => m.to === slug || (m.to === "*" && m.from !== slug),
  ).length;
  const seen = loadedMessages.filter((m) => messageAckedBy(m, slug)).length;

  async function send() {
    const body = text.trim();
    if (!body) return;
    setBusy(true);
    try {
      await postJSON("/api/board/send", {
        from: "operator",
        to: slug,
        topic: topic.trim() || "dm",
        text: body,
      });
      setText("");
      ui.toast(`message sent to ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function sendAsAgent() {
    const body = outText.trim();
    const target = outTo.trim();
    if (!body || !target) return;
    setBusy(true);
    try {
      await postJSON("/api/board/send", {
        from: slug,
        to: target,
        topic: outTopic.trim() || "dm",
        text: body,
      });
      setOutText("");
      ui.toast(`message sent from ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function ack(id: string) {
    setBusy(true);
    try {
      await postJSON("/api/board/ack", { id, by: slug });
      ui.toast(`message acknowledged for ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function reply(id: string) {
    const body = replyText.trim();
    if (!body) return;
    setBusy(true);
    try {
      await postJSON("/api/board/send", {
        from: slug,
        reply_to: id,
        text: body,
      });
      setReplyTo("");
      setReplyText("");
      ui.toast(`reply sent as ${slug}`, "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Stat
          icon={Mail}
          label="waiting"
          value={waiting.length}
          accent={waiting.length > 0}
        />
        <Stat icon={ArrowRight} label="received" value={received} />
        <Stat icon={Send} label="sent" value={sent} />
        <Stat icon={CheckCheck} label="seen" value={seen} accent={seen > 0} />
      </div>

      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <Send className="size-3" /> message this agent
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <input
            value={topic}
            onChange={(e) => setTopic(e.target.value)}
            aria-label="Mailbox topic"
            placeholder="topic"
            className="h-8 rounded-md border border-border bg-card px-2 font-mono text-xs outline-none focus-visible:border-accent sm:w-32"
          />
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) void send();
            }}
            aria-label="Mailbox message"
            placeholder={`to ${slug}`}
            className="h-8 min-w-0 flex-1 rounded-md border border-border bg-card px-2 text-xs outline-none focus-visible:border-accent"
          />
          <Button size="sm" onClick={send} disabled={busy || !text.trim()}>
            <Send className="size-3.5" /> Send
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-accent/30 bg-accent/5 p-2.5">
        <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-accent">
          <Share2 className="size-3" /> send as this agent
        </div>
        <div className="grid gap-2 sm:grid-cols-[9rem_9rem_1fr_auto]">
          <input
            value={outTo}
            onChange={(e) => setOutTo(e.target.value)}
            aria-label="Agent outbox recipient"
            placeholder="to agent or *"
            className="h-8 rounded-md border border-border bg-card px-2 font-mono text-xs outline-none focus-visible:border-accent"
          />
          <input
            value={outTopic}
            onChange={(e) => setOutTopic(e.target.value)}
            aria-label="Agent outbox topic"
            placeholder="topic"
            className="h-8 rounded-md border border-border bg-card px-2 font-mono text-xs outline-none focus-visible:border-accent"
          />
          <input
            value={outText}
            onChange={(e) => setOutText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) void sendAsAgent();
            }}
            aria-label="Agent outbox message"
            placeholder={`from ${slug}`}
            className="h-8 min-w-0 rounded-md border border-border bg-card px-2 text-xs outline-none focus-visible:border-accent"
          />
          <Button size="sm" onClick={sendAsAgent} disabled={busy || !outTo.trim() || !outText.trim()}>
            <Send className="size-3.5" /> Send as agent
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-panel/30 p-2.5">
        <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
          <Mail className="size-3" /> inbox priority summary
          <Button className="ml-auto" variant="ghost" size="sm" onClick={() => onManage("board")}>
            Open board <ArrowUpRight className="size-3.5" />
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-[11px]">
          <Badge variant={inboxPriority.tone === "good" ? "good" : inboxPriority.tone === "warn" ? "warn" : "default"}>
            {inboxPriority.label}
          </Badge>
          <span className="min-w-0 flex-1 text-muted" title={inboxPriority.detail}>
            {inboxPriority.detail}
          </span>
        </div>
      </div>

      {messages.length === 0 ? (
        <EmptyState
          icon={Mail}
          title="No messages"
          hint={`Board posts ${slug} sent, was addressed (to: ${slug}), or received as a broadcast appear here.`}
        />
      ) : (
        <ul className="space-y-2">
          {messages.slice(0, 60).map((m, i) => {
            const outbound = m.from === slug;
            const waitingHere = waiting.some((w) => w.id === m.id);
            const escalation = m.id ? escalationByMessage.get(m.id) : undefined;
            const woke = mailboxWakeFor(wokeMessages, m.id);
            const seenBy = messageAckedByLabel(m);
            return (
              <li
                key={m.id || i}
                className="rounded-lg border border-border bg-panel/30 p-2.5"
              >
                <div className="flex flex-wrap items-center gap-2 text-[11px]">
                  <Badge
                    variant={
                      outbound ? "good" : waitingHere ? "bad" : "default"
                    }
                  >
                    {outbound
                      ? "sent"
                      : m.to === "*"
                        ? "broadcast"
                        : "received"}
                  </Badge>
                  {m.from && (
                    <span className="font-mono text-xs text-muted">
                      from {m.from}
                    </span>
                  )}
                  {m.to && (
                    <span className="inline-flex items-center gap-1 font-mono text-xs text-muted">
                      {m.to === "*" ? (
                        <Megaphone className="size-3" />
                      ) : (
                        <ArrowRight className="size-3" />
                      )}
                      {m.to}
                    </span>
                  )}
                  {m.topic && (
                    <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-accent">
                      {m.topic}
                    </span>
                  )}
                  {m.reply_to && (
                    <span className="rounded bg-card px-1.5 py-0.5 font-mono text-xs text-muted">
                      reply
                    </span>
                  )}
                  {seenBy && (
                    <span
                      className="inline-flex items-center gap-1 rounded bg-good/10 px-1.5 py-0.5 text-xs text-good"
                      title={`acknowledged by ${seenBy}`}
                    >
                      <CheckCheck className="size-3" /> seen by {seenBy}
                    </span>
                  )}
                  {m.help && (
                    <span className="inline-flex items-center gap-1 rounded bg-bad/15 px-1.5 py-0.5 text-xs text-bad">
                      <LifeBuoy className="size-3" /> help
                    </span>
                  )}
                  {woke && (
                    <span
                      className="inline-flex items-center gap-1 rounded bg-accent/15 px-1.5 py-0.5 text-xs text-accent"
                      title={`This message woke ${slug}${woke.correlation_id ? ` · run ${woke.correlation_id}` : ""}`}
                    >
                      <Zap className="size-3" /> woke {slug}
                    </span>
                  )}
                  {escalation?.origin_kind === "doctor" && (
                    <CommsCausalityLineage
                      slug={slug}
                      lineage={escalationCausalityLineage(escalation)}
                      onFocusRun={onFocusRun}
                    />
                  )}
                  {escalation?.origin_kind === "delegated" && (
                    <CommsCausalityLineage
                      slug={slug}
                      lineage={escalationCausalityLineage(escalation)}
                      onFocusRun={onFocusRun}
                    />
                  )}
                  {waitingHere && (
                    <span className="ml-auto flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || !m.id}
                        title={`Reply as ${slug}`}
                        onClick={() => {
                          setReplyTo(replyTo === m.id ? "" : m.id || "");
                          setReplyText("");
                        }}
                      >
                        <CornerDownRight className="size-3.5" /> Reply
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={busy || !m.id}
                        title={`Acknowledge for ${slug}`}
                        onClick={() => m.id && ack(m.id)}
                      >
                        <CheckCheck className="size-3.5" /> Ack
                      </Button>
                    </span>
                  )}
                  <span
                    className={cn(
                      "font-mono text-xs text-muted",
                      !waitingHere && "ml-auto",
                    )}
                  >
                    {fmtAgo(m.ts_unix_ms)}
                  </span>
                </div>
                {m.text && (
                  <div className="mt-1 whitespace-pre-wrap text-[11px] text-muted">
                    {clip(m.text, 280)}
                  </div>
                )}
                {m.id && replyTo === m.id && (
                  <div className="mt-2 flex flex-col gap-1.5 rounded-md border border-border bg-card/60 p-2 sm:flex-row">
                    <input
                      value={replyText}
                      onChange={(e) => setReplyText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && !e.shiftKey) void reply(m.id || "");
                      }}
                      aria-label={`Reply to ${m.id}`}
                      placeholder={`reply as ${slug}`}
                      className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
                    />
                    <Button size="sm" disabled={busy || !replyText.trim()} onClick={() => reply(m.id || "")}>
                      <CornerDownRight className="size-3.5" /> Send reply
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          location.hash = `board?agent=${encodeURIComponent(slug)}`;
        }}
      >
        Open Board <ArrowUpRight className="size-3.5" />
      </Button>
    </div>
  );
}

