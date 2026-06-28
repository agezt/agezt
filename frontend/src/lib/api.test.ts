// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

describe("api helpers", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
    window.history.replaceState(null, "", "/?token=tok-123");
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("eventsURLAsync returns /events URL with ephemeral SSE token", async () => {
    const { eventsURLAsync } = await import("@/lib/api");
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({ token: "sse-secret" }))) as typeof fetch;
    const url = await eventsURLAsync();
    expect(url).toContain("/events?st=");
    expect(url).not.toContain("token="); // main token never in URL
    expect(url).toContain("sse-secret");
  });

  it("adds Authorization headers for fetch calls", async () => {
    const { authHeaders } = await import("@/lib/api");
    expect(authHeaders().get("Authorization")).toBe("Bearer tok-123");
  });

  it("getJSON throws daemon error payloads", async () => {
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({ error: "bad thing" }), { status: 500 })) as typeof fetch;
    const { getJSON } = await import("@/lib/api");
    await expect(getJSON("/api/x")).rejects.toThrow("bad thing");
  });
});
