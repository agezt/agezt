// speech.ts wraps the browser SpeechSynthesis API to read the agent's answers
// aloud — voice OUTPUT, the other half of the chat voice loop (you speak via the
// mic, it speaks back). Runs entirely in the browser: no backend, no config, no
// network. Every call is a no-op when the browser lacks speech synthesis, so
// callers don't need to guard.

export function speechSupported(): boolean {
  return typeof window !== "undefined" && "speechSynthesis" in window && typeof SpeechSynthesisUtterance !== "undefined";
}

// speak reads text aloud, cancelling anything already in flight first (so a new
// answer interrupts an old one rather than queueing behind it). onEnd fires when
// speech finishes or is cancelled, letting a button reflect the speaking state.
export function speak(text: string, onEnd?: () => void): void {
  if (!speechSupported()) {
    onEnd?.();
    return;
  }
  const t = text.trim();
  if (!t) {
    onEnd?.();
    return;
  }
  window.speechSynthesis.cancel();
  const u = new SpeechSynthesisUtterance(t);
  if (onEnd) {
    u.onend = () => onEnd();
    u.onerror = () => onEnd();
  }
  window.speechSynthesis.speak(u);
}

// stopSpeaking cancels any in-flight speech.
export function stopSpeaking(): void {
  if (speechSupported()) window.speechSynthesis.cancel();
}
