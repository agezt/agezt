import { withToken } from "@/lib/api";

// transcribeAudio uploads a recorded clip to the webui /api/transcribe route and
// returns the recognised text. The daemon hands the audio to its configured STT
// backend (the same one `agt transcribe` and the OpenAI-API surface use). Throws
// with the server's message on failure — notably a clear "not configured" when no
// STT endpoint is set, so the caller can tell the user voice isn't set up.
export async function transcribeAudio(blob: Blob, filename = "clip.webm"): Promise<string> {
  const form = new FormData();
  form.append("file", blob, filename);
  // No explicit Content-Type: the browser sets multipart/form-data with the
  // boundary. Auth rides the ?token= query param like every other webui call.
  const res = await fetch(withToken("/api/transcribe"), { method: "POST", body: form });
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = String(j.error);
    } catch {
      /* no JSON body */
    }
    throw new Error(msg);
  }
  const j = await res.json();
  return String(j?.text ?? "");
}
