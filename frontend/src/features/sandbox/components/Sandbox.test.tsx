// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

// Mock the api layer so the view's fetches are deterministic.
const getJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/app/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Sandbox, isBuildNoise } from "@/features/sandbox/components/Sandbox";
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

// A checked-out repo inside a sandbox project brought 500 rows of
// `.git/hooks/*.sample`, rendered for every project at once, side by side. The
// page became an unreadable wall of files no agent wrote.
describe("isBuildNoise", () => {
  it("flags VCS internals, vendored deps, caches and build output", () => {
    expect(isBuildNoise(".git/hooks/pre-commit.sample")).toBe(true);
    expect(isBuildNoise("repo/.git/config")).toBe(true);
    expect(isBuildNoise(".deps/bs4/__pycache__/css.cpython-314.pyc")).toBe(true);
    expect(isBuildNoise("node_modules/left-pad/index.js")).toBe(true);
    expect(isBuildNoise(".venv/lib/site-packages/x.py")).toBe(true);
    expect(isBuildNoise(".deps/beautifulsoup4-4.15.0.dist-info/METADATA")).toBe(true);
    expect(isBuildNoise("a/b/__pycache__/mod.cpython-314.pyc")).toBe(true);
  });

  it("leaves agent-authored files alone", () => {
    expect(isBuildNoise("add.py")).toBe(false);
    expect(isBuildNoise("src/report.md")).toBe(false);
    expect(isBuildNoise("scripts/build.sh")).toBe(false);
    // A directory merely *named* like a file extension is not noise.
    expect(isBuildNoise("git/notes.txt")).toBe(false);
  });
});

describe("Sandbox file browser", () => {
  const noisy = {
    name: "repo",
    files: [
      { name: "main.py", bytes: 10 },
      { name: "README.md", bytes: 20 },
      { name: ".git/config", bytes: 30 },
      { name: ".git/hooks/pre-commit.sample", bytes: 40 },
      { name: "__pycache__/main.cpython-314.pyc", bytes: 50 },
    ],
    file_count: 5,
    total_bytes: 150,
    modified_unix: 1,
  };

  beforeEach(() => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/sandbox") return Promise.resolve({ projects: [noisy] });
      if (path === "/api/warden_log") return Promise.resolve({ executions: [], next_cursor: null });
      if (path === "/api/netguard_log") return Promise.resolve({ blocks: [], next_cursor: null });
      return Promise.resolve({});
    });
  });

  it("keeps files out of the card and shows the authored count", async () => {
    render(withUI(<Sandbox />));
    await waitFor(() => expect(screen.getByText("repo")).toBeTruthy());
    // The card is a census, not a file dump: nothing is listed until you open it.
    expect(screen.queryByText("main.py")).toBeNull();
    // 2 authored files, 3 generated.
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByText(/3 generated/)).toBeTruthy();
  });

  it("hides tool-generated files until asked", async () => {
    render(withUI(<Sandbox />));
    await waitFor(() => expect(screen.getByText("repo")).toBeTruthy());
    fireEvent.click(screen.getByText("repo"));

    await waitFor(() => expect(screen.getByText("main.py")).toBeTruthy());
    expect(screen.getByText("README.md")).toBeTruthy();
    expect(screen.queryByText(".git/config")).toBeNull();
    expect(screen.getByText("2 of 5 files")).toBeTruthy();

    fireEvent.click(screen.getByText(/Show generated files \(3\)/));
    expect(screen.getByText(".git/config")).toBeTruthy();
    expect(screen.getByText("5 of 5 files")).toBeTruthy();
  });

  it("filters the file list", async () => {
    render(withUI(<Sandbox />));
    await waitFor(() => expect(screen.getByText("repo")).toBeTruthy());
    fireEvent.click(screen.getByText("repo"));
    await waitFor(() => expect(screen.getByText("main.py")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Filter files"), { target: { value: "readme" } });
    expect(screen.getByText("README.md")).toBeTruthy();
    expect(screen.queryByText("main.py")).toBeNull();
  });
});

// The pagination law: no list renders unbounded, however big the payload.
describe("Sandbox file windowing", () => {
  it("windows a huge project instead of rendering every row", async () => {
    const files = Array.from({ length: 250 }, (_, i) => ({ name: `src/f${i}.py`, bytes: 1 }));
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/sandbox")
        return Promise.resolve({
          projects: [{ name: "big", files, file_count: files.length, total_bytes: 250, modified_unix: 1 }],
        });
      if (path === "/api/warden_log") return Promise.resolve({ executions: [], next_cursor: null });
      if (path === "/api/netguard_log") return Promise.resolve({ blocks: [], next_cursor: null });
      return Promise.resolve({});
    });

    render(withUI(<Sandbox />));
    await waitFor(() => expect(screen.getByText("big")).toBeTruthy());
    fireEvent.click(screen.getByText("big"));

    await waitFor(() => expect(screen.getByText("src/f0.py")).toBeTruthy());
    // 60 rendered, not 250 — the 61st is behind the Load-more footer.
    expect(screen.getByText("src/f59.py")).toBeTruthy();
    expect(screen.queryByText("src/f60.py")).toBeNull();
    expect(screen.getByText("250 of 250 files")).toBeTruthy();
  });
});
