// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { transcribeAudio } from "./transcribe";

afterEach(() => vi.restoreAllMocks());

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: status >= 200 && status < 300,
      status,
      json: async () => body,
    })) as unknown as typeof fetch,
  );
}

describe("transcribeAudio", () => {
  it("posts the clip as multipart and returns the recognised text", async () => {
    const calls: { url: string; init: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init: RequestInit) => {
        calls.push({ url, init });
        return { ok: true, status: 200, json: async () => ({ text: "hello jarvis" }) };
      }) as unknown as typeof fetch,
    );

    const blob = new Blob(["audio"], { type: "audio/webm" });
    const text = await transcribeAudio(blob, "clip.webm");
    expect(text).toBe("hello jarvis");

    // POSTed to /api/transcribe with a FormData body carrying the file.
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toContain("/api/transcribe");
    expect(calls[0].init.method).toBe("POST");
    const form = calls[0].init.body as FormData;
    expect(form).toBeInstanceOf(FormData);
    expect(form.get("file")).toBeInstanceOf(Blob);
  });

  it("surfaces the server's error message (e.g. STT not configured)", async () => {
    mockFetch(501, { error: "speech-to-text is not configured: set AGEZT_STT_API_KEY" });
    const blob = new Blob(["x"], { type: "audio/webm" });
    await expect(transcribeAudio(blob)).rejects.toThrow(/not configured/);
  });

  it("falls back to an HTTP status message when there is no JSON error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        status: 502,
        json: async () => {
          throw new Error("no body");
        },
      })) as unknown as typeof fetch,
    );
    await expect(transcribeAudio(new Blob(["x"]))).rejects.toThrow(/502/);
  });
});
