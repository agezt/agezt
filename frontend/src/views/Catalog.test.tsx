// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
import { Catalog } from "@/views/Catalog";

const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

const toast = vi.fn();
vi.mock("@/components/ui/feedback", () => ({
  useUI: () => ({ toast }),
}));

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
  toast.mockReset();
  postAction.mockResolvedValue({});
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/tools_catalog") {
      return Promise.resolve({
        tools: [{ name: "web_search", description: "Search the web", capability: "net.fetch" }],
      });
    }
    if (path === "/api/edict_show") return Promise.resolve({ levels: { "net.fetch": "L2" } });
    if (path === "/api/tools") return Promise.resolve({ by_tool: { web_search: { calls: 3, errors: 1 } } });
    return Promise.resolve({});
  });
});

describe("Catalog", () => {
  it("sets capability trust level from the compact level palette", async () => {
    render(<Catalog />);
    await screen.findByText("web_search");

    fireEvent.click(screen.getByRole("button", { name: "Trust level for net.fetch" }));
    fireEvent.click(within(screen.getByRole("group", { name: "Set trust level for net.fetch" })).getByRole("button", { name: "L4" }));

    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/edict/set_level", { capability: "net.fetch", level: "L4" }));
    expect(toast).toHaveBeenCalledWith(expect.stringContaining("net.fetch"), "success");
    expect(toast).toHaveBeenCalledWith(expect.stringContaining("L4"), "success");
  });
});
