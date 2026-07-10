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

  it("postJSON forwards AbortSignal for cancellable ACP actions", async () => {
    const controller = new AbortController();
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ ok: true }))) as unknown as typeof fetch;
    globalThis.fetch = fetchMock;
    const { postJSON } = await import("@/lib/api");
    await postJSON("/api/acp/delegate", { task: "work" }, { signal: controller.signal });
    expect(fetchMock).toHaveBeenCalledWith("/api/acp/delegate", expect.objectContaining({ signal: controller.signal }));
  });

  // Regression: when the kernel takes too long on /api/agents (large journal +
  // 11× journal.Range walks were tripping the 10-min control-plane read
  // deadline), the SPA must surface that as a fast client-side abort so the
  // roster view can keep its previous render instead of blocking forever
  // (and stacking up aborted fetches every 8s poll).
  it("getJSON times out via timeoutMs and surfaces a typed error", async () => {
    let abortObserved = false;
    globalThis.fetch = vi.fn((_url: unknown, init: { signal?: AbortSignal }) => {
      // Mirror real fetch behavior: hang until the supplied AbortSignal fires,
      // then reject with an AbortError. Real fetch does the same — it owns the
      // signal lifetime and rejects the request promise when the signal aborts.
      const sig = init?.signal;
      return new Promise<Response>((_resolve, reject) => {
        if (!sig) return;
        if (sig.aborted) {
          abortObserved = true;
          reject(new DOMException("aborted", "AbortError"));
          return;
        }
        sig.addEventListener("abort", () => {
          abortObserved = true;
          reject(new DOMException("aborted", "AbortError"));
        });
      }) as unknown as ReturnType<typeof fetch>;
    }) as unknown as typeof fetch;
    const { getJSON, HTTPError } = await import("@/lib/api");
    let caught: unknown;
    try {
      await getJSON<{ ok: true }>("/api/agents", undefined, { timeoutMs: 50 });
    } catch (e) {
      caught = e;
    }
    expect(caught).toBeInstanceOf(HTTPError);
    expect((caught as InstanceType<typeof HTTPError>).status).toBe(0);
    expect((caught as Error).message).toMatch(/aborted/i);
    expect(abortObserved).toBe(true);
  });

  it("getJSON honours caller-supplied AbortSignal", async () => {
    const controller = new AbortController();
    let sawAbort = false;
    globalThis.fetch = vi.fn((_url: unknown, init: { signal?: AbortSignal }) => {
      const sig = init?.signal;
      return new Promise<Response>((_resolve, reject) => {
        if (!sig) return;
        if (sig.aborted) {
          sawAbort = true;
          reject(new DOMException("aborted", "AbortError"));
          return;
        }
        sig.addEventListener("abort", () => {
          sawAbort = true;
          reject(new DOMException("aborted", "AbortError"));
        });
      }) as unknown as ReturnType<typeof fetch>;
    }) as unknown as typeof fetch;
    const { getJSON, HTTPError } = await import("@/lib/api");
    const p = getJSON<{ ok: true }>("/api/agents", undefined, { signal: controller.signal });
    // Cancel before the kernel responds.
    setTimeout(() => controller.abort(), 5);
    let caught: unknown;
    try { await p; } catch (e) { caught = e; }
    expect(caught).toBeInstanceOf(HTTPError);
    expect(sawAbort).toBe(true);
  });
});
