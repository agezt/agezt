// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  withToken: (p: string) => p,
}));

const streamMarket = vi.fn();
vi.mock("@/lib/market", async (importActual) => {
  const actual = await importActual<typeof import("@/lib/market")>();
  return { ...actual, streamMarket: (...a: unknown[]) => streamMarket(...a) };
});

import { Market } from "@/views/Market";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

const PACKS = [
  {
    name: "web-research-pro",
    version: "1.0.0",
    description: "Research the web end to end",
    category: "research",
    tags: ["web", "fetch"],
    signed: true,
    skill_count: 1,
    mcp_count: 1,
    tool_count: 2,
    installed: false,
  },
  {
    name: "git-workshop",
    version: "1.0.0",
    description: "Git power tools",
    category: "engineering",
    skill_count: 1,
    mcp_count: 0,
    tool_count: 0,
    installed: true,
  },
];

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  // Route by endpoint: the gallery and the sources panel both load on mount.
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/market/sources") return Promise.resolve({ sources: [] });
    if (path.startsWith("/api/market/show"))
      return Promise.resolve({ skills: [{ name: "fetcher", description: "fetch the web" }], mcp_servers: ["fetch"], tools: ["rg"] });
    return Promise.resolve({ packs: PACKS });
  });
  postJSON.mockResolvedValue({});
  streamMarket.mockReset();
  // Default: emit a progress step then a done frame, like the real SSE stream.
  streamMarket.mockImplementation(async (_path, _body, onFrame: (f: unknown) => void) => {
    onFrame({ kind: "market.install.progress", payload: { stage: "skill", name: "demo", ok: true, detail: "active" } });
    onFrame({ kind: "done", result: { unsigned: false, tool_reqs: [] } });
  });
});

const galleryLoads = () => getJSON.mock.calls.filter((c) => c[0] === "/api/market").length;

describe("Market", () => {
  it("renders the catalogue with content counts and install state", async () => {
    render(withUI(<Market />));
    expect(await screen.findByText("web-research-pro")).toBeTruthy();
    expect(screen.getByText("git-workshop")).toBeTruthy();
    // git-workshop is installed → shows an installed badge (exact text)
    expect(screen.getByText("installed")).toBeTruthy();
    // header summary counts packs + installed
    expect(screen.getByText(/2 packs · 1 installed/)).toBeTruthy();
  });

  it("filters by search query", async () => {
    render(withUI(<Market />));
    await screen.findByText("web-research-pro");
    fireEvent.change(screen.getByLabelText("Search packs"), { target: { value: "git" } });
    await waitFor(() => expect(screen.queryByText("web-research-pro")).toBeNull());
    expect(screen.getByText("git-workshop")).toBeTruthy();
  });

  it("installs a pack via the stream and reloads the catalogue", async () => {
    render(withUI(<Market />));
    await screen.findByText("web-research-pro");
    const before = galleryLoads();
    const installBtn = screen.getByRole("button", { name: /^install$/i });
    fireEvent.click(installBtn);
    await waitFor(() =>
      expect(streamMarket).toHaveBeenCalledWith(
        "/api/market/install",
        { name: "web-research-pro", marketplace: "" },
        expect.any(Function),
      ),
    );
    // reloaded the gallery after the stream's done frame
    await waitFor(() => expect(galleryLoads()).toBe(before + 1));
  });

  it("adds a remote source and triggers a sync", async () => {
    render(withUI(<Market />));
    await screen.findByText("web-research-pro");
    fireEvent.click(screen.getByRole("button", { name: /sources/i }));
    fireEvent.change(screen.getByLabelText("Marketplace URL"), {
      target: { value: "https://packs.example.com/marketplace.json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^add$/i }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/market/source/add", {
        url: "https://packs.example.com/marketplace.json",
        name: "",
      }),
    );
    // adding a source kicks off a sync
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/market/sync", { name: "" }));
  });

  it("lazily reveals a pack's contents on 'What's inside'", async () => {
    render(withUI(<Market />));
    await screen.findByText("web-research-pro");
    // Details aren't fetched until expanded.
    expect(getJSON.mock.calls.some((c) => String(c[0]).startsWith("/api/market/show"))).toBe(false);
    fireEvent.click(screen.getAllByRole("button", { name: /what's inside/i })[0]);
    expect(await screen.findByText(/fetcher/)).toBeTruthy();
    expect(screen.getByText(/fetch the web/)).toBeTruthy();
    await waitFor(() =>
      expect(getJSON.mock.calls.some((c) => String(c[0]).startsWith("/api/market/show"))).toBe(true),
    );
  });

  it("filters to installed packs only", async () => {
    render(withUI(<Market />));
    await screen.findByText("web-research-pro");
    fireEvent.click(screen.getByRole("switch", { name: /installed only/i }));
    await waitFor(() => expect(screen.queryByText("web-research-pro")).toBeNull());
    expect(screen.getByText("git-workshop")).toBeTruthy(); // the installed one stays
  });
});
