import { useState } from "react";
import { Telescope, Send, Loader2, ChevronDown, CheckCircle2, XCircle, HelpCircle, ExternalLink } from "lucide-react";
import { postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { PageHeader } from "@/components/ui/page-header";

// Research view (M1001): the operator's seat at the deep-research harness
// (kernel/runtime). One question fans out into sub-questions, each gathers
// independent web sources (web_search + browser.read), the synthesis may only
// state claims it can cite, and each cited claim is then adversarially verified
// against its own source. The agent reaches the same engine via the `research`
// tool; here a blocking POST to /api/research/ask returns the finished report.

interface ResearchSource {
  id: string;
  url: string;
  title: string;
  rank: number;
  hash?: string;
}

interface ResearchClaim {
  text: string;
  source_ids: string[];
  verdict: string; // supported | refuted | uncertain
  note?: string;
}

interface ResearchReport {
  question: string;
  sub_questions?: string[];
  sources?: ResearchSource[];
  markdown: string;
  claims?: ResearchClaim[];
  confidence: number;
  cited_sources?: number;
  verified?: boolean;
  notes?: string[];
}

const VERDICT_META: Record<string, { label: string; tint: string; Icon: typeof CheckCircle2 }> = {
  supported: { label: "supported", tint: "text-emerald-500", Icon: CheckCircle2 },
  refuted: { label: "refuted", tint: "text-red-500", Icon: XCircle },
  uncertain: { label: "uncertain", tint: "text-amber-500", Icon: HelpCircle },
};

export function Research() {
  const [question, setQuestion] = useState("");
  const [maxSources, setMaxSources] = useState(8);
  const [maxSubQ, setMaxSubQ] = useState(3);
  const [maxVerify, setMaxVerify] = useState(6);
  const [verify, setVerify] = useState(true);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const [report, setReport] = useState<ResearchReport | null>(null);

  async function go() {
    const q = question.trim();
    if (!q || running) return;
    setRunning(true);
    setError("");
    try {
      const res = await postJSON<ResearchReport>("/api/research/ask", {
        question: q,
        max_sources: maxSources,
        max_sub_questions: maxSubQ,
        verify,
        max_verify_claims: maxVerify,
      });
      setReport(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRunning(false);
    }
  }

  const confPct = report ? Math.round((report.confidence || 0) * 100) : 0;

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        icon={Telescope}
        title="Research"
        description="Deep research: decompose a question, gather independent web sources, synthesize a cited answer, and adversarially verify every claim."
      />

      {/* Ask */}
      <Card glass className="gap-3 p-4">
        <textarea
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) go();
          }}
          placeholder="Ask a research question — e.g. “What are the tradeoffs between SQLite and Postgres for a single-node app in 2026?”"
          rows={3}
          className="w-full resize-y rounded-lg border border-border bg-panel/60 p-3 text-sm text-foreground outline-none placeholder:text-muted focus:border-accent/60"
          aria-label="Research question"
        />

        <div className="flex flex-wrap items-center gap-2">
          <Button onClick={go} disabled={running || !question.trim()} className="gap-1.5">
            {running ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
            {running ? "Researching…" : "Research"}
          </Button>
          <label className="flex cursor-pointer items-center gap-1.5 text-xs text-muted">
            <input type="checkbox" checked={verify} onChange={(e) => setVerify(e.target.checked)} />
            Adversarial verification
          </label>
          <button
            type="button"
            onClick={() => setShowAdvanced((v) => !v)}
            className="ml-auto flex items-center gap-1 text-xs text-muted hover:text-foreground"
          >
            <ChevronDown className={cn("size-3.5 transition-transform", showAdvanced && "rotate-180")} />
            Advanced
          </button>
        </div>

        {showAdvanced && (
          <div className="grid grid-cols-1 gap-3 border-t border-border/60 pt-3 sm:grid-cols-3">
            <NumField label="Max sources" value={maxSources} onChange={setMaxSources} min={1} max={20} />
            <NumField label="Max sub-questions" value={maxSubQ} onChange={setMaxSubQ} min={1} max={8} />
            <NumField label="Max verify claims" value={maxVerify} onChange={setMaxVerify} min={0} max={12} disabled={!verify} />
          </div>
        )}
      </Card>

      {error && <ErrorText>{error}</ErrorText>}

      {report && (
        <>
          {/* Verdict header */}
          <Card glass className="flex-row flex-wrap items-center gap-3 p-3">
            <span className="font-semibold text-foreground">{confPct}% confidence</span>
            {(() => {
              const refutedCount = report.claims?.filter((c) => c.verdict === "refuted").length ?? 0;
              if (report.verified && refutedCount === 0) {
                return (
                  <span className="flex items-center gap-1.5 text-sm font-medium text-emerald-500">
                    <CheckCircle2 className="size-4" /> verified
                  </span>
                );
              }
              if (report.verified && refutedCount > 0) {
                // Verification ran but refuted claim(s) — never show a green tick.
                return (
                  <span className="flex items-center gap-1.5 text-sm font-medium text-red-500">
                    <XCircle className="size-4" /> verified · {refutedCount} refuted
                  </span>
                );
              }
              return (
                <span className="flex items-center gap-1.5 text-sm text-muted">
                  <HelpCircle className="size-4" /> unverified
                </span>
              );
            })()}
            <span className="text-xs text-muted">
              {(report.sources?.length ?? 0)} source{(report.sources?.length ?? 0) === 1 ? "" : "s"}
              {report.cited_sources != null && ` · ${report.cited_sources} cited`}
            </span>
          </Card>

          {(report.notes?.length ?? 0) > 0 && (
            <Card glass className="gap-1 border-amber-500/30 p-3">
              {report.notes!.map((n, i) => (
                <p key={i} className="text-xs text-amber-500">
                  {n}
                </p>
              ))}
            </Card>
          )}

          {/* Answer */}
          {report.markdown && (
            <Card glass className="gap-2 p-4">
              <div className="text-xs font-semibold uppercase tracking-normal text-muted">Answer</div>
              <Markdown source={report.markdown} />
            </Card>
          )}

          {/* Verified claims */}
          {(report.claims?.length ?? 0) > 0 && (
            <Card glass className="gap-2 p-4">
              <div className="text-xs font-semibold uppercase tracking-normal text-muted">
                Verified claims ({report.claims!.length})
              </div>
              <div className="flex flex-col gap-2">
                {report.claims!.map((c, i) => {
                  const meta = VERDICT_META[c.verdict] || VERDICT_META.uncertain;
                  return (
                    <div key={i} className="flex items-start gap-2 rounded-lg border border-border bg-panel/40 p-2.5">
                      <meta.Icon className={cn("mt-0.5 size-4 shrink-0", meta.tint)} />
                      <div className="min-w-0">
                        <p className="text-sm text-foreground">{c.text}</p>
                        <div className="mt-1 flex flex-wrap items-center gap-1.5">
                          <Badge className={cn("font-mono uppercase", meta.tint)}>{meta.label}</Badge>
                          {c.source_ids?.map((sid) => (
                            <span key={sid} className="font-mono text-[11px] text-muted">
                              {sid}
                            </span>
                          ))}
                          {c.note && <span className="text-[11px] text-muted">— {c.note}</span>}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </Card>
          )}

          {/* Sources */}
          {(report.sources?.length ?? 0) > 0 && (
            <Card glass className="gap-2 p-4">
              <div className="text-xs font-semibold uppercase tracking-normal text-muted">
                Sources ({report.sources!.length})
              </div>
              <div className="flex flex-col gap-1.5">
                {report.sources!.map((s) => (
                  <a
                    key={s.id}
                    href={s.url}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-panel/60"
                  >
                    <span className="font-mono text-[11px] text-accent">{s.id}</span>
                    <span className="min-w-0 flex-1 truncate text-foreground">{s.title || s.url}</span>
                    <ExternalLink className="size-3.5 shrink-0 text-muted" />
                  </a>
                ))}
              </div>
            </Card>
          )}
        </>
      )}
    </div>
  );
}

function NumField({
  label,
  value,
  onChange,
  min,
  max,
  disabled,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
  min: number;
  max: number;
  disabled?: boolean;
}) {
  return (
    <label className={cn("flex flex-col gap-1 text-xs", disabled && "opacity-50")}>
      <span className="text-muted">{label}</span>
      <input
        type="number"
        value={value}
        min={min}
        max={max}
        disabled={disabled}
        onChange={(e) => {
          const n = parseInt(e.target.value, 10);
          if (!Number.isNaN(n)) onChange(Math.max(min, Math.min(max, n)));
        }}
        className="h-9 rounded-lg border border-border bg-panel/60 px-3 text-sm text-foreground outline-none focus:border-accent/60"
      />
    </label>
  );
}
