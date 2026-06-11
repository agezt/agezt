import { useEffect, useState } from "react";
import { Scale, Users, Send, Loader2, Gavel, AlertTriangle } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { useUI } from "@/components/ui/feedback";

// Council of Elders view (M839): consult the multi-model panel (kernel/runtime
// M837). It shows which models sit on the council, takes a question, convenes the
// panel (each member opines, they deliberate seeing each other, the chair
// synthesizes), and renders the opinions + the consensus. The agent reaches the
// same engine via the `council` tool; this is the operator's seat at the table.

interface Member {
  seat: string;
  model: string;
}
interface Opinion {
  seat: string;
  model: string;
  round: number;
  text: string;
  error?: string;
}
interface CouncilResult {
  question: string;
  consensus: string;
  dissent?: string;
  rounds: number;
  members: Member[];
  opinions: Opinion[];
}

export function Council() {
  const ui = useUI();
  const [members, setMembers] = useState<Member[]>([]);
  const [question, setQuestion] = useState("");
  const [rounds, setRounds] = useState(1);
  const [asking, setAsking] = useState(false);
  const [result, setResult] = useState<CouncilResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getJSON<{ members: Member[] }>("/api/council/members")
      .then((d) => setMembers(d.members ?? []))
      .catch((e) => setError((e as Error).message));
  }, []);

  async function ask() {
    const q = question.trim();
    if (!q || asking) return;
    setAsking(true);
    setResult(null);
    setError(null);
    try {
      const res = await postJSON<CouncilResult>("/api/council/ask", { question: q, rounds });
      setResult(res);
    } catch (e) {
      setError((e as Error).message);
      ui.toast((e as Error).message, "error");
    } finally {
      setAsking(false);
    }
  }

  // Group opinions by round for a readable transcript.
  const byRound = (result?.opinions ?? []).reduce<Record<number, Opinion[]>>((acc, op) => {
    (acc[op.round] ||= []).push(op);
    return acc;
  }, {});

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Scale className="size-4 text-accent" /> Council of Elders
        </h2>
        <span className="inline-flex items-center gap-1 text-xs text-muted">
          <Users className="size-3.5" />
          {members.length > 0 ? members.map((m) => m.seat).join(" · ") : "no keyed members"}
        </span>
      </div>

      {members.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {members.map((m) => (
            <Badge key={m.seat} variant="default" title={m.model}>
              {m.seat}: <span className="font-mono opacity-80">{m.model}</span>
            </Badge>
          ))}
        </div>
      )}

      <div className="flex flex-col gap-2 rounded-lg border border-border bg-card p-3">
        <textarea
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") ask();
          }}
          placeholder="Put a hard question before the council… (⌘/Ctrl+Enter to convene)"
          rows={3}
          className="w-full resize-y rounded-md border border-border bg-panel px-3 py-2 text-sm outline-none focus-visible:border-accent"
          disabled={asking}
        />
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1.5 text-[11px] text-muted">
            deliberation rounds
            <input
              type="number"
              min={0}
              max={5}
              value={rounds}
              onChange={(e) => setRounds(Math.max(0, Math.min(5, Number(e.target.value) || 0)))}
              className="w-14 rounded-md border border-border bg-panel px-2 py-1 text-xs outline-none focus-visible:border-accent"
              disabled={asking}
            />
          </label>
          <Button className="ml-auto" size="sm" onClick={ask} disabled={asking || !question.trim() || members.length === 0}>
            {asking ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
            {asking ? "Deliberating…" : "Convene"}
          </Button>
        </div>
      </div>

      {error && <ErrorText>{error}</ErrorText>}

      <div className="min-h-0 flex-1 space-y-3 overflow-auto">
        {asking && (
          <div className="flex items-center gap-2 rounded-lg border border-dashed border-border bg-panel/40 px-3 py-6 text-sm text-muted">
            <Loader2 className="size-4 animate-spin text-accent" />
            The council is deliberating across {members.length} model{members.length === 1 ? "" : "s"} — this can take a moment.
          </div>
        )}

        {result && (
          <>
            {/* Consensus first — the verdict. */}
            <div className="rounded-lg border border-accent/40 bg-accent/5 p-4">
              <div className="mb-1.5 flex items-center gap-2 text-sm font-semibold text-accent">
                <Gavel className="size-4" /> Consensus
              </div>
              <Markdown source={result.consensus} className="text-sm text-foreground/90" />
              {result.dissent && (
                <div className="mt-3 rounded-md border border-warn/40 bg-warn/10 p-2.5">
                  <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-warn">
                    <AlertTriangle className="size-3" /> Dissent
                  </div>
                  <Markdown source={result.dissent} className="text-xs text-foreground/80" />
                </div>
              )}
            </div>

            {/* The deliberation transcript. */}
            {Object.keys(byRound)
              .map(Number)
              .sort((a, b) => a - b)
              .map((round) => (
                <div key={round}>
                  <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted">
                    {round === 0 ? "Opening positions" : `Deliberation round ${round}`}
                  </div>
                  <div className="grid gap-2 md:grid-cols-2">
                    {byRound[round].map((op, i) => (
                      <div key={i} className="rounded-lg border border-border bg-card p-3">
                        <div className="mb-1 flex items-center gap-2">
                          <span className="text-xs font-semibold text-foreground/80">{op.seat}</span>
                          <Badge variant="default" className="font-mono text-[10px]">{op.model}</Badge>
                        </div>
                        {op.error ? (
                          <span className="text-xs text-bad">error: {op.error}</span>
                        ) : (
                          <Markdown source={op.text} className="text-xs text-foreground/85" />
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              ))}
          </>
        )}

        {!result && !asking && members.length === 0 && !error && (
          <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-muted">
            No keyed providers — add an API key for at least one provider and the council will convene.
          </div>
        )}
      </div>
    </div>
  );
}
