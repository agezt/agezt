import { useRef, useState } from "react";
import { Sparkles, Send, Loader2, Brain } from "lucide-react";
import { getJSON } from "@/lib/api";
import { money } from "@/lib/format";
import { Markdown } from "@/components/Markdown";
import { ErrorText } from "@/components/JsonView";
import { streamRun, foldChatFrame, newTurn, type ChatTurn } from "@/lib/chat";
import { Page } from "@/components/ui/page";

const SUGGESTED = [
  "Summarize the system's health right now.",
  "Which tools are failing, and what might explain it?",
  "What is driving the spend, and where could we save?",
  "Are there anomalies or risks in the recent runs?",
];

// gatherSnapshot pulls a compact, current picture of the daemon from the
// read-only stat endpoints, formatted as plain text for the analyst prompt.
async function gatherSnapshot(): Promise<string> {
  const [stats, tools, cache, runs] = await Promise.allSettled([
    getJSON<any>("/api/stats"),
    getJSON<any>("/api/tools"),
    getJSON<any>("/api/cache"),
    // Only the newest handful of runs feed the prompt — a bounded fetch keeps
    // the snapshot cheap on daemons with a long run history.
    getJSON<any>("/api/runs", { limit: "50" }),
  ]);
  const lines: string[] = [];
  if (stats.status === "fulfilled") {
    const s = stats.value;
    lines.push(
      `RUNS: total=${s.total ?? 0} completed=${s.completed ?? 0} failed=${s.failed ?? 0} running=${s.running ?? 0} success_rate=${Math.round((s.success_rate ?? 0) * 100)}% avg_iters=${(s.avg_iters ?? 0).toFixed?.(1) ?? s.avg_iters} spend=${money(s.spent_microcents)} delegations=${s.delegations ?? 0}`,
    );
    if (s.by_model) {
      for (const [m, v] of Object.entries<any>(s.by_model)) {
        lines.push(`  model ${m}: runs=${v.runs ?? 0} spend=${money(v.spent_microcents ?? 0)}`);
      }
    }
  }
  if (tools.status === "fulfilled") {
    const t = tools.value;
    lines.push(`TOOLS: calls=${t.total ?? 0} errored=${t.errored ?? 0} error_rate=${Math.round((t.error_rate ?? 0) * 100)}%`);
    for (const [name, v] of Object.entries<any>(t.by_tool || {})) {
      lines.push(`  ${name}: calls=${v.calls ?? 0} errors=${v.errors ?? 0} avg_ms=${v.avg_ms ?? "-"}`);
    }
  }
  if (cache.status === "fulfilled") {
    const c = cache.value;
    lines.push(`CACHE: saved=${money(c.saved_microcents)} cached_tokens=${c.cached_input_tokens ?? 0} priced_calls=${c.calls ?? 0}`);
  }
  if (runs.status === "fulfilled") {
    const rs = (runs.value.runs || []).slice(0, 10);
    lines.push("RECENT RUNS:");
    for (const r of rs) {
      lines.push(
        `  [${r.status}] ${String(r.intent || r.correlation_id || "").slice(0, 70)} · ${money(r.spent_mc)} · ${r.iters ?? 0} iters${r.reason ? ` · ${r.reason}` : ""}`,
      );
    }
  }
  return lines.join("\n");
}

// Analyst is the AI observability assistant: it gathers a live system snapshot
// and asks the daemon's own model to analyse it and answer the operator's
// question — your agentic OS reasoning about itself. The reply streams with the
// model's reasoning, rendered as Markdown.
export function Analyst() {
  const [q, setQ] = useState("");
  const [turn, setTurn] = useState<ChatTurn | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [reasoningOpen, setReasoningOpen] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  async function ask(question: string) {
    const text = question.trim();
    if (!text || busy) return;
    setBusy(true);
    setErr(null);
    setTurn(newTurn());
    try {
      const snapshot = await gatherSnapshot();
      const intent =
        "You are AGEZT's observability analyst, embedded in a running agent operating system. " +
        "Using ONLY the live system snapshot below, answer the operator's question concisely and concretely — cite the actual numbers, call out anything notable, and give a short recommendation if relevant. " +
        "Do NOT call any tools; reason purely from the snapshot.\n\n" +
        "== SYSTEM SNAPSHOT ==\n" +
        snapshot +
        "\n\n== QUESTION ==\n" +
        text;
      const ac = new AbortController();
      abortRef.current = ac;
      await streamRun({ intent }, (f) => setTurn((prev) => foldChatFrame(prev ?? newTurn(), f)), ac.signal);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
      abortRef.current = null;
    }
  }

  const answer = turn?.answer || turn?.streamedText || "";

  return (
    <Page icon={Sparkles} title="Analyst" description="ask the daemon about itself" width="readable">
      {/* Ask box */}
      <div className="flex items-center gap-2">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && ask(q)}
          placeholder="Ask about the running system…"
          disabled={busy}
          className="h-9 flex-1 rounded-md border border-border bg-panel px-3 text-sm outline-none focus:border-accent disabled:opacity-50"
        />
        <button
          onClick={() => ask(q)}
          disabled={busy || q.trim() === ""}
          className="inline-flex h-9 items-center gap-1.5 rounded-md border border-accent px-3 text-sm text-accent transition-colors hover:bg-accent hover:text-white disabled:opacity-50"
        >
          {busy ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />} Ask
        </button>
      </div>

      {/* Suggested questions */}
      {!turn && (
        <div className="flex flex-wrap gap-1.5">
          {SUGGESTED.map((s) => (
            <button
              key={s}
              onClick={() => {
                setQ(s);
                ask(s);
              }}
              className="rounded-full border border-border px-2.5 py-1 text-xs text-muted transition-colors hover:border-accent hover:text-foreground"
            >
              {s}
            </button>
          ))}
        </div>
      )}

      {err && <ErrorText>{err}</ErrorText>}

      {/* Answer */}
      {turn && (
        <div className="glass rounded-xl p-4">
          {turn.reasoning && (
            <div className="mb-3">
              <button
                onClick={() => setReasoningOpen((v) => !v)}
                className="inline-flex items-center gap-1.5 text-xs text-muted hover:text-foreground"
              >
                <Brain className="size-3.5" /> {turn.status === "streaming" && !answer ? "Thinking…" : "Reasoning"}
              </button>
              {reasoningOpen && (
                <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-panel p-2 text-[11px] text-muted">
                  {turn.reasoning}
                </pre>
              )}
            </div>
          )}
          {answer ? (
            <Markdown source={answer} />
          ) : (
            <div className="flex items-center gap-2 text-sm text-muted">
              <Loader2 className="size-4 animate-spin" /> analysing the system…
            </div>
          )}
          {turn.status === "done" && (
            <div className="mt-3 border-t border-border/60 pt-2 text-xs text-muted">
              {turn.model} · {turn.iters} iter{turn.iters === 1 ? "" : "s"} · {money(turn.costMicrocents)}
            </div>
          )}
        </div>
      )}
    </Page>
  );
}
