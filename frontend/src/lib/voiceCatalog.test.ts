import { describe, expect, it } from "vitest";
import {
  STT_PROVIDERS,
  TTS_PROVIDERS,
  dialectOf,
  selectProvider,
  voicesFor,
  type SpeechProvider,
} from "@/lib/voiceCatalog";

describe("voice provider catalog helpers", () => {
  it("matches compatible endpoints across case, slash, and v1 variants", () => {
    expect(selectProvider(STT_PROVIDERS, " HTTPS://API.GROQ.COM/openai/v1/ ", "openai")?.id).toBe("groq");
    expect(selectProvider(STT_PROVIDERS, "https://unknown.example/v1", "openai")).toBeUndefined();
    expect(selectProvider(STT_PROVIDERS, " ", "openai")).toBeUndefined();
    expect(selectProvider(STT_PROVIDERS)).toBeUndefined();
  });

  it("selects native dialects and defaults OpenAI-compatible dialects", () => {
    expect(selectProvider(TTS_PROVIDERS, undefined, "ELEVENLABS")?.id).toBe("elevenlabs");
    expect(selectProvider(TTS_PROVIDERS, undefined, "unknown")).toBeUndefined();
    expect(dialectOf(TTS_PROVIDERS.find((p) => p.id === "cartesia")!)).toBe("cartesia");
    expect(dialectOf(STT_PROVIDERS.find((p) => p.id === "openai")!)).toBe("openai");
  });

  it("returns model-specific, default, empty, and absent voice lists", () => {
    const groq = TTS_PROVIDERS.find((p) => p.id === "groq");
    expect(voicesFor(groq, "canopylabs/orpheus-v1-english")).toHaveLength(6);
    expect(voicesFor(groq)).toEqual([]);
    expect(voicesFor(undefined)).toEqual([]);
    expect(voicesFor({ voices: undefined } as SpeechProvider, "missing")).toEqual([]);
  });
});
