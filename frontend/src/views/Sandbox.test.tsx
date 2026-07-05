// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

// Mock the api layer so the view's fetches are deterministic.
const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Sandbox } from "@/views/Sandbox";
import { UIProvider } from "@/components/ui/feedback";

// The Sandbox cards use useUI() (toast/confirm), which needs the provider.
function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postAction.mockReset();
});

describe("Sandbox view", () => {
  it("shows the empty state when no projects exist", async () => {
    // The view now also paginates /api/warden_log + /api/netguard_log via the
    // cursor pager (usePanel-backed); answer them with empty envelopes so the
    // panels render their "no entries yet" state without throwing.
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/sandbox") return Promise.resolve({ projects: [] });
      if (path === "/api/warden_log") return Promise.resolve({ executions: [], next_cursor: null });
      if (path === "/api/netguard_log") return Promise.resolve({ blocks: [], next_cursor: null });
      return Promise.resolve({});
    });
    render(withUI(<Sandbox />));
    await waitFor(() => expect(screen.getByText("No sandbox projects yet")).toBeTruthy());
  });

  it("lists projects and reveals a file's content on click", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/sandbox") {
        return Promise.resolve({
          projects: [
            {
              name: "calc",
              files: [{ name: "add.py", bytes: 24 }],
              file_count: 1,
              total_bytes: 24,
              modified_unix: 1_700_000_000,
            },
          ],
        });
      }
      return Promise.resolve({ content: "def add(a,b): return a+b", truncated: false });
    });

    render(withUI(<Sandbox />));
    // Project card appears.
    await waitFor(() => expect(screen.getByText("calc")).toBeTruthy());
    // Expand the project → the file row shows.
    fireEvent.click(screen.getByText("calc"));
    expect(screen.getByText("add.py")).toBeTruthy();
    // Expand the file → its content is fetched and rendered.
    fireEvent.click(screen.getByText("add.py"));
    await waitFor(() => expect(screen.getByText("def add(a,b): return a+b")).toBeTruthy());
    // The file fetch went through /api/sandbox_file with the right args.
    expect(getJSON).toHaveBeenCalledWith("/api/sandbox_file", { project: "calc", file: "add.py" });
  });

  it("deletes a project after confirmation", async () => {
    getJSON.mockResolvedValue({
      projects: [{ name: "calc", files: [{ name: "add.py", bytes: 4 }], file_count: 1, total_bytes: 4, modified_unix: 1 }],
    });
    postAction.mockResolvedValue({ deleted: "calc" });

    render(withUI(<Sandbox />));
    await waitFor(() => expect(screen.getByText("calc")).toBeTruthy());

    // Trash → confirm modal → click its Delete button.
    fireEvent.click(screen.getByTitle("Delete project"));
    const confirm = await screen.findByRole("button", { name: "Delete" });
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/sandbox/delete", { project: "calc" }),
    );
  });
});
