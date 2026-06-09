// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

// Mock the api layer so the view's fetches are deterministic.
const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { Sandbox } from "@/views/Sandbox";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

describe("Sandbox view", () => {
  it("shows the empty state when no projects exist", async () => {
    getJSON.mockResolvedValueOnce({ projects: [] });
    render(<Sandbox />);
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

    render(<Sandbox />);
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
});
