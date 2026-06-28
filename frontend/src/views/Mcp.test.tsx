// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import {
  Mcp,
  NewServerForm,
  serverNameOk,
  splitArgs,
  splitTools,
  parseEnv,
  parseHeaders,
  urlOk,
  transportOf,
  filterCatalog,
  CATALOG,
  CATEGORY_LABELS,
} from "@/views/Mcp";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("serverNameOk", () => {
  it("mirrors the kernel mcp name rule (no underscore/dash — the toolmap parses the prefix)", () => {
    for (const s of ["everything", "a", "fake9", "x".repeat(16)]) expect(serverNameOk(s)).toBe(true);
    for (const s of ["", "Fake", "my_server", "my-server", "9fake", "x".repeat(17)])
      expect(serverNameOk(s)).toBe(false);
  });
});

describe("splitArgs", () => {
  it("splits the one-line args field on whitespace, dropping blanks", () => {
    expect(splitArgs("-y  @modelcontextprotocol/server-everything ")).toEqual([
      "-y",
      "@modelcontextprotocol/server-everything",
    ]);
    expect(splitArgs("")).toEqual([]);
  });
});

describe("splitTools", () => {
  it("splits the allowlist on whitespace and commas, dropping blanks", () => {
    expect(splitTools("create_issue, search_code  get_file")).toEqual([
      "create_issue",
      "search_code",
      "get_file",
    ]);
    expect(splitTools("")).toEqual([]);
  });
});

describe("parseEnv", () => {
  it("parses KEY=value lines, keeps later '=' in the value, drops blanks/comments", () => {
    expect(parseEnv("GITHUB_TOKEN=ghp_x\n# a note\n\nB64=aGk=\nbad line\n")).toEqual({
      GITHUB_TOKEN: "ghp_x",
      B64: "aGk=",
    });
    expect(parseEnv("")).toEqual({});
  });
});

describe("parseHeaders", () => {
  it("parses Name: value lines, keeps later ':' in the value, drops blanks/comments", () => {
    expect(parseHeaders("Authorization: Bearer ab:cd\n# note\n\nX-Trace: 1\nno-colon\n")).toEqual({
      Authorization: "Bearer ab:cd",
      "X-Trace": "1",
    });
    expect(parseHeaders("")).toEqual({});
  });
});

describe("urlOk", () => {
  it("accepts http(s) URLs with a host, rejects everything else", () => {
    for (const s of ["https://api.example.com/mcp", "http://localhost:8080/v1"]) expect(urlOk(s)).toBe(true);
    for (const s of ["", "ftp://x/y", "not a url", "https://"]) expect(urlOk(s)).toBe(false);
  });
});

describe("CATALOG", () => {
  it("every preset has a kernel-valid name, a description, and exactly one transport shape", () => {
    expect(CATALOG.length).toBeGreaterThan(0);
    for (const e of CATALOG) {
      expect(serverNameOk(e.name)).toBe(true); // ≤16 lowercase alnum (tool-prefix rule)
      expect(e.description.trim()).not.toBe("");
      if (transportOf(e) === "http") {
        expect(urlOk(e.url!)).toBe(true);
        expect(e.command).toBeUndefined();
      } else {
        expect((e.command || "").trim()).not.toBe("");
        expect(splitArgs(e.args || "").length).toBeGreaterThan(0);
      }
    }
  });
  it("includes at least one remote (http) preset", () => {
    expect(CATALOG.some((e) => transportOf(e) === "http")).toBe(true);
  });
  it("preset names are unique", () => {
    const names = CATALOG.map((e) => e.name);
    expect(new Set(names).size).toBe(names.length);
  });
  it("every preset has a known category and every category is non-empty (M912)", () => {
    const cats = Object.keys(CATEGORY_LABELS);
    for (const e of CATALOG) expect(cats).toContain(e.category);
    for (const c of cats) expect(CATALOG.some((e) => e.category === c)).toBe(true);
  });
  it("is a real library, not a stub — 35+ presets spanning stdio and remote", () => {
    expect(CATALOG.length).toBeGreaterThanOrEqual(35);
  });
});

describe("filterCatalog", () => {
  it("narrows by category chip and matches name/description case-insensitively", () => {
    expect(filterCatalog(CATALOG, "all", "")).toHaveLength(CATALOG.length);
    for (const e of filterCatalog(CATALOG, "data", "")) expect(e.category).toBe("data");
    const byName = filterCatalog(CATALOG, "all", "FIRE");
    expect(byName.some((e) => e.name === "firecrawl")).toBe(true);
    const byDesc = filterCatalog(CATALOG, "web", "duckduckgo");
    expect(byDesc.some((e) => e.name === "duckduckgo")).toBe(true);
    expect(filterCatalog(CATALOG, "core", "firecrawl")).toHaveLength(0); // category AND query
    expect(filterCatalog(CATALOG, "all", "zzz-no-such-server")).toHaveLength(0);
  });
});

describe("NewServerForm", () => {
  it("disables Register until name+command are valid, then posts the server shape", async () => {
    const onCreated = vi.fn();
    render(<NewServerForm onCreated={onCreated} onError={() => {}} />);
    const btn = () => screen.getByRole("button", { name: /Register server/ }) as HTMLButtonElement;
    expect(btn().disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Server name"), { target: { value: "my_server" } });
    fireEvent.change(screen.getByLabelText("Server command"), { target: { value: "npx" } });
    expect(btn().disabled).toBe(true); // underscore name invalid

    fireEvent.change(screen.getByLabelText("Server name"), { target: { value: "everything" } });
    fireEvent.change(screen.getByLabelText("Server arguments"), {
      target: { value: "-y @modelcontextprotocol/server-everything" },
    });
    expect(btn().disabled).toBe(false);
    fireEvent.click(btn());

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/mcp/add",
        expect.objectContaining({
          server: expect.objectContaining({
            name: "everything",
            command: "npx",
            args: ["-y", "@modelcontextprotocol/server-everything"],
          }),
        }),
      ),
    );
    expect(onCreated).toHaveBeenCalledWith("everything");
  });

  it("in Remote mode requires a valid URL and posts the http shape with headers (M904)", async () => {
    const onCreated = vi.fn();
    render(<NewServerForm onCreated={onCreated} onError={() => {}} />);
    const btn = () => screen.getByRole("button", { name: /Register server/ }) as HTMLButtonElement;

    fireEvent.click(screen.getByRole("tab", { name: /Remote/ }));
    fireEvent.change(screen.getByLabelText("Server name"), { target: { value: "githubremote" } });
    expect(screen.queryByLabelText("Server command")).toBeNull(); // command field hidden in remote mode
    expect(btn().disabled).toBe(true); // no URL yet

    fireEvent.change(screen.getByLabelText("Server URL"), { target: { value: "https://api.example.com/mcp" } });
    fireEvent.click(screen.getByRole("button", { name: /Advanced guardrails/ }));
    fireEvent.change(screen.getByLabelText("Server headers"), {
      target: { value: "Authorization: Bearer ghp_x" },
    });
    expect(btn().disabled).toBe(false);
    fireEvent.click(btn());

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/mcp/add",
        expect.objectContaining({
          server: expect.objectContaining({
            name: "githubremote",
            url: "https://api.example.com/mcp",
            headers: { Authorization: "Bearer ghp_x" },
          }),
        }),
      ),
    );
    // The http shape must NOT carry a command.
    const posted = postJSON.mock.calls[0][1] as { server: Record<string, unknown> };
    expect(posted.server.command).toBeUndefined();
    expect(onCreated).toHaveBeenCalledWith("githubremote");
  });

  it("ticking Lazy load posts lazy:true (M906)", async () => {
    render(<NewServerForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Server name"), { target: { value: "github" } });
    fireEvent.change(screen.getByLabelText("Server command"), { target: { value: "npx" } });
    fireEvent.click(screen.getByRole("button", { name: /Advanced guardrails/ }));
    fireEvent.click(screen.getByLabelText("Lazy load tools"));
    fireEvent.click(screen.getByRole("button", { name: /Register server/ }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/mcp/add",
        expect.objectContaining({ server: expect.objectContaining({ name: "github", lazy: true }) }),
      ),
    );
  });
});

describe("Mcp", () => {
  const twoServers = {
    servers: [
      {
        id: "01A", name: "fake", command: "python", args: ["server.py"],
        enabled: true, attached: true, tool_count: 2,
      },
      { id: "01B", name: "other", command: "npx", enabled: false, attached: false },
    ],
    count: 2,
    attached_count: 1,
  };

  it("renders servers from /api/mcp with attachment status and command line", async () => {
    getJSON.mockResolvedValue(twoServers);
    render(withUI(<Mcp />));
    await waitFor(() => expect(screen.getByText("fake")).toBeTruthy());
    expect(screen.getByText("attached · 2 tools")).toBeTruthy();
    expect(screen.getByText("registered")).toBeTruthy();
    expect(screen.getByText("auto-attach")).toBeTruthy();
    expect(screen.getByText("python server.py")).toBeTruthy();
    expect(getJSON).toHaveBeenCalledWith("/api/mcp");
  });

  it("shows Attach only for detached servers and Detach only for live ones", async () => {
    getJSON.mockResolvedValue(twoServers);
    render(withUI(<Mcp />));
    await waitFor(() => expect(screen.getByText("fake")).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Attach fake" })).toBeNull();
    expect(screen.getByRole("button", { name: "Detach fake" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Attach other" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Detach other" })).toBeNull();
  });

  it("attach asks for confirmation, posts /api/mcp/attach, and reports the discovered tool count", async () => {
    getJSON.mockResolvedValue(twoServers);
    postAction.mockResolvedValue({ tools: ["mcp_other_a", "mcp_other_b", "mcp_other_c"] });
    render(withUI(<Mcp />));
    await waitFor(() => expect(screen.getByText("other")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "Attach other" }));
    fireEvent.click(await screen.findByRole("button", { name: /^Attach$/ }));

    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/mcp/attach", { ref: "other" }));
    await waitFor(() => expect(screen.getByText(/3 tool\(s\) live/)).toBeTruthy());
  });

  it("auto-attach toggle posts /api/mcp/enable with the flipped flag", async () => {
    getJSON.mockResolvedValue(twoServers);
    render(withUI(<Mcp />));
    await waitFor(() => expect(screen.getByText("fake")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Disable auto-attach for fake" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/mcp/enable", { ref: "fake", enabled: "false" }),
    );
  });

  it("shows the empty state when nothing is registered", async () => {
    getJSON.mockResolvedValue({ servers: [], count: 0, attached_count: 0 });
    render(withUI(<Mcp />));
    await waitFor(() => expect(screen.getByText("No MCP servers yet")).toBeTruthy());
  });

  it("gallery filters by category chip and search box (M912)", async () => {
    getJSON.mockResolvedValue({ servers: [], count: 0, attached_count: 0 });
    render(withUI(<Mcp />));
    fireEvent.click(screen.getByRole("button", { name: /Popular servers/ }));
    await waitFor(() => expect(screen.getByText("firecrawl")).toBeTruthy());

    // Category chip: Databases hides web presets, keeps db ones.
    fireEvent.click(screen.getByRole("button", { name: "Databases" }));
    expect(screen.queryByText("firecrawl")).toBeNull();
    expect(screen.getByText("mongodb")).toBeTruthy();

    // Search inside the category, then a query with no hits.
    fireEvent.change(screen.getByLabelText("Search catalog"), { target: { value: "qdrant" } });
    expect(screen.getByText("qdrant")).toBeTruthy();
    expect(screen.queryByText("mongodb")).toBeNull();
    fireEvent.change(screen.getByLabelText("Search catalog"), { target: { value: "nope" } });
    expect(screen.getByText(/No presets match/)).toBeTruthy();
  });
});
