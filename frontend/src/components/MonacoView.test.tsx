// @vitest-environment jsdom
//
// MonacoView can't render the real @monaco-editor/react in vitest — the editor
// needs a worker + jsdom doesn't ship one. We mock the heavy import and test
// the wrapper's own behaviour: the language tag, the copy / expand affordances,
// the read-only default, the empty-value short-circuit, and the
// dispose-on-unmount hook.

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { MonacoView } from "./MonacoView";

let lastOnMount: ((editor: unknown) => void) | undefined;
let lastProps: Record<string, unknown> | undefined;
let disposeCalls = 0;
let disposedEditor: { dispose: () => void } | null = null;

type fakeEditorArgs = {
  value?: string;
  language?: string;
  onChange?: (v: string | undefined) => void;
  onMount?: (e: unknown) => void;
  options?: { readOnly?: boolean; minimap?: { enabled: boolean }; lineNumbers?: string };
};

const fakeEditor = vi.fn((props: fakeEditorArgs) => {
  lastProps = props;
  lastOnMount = props.onMount;
  // The real @monaco-editor/react calls onMount after the editor DOM is
  // rendered (post-commit). Don't fire it synchronously here — doing so
  // would setState during render and trigger React's "Cannot update a
  // component while rendering a different component" warning. The
  // dispose-on-unmount test below calls `driveMount()` after render to
  // hand MonacoView an editor handle, mirroring the real lifecycle.
  return (
    <div
      data-testid="monaco-editor"
      data-lang={props.language || ""}
      data-readonly={props.options?.readOnly ? "true" : "false"}
      data-linenos={props.options?.lineNumbers || ""}
      data-minimap={props.options?.minimap?.enabled ? "true" : "false"}
    >
      {props.value}
    </div>
  );
});

// driveMount simulates the post-mount callback the real editor fires once
// the DOM is in place. The dispose-on-unmount test uses this to obtain an
// editor handle and verify the wrapper tears it down.
function driveMount() {
  if (!lastOnMount) return;
  const fakeEditorHandle = {
    dispose: () => {
      disposeCalls++;
      disposedEditor = fakeEditorHandle;
    },
  };
  lastOnMount(fakeEditorHandle);
}

vi.mock("@monaco-editor/react", () => ({
  __esModule: true,
  loader: { config: vi.fn() },
  Editor: (props: fakeEditorArgs) => fakeEditor(props),
}));

beforeEach(() => {
  fakeEditor.mockClear();
  lastOnMount = undefined;
  lastProps = undefined;
  disposeCalls = 0;
  disposedEditor = null;
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

  it("renders an explicit empty-state placeholder instead of mounting Monaco for an empty buffer", () => {
    render(<MonacoView value="" path="notes/empty.md" />);
    // The mock Editor function should NOT have been called — we skip the
    // Monaco mount for an empty buffer so it doesn't leak a model.
    expect(fakeEditor).not.toHaveBeenCalled();
    // The placeholder is recognisable and mentions the buffer name.
    expect(screen.getByText(/empty notes\/empty\.md/)).toBeTruthy();
  });

  it("calls the editor's dispose() when the component unmounts", () => {
    const { unmount } = render(<MonacoView value="x" path="x.ts" />);
    expect(disposeCalls).toBe(0);
    // Simulate the post-commit lifecycle: real onMount fires AFTER render.
    driveMount();
    expect(disposeCalls).toBe(0);
    unmount();
    expect(disposeCalls).toBe(1);
    expect(disposedEditor).not.toBeNull();
  });
});

