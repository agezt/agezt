import { useRef, useState } from "react";
import { Mic, Square, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUI } from "@/components/ui/feedback";
import { transcribeAudio } from "@/lib/voice";

// MicButton records a short voice message and transcribes it into the chat
// composer (M689) — the "talk to Jarvis" affordance. It captures audio with the
// browser MediaRecorder, posts the clip to /api/transcribe, and hands the text
// back via onText. Degrades gracefully: no mic / denied permission / STT not
// configured all surface a clear toast rather than failing silently.
export function MicButton({ onText, disabled }: { onText: (text: string) => void; disabled?: boolean }) {
  const ui = useUI();
  const [recording, setRecording] = useState(false);
  const [working, setWorking] = useState(false);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const streamRef = useRef<MediaStream | null>(null);

  function cleanup() {
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;
    recorderRef.current = null;
  }

  async function finish() {
    const type = recorderRef.current?.mimeType || "audio/webm";
    const blob = new Blob(chunksRef.current, { type });
    cleanup();
    if (blob.size === 0) return;
    setWorking(true);
    try {
      const text = await transcribeAudio(blob);
      if (text.trim()) onText(text.trim());
      else ui.toast("Didn't catch that — try again", "info");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setWorking(false);
    }
  }

  async function start() {
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
      ui.toast("Voice recording isn't supported in this browser", "error");
      return;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      chunksRef.current = [];
      const rec = new MediaRecorder(stream);
      rec.ondataavailable = (e) => {
        if (e.data && e.data.size > 0) chunksRef.current.push(e.data);
      };
      rec.onstop = () => void finish();
      recorderRef.current = rec;
      rec.start();
      setRecording(true);
    } catch {
      ui.toast("Couldn't access the microphone — check the browser's permission", "error");
      cleanup();
    }
  }

  function stop() {
    recorderRef.current?.stop();
    setRecording(false);
  }

  if (working) {
    return (
      <Button variant="ghost" size="icon" disabled title="Transcribing…">
        <Loader2 className="size-4 animate-spin" />
      </Button>
    );
  }
  return recording ? (
    <Button variant="danger" size="icon" onClick={stop} title="Stop recording">
      <Square className="size-4" />
    </Button>
  ) : (
    <Button variant="ghost" size="icon" onClick={start} disabled={disabled} title="Record a voice message">
      <Mic className="size-4" />
    </Button>
  );
}
