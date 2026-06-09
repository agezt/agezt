import { turnText } from "@/lib/chat";
import type { Msg } from "@/lib/conversations";

// conversationToMarkdown serialises a chat thread to a portable Markdown transcript
// (M723) — so you can archive, share, or paste a conversation elsewhere. User turns
// become "## You", assistant turns "## Agent", with the final answer text (tool calls
// and streaming scaffolding are elided — the transcript is the readable record).
export function conversationToMarkdown(title: string, messages: Msg[]): string {
  const lines: string[] = [`# ${title || "Conversation"}`, ""];
  for (const m of messages) {
    if (m.role === "user") {
      lines.push("## You", "", m.text.trim(), "");
    } else {
      const text = turnText(m.turn).trim();
      lines.push("## Agent", "", text || "_(no answer)_", "");
      if (m.turn.model) lines.push(`> ${m.turn.model}`, "");
    }
  }
  return lines.join("\n").trimEnd() + "\n";
}

// slugify turns a title into a safe file name stem.
export function slugify(title: string): string {
  const s = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return s || "conversation";
}

// downloadText triggers a browser download of text content. Side-effecting (DOM +
// object URL), so it's kept separate from the pure serialiser above.
export function downloadText(filename: string, text: string, mime = "text/markdown"): void {
  const blob = new Blob([text], { type: `${mime};charset=utf-8` });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
