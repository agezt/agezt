import { useEffect, useRef, useState } from "react";
import { turnText } from "@/lib/chat";
import { type Msg } from "@/lib/conversations";
import { speak, stopSpeaking } from "@/lib/speech";

const AUTOSPEAK_KEY = "agezt.chat.autospeak";

interface UseVoiceParams {
  busy: boolean;
  messages: Msg[];
}

export function useVoice({ busy, messages }: UseVoiceParams) {
  // autoSpeak: read each completed answer aloud (browser TTS). Persisted, off by
  // default so the UI is silent unless the user opts in.
  const [autoSpeak, setAutoSpeak] = useState(() => {
    try {
      return localStorage.getItem(AUTOSPEAK_KEY) === "1";
    } catch {
      return false;
    }
  });
  const prevBusy = useRef(busy);
  const lastSpokeRef = useRef("");

  function toggleAutoSpeak() {
    setAutoSpeak((v) => {
      const next = !v;
      try {
        localStorage.setItem(AUTOSPEAK_KEY, next ? "1" : "0");
      } catch {
        /* ignore storage errors */
      }
      if (!next) stopSpeaking();
      return next;
    });
  }

  // Auto-speak the latest answer when a run finishes (busy true→false). Keyed on
  // the busy transition — not on `messages` content — so navigating, reloading,
  // or switching conversations never reads an old answer aloud, and each answer
  // is spoken at most once. Stop any speech when the component unmounts.
  useEffect(() => {
    if (prevBusy.current && !busy && autoSpeak) {
      const last = messages[messages.length - 1];
      if (last?.role === "assistant" && last.turn.status === "done") {
        // Speak each answer at most once: key on the run id (or the text, before
        // a correlation lands) so a follow-up state update can't read it twice.
        const key = last.turn.correlationId || turnText(last.turn);
        if (key && key !== lastSpokeRef.current) {
          lastSpokeRef.current = key;
          speak(turnText(last.turn));
        }
      }
    }
    prevBusy.current = busy;
  }, [busy, autoSpeak, messages]);

  useEffect(() => () => stopSpeaking(), []);

  return {
    autoSpeak,
    toggleAutoSpeak,
  };
}
