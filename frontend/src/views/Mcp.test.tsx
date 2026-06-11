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

import { Mcp, NewServerForm, serverNameOk, splitArgs, CATALOG } from "@/views/Mcp";
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

describe("CATALOG", () => {
  it("every preset has a kernel-valid name, a command, and args", () => {
    expect(CATALOG.length).toBeGreaterThan(0);
    for (const e of CATALOG) {
      expect(serverNameOk(e.name)).toBe(true); // ≤16 lowercase alnum (tool-prefix rule)
      expect(e.command.trim()).not.toBe("");
      expect(splitArgs(e.args).length).toBeGreaterThan(0);
      expect(e.description.trim()).not.toBe("");
    }
  });
  it("preset names are unique", () => {
    const names = CATALOG.map((e) => e.name);
    expect(new Set(names).size).toBe(names.length);
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
});
