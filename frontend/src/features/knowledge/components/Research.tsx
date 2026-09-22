// Research.tsx — Day 26 bring-back.
//
// "Research" is one of three Thinking Partners under Knowledge. It calls
// /api/research/ask (the daemon's multi-source RAG planner) and renders the
// answer with its sub-questions, sources, and verified claims laid out for
// review. The original was a single full-page Chat-with-tools surface; this
// rebuild keeps the same affordances (sub-question list, source pills,
// claim verification toggles, transcript) but is now a focused, single-shot
// query screen rather than a conversation.
import { useState } from "react";
import { BookOpen, ChevronDown, ChevronRight, FileText, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { postJSON } from "@/app/api";
import { cn, fmtTime } from "@/app/utils/utils";
import { useUI } from "@/components/ui/feedback";

interface ResearchResult {
  answer?: string;
  sub_questions?: { id?: string; question?: string; answer?: string }[];
  sources?: { id?: string; title?: string; url?: string; snippet?: string; confidence?: number }[];
  verified_claims?: { claim?: string; verified?: boolean; sources?: string[] }[];
  transcript?: string[];
  duration_ms?: number;
  [k: string]: unknown;
}

const DEFAULT_RESULT: ResearchResult = {};

export function Research() {
  const ui = useUI();
  const [question, setQuestion] = useState("");
  const [maxSub, setMaxSub] = useState(3);
  const [maxSources, setMaxSources] = useState(5);
  const [verify, setVerify] = useState(true);
  const [maxClaims, setMaxClaims] = useState(5);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ResearchResult>(DEFAULT_RESULT);
  const [history, setHistory] = useState<{ q: string; at: number; result: ResearchResult }[]>([]);

  async function ask() {
    const q = question.trim();
    if (!q) return;
    setBusy(true);
    try {
      const r = await postJSON<ResearchResult>("/api/research/ask", {
        question: q,
        max_sub_questions: maxSub,
        max_sources: maxSources,
        verify,
        max_verify_claims: maxClaims,
      });
      setResult(r || DEFAULT_RESULT);
      setHistory((h) => [{ q, at: Date.now(), result: r || DEFAULT_RESULT }, ...h].slice(0, 8));
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex h-full gap-4 p-4">
      <aside className="hidden w-64 shrink-0 flex-col gap-3 lg:flex">
        <div className="rounded-lg border border-border bg-card p-3">
          <h3 className="text-sm font-semibold text-fg">Recent questions</h3>
          {history.length === 0 ? (
            <p className="mt-2 text-xs text-muted">
              Once you ask, the last eight questions land here for quick replay.
            </p>
          ) : (
            <ul className="mt-2 space-y-1.5">
              {history.map((h, i) => (
                <li key={i}>
                  <button
                    type="button"
                    onClick={() => {
                      setQuestion(h.q);
                      setResult(h.result);
                    }}
                    className="line-clamp-2 w-full rounded-md bg-bg px-2 py-1.5 text-left text-xs text-fg/90 hover:bg-accent/10"
                  >
                    <span className="block truncate">{h.q}</span>
                    <span className="mt-0.5 block text-[10px] text-muted">{fmtTime(h.at)}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col gap-4">
        <header className="rounded-lg border border-border bg-card p-4">
          <div className="flex items-center gap-2">
            <Sparkles className="size-5 text-accent" />
            <h2 className="text-lg font-semibold text-fg">Research</h2>
            <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-accent">
              Thinking partner
            </span>
          </div>
          <p className="mt-1 text-sm text-muted">
            Ask a question. The daemon drafts sub-questions, gathers sources, and verifies its
            claims before answering.
          </p>
          <textarea
            className="mt-3 h-28 w-full rounded-md border border-border bg-bg p-2 text-sm text-fg placeholder:text-muted focus:border-accent focus:outline-none"
            placeholder="e.g. What's the safest way to migrate the agent roster to the new key store?"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
          />
          <div className="mt-3 grid gap-3 sm:grid-cols-4">
            <NumberField label="Sub-questions" value={maxSub} onChange={setMaxSub} min={1} max={8} />
            <NumberField label="Sources" value={maxSources} onChange={setMaxSources} min={1} max={20} />
            <NumberField
              label="Claims to verify"
              value={maxClaims}
              onChange={setMaxClaims}
              min={1}
              max={20}
            />
            <label className="flex items-center gap-2 self-end rounded-md border border-border bg-bg px-3 py-2 text-sm text-fg">
              <input
                type="checkbox"
                checked={verify}
                onChange={(e) => setVerify(e.target.checked)}
                className="size-4 accent-accent"
              />
              Verify claims
            </label>
          </div>
          <div className="mt-3 flex items-center justify-end gap-2">
            <Button onClick={ask} disabled={busy || !question.trim()}>
              {busy ? "Researching…" : "Ask Research"}
            </Button>
          </div>
        </header>

        <Result result={result} busy={busy} />
      </div>
    </div>
  );
}

function NumberField({
  label,
  value,
  onChange,
  min,
  max,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
  min: number;
  max: number;
}) {
  return (
    <label className="flex flex-col gap-1 text-xs font-medium text-muted">
      <span>{label}</span>
      <input
        type="number"
        value={value}
        min={min}
        max={max}
        onChange={(e) => {
          const n = Number(e.target.value);
          if (!Number.isFinite(n)) return;
          onChange(Math.max(min, Math.min(max, n)));
        }}
        className="w-full rounded-md border border-border bg-bg px-2 py-1.5 text-sm text-fg focus:border-accent focus:outline-none"
      />
    </label>
  );
}

function Result({ result, busy }: { result: ResearchResult; busy: boolean }) {
  if (busy) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-dashed border-border bg-card">
        <p className="text-sm text-muted">Composing answer…</p>
      </div>
    );
  }
  const subQuestions = result.sub_questions || [];
  const sources = result.sources || [];
  const claims = result.verified_claims || [];

  if (!result.answer && subQuestions.length === 0 && sources.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-dashed border-border bg-card">
        <p className="text-sm text-muted">
          Ask a question to see sub-questions, sources, and verified claims.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 overflow-auto pb-8">
      {result.answer && (
        <section className="rounded-lg border border-border bg-card p-4">
          <h3 className="text-sm font-semibold text-fg">Answer</h3>
          <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-fg/90">
            {result.answer}
          </p>
          {typeof result.duration_ms === "number" && (
            <p className="mt-2 text-[10px] uppercase tracking-wide text-muted">
              Composed in {(result.duration_ms / 1000).toFixed(1)}s
            </p>
          )}
        </section>
      )}

      {subQuestions.length > 0 && (
        <Section icon={BookOpen} title="Sub-questions">
          <ul className="flex flex-col gap-2">
            {subQuestions.map((sq, i) => (
              <li
                key={sq.id || i}
                className="rounded-md border border-border bg-bg/60 px-3 py-2 text-sm text-fg"
              >
                <p className="font-medium">{sq.question || `Sub-question ${i + 1}`}</p>
                {sq.answer && (
                  <p className="mt-1 text-xs leading-relaxed text-fg/80">{sq.answer}</p>
                )}
              </li>
            ))}
          </ul>
        </Section>
      )}

      {sources.length > 0 && (
        <Section icon={FileText} title={`Sources (${sources.length})`}>
          <ul className="grid gap-2 sm:grid-cols-2">
            {sources.map((s, i) => (
              <li
                key={s.id || i}
                className={cn(
                  "rounded-md border border-border bg-bg/60 px-3 py-2 text-xs",
                  "hover:border-accent/60",
                )}
              >
                <p className="line-clamp-2 font-medium text-fg">{s.title || s.url || `Source ${i + 1}`}</p>
                {s.url && (
                  <p className="mt-1 truncate text-[10px] text-muted">{s.url}</p>
                )}
                {s.snippet && (
                  <p className="mt-1 line-clamp-3 text-fg/80">{s.snippet}</p>
                )}
                {typeof s.confidence === "number" && (
                  <p className="mt-1 text-[10px] uppercase tracking-wide text-muted">
                    confidence {(s.confidence * 100).toFixed(0)}%
                  </p>
                )}
              </li>
            ))}
          </ul>
        </Section>
      )}

      {claims.length > 0 && (
        <Section icon={Sparkles} title={`Verified claims (${claims.length})`}>
          <ul className="flex flex-col gap-1.5">
            {claims.map((c, i) => (
              <li
                key={i}
                className={cn(
                  "flex items-start gap-2 rounded-md border px-3 py-2 text-xs",
                  c.verified
                    ? "border-success/30 bg-success/5 text-fg"
                    : "border-warning/30 bg-warning/5 text-fg",
                )}
              >
                <ChevronRow verified={c.verified} />
                <span className="flex-1">{c.claim}</span>
                {c.sources && c.sources.length > 0 && (
                  <span className="text-[10px] uppercase tracking-wide text-muted">
                    {c.sources.length} source{c.sources.length === 1 ? "" : "s"}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </Section>
      )}

      {result.transcript && result.transcript.length > 0 && <TranscriptView lines={result.transcript} />}
    </div>
  );
}

function Section({
  icon: Icon,
  title,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-fg">
        <Icon className="size-4 text-accent" />
        {title}
      </h3>
      <div className="mt-2">{children}</div>
    </section>
  );
}

function ChevronRow({ verified }: { verified?: boolean }) {
  return verified ? (
    <ChevronRight className="mt-0.5 size-3.5 shrink-0 text-success" />
  ) : (
    <ChevronDown className="mt-0.5 size-3.5 shrink-0 text-warning" />
  );
}

function TranscriptView({ lines }: { lines: string[] }) {
  const [open, setOpen] = useState(false);
  return (
    <section className="rounded-lg border border-border bg-card p-3">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 text-left text-xs font-medium text-muted hover:text-fg"
      >
        {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
        Transcript ({lines.length})
      </button>
      {open && (
        <pre className="mt-2 max-h-48 overflow-auto rounded-md bg-bg/60 p-2 text-[10px] leading-relaxed text-fg/80">
          {lines.join("\n")}
        </pre>
      )}
    </section>
  );
}
