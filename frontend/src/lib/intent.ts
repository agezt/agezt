// humanizeIntent turns a run's raw intent into a one-line, human title for
// list rows. Raw intents are frequently NOT what the operator typed:
//
//   - Chat runs carry the whole rolling transcript ("User: … Assistant: …
//     User: …") — the newest user message is the title.
//   - Analyst/composed prompts embed the real ask after an "== QUESTION =="
//     marker, below a system preamble and a live snapshot.
//   - Plain intents ("Reply with exactly: OK", schedule labels) pass through.
//
// Without this, the Runs/Activity/Replay lists open with "You are AGEZT's
// observability analyst, embedded in a running agent operating system…" —
// prompt plumbing, not an answer to "what was this run about?".
//
// Search/filtering should keep matching the RAW intent (the transcript is
// real, findable content); this is a display-only transform.

const QUESTION_MARK = "== QUESTION ==";
const MAX_TITLE = 200;

function firstLine(s: string): string {
  const line = s.trim().split(/\r?\n/, 1)[0]?.trim() ?? "";
  return line.length > MAX_TITLE ? `${line.slice(0, MAX_TITLE)}…` : line;
}

export function humanizeIntent(intent?: string): string {
  const raw = (intent || "").trim();
  if (!raw) return "";

  // Composed prompts: the text after the LAST question marker is the ask.
  const qi = raw.lastIndexOf(QUESTION_MARK);
  if (qi >= 0) {
    const q = firstLine(raw.slice(qi + QUESTION_MARK.length));
    if (q) return q;
  }

  // Conversation transcripts: take the newest "User:" segment, cut before any
  // trailing "Assistant:" continuation.
  if (/^user\s*:/i.test(raw)) {
    const parts = raw.split(/\buser\s*:\s*/gi).filter((s) => s.trim());
    let last = parts[parts.length - 1] || "";
    const ai = last.search(/\bassistant\s*:/i);
    if (ai >= 0) last = last.slice(0, ai);
    const t = firstLine(last);
    if (t) return t;
  }

  return firstLine(raw);
}
