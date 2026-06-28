import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Bot,
  Cpu,
  Radio,
  Terminal,
  X,
  ChevronDown,
  ChevronUp,
  Clock,
  Coins,
  ArrowRight,
  CheckCircle2,
  XCircle,
  Loader2,
  Bug,
  type LucideIcon,
} from "lucide-react";
import { useEvents, type AgentEvent } from "@/lib/events";
import { cn, fmtTime } from "@/lib/utils";
import { money } from "@/lib/format";

// ───────────────────────── Inspector types ─────────────────────────

interface LLMCall {
  id: string;
  tsMs: number;
  model?: string;
  provider?: string;
  inputTokens?: number;
  outputTokens?: number;
  cachedTokens?: number;
  durationMs?: number;
  status: "pending" | "streaming" | "done" | "error";
  error?: string;
  correlationId?: string;
}

interface ToolCall {
  id: string;
  tsMs: number;
  tool: string;
  args?: string;
  result?: string;
  durationMs?: number;
  allow?: boolean;
  error?: boolean;
  correlationId?: string;
}

type TabId = "llm" | "tools" | "events";

const TAB_META: { id: TabId; label: string; icon: LucideIcon }[] = [
  { id: "llm", label: "LLM Calls", icon: Cpu },
  { id: "tools", label: "Tool Calls", icon: Terminal },
  { id: "events", label: "Event Log", icon: Radio },
];

// ───────────────────────── Component ─────────────────────────

export function Inspector({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { events, subscribe } = useEvents();
  const [tab, setTab] = useState<TabId>("llm");
  const [llmCalls, setLlmCalls] = useState<LLMCall[]>([]);
  const [toolCalls, setToolCalls] = useState<ToolCall[]>([]);
  const [eventLog, setEventLog] = useState<AgentEvent[]>([]);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [badge, setBadge] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  // Track last-seen timestamp for new-event badge
  const lastSeenRef = useRef(Date.now());

  // Subscribe to live events
  useEffect(() => {
    return subscribe((ev: AgentEvent) => {
      const kind = ev.kind || "";

      // LLM calls
      if (kind === "llm.request") {
        const p = ev.payload || {};
        setLlmCalls((prev) => [
          {
            id: ev.id || `llm-${Date.now()}-${Math.random()}`,
            tsMs: ev.ts_unix_ms || Date.now(),
            model: p.model || ev.subject,
            provider: p.provider,
            status: "pending",
            correlationId: ev.correlation_id,
          },
          ...prev.slice(0, 199),
        ]);
        if (open) setBadge((b) => b + 1);
      }
      if (kind === "llm.response") {
        const p = ev.payload || {};
        const id = ev.id || "";
        const prefix = id ? `llm-${id}` : "";
        setLlmCalls((prev) =>
          prev.map((c) =>
            (prefix && c.id.startsWith(prefix)) || (!prefix && c.status === "pending")
              ? {
                  ...c,
                  status: p.error ? "error" : "done",
                  inputTokens: p.input_tokens || p.inputTokens || c.inputTokens,
                  outputTokens: p.output_tokens || p.outputTokens || c.outputTokens,
                  cachedTokens: p.cached_tokens || p.cachedTokens || c.cachedTokens,
                  durationMs: p.duration_ms || p.durationMs || c.durationMs,
                  error: p.error,
                }
              : c,
          ),
        );
      }
      if (kind === "llm.token") {
        setLlmCalls((prev) =>
          prev.length > 0
            ? [{ ...prev[0], status: "streaming" as const }, ...prev.slice(1)]
            : prev,
        );
      }

      // Tool calls
      if (kind === "tool.invoked" || kind === "tool.result") {
        const p = ev.payload || {};
        setToolCalls((prev) => [
          {
            id: ev.id || `tool-${Date.now()}-${Math.random()}`,
            tsMs: ev.ts_unix_ms || Date.now(),
            tool: ev.subject || p.tool || "?",
            args: kind === "tool.invoked" ? JSON.stringify(p.args || p.input, null, 2) : undefined,
            result: kind === "tool.result" ? JSON.stringify(p.output || p.result, null, 2).slice(0, 500) : undefined,
            durationMs: p.duration_ms || p.durationMs,
            allow: p.allow,
            error: p.error,
            correlationId: ev.correlation_id,
          },
          ...prev.slice(0, 199),
        ]);
        if (open) setBadge((b) => b + 1);
      }

      // Raw event log — keep last 100
      setEventLog((prev) => [ev, ...prev].slice(0, 100));
    });
  }, [subscribe, open]);

  // Reset badge when viewing
  useEffect(() => {
    if (open) setBadge(0);
  }, [open, tab]);

  const activeCount = useMemo(
    () => llmCalls.filter((c) => c.status === "pending" || c.status === "streaming").length,
    [llmCalls],
  );

  if (!open) return null;

  return (
    <div className="flex max-h-[45vh] shrink-0 flex-col overflow-hidden rounded-t-xl border border-border/60 bg-background/95 backdrop-blur-sm shadow-2xl">
      {/* Header bar */}
      <div className="flex items-center gap-1.5 border-b border-border/40 px-2 py-1">
        {/* Tabs */}
        {TAB_META.map((t) => {
          const count =
            t.id === "llm"
              ? llmCalls.length
              : t.id === "tools"
                ? toolCalls.length
                : eventLog.length;
          return (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={cn(
                "flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium transition-colors",
                tab === t.id
                  ? "bg-accent/15 text-accent"
                  : "text-muted hover:bg-panel hover:text-foreground",
              )}
            >
              <t.icon className="size-3.5" />
              {t.label}
              <span className="rounded bg-panel/60 px-1 font-mono text-[10px] tabular-nums">
                {count}
              </span>
            </button>
          );
        })}

        {/* Active indicator */}
        {activeCount > 0 && (
          <span className="ml-1 inline-flex items-center gap-1.5 rounded-full bg-accent/10 px-2 py-0.5 text-[10px] text-accent">
            <span className="think-dot"><span /><span /><span /></span>
            {activeCount} active
          </span>
        )}

        {/* Clear */}
        <button
          onClick={() => {
            setLlmCalls([]);
            setToolCalls([]);
            setEventLog([]);
          }}
          className="ml-auto rounded px-1.5 py-0.5 text-[10px] text-muted hover:text-foreground"
          title="Clear all logs"
        >
          Clear
        </button>

        {/* Close */}
        <button
          onClick={onClose}
          className="rounded-md p-1 text-muted hover:bg-panel hover:text-foreground"
          title="Close inspector (Ctrl+Shift+I)"
        >
          <ChevronDown className="size-3.5" />
        </button>
      </div>

      {/* Body */}
      <div ref={listRef} className="min-h-0 flex-1 overflow-auto font-mono text-[11px] leading-relaxed">
        {tab === "llm" && (
          <div className="divide-y divide-border/20">
            {llmCalls.length === 0 ? (
              <EmptyInspector icon={Cpu} text="No LLM calls yet — start a chat or run a task." />
            ) : (
              llmCalls.slice(0, 200).map((c) => (
                <LLMEntry key={c.id} call={c} expanded={expandedId === c.id} onToggle={() => setExpandedId(expandedId === c.id ? null : c.id)} />
              ))
            )}
          </div>
        )}

        {tab === "tools" && (
          <div className="divide-y divide-border/20">
            {toolCalls.length === 0 ? (
              <EmptyInspector icon={Terminal} text="No tool calls observed yet." />
            ) : (
              toolCalls.slice(0, 200).map((c) => (
                <ToolEntry key={c.id} call={c} expanded={expandedId === c.id} onToggle={() => setExpandedId(expandedId === c.id ? null : c.id)} />
              ))
            )}
          </div>
        )}

        {tab === "events" && (
          <div className="divide-y divide-border/20">
            {eventLog.length === 0 ? (
              <EmptyInspector icon={Radio} text="Waiting for events…" />
            ) : (
              eventLog.slice(0, 200).map((e, i) => (
                <EventEntry key={e.id || i} ev={e} />
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ───────────────────────── Sub-entries ─────────────────────────

function LLMEntry({ call, expanded, onToggle }: { call: LLMCall; expanded: boolean; onToggle: () => void }) {
  const StatusIcon = call.status === "done" ? CheckCircle2 : call.status === "error" ? XCircle : Loader2;
  const statusColor = call.status === "done" ? "text-good" : call.status === "error" ? "text-bad" : "text-accent";
  return (
    <div>
      <button onClick={onToggle} className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left hover:bg-panel/50">
        <StatusIcon className={cn("size-3 shrink-0", statusColor, call.status === "pending" && "animate-spin")} />
        <span className="w-14 shrink-0 text-muted">{fmtTime(call.tsMs)}</span>
        <span className="min-w-0 flex-1 truncate text-foreground/90">{call.model || "—"}</span>
        {call.provider && <span className="hidden shrink-0 text-muted sm:inline">{call.provider}</span>}
        {call.inputTokens != null && (
          <span className="shrink-0 text-muted">↑{call.inputTokens}</span>
        )}
        {call.outputTokens != null && (
          <span className="shrink-0 text-muted">↓{call.outputTokens}</span>
        )}
        {call.durationMs != null && (
          <span className="shrink-0 text-muted">{call.durationMs > 1000 ? `${(call.durationMs / 1000).toFixed(1)}s` : `${call.durationMs}ms`}</span>
        )}
        {expanded ? <ChevronUp className="size-3 text-muted" /> : <ChevronDown className="size-3 text-muted" />}
      </button>
      {expanded && (
        <div className="space-y-1 border-t border-border/10 bg-panel/20 px-2.5 py-2">
          {call.correlationId && <DetailRow label="corr" value={call.correlationId} />}
          {call.provider && <DetailRow label="provider" value={call.provider} />}
          {call.model && <DetailRow label="model" value={call.model} />}
          {call.inputTokens != null && <DetailRow label="input tokens" value={String(call.inputTokens)} />}
          {call.outputTokens != null && <DetailRow label="output tokens" value={String(call.outputTokens)} />}
          {call.cachedTokens != null && call.cachedTokens > 0 && <DetailRow label="cached" value={String(call.cachedTokens)} />}
          {call.durationMs != null && <DetailRow label="duration" value={`${call.durationMs}ms`} />}
          {call.error && <DetailRow label="error" value={call.error} tone="bad" />}
        </div>
      )}
    </div>
  );
}

function ToolEntry({ call, expanded, onToggle }: { call: ToolCall; expanded: boolean; onToggle: () => void }) {
  const statusColor = call.error ? "text-bad" : call.allow === false ? "text-warn" : "text-foreground/90";
  return (
    <div>
      <button onClick={onToggle} className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left hover:bg-panel/50">
        <Terminal className={cn("size-3 shrink-0", statusColor)} />
        <span className="w-14 shrink-0 text-muted">{fmtTime(call.tsMs)}</span>
        <span className="min-w-0 flex-1 truncate font-medium text-foreground/90">{call.tool}</span>
        {call.durationMs != null && (
          <span className="shrink-0 text-muted">{call.durationMs}ms</span>
        )}
        {call.allow === false && <span className="shrink-0 text-xs text-warn">denied</span>}
        {call.error && <span className="shrink-0 text-xs text-bad">error</span>}
        {expanded ? <ChevronUp className="size-3 text-muted" /> : <ChevronDown className="size-3 text-muted" />}
      </button>
      {expanded && (
        <div className="space-y-1 border-t border-border/10 bg-panel/20 px-2.5 py-2">
          {call.correlationId && <DetailRow label="corr" value={call.correlationId} />}
          {call.args && (
            <div>
              <div className="text-[10px] text-muted">Args</div>
              <pre className="mt-0.5 overflow-auto rounded bg-panel/50 p-1.5 text-[10px] text-foreground/80">{call.args}</pre>
            </div>
          )}
          {call.result && (
            <div>
              <div className="text-[10px] text-muted">Result</div>
              <pre className="mt-0.5 overflow-auto rounded bg-panel/50 p-1.5 text-[10px] text-foreground/80">{call.result}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function EventEntry({ ev }: { ev: AgentEvent }) {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button onClick={() => setOpen(!open)} className="flex w-full items-center gap-2 px-2.5 py-1 text-left hover:bg-panel/50">
        <span className="w-14 shrink-0 text-muted">{ev.ts_unix_ms ? fmtTime(ev.ts_unix_ms) : "—"}</span>
        <span className="w-28 shrink-0 truncate text-accent">{ev.kind || "—"}</span>
        <span className="min-w-0 flex-1 truncate text-muted">{ev.subject || ev.correlation_id || ""}</span>
      </button>
      {open && (
        <div className="border-t border-border/10 bg-panel/20 px-2.5 py-2">
          <pre className="overflow-auto text-[10px] text-foreground/70">{JSON.stringify(ev, null, 2)}</pre>
        </div>
      )}
    </div>
  );
}

function DetailRow({ label, value, tone }: { label: string; value: string; tone?: "good" | "bad" | "warn" }) {
  return (
    <div className="flex gap-2 text-[10px]">
      <span className="w-20 shrink-0 text-muted">{label}</span>
      <span className={cn("min-w-0 flex-1 break-all", tone === "bad" && "text-bad", tone === "warn" && "text-warn")}>{value}</span>
    </div>
  );
}

function EmptyInspector({ icon: Icon, text }: { icon: LucideIcon; text: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-12 text-center">
      <Icon className="size-6 text-muted/40" />
      <p className="text-xs text-muted/60">{text}</p>
    </div>
  );
}

// InspectorClosedBar — a 1-line footer bar shown when the inspector is closed.
// Shows live LLM call count and a "bugs" icon. Click to open the full panel.
export function InspectorClosedBar({
  onOpen,
  activeLlmCount,
}: {
  onOpen: () => void;
  activeLlmCount: number;
}) {
  return (
    <button
      onClick={onOpen}
      title="Open debug inspector (Ctrl+Shift+I)"
      className="flex shrink-0 items-center gap-2 bg-panel/80 px-3 py-0.5 text-[11px] text-muted/70 backdrop-blur-sm transition-colors hover:bg-panel hover:text-muted"
    >
      <Bug className="size-3" />
      <span>LLM calls</span>
      {activeLlmCount > 0 ? (
        <span className="inline-flex items-center gap-1 rounded bg-accent/10 px-1 font-medium text-accent">
          <span className="think-dot"><span /><span /><span /></span>
          {activeLlmCount} active
        </span>
      ) : (
        <span className="text-muted/50">idle</span>
      )}
      <span className="ml-auto text-[10px] text-muted/40">Ctrl+Shift+I</span>
    </button>
  );
}
