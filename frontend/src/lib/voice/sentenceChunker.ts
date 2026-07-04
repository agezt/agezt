// sentenceChunker turns a STREAMING text answer into speakable sentences as they
// complete, so Voice Mode can start talking before the whole reply has arrived
// (low-latency, "streaming speech"). You feed it deltas as they stream in; it
// returns any sentences that just finalized and holds the trailing fragment until
// its terminator arrives. Code fences (```…```) are skipped entirely — reading a
// code block aloud is noise, not speech.
//
// It is deliberately dependency-free and pure-ish (all state lives in the closure)
// so the boundary logic is unit-testable without audio.

// Abbreviations whose trailing "." must NOT end a sentence. Lower-cased; matched
// against the last whitespace-delimited word before the period.
const ABBREVIATIONS = new Set([
  "mr",
  "mrs",
  "ms",
  "dr",
  "prof",
  "sr",
  "jr",
  "st",
  "vs",
  "etc",
  "e.g",
  "i.e",
  "fig",
  "no",
  "vol",
  "approx",
]);

// endsWithAbbreviation reports whether `buf` (ending in a period) ends on a known
// abbreviation rather than a real sentence stop, so "e.g." doesn't get spoken as
// two fragments.
function endsWithAbbreviation(buf: string): boolean {
  // Last run of letters/dots immediately before the final period.
  const m = buf.match(/([A-Za-z.]+)\.$/);
  if (!m) return false;
  const word = m[1].toLowerCase().replace(/\.$/, "");
  if (ABBREVIATIONS.has(word)) return true;
  // A single capital letter ("J.") is an initial, not a stop.
  if (/^[a-z]$/i.test(word)) return true;
  return false;
}

export interface SpeechChunker {
  // push consumes a streamed text delta and returns sentences that just became
  // complete (already trimmed; never empty strings).
  push(delta: string): string[];
  // flush returns whatever speakable text remains (the trailing fragment) and
  // clears the buffer. Call once the run finishes. Returns null when nothing is
  // pending.
  flush(): string | null;
}

// createSpeechChunker builds a stateful chunker. minLen avoids emitting trivially
// short fragments ("Ok.") as their own utterance when more is still streaming —
// they're held and merged with the next sentence instead.
export function createSpeechChunker(minLen = 0): SpeechChunker {
  let buf = ""; // speakable text accumulated since the last emit
  let inFence = false; // inside a ``` code block (dropped from speech)
  let fenceMarker = ""; // partial backtick run being matched at a chunk boundary

  function feed(text: string, out: string[]) {
    for (let i = 0; i < text.length; i++) {
      const ch = text[i];
      // Track ``` fences. Count consecutive backticks; three toggles the fence.
      if (ch === "`") {
        fenceMarker += "`";
        if (fenceMarker.length === 3) {
          inFence = !inFence;
          fenceMarker = "";
        }
        continue;
      }
      // A non-backtick ends any partial backtick run. Lone/double backticks are
      // inline code punctuation — outside a fence we just drop them (don't speak).
      fenceMarker = "";
      if (inFence) continue;

      buf += ch;
      if ((ch === "." || ch === "!" || ch === "?") && !endsWithAbbreviation(buf)) {
        // Peek: a digit right after a period is a decimal ("3.14"), not a stop.
        const next = text[i + 1];
        if (ch === "." && next && /\d/.test(next)) continue;
        const sentence = buf.trim();
        if (sentence.length > minLen) {
          out.push(sentence);
          buf = "";
        }
      }
    }
  }

  return {
    push(delta: string): string[] {
      const out: string[] = [];
      if (delta) feed(delta, out);
      return out;
    },
    flush(): string | null {
      const rest = buf.trim();
      buf = "";
      fenceMarker = "";
      // An unterminated fence at end-of-stream: whatever leaked into buf before it
      // is still speakable; the fence content itself was already dropped.
      return rest.length > 0 ? rest : null;
    },
  };
}
