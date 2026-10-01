// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { getJSON, postJSON } = vi.hoisted(() => ({
  getJSON: vi.fn(),
  postJSON: vi.fn(),
}));

vi.mock("@/app/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));

import { Chains } from "./Chains";
import { UIProvider } from "@/components/ui/feedback";

// The pure chain algebra (rename, move, remove, validate) is already covered in
// lib/chains.test.ts. What had no coverage is the view: which buttons exist,
// when Save is enabled, what it posts, and what the operator is told when the
// daemon will not answer.
describe("Chains view", () => {
  beforeEach(() => {
    getJSON.mockReset();
    postJSON.mockReset();
    postJSON.mockResolvedValue({});
    getJSON.mockImplementation(async (url: unknown) => {
      const u = String(url);
      if (u === "/api/chains") return { chains: { fallback: ["alpha", "beta"] }, default_chain: "fallback" };
      if (u === "/api/catalog") return { models: [{ id: "alpha" }, { id: "beta" }] };
      return {};
    });
  });
  afterEach(cleanup);

  const renderIt = () =>
    render(
      <UIProvider>
        <Chains />
      </UIProvider>,
    );

  it("lists the chain's models in order, with the ends unable to move further", async () => {
    renderIt();
    // The chain name appears in several places (heading, default badge, tooltip),
    // so assert on the reordering controls — which are what the order IS.
    await waitFor(() => expect(screen.getAllByTitle("Move up")).toHaveLength(2));
    const up = screen.getAllByTitle("Move up");
    const down = screen.getAllByTitle("Move down");
    expect((up[0] as HTMLButtonElement).disabled).toBe(true);
    expect((up[1] as HTMLButtonElement).disabled).toBe(false);
    expect((down[0] as HTMLButtonElement).disabled).toBe(false);
    expect((down[1] as HTMLButtonElement).disabled).toBe(true);
  });

  it("offers the empty state, and creates a chain from it", async () => {
    getJSON.mockImplementation(async (url: unknown) => {
      const u = String(url);
      if (u === "/api/chains") return { chains: {}, default_chain: "" };
      if (u === "/api/catalog") return { models: [] };
      return {};
    });
    renderIt();
    expect(await screen.findByText(/No fallback chains yet/i)).toBeTruthy();
    // Selected by title: the control carries no text of its own, only an icon.
    fireEvent.click(screen.getByTitle("Create a new fallback chain"));
    await waitFor(() => expect(screen.queryByText(/No fallback chains yet/i)).toBeNull());
  });

  it("keeps Save disabled until something changes", async () => {
    renderIt();
    await waitFor(() => expect(screen.getByTitle("Save chains")).toBeTruthy());
    expect((screen.getByTitle("Save chains") as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getAllByTitle("Remove")[0]);
    await waitFor(() => expect((screen.getByTitle("Save chains") as HTMLButtonElement).disabled).toBe(false));
  });

  it("posts the whole chain set on save, then goes clean again", async () => {
    renderIt();
    await waitFor(() => expect(screen.getByTitle("Save chains")).toBeTruthy());

    fireEvent.click(screen.getAllByTitle("Move down")[0]);
    fireEvent.click(screen.getByTitle("Save chains"));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/chains/set", expect.anything()));
    // A second Save is disabled again, so a double-click cannot double-post.
    await waitFor(() => expect((screen.getByTitle("Save chains") as HTMLButtonElement).disabled).toBe(true));
  });

  it("names what uses a chain, so deleting it is not a leap of faith", async () => {
    getJSON.mockImplementation(async (url: unknown) => {
      const u = String(url);
      if (u === "/api/chains") {
        return {
          chains: { fallback: ["alpha"] },
          default_chain: "fallback",
          usage: { fallback: { agents: ["writer"], tasks: ["summarise"] } },
        };
      }
      if (u === "/api/catalog") return { models: [{ id: "alpha" }] };
      return {};
    });
    renderIt();
    expect(await screen.findByTitle("Referenced by: writer")).toBeTruthy();
    expect(screen.getByTitle("Task types: summarise")).toBeTruthy();
  });

  // The view reads two endpoints and this is the failure nobody would notice:
  // an unread chain set renders as the empty state, which reads as "you have no
  // chains" — advice to create one, on top of a daemon that is not answering.
  it("distinguishes an unread chain set from an empty one", async () => {
    getJSON.mockImplementation(async (url: unknown) => {
      if (String(url) === "/api/chains") throw new Error("chains endpoint down");
      return { models: [] };
    });
    renderIt();
    expect(await screen.findByText(/chains endpoint down/)).toBeTruthy();
    expect(screen.queryByText(/No fallback chains yet/i)).toBeNull();
  });

  it("reloads on demand", async () => {
    renderIt();
    await waitFor(() => expect(screen.getByTitle("Reload")).toBeTruthy());
    const before = getJSON.mock.calls.length;
    fireEvent.click(screen.getByTitle("Reload"));
    await waitFor(() => expect(getJSON.mock.calls.length).toBeGreaterThan(before));
  });
});