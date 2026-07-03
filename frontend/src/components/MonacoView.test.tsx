// @vitest-environment jsdom
//
// MonacoView can't render the real @monaco-editor/react in vitest — the editor
// needs a worker + jsdom doesn't ship one. We mock the heavy import and test
// the wrapper's own behaviour: the language tag, the copy / expand affordances,
// and the read-only default.

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { MonacoView } from "./MonacoView";

const fakeEditor = vi.fn((props: { value?: string; language?: string; onChange?: (v: string | undefined) => void; options?: { readOnly?: boolean; minimap?: { enabled: boolean }; lineNumbers?: string } }) => (
  <div
    data-testid="monaco-editor"
    data-lang={props.language || ""}
    data-readonly={props.options?.readOnly ? "true" : "false"}
    data-linenos={props.options?.lineNumbers || ""}
    data-minimap={props.options?.minimap?.enabled ? "true" : "false"}
  >
    {props.value}
  </div>
));

vi.mock("@monaco-editor/react", () => ({
  __esModule: true,
  loader: { config: vi.fn() },
  Editor: (props: Parameters<typeof fakeEditor>[0]) => fakeEditor(props),
}));

beforeEach(() => {
  fakeEditor.mockClear();
});
afterEach(cleanup);

describe("MonacoView", () => {
  it("renders a Monaco Editor with the language derived from the path", async () => {
    render(<MonacoView value="package main\nfunc main(){}" path="cmd/foo/main.go" />);
    // The lazy Suspense resolves on the next microtask; wait for the editor.
    const ed = await screen.findByTestId("monaco-editor");
    expect(ed.getAttribute("data-lang")).toBe("go");
    expect(ed.textContent).toContain("package main");
  });

  it("defaults to read-only and suppresses line numbers / minimap", async () => {
    render(<MonacoView value="x" path="notes/x.md" />);
    const ed = await screen.findByTestId("monaco-editor");
    expect(ed.getAttribute("data-readonly")).toBe("true");
    expect(ed.getAttribute("data-linenos")).toBe("off");
    expect(ed.getAttribute("data-minimap")).toBe("false");
  });

  it("shows the language and copy/expand affordances in the header", () => {
    render(<MonacoView value="let x = 1;" path="src/foo.ts" />);
    expect(screen.getByText("typescript")).toBeTruthy();
    expect(screen.getByTitle("Copy")).toBeTruthy();
    // Default is expanded (collapsed=false), so the affordance is "collapse".
    expect(screen.getByRole("button", { name: /collapse code block/i })).toBeTruthy();
  });

  it("starts collapsed (with an expand button) when collapsed=true", () => {
    render(<MonacoView value="x" path="src/foo.ts" collapsed />);
    expect(screen.getByRole("button", { name: /expand code block/i })).toBeTruthy();
  });
});
