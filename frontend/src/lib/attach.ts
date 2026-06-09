// Attachments let you hand the agent an existing system entity — a skill, a
// stored memory, or a past run — as context for your next message. There's no
// special backend channel: the referenced content is folded into a preamble in
// front of the intent, so the run sees it without losing the daemon's own system
// prompt. These helpers are pure (mappers + the preamble builder) and unit-tested.

export type RefKind = "skill" | "memory" | "run";

export interface AttachRef {
  kind: RefKind;
  id: string;
  label: string; // short, shown on the chip
  content: string; // the body injected as context
}

const MAX_CONTENT = 4000; // keep a single attachment from blowing the context

function clip(s: string): string {
  const t = (s || "").trim();
  return t.length > MAX_CONTENT ? t.slice(0, MAX_CONTENT) + "…(truncated)" : t;
}

// --- mappers from the existing list APIs into AttachRefs ------------------

export function skillToRef(s: { id?: string; name?: string; description?: string; body?: string }): AttachRef | null {
  if (!s.id) return null;
  const body = s.body || s.description || "";
  return { kind: "skill", id: s.id, label: s.name || s.id, content: clip(`${s.name || s.id}\n${body}`) };
}

export function memoryToRef(m: {
  id?: string;
  subject?: string;
  content?: string;
  type?: string;
}): AttachRef | null {
  if (!m.id) return null;
  return {
    kind: "memory",
    id: m.id,
    label: m.subject || m.type || "memory",
    content: clip(`${m.subject ? m.subject + ": " : ""}${m.content || ""}`),
  };
}

export function runToRef(r: {
  correlation_id?: string;
  intent?: string;
  status?: string;
  answer?: string;
}): AttachRef | null {
  const id = r.correlation_id;
  if (!id) return null;
  const label = (r.intent || id).slice(0, 48);
  const parts = [r.intent && `intent: ${r.intent}`, r.status && `status: ${r.status}`, r.answer && `answer: ${r.answer}`].filter(
    Boolean,
  );
  return { kind: "run", id, label, content: clip(parts.join("\n")) };
}

// buildContext renders the selected refs into a single preamble string. Empty
// when nothing is attached (so an unattached send is byte-for-byte unchanged).
export function buildContext(refs: AttachRef[]): string {
  if (refs.length === 0) return "";
  const blocks = refs.map((r) => `### Attached ${r.kind}: ${r.label}\n${r.content}`);
  return `The user attached the following context — use it to answer.\n\n${blocks.join("\n\n")}`;
}

// withContext prepends the preamble to the intent for the actual run, leaving the
// intent unchanged when nothing is attached. The clean intent is what's shown as
// the user's message; this is only what the daemon receives.
export function withContext(intent: string, refs: AttachRef[]): string {
  const ctx = buildContext(refs);
  return ctx ? `${ctx}\n\n---\n\n${intent}` : intent;
}
