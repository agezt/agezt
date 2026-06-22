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

  it("adds token query params for EventSource-style URLs", async () => {
    const { withToken } = await import("@/lib/api");
    expect(withToken("/events", { tail: "1" })).toBe("/events?tail=1&token=tok-123");
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
