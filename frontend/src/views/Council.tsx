import { useEffect, useState, type ReactNode } from "react";
import { Scale, Users, Send, Loader2, Gavel, AlertTriangle, Pencil, Plus, X, Check, Globe, type LucideIcon } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { Markdown } from "@/components/Markdown";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import { ModelPicker } from "@/components/ModelPicker";
import { useCouncilStore, startCouncilRun, applyCouncilResult, genCouncilCorr } from "@/lib/councilStore";
import {
  type CouncilRun,
  seatStatus,
  currentRound,
  opinionsByRound,
  roundLabel,
  progressLabel,
  lastOpinionFor,
} from "@/lib/council";

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
  as_of?: string;
  brief?: string;
}

export function Council() {
  const ui = useUI();
  const [members, setMembers] = useState<Member[]>([]);
  const [question, setQuestion] = useState("");
  const [rounds, setRounds] = useState(1);
  const [error, setError] = useState<string | null>(null);
  // The run this view follows: the one it started, else the store's active run so
  // returning to the page mid-deliberation resumes watching it (M987).
  const [corr, setCorr] = useState<string | null>(null);
  const { runs, activeCorr } = useCouncilStore();
  const shownCorr = corr ?? activeCorr;
  const run = shownCorr ? runs[shownCorr] ?? null : null;
  const asking = !!run && !run.done;

  // Edit mode state (M839: per-member model picker).
  const [editMode, setEditMode] = useState(false);
  const [draftMembers, setDraftMembers] = useState<Member[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    loadMembers();
  }, []);

  function loadMembers() {
    getJSON<{ members: Member[] }>("/api/council/members")
      .then((d) => {
        setMembers(d.members ?? []);
        setDraftMembers(d.members ?? []);
      })
      .catch((e) => setError((e as Error).message));
  }

  function enterEdit() {
    setDraftMembers([...members]);
    setEditMode(true);
  }

  function cancelEdit() {
    setDraftMembers([...members]);
    setEditMode(false);
  }

  async function saveMembers() {
    if (saving) return;
    setSaving(true);
    try {
      const res = await postJSON<{ saved: boolean; unknown_models?: string[] }>("/api/council/set", {
        members: draftMembers.map((m) => ({ seat: m.seat, model: m.model })),
      });
      await loadMembers();
      setEditMode(false);
      if (res.unknown_models?.length) {
        ui.toast(`Saved — unknown models: ${res.unknown_models.join(", ")}`, "info");
      } else {
        ui.toast("Council membership updated", "success");
      }
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  function addMember() {
    setDraftMembers((prev) => [
      ...prev,
      { seat: `Elder ${prev.length + 1}`, model: "" },
    ]);
  }

  function removeMember(index: number) {
    setDraftMembers((prev) => prev.filter((_, i) => i !== index));
  }

  function updateMemberModel(index: number, model: string) {
    setDraftMembers((prev) =>
      prev.map((m, i) => (i === index ? { ...m, model } : m))
    );
  }

  function updateMemberSeat(index: number, seat: string) {
    setDraftMembers((prev) =>
      prev.map((m, i) => (i === index ? { ...m, seat } : m))
    );
  }

  async function ask() {
    const q = question.trim();
    if (!q || asking) return;
    const newCorr = genCouncilCorr();
    setCorr(newCorr);
    setError(null);
    // Seed the run immediately so the live view appears at once, then convene.
    startCouncilRun(newCorr, q, members.map((m) => ({ seat: m.seat, model: m.model })), rounds);
    try {
      const res = await postJSON<CouncilResult>("/api/council/ask", { question: q, rounds, corr: newCorr });
      applyCouncilResult(newCorr, { ...res, asOf: res.as_of, brief: res.brief });
    } catch (e) {
      // The deliberation streams over events independently of this POST, so if the
      // request times out but the verdict still arrived, the error is moot — only
      // surface it when no consensus landed.
      setError((e as Error).message);
      ui.toast((e as Error).message, "error");
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {/* Header */}
      <PageHeader
        icon={Scale}
        title="Council of Elders"
        description={
          <span className="inline-flex items-center gap-1">
            <Users className="size-3.5" />
            {members.length > 0 ? members.map((m) => m.seat).join(" · ") : "no keyed members"}
          </span>
        }
        actions={
          !editMode ? (
            <Button variant="ghost" size="sm" className="h-6 gap-1 px-2 text-xs" onClick={enterEdit}>
              <Pencil className="size-3" /> Edit
            </Button>
          ) : (
            <span className="text-xs text-muted">Editing</span>
          )
        }
      />

      {/* Member badges (view mode) */}
      {!editMode && members.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {members.map((m) => (
            <Badge key={m.seat} variant="default" title={m.model}>
              {m.seat}: <span className="font-mono opacity-80">{m.model}</span>
            </Badge>
          ))}
        </div>
      )}

      {/* Edit mode: per-member model picker */}
      {editMode && (
        <CouncilModal title="Council seats" icon={Users} onClose={cancelEdit}>
          <div className="flex flex-col gap-2">
            {draftMembers.map((m, i) => (
              <div key={i} className="flex items-center gap-2">
                <input
                  type="text"
                  value={m.seat}
                  onChange={(e) => updateMemberSeat(i, e.target.value)}
                  placeholder="Seat name"
                  className="h-7 w-28 rounded border border-border bg-panel px-2 text-xs outline-none focus-visible:border-accent"
                />
                <ModelPicker
                  value={m.model}
                  onChange={(id) => updateMemberModel(i, id)}
                />
                <button
                  type="button"
                  onClick={() => removeMember(i)}
                  className="flex h-7 w-7 shrink-0 items-center justify-center rounded text-muted hover:text-bad"
                  title="Remove member"
                >
                  <X className="size-3" />
                </button>
              </div>
            ))}
          </div>
          <div className="mt-3 flex items-center gap-2">
            <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={addMember}>
              <Plus className="size-3" /> Add seat
            </Button>
            <span className="ml-auto text-xs text-muted">{draftMembers.length} seats</span>
          </div>
          <div className="mt-3 flex items-center gap-2 border-t border-border/70 pt-3">
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto h-7 gap-1 px-2 text-xs"
              onClick={cancelEdit}
              disabled={saving}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              className="h-7 gap-1 px-2 text-xs"
              onClick={saveMembers}
              disabled={saving || draftMembers.some((m) => !m.model)}
            >
              {saving ? <Loader2 className="size-3 animate-spin" /> : <Check className="size-3" />}
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
          {draftMembers.some((m) => !m.model) && (
            <p className="text-xs text-muted">Each seat must have a model selected.</p>
          )}
        </CouncilModal>
      )}

      <div className="glass flex flex-col gap-2 rounded-xl p-3">
        <textarea
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") ask();
          }}
          placeholder="Put a hard question before the council… (⌘/Ctrl+Enter to convene)"
          aria-label="Council question"
          rows={3}
          className="w-full resize-y rounded-md border border-border bg-panel px-3 py-2 text-sm outline-none focus-visible:border-accent"
          disabled={asking}
        />
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 rounded-lg border border-border bg-panel p-1" aria-label="Deliberation rounds">
            {[0, 1, 2, 3, 4, 5].map((n) => (
              <button
                key={n}
                type="button"
                onClick={() => setRounds(n)}
                disabled={asking}
                className={cn(
                  "h-6 min-w-7 rounded-md px-2 text-xs font-medium transition-colors",
                  rounds === n ? "bg-accent text-accent-foreground" : "text-muted hover:bg-card hover:text-foreground",
                )}
                aria-pressed={rounds === n}
                title={`${n} deliberation round${n === 1 ? "" : "s"}`}
              >
                {n}
              </button>
            ))}
          </div>
          <span className="text-[11px] text-muted">deliberation rounds</span>
          <Button className="ml-auto" size="sm" onClick={ask} disabled={asking || !question.trim() || members.length === 0}>
            {asking ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}
            {asking ? "Deliberating…" : "Convene"}
          </Button>
        </div>
      </div>

      {error && (!run || !run.done) && <ErrorText>{error}</ErrorText>}

      <div className="min-h-0 flex-1 space-y-3 overflow-auto">
        {run && <CouncilLive run={run} />}

        {!run && members.length === 0 && !error && (
          <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-muted">
            No keyed providers — add an API key for at least one provider and the council will convene.
          </div>
        )}
      </div>
    </div>
  );
}

function CouncilModal({
  title,
  icon: Icon,
  children,
  onClose,
}: {
  title: string;
  icon: LucideIcon;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Icon className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Pick the named models that deliberate as a panel.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close council modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}

// CouncilLive renders a deliberation as it unfolds (M987): a live strip of seat
// nodes (each waiting → thinking → done), the verdict once it lands, and the full
// round-by-round path of who said what. Folded purely from the council.* event
// stream, so it shows the same whether watched live or reopened mid-run.
function CouncilLive({ run }: { run: CouncilRun }) {
  const round = currentRound(run);
  const groups = opinionsByRound(run);
  const thinkingNow = Object.values(run.thinking);

  return (
    <div className="space-y-3">
      {/* Phase strip: where the deliberation is right now. */}
      <div className="glass flex flex-wrap items-center gap-2 rounded-xl px-3 py-2 text-sm">
        {run.done ? (
          <Gavel className="size-4 text-accent" />
        ) : (
          <Loader2 className="size-4 animate-spin text-accent" />
        )}
        <span className={cn("font-medium", run.done ? "text-foreground" : "text-foreground/90")}>{progressLabel(run)}</span>
        {run.rounds > 0 && (
          <Badge variant="default" className="ml-auto text-xs">
            round {Math.min(round, run.rounds)} / {run.rounds}
          </Badge>
        )}
      </div>

      {/* Research brief: the dated web evidence every seat was grounded in. */}
      {run.brief && (
        <details className="view-enter glass rounded-xl px-3 py-2 text-sm">
          <summary className="flex cursor-pointer items-center gap-2 font-medium text-foreground/90">
            <Globe className="size-4 text-accent" /> Research brief
            {run.asOf && <Badge variant="default" className="text-[10px]">web · {run.asOf}</Badge>}
          </summary>
          <pre className="mt-2 whitespace-pre-wrap break-words font-sans text-xs leading-relaxed text-foreground/80">{run.brief}</pre>
        </details>
      )}

      {/* Seat nodes — the table. Each lights up as it takes its turn. */}
      {run.seats.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {run.seats.map((s, i) => (
            <SeatNode key={s.seat + i} run={run} seat={s.seat} model={s.model} hue={250 + i * 47} />
          ))}
        </div>
      )}

      {/* Verdict first, once reached. */}
      {run.consensus && (
        <div className="view-enter rounded-lg border border-accent/40 bg-accent/5 p-4">
          <div className="mb-1.5 flex items-center gap-2 text-sm font-semibold text-accent">
            <Gavel className="size-4" /> Consensus
          </div>
          <Markdown source={run.consensus} className="text-sm text-foreground/90" />
          {run.dissent && (
            <div className="mt-3 rounded-md border border-warn/40 bg-warn/10 p-2.5">
              <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-normal text-warn">
                <AlertTriangle className="size-3" /> Dissent
              </div>
              <Markdown source={run.dissent} className="text-xs text-foreground/80" />
            </div>
          )}
        </div>
      )}

      {/* The path to the verdict: each round's opinions as they land, with live
          "deliberating…" placeholders for members still mid-turn this round. */}
      {groups.map((g) => {
        const stillThinking = thinkingNow.filter((t) => t.round === g.round);
        return (
          <div key={g.round}>
            <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-normal text-muted">{roundLabel(g.round)}</div>
            <div className="grid gap-2 md:grid-cols-2">
              {g.opinions.map((op, i) => (
                <div key={op.seat + i} className="view-enter glass rounded-xl p-3">
                  <div className="mb-1 flex items-center gap-2">
                    <span className="text-xs font-semibold text-foreground/80">{op.seat}</span>
                    <Badge variant="default" className="font-mono text-xs">{op.model}</Badge>
                  </div>
                  {op.error ? (
                    <span className="text-xs text-bad">error: {op.error}</span>
                  ) : (
                    <Markdown source={op.text} className="text-xs text-foreground/85" />
                  )}
                </div>
              ))}
              {stillThinking.map((t) => (
                <div key={"think-" + t.seat} className="glass flex items-center gap-2 rounded-xl p-3 text-xs text-muted">
                  <Loader2 className="size-3.5 animate-spin text-accent" />
                  <span className="font-semibold text-foreground/70">{t.seat}</span> is deliberating…
                </div>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

// SeatNode is one advisor at the table: a coloured monogram whose ring reflects
// its live status — waiting (dim), thinking (pulsing accent), done (green check),
// or error (red).
function SeatNode({ run, seat, model, hue }: { run: CouncilRun; seat: string; model: string; hue: number }) {
  const status = seatStatus(run, seat);
  const op = lastOpinionFor(run, seat);
  const initials = seat
    .split(/\s+/)
    .map((w) => w[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
  const ring =
    status === "thinking"
      ? "border-accent animate-pulse"
      : status === "done"
        ? "border-good"
        : status === "error"
          ? "border-bad"
          : "border-border opacity-60";
  return (
    <div
      className={cn("glass flex items-center gap-2 rounded-xl px-2.5 py-1.5", status === "thinking" && "shadow-e2")}
      title={op?.error ? `error: ${op.error}` : model}
    >
      <span
        className={cn("relative flex size-7 shrink-0 items-center justify-center rounded-full border-2 text-xs font-bold", ring)}
        style={{ background: `oklch(0.6 0.14 ${hue} / 0.18)`, color: `oklch(0.7 0.15 ${hue})` }}
      >
        {status === "thinking" ? <Loader2 className="size-3.5 animate-spin" /> : status === "error" ? <X className="size-3.5 text-bad" /> : status === "done" ? <Check className="size-3.5 text-good" /> : initials}
      </span>
      <span className="flex min-w-0 flex-col leading-tight">
        <span className="truncate text-xs font-semibold text-foreground/85">{seat}</span>
        <span className="truncate font-mono text-xs text-muted">{model}</span>
      </span>
    </div>
  );
}
