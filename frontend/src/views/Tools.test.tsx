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

import {
  Tools,
  toolSource,
  mergeToolViews,
  filterTools,
  capabilityCounts,
  type ToolView,
} from "@/views/Tools";

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

describe("toolSource (M916)", () => {
  it("infers the source from the tool-name prefix", () => {
    expect(toolSource("mcp_github_create_issue")).toBe("mcp");
    expect(toolSource("forge_pdf")).toBe("forged");
    expect(toolSource("skill_invoke")).toBe("skill");
    expect(toolSource("shell")).toBe("builtin");
  });
});

describe("mergeToolViews (M916)", () => {
  it("joins catalog with usage and sorts used-first then alphabetical", () => {
    const catalog = [
      { name: "zeta", description: "z", capability: "fs.read" },
      { name: "shell", description: "run", capability: "code.exec" },
      { name: "alpha", description: "a", capability: "fs.read" },
    ];
    const byTool = { shell: { calls: 5, errors: 1, avg_ms: 12 }, zeta: { calls: 2, errors: 0 } };
    const views = mergeToolViews(catalog, byTool);
    expect(views.map((v) => v.name)).toEqual(["shell", "zeta", "alpha"]); // 5, 2, idle(alpha<zeta but zeta used)
    expect(views[0]).toMatchObject({ calls: 5, errors: 1, avgMs: 12, capability: "code.exec", source: "builtin" });
    expect(views[2]).toMatchObject({ name: "alpha", calls: 0 });
  });
});

describe("filterTools (M916)", () => {
  const views: ToolView[] = [
    { name: "shell", description: "run a command", capability: "code.exec", source: "builtin", calls: 1, errors: 0 },
    { name: "web_search", description: "search the web", capability: "net.fetch", source: "builtin", calls: 0, errors: 0 },
    { name: "mcp_x_do", description: "remote", capability: "mcp.call", source: "mcp", calls: 0, errors: 0 },
  ];
  it("matches name/description/capability case-insensitively", () => {
    expect(filterTools(views, "WEB", "").map((v) => v.name)).toEqual(["web_search"]);
    expect(filterTools(views, "command", "").map((v) => v.name)).toEqual(["shell"]);
    expect(filterTools(views, "mcp.call", "").map((v) => v.name)).toEqual(["mcp_x_do"]);
  });
  it("narrows to an exact capability, composed with the query", () => {
    expect(filterTools(views, "", "net.fetch").map((v) => v.name)).toEqual(["web_search"]);
    expect(filterTools(views, "shell", "net.fetch")).toHaveLength(0);
  });
});

describe("capabilityCounts (M916)", () => {
  it("tallies per capability, sorted by count then name, skipping blanks", () => {
    const views: ToolView[] = [
      { name: "a", description: "", capability: "fs.read", source: "builtin", calls: 0, errors: 0 },
      { name: "b", description: "", capability: "fs.read", source: "builtin", calls: 0, errors: 0 },
      { name: "c", description: "", capability: "code.exec", source: "builtin", calls: 0, errors: 0 },
      { name: "d", description: "", capability: "", source: "builtin", calls: 0, errors: 0 },
    ];
    expect(capabilityCounts(views)).toEqual([
      { capability: "fs.read", n: 2 },
      { capability: "code.exec", n: 1 },
    ]);
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

  it("shows a tool's call count when used, 'idle' otherwise (M916)", async () => {
    render(<Tools />);
    // shell has 3 calls in /api/tools → "3 calls"; web_search has none → "idle".
    await waitFor(() => expect(screen.getByText("web_search")).toBeTruthy());
    expect(screen.getByText("3 calls")).toBeTruthy();
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
