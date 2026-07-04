import { useEffect, useRef, useState } from "react";
import { buildContext, type AttachRef } from "@/lib/attach";
import { stopSpeaking } from "@/lib/speech";

interface UseComposerParams {
  busy: boolean;
  enqueue: (text: string) => void;
  send: (intent: string, context?: string) => void;
  setPinned: (value: boolean) => void;
}

export function useComposer({ busy, enqueue, send, setPinned }: UseComposerParams) {
  const [input, setInput] = useState("");
  // Attachments staged for the next message: existing skills/memories/runs to
  // hand the agent as context. Cleared once the message is sent.
  const [attached, setAttached] = useState<AttachRef[]>([]);
  const [attachOpen, setAttachOpen] = useState(false);
  const taRef = useRef<HTMLTextAreaElement>(null);

  // Grow the composer with its content (up to max-h-40 = 160px), then scroll.
  useEffect(() => {
    const el = taRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 160) + "px";
  }, [input]);

  // Submit the composer: pin to bottom, clear the box, hand the intent to the
  // engine with any attached context (skills/memories/runs) prepended. While a
  // run is in flight, Enter instead QUEUES the message (M962) — it auto-sends
  // when the current run finishes; Steer/BTW are the explicit interrupt buttons.
  function doSend() {
    const t = input.trim();
    if (!t) return;
    if (busy) {
      enqueue(t);
      setInput("");
      return;
    }
    stopSpeaking(); // a new turn interrupts any answer being read aloud
    setPinned(true);
    setInput("");
    const ctx = buildContext(attached);
    setAttached([]);
    send(t, ctx);
  }

  function addAttachment(ref: AttachRef) {
    setAttached((prev) => (prev.some((r) => r.id === ref.id) ? prev : [...prev, ref]));
  }

  function removeAttachment(id: string) {
    setAttached((prev) => prev.filter((r) => r.id !== id));
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends; Shift+Enter inserts a newline (ChatGPT/Claude convention).
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      doSend();
    }
  }

  return {
    input,
    setInput,
    attached,
    attachOpen,
    setAttachOpen,
    taRef,
    doSend,
    addAttachment,
    removeAttachment,
    onKeyDown,
  };
}
