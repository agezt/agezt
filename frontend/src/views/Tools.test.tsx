// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
}));
// Avoid the SSE EventSource (not in jsdom): stub the events hook.
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { Tools } from "@/views/Tools";

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/tools") return Promise.resolve({ total: 3, errored: 0, error_rate: 0, by_tool: { shell: { calls: 3, errors: 0, avg_ms: 12 } } });
    if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
    if (path === "/api/tools_catalog")
      return Promise.resolve({ tools: [
        { name: "shell", description: "Run a shell command in the warden sandbox." },
        { name: "web_search", description: "Search the web via DuckDuckGo." },
      ], count: 2 });
    return Promise.resolve({});
  });
});

describe("Tools — available-tools catalog (M771)", () => {
  it("lists the agent's available tools with descriptions", async () => {
    render(<Tools />);
    await waitFor(() => expect(screen.getByText(/Available tools — what the agent can do \(2\)/)).toBeTruthy());
    // "shell" also appears in the usage card; web_search is catalog-only.
    expect(screen.getAllByText("shell").length).toBeGreaterThan(0);
    expect(screen.getByText("web_search")).toBeTruthy();
    expect(screen.getByText(/Run a shell command/)).toBeTruthy();
  });

  it("marks a tool 'used' when it appears in usage stats, 'idle' otherwise", async () => {
    render(<Tools />);
    // shell has 3 calls in /api/tools → "used"; web_search has none → "idle".
    await waitFor(() => expect(screen.getByText("web_search")).toBeTruthy());
    expect(screen.getByText("used")).toBeTruthy();
    expect(screen.getByText("idle")).toBeTruthy();
  });

  it("shows an empty state when no tools are registered", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/tools") return Promise.resolve({ total: 0, by_tool: {} });
      if (path === "/api/tool_log") return Promise.resolve({ invocations: [] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      return Promise.resolve({});
    });
    render(<Tools />);
    await waitFor(() => expect(screen.getByText(/Available tools — what the agent can do \(0\)/)).toBeTruthy());
    expect(screen.getByText("no tools registered")).toBeTruthy();
  });
});
